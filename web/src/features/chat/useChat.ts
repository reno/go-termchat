import { useCallback, useEffect, useRef, useState } from "react";

import type { SessionClients } from "@/lib/clients";
import { newClientId } from "@/lib/id";

import { applyTheme, helpText, mentionDraft, parseCommand } from "./commands";
import type { MessageView } from "./model";
import { useChatStore } from "./store";

import { MessageKind as ProtoKind } from "@gen/chat/v1/chat_pb";

export const DEFAULT_CHANNEL = "?teste";
export const SEND_FAILED = "message failed to send. press up and send it again.";
export const DM_UNDELIVERED = "not delivered — that nick is not connected.";
export const WINDOW_TRUNCATED = "[older messages were forgotten]";

function nickChanged(nick: string): string {
  return `you are now ${nick}`;
}

export function useChat(clients: SessionClients, me: string | null, onLeave: () => void) {
  const store = useChatStore();
  const [draft, setDraft] = useState("");
  const activeId = store.activeId;
  const typing = useRef(false);
  const cursors = useRef(new Map<string, string>());
  const loading = useRef(false);

  const { setMe, setActive, upsertChannel, setMembers, systemLine, applyEvent } = store;

  useEffect(() => {
    setMe(me);
  }, [me, setMe]);

  const openChannel = useCallback(
    async (name: string) => {
      const joined = await clients.chat.joinChannel({ name });
      const channel = joined.channel;
      if (channel === undefined) {
        return;
      }
      upsertChannel({
        id: channel.id,
        name: channel.name,
        memberCount: channel.memberCount,
      });
      setActive(channel.id);

      const [history, roster] = await Promise.all([
        clients.chat.listMessages({ channelId: channel.id, pageSize: 100 }),
        clients.chat.listMembers({ channelId: channel.id }),
      ]);

      const lines: MessageView[] = [...history.messages].reverse().map((message) => ({
        id: message.id,
        clientId: message.clientMessageId,
        channelId: channel.id,
        nick: message.authorNick,
        body: message.body,
        kind:
          message.kind === ProtoKind.SYSTEM
            ? "system"
            : message.kind === ProtoKind.ACTION
              ? "action"
              : "normal",
        state: "sent",
        createdAt: Number(message.createdAt?.seconds ?? 0n) * 1000,
      }));

      if (history.nextCursor?.value !== undefined && history.nextCursor.value !== "") {
        cursors.current.set(channel.id, history.nextCursor.value);
      } else {
        cursors.current.delete(channel.id);
      }

      useChatStore.getState().prependMessages(channel.id, lines);
      setMembers(
        channel.id,
        roster.members.map((member) => ({ nick: member.nick, tag: member.typing ? "typing" : "" })),
      );
    },
    [clients, setActive, setMembers, upsertChannel],
  );

  useEffect(() => {
    if (me === null) {
      return;
    }
    void openChannel(DEFAULT_CHANNEL);
  }, [me, openChannel]);

  useEffect(() => {
    if (me === null) {
      return;
    }
    const controller = new AbortController();

    void (async () => {
      try {
        for await (const frame of clients.chat.streamEvents(
          { lastEventId: "" },
          { signal: controller.signal },
        )) {
          if (frame.event !== undefined) {
            applyEvent(frame.event);
          }
        }
      } catch {
        // The transport retries; a closed stream is not an error the transcript reports.
      }
    })();

    return () => {
      controller.abort();
    };
  }, [clients, me, applyEvent]);

  const send = useCallback(async () => {
    const parsed = parseCommand(draft);
    if (parsed === null || activeId === null) {
      return;
    }
    setDraft("");

    switch (parsed.kind) {
      case "usage":
        systemLine(activeId, parsed.message);
        return;

      case "help":
        systemLine(activeId, helpText());
        return;

      case "clear":
        useChatStore.getState().clearChannel(activeId);
        return;

      case "theme":
        applyTheme(parsed.theme, document.documentElement);
        return;

      case "leave":
        onLeave();
        return;

      case "nick":
        try {
          const renamed = await clients.sessions.setNick({ nick: parsed.nick });
          const claimed = renamed.session?.nick ?? parsed.nick;
          setMe(claimed);
          systemLine(activeId, nickChanged(claimed));
        } catch {
          systemLine(activeId, "that nick is taken.");
        }
        return;

      case "join":
        try {
          await openChannel(parsed.channel);
        } catch {
          systemLine(activeId, "could not join that channel.");
        }
        return;

      case "msg":
        try {
          const sent = await clients.chat.sendDirectMessage({
            clientMessageId: newClientId(),
            recipientNick: parsed.to,
            body: parsed.body,
          });
          if (!sent.delivered) {
            systemLine(activeId, DM_UNDELIVERED);
          }
        } catch {
          systemLine(activeId, DM_UNDELIVERED);
        }
        return;

      case "me":
      case "say": {
        const clientId = newClientId();
        const body = parsed.kind === "me" ? parsed.action : parsed.body;
        const kind = parsed.kind === "me" ? "action" : "normal";

        useChatStore.getState().appendMessage({
          id: "",
          clientId,
          channelId: activeId,
          nick: me ?? "",
          body,
          kind,
          state: "pending",
          createdAt: Date.now(),
        });

        try {
          const sent = await clients.chat.sendMessage({
            channelId: activeId,
            clientMessageId: clientId,
            body,
            kind: parsed.kind === "me" ? ProtoKind.ACTION : ProtoKind.NORMAL,
          });
          const message = sent.message;
          if (message !== undefined) {
            useChatStore.getState().reconcile(clientId, {
              id: message.id,
              clientId,
              channelId: activeId,
              nick: message.authorNick,
              body: message.body,
              kind,
              state: "sent",
              createdAt: Date.now(),
            });
          }
        } catch {
          useChatStore.getState().markFailed(clientId);
          systemLine(activeId, SEND_FAILED);
        }
      }
    }
  }, [activeId, clients, draft, me, onLeave, openChannel, setMe, systemLine]);

  const loadOlder = useCallback(async () => {
    if (activeId === null || loading.current) {
      return;
    }
    const cursor = cursors.current.get(activeId);
    if (cursor === undefined) {
      return;
    }
    loading.current = true;

    try {
      const page = await clients.chat.listMessages({
        channelId: activeId,
        pageSize: 100,
        cursor: { value: cursor },
      });

      const older: MessageView[] = [...page.messages].reverse().map((message) => ({
        id: message.id,
        clientId: message.clientMessageId,
        channelId: activeId,
        nick: message.authorNick,
        body: message.body,
        kind:
          message.kind === ProtoKind.SYSTEM
            ? "system"
            : message.kind === ProtoKind.ACTION
              ? "action"
              : "normal",
        state: "sent",
        createdAt: Number(message.createdAt?.seconds ?? 0n) * 1000,
      }));

      useChatStore.getState().prependMessages(activeId, older);

      if (page.nextCursor?.value !== undefined && page.nextCursor.value !== "") {
        cursors.current.set(activeId, page.nextCursor.value);
      } else {
        cursors.current.delete(activeId);
        if (page.windowTruncated) {
          systemLine(activeId, WINDOW_TRUNCATED);
        }
      }
    } catch {
      cursors.current.delete(activeId);
    } finally {
      loading.current = false;
    }
  }, [activeId, clients, systemLine]);

  const setTyping = useCallback(
    (active: boolean) => {
      if (activeId === null || typing.current === active) {
        return;
      }
      typing.current = active;
      void clients.chat.setTyping({ channelId: activeId, typing: active }).catch(() => undefined);
    },
    [activeId, clients],
  );

  const mention = useCallback((nick: string) => {
    setDraft((current) => mentionDraft(current, nick));
  }, []);

  const selectChannel = useCallback(
    (channelId: string) => {
      setActive(channelId);
    },
    [setActive],
  );

  return { draft, setDraft, send, mention, selectChannel, setTyping, openChannel, loadOlder };
}
