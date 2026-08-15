import { useCallback, useEffect, useRef, useState } from "react";

import type { SessionClients } from "@/lib/clients";
import {
  clearChannelCache,
  queuedCount,
  loadChannels,
  loadMessages,
  saveChannels,
  saveMessages,
  saveSession,
} from "@/lib/db";
import { newClientId } from "@/lib/id";
import { flush, isOnline, onOnline, queue } from "@/lib/outbox";
import { UPDATE_AVAILABLE, watchForUpdates } from "@/lib/updates";

import { channelFromSearch, searchWithChannel } from "./channelUrl";
import { applyTheme, helpText, mentionDraft, parseCommand } from "./commands";
import type { MessageView } from "./model";
import { useChatStore } from "./store";

import { MessageKind as ProtoKind } from "@gen/chat/v1/chat_pb";

export const DEFAULT_CHANNEL = "?teste";
export const SEND_FAILED = "message failed to send. press up and send it again.";
export const DM_UNDELIVERED = "not delivered — that nick is not connected.";
export const WINDOW_TRUNCATED = "[older messages were forgotten]";
export const OFFLINE = "[offline]";
export const BACK_ONLINE = "[online]";

export function queuedLine(count: number): string {
  return `[${String(count)} queued]`;
}

export function reconnectingLine(seconds: number): string {
  return `[reconnecting in ${String(seconds)}s]`;
}

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
  const [online, setOnline] = useState(true);

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
    const wanted = channelFromSearch(globalThis.location.search) ?? DEFAULT_CHANNEL;

    void (async () => {
      try {
        const cached = await loadChannels();
        if (cached.length > 0) {
          useChatStore.getState().setChannels(cached);
          for (const channel of cached) {
            useChatStore.getState().prependMessages(channel.id, await loadMessages(channel.id));
          }
          const preferred = cached.find((channel) => channel.name === wanted) ?? cached[0];
          if (preferred !== undefined && useChatStore.getState().activeId === null) {
            setActive(preferred.id);
          }
        }
        await saveSession(me);
      } catch {
        // A cache that will not open costs a slower first paint, nothing else.
      }

      try {
        await openChannel(wanted);
      } catch {
        if (wanted !== DEFAULT_CHANNEL) {
          await openChannel(DEFAULT_CHANNEL).catch(() => undefined);
        }
      }
    })();
  }, [me, openChannel, setActive]);

  useEffect(() => {
    const active = store.channels.find((channel) => channel.id === activeId);
    if (active === undefined) {
      return;
    }
    const current = channelFromSearch(globalThis.location.search);
    if (current === active.name) {
      return;
    }
    const url =
      searchWithChannel(globalThis.location.search, active.name) + globalThis.location.hash;
    if (current === null) {
      globalThis.history.replaceState(globalThis.history.state, "", url);
    } else {
      globalThis.history.pushState(globalThis.history.state, "", url);
    }
  }, [activeId, store.channels]);

  useEffect(() => {
    const onPopState = () => {
      const wanted = channelFromSearch(globalThis.location.search);
      if (wanted === null) {
        return;
      }
      const state = useChatStore.getState();
      const known = state.channels.find((channel) => channel.name === wanted);
      if (known !== undefined) {
        if (known.id !== state.activeId) {
          setActive(known.id);
        }
        return;
      }
      void openChannel(wanted).catch(() => undefined);
    };

    globalThis.addEventListener("popstate", onPopState);
    return () => {
      globalThis.removeEventListener("popstate", onPopState);
    };
  }, [openChannel, setActive]);

  useEffect(() => {
    if (me === null) {
      return;
    }
    const controller = new AbortController();
    const stopped = { value: false };
    const isStopped = () => stopped.value;
    let attempt = 0;
    let timer: ReturnType<typeof setTimeout> | undefined;

    const notify = (body: string) => {
      const target = useChatStore.getState().activeId;
      if (target !== null) {
        systemLine(target, body);
      }
    };

    const run = async () => {
      while (!isStopped()) {
        try {
          for await (const frame of clients.chat.streamEvents(
            { lastEventId: useChatStore.getState().lastEventId },
            { signal: controller.signal },
          )) {
            attempt = 0;
            if (frame.event !== undefined) {
              applyEvent(frame.event);
            }
          }
        } catch {
          if (isStopped()) {
            return;
          }
        }

        if (isStopped()) {
          return;
        }

        attempt += 1;
        const seconds = Math.min(2 ** attempt, 30);
        notify(reconnectingLine(seconds));

        await new Promise<void>((resolve) => {
          timer = setTimeout(resolve, seconds * 1000);
        });
      }
    };

    void run();

    return () => {
      stopped.value = true;
      controller.abort();
      if (timer !== undefined) {
        clearTimeout(timer);
      }
    };
  }, [clients, me, applyEvent, systemLine]);

  useEffect(() => {
    const drain = () => {
      setOnline(true);
      void (async () => {
        const delivered = await flush(async (entry) => {
          await clients.chat.sendMessage({
            channelId: entry.channelId,
            clientMessageId: entry.clientMessageId,
            body: entry.body,
            kind: entry.kind === "action" ? ProtoKind.ACTION : ProtoKind.NORMAL,
          });
        });
        const target = useChatStore.getState().activeId;
        if (target !== null && delivered > 0) {
          systemLine(target, BACK_ONLINE);
        }
      })();
    };

    const dropped = () => {
      setOnline(false);
      const target = useChatStore.getState().activeId;
      if (target !== null) {
        systemLine(target, OFFLINE);
      }
    };

    globalThis.addEventListener("offline", dropped);
    const stopOnline = onOnline(drain);

    if (isOnline()) {
      drain();
    }

    return () => {
      globalThis.removeEventListener("offline", dropped);
      stopOnline();
    };
  }, [clients, systemLine]);

  useEffect(() => {
    const unsubscribe = useChatStore.subscribe((state) => {
      void saveChannels(state.channels).catch(() => undefined);
      for (const [channelId, lines] of Object.entries(state.messages)) {
        void saveMessages(channelId, lines).catch(() => undefined);
      }
    });
    return unsubscribe;
  }, []);

  useEffect(() => {
    return watchForUpdates(() => {
      const target = useChatStore.getState().activeId;
      if (target !== null) {
        systemLine(target, UPDATE_AVAILABLE);
      }
    });
  }, [systemLine]);

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
        void clearChannelCache(activeId).catch(() => undefined);
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
          if (isOnline()) {
            useChatStore.getState().markFailed(clientId);
            systemLine(activeId, SEND_FAILED);
            return;
          }
          await queue({
            clientMessageId: clientId,
            channelId: activeId,
            body,
            kind,
            queuedAt: Date.now(),
          });
          const depth = await queuedCount();
          systemLine(activeId, queuedLine(depth));
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

  return {
    draft,
    setDraft,
    send,
    mention,
    selectChannel,
    setTyping,
    openChannel,
    loadOlder,
    online,
  };
}
