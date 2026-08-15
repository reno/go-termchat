import { create } from "zustand";

import type { ChannelView, MemberView, MessageKind, MessageView } from "./model";

import type { Event } from "@gen/chat/v1/chat_pb";
import { MessageKind as ProtoKind } from "@gen/chat/v1/chat_pb";
import { Presence } from "@gen/chat/v1/session_pb";

const MAX_LINES = 500;

function kindOf(kind: ProtoKind): MessageKind {
  switch (kind) {
    case ProtoKind.SYSTEM:
      return "system";
    case ProtoKind.ACTION:
      return "action";
    case ProtoKind.DIRECT:
      return "dm";
    case ProtoKind.NORMAL:
    case ProtoKind.UNSPECIFIED:
      return "normal";
  }
}

export interface ChatState {
  me: string | null;
  activeId: string | null;
  channels: ChannelView[];
  messages: Record<string, MessageView[]>;
  members: Record<string, MemberView[]>;
  lastEventId: string;

  setMe: (nick: string | null) => void;
  setChannels: (channels: ChannelView[]) => void;
  upsertChannel: (channel: ChannelView) => void;
  setActive: (channelId: string) => void;
  setMembers: (channelId: string, members: MemberView[]) => void;
  prependMessages: (channelId: string, messages: MessageView[]) => void;
  appendMessage: (message: MessageView) => void;
  reconcile: (clientId: string, message: MessageView) => void;
  markFailed: (clientId: string) => void;
  systemLine: (channelId: string, body: string) => void;
  clearChannel: (channelId: string) => void;
  applyEvent: (event: Event) => void;
  reset: () => void;
}

function capped(lines: MessageView[]): MessageView[] {
  return lines.length <= MAX_LINES ? lines : lines.slice(lines.length - MAX_LINES);
}

function withMember(members: MemberView[], nick: string, tag: string): MemberView[] {
  const existing = members.findIndex((member) => member.nick === nick);
  if (existing === -1) {
    return [...members, { nick, tag }];
  }
  const next = [...members];
  next[existing] = { nick, tag };
  return next;
}

let systemCounter = 0;

export function systemMessage(channelId: string, body: string): MessageView {
  systemCounter += 1;
  return {
    id: `local-system-${String(systemCounter)}`,
    clientId: "",
    channelId,
    nick: "*",
    body,
    kind: "system",
    state: "sent",
    createdAt: Date.now(),
  };
}

export const useChatStore = create<ChatState>()((set) => ({
  me: null,
  activeId: null,
  channels: [],
  messages: {},
  members: {},
  lastEventId: "",

  setMe: (nick) => {
    set({ me: nick });
  },

  setChannels: (channels) => {
    set({ channels });
  },

  upsertChannel: (channel) => {
    set((state) => {
      const index = state.channels.findIndex((known) => known.id === channel.id);
      const channels =
        index === -1
          ? [...state.channels, channel].sort((a, b) => a.name.localeCompare(b.name))
          : state.channels.map((known) => (known.id === channel.id ? channel : known));
      return { channels };
    });
  },

  setActive: (channelId) => {
    set({ activeId: channelId });
  },

  setMembers: (channelId, members) => {
    set((state) => ({ members: { ...state.members, [channelId]: members } }));
  },

  prependMessages: (channelId, messages) => {
    set((state) => {
      const known = new Set((state.messages[channelId] ?? []).map((message) => message.id));
      const fresh = messages.filter((message) => !known.has(message.id));
      return {
        messages: {
          ...state.messages,
          [channelId]: [...fresh, ...(state.messages[channelId] ?? [])],
        },
      };
    });
  },

  appendMessage: (message) => {
    set((state) => {
      const lines = state.messages[message.channelId] ?? [];
      if (message.id !== "" && lines.some((line) => line.id === message.id)) {
        return state;
      }
      if (message.clientId !== "") {
        const pending = lines.findIndex(
          (line) => line.clientId === message.clientId && line.state !== "sent",
        );
        if (pending !== -1) {
          const next = [...lines];
          next[pending] = message;
          return { messages: { ...state.messages, [message.channelId]: next } };
        }
      }
      return {
        messages: { ...state.messages, [message.channelId]: capped([...lines, message]) },
      };
    });
  },

  reconcile: (clientId, message) => {
    set((state) => {
      const lines = state.messages[message.channelId] ?? [];
      const index = lines.findIndex((line) => line.clientId === clientId);
      if (index === -1) {
        return {
          messages: { ...state.messages, [message.channelId]: capped([...lines, message]) },
        };
      }
      const next = [...lines];
      next[index] = message;
      return { messages: { ...state.messages, [message.channelId]: next } };
    });
  },

  markFailed: (clientId) => {
    set((state) => {
      const messages: Record<string, MessageView[]> = {};
      for (const [channelId, lines] of Object.entries(state.messages)) {
        messages[channelId] = lines.map((line) =>
          line.clientId === clientId ? { ...line, state: "failed" } : line,
        );
      }
      return { messages };
    });
  },

  systemLine: (channelId, body) => {
    set((state) => ({
      messages: {
        ...state.messages,
        [channelId]: capped([...(state.messages[channelId] ?? []), systemMessage(channelId, body)]),
      },
    }));
  },

  clearChannel: (channelId) => {
    set((state) => ({ messages: { ...state.messages, [channelId]: [] } }));
  },

  applyEvent: (event) => {
    set((state) => {
      const next: Partial<ChatState> = { lastEventId: event.eventId };
      const payload = event.payload;

      switch (payload.case) {
        case "messageCreated": {
          const message = payload.value.message;
          if (message === undefined) {
            return next;
          }
          const view: MessageView = {
            id: message.id,
            clientId: message.clientMessageId,
            channelId: message.channelId,
            nick: message.authorNick,
            body: message.body,
            kind: kindOf(message.kind),
            state: "sent",
            createdAt: Number(message.createdAt?.seconds ?? 0n) * 1000,
          };
          const target = view.channelId === "" ? (state.activeId ?? "") : view.channelId;
          const lines = state.messages[target] ?? [];

          if (view.id !== "" && lines.some((line) => line.id === view.id)) {
            return next;
          }
          const pending =
            view.clientId === ""
              ? -1
              : lines.findIndex((line) => line.clientId === view.clientId && line.state !== "sent");

          if (pending !== -1) {
            const replaced = [...lines];
            replaced[pending] = { ...view, channelId: target };
            return { ...next, messages: { ...state.messages, [target]: replaced } };
          }
          return {
            ...next,
            messages: {
              ...state.messages,
              [target]: capped([...lines, { ...view, channelId: target }]),
            },
          };
        }

        case "directMessage": {
          const message = payload.value.message;
          if (message === undefined) {
            return next;
          }
          const outgoing = message.authorNick === state.me;
          const target = state.activeId ?? "";
          const view: MessageView = {
            id: message.id,
            clientId: message.clientMessageId,
            channelId: target,
            nick: outgoing ? `→ ${payload.value.recipientNick}` : `→ ${message.authorNick}`,
            body: message.body,
            kind: "dm",
            state: "sent",
            createdAt: Number(message.createdAt?.seconds ?? 0n) * 1000,
          };
          const lines = state.messages[target] ?? [];
          if (view.id !== "" && lines.some((line) => line.id === view.id)) {
            return next;
          }
          return { ...next, messages: { ...state.messages, [target]: capped([...lines, view]) } };
        }

        case "memberJoined": {
          const { channelId, member } = payload.value;
          if (member === undefined) {
            return next;
          }
          return {
            ...next,
            members: {
              ...state.members,
              [channelId]: withMember(state.members[channelId] ?? [], member.nick, ""),
            },
          };
        }

        case "memberLeft": {
          const { channelId, nick } = payload.value;
          return {
            ...next,
            members: {
              ...state.members,
              [channelId]: (state.members[channelId] ?? []).filter(
                (member) => member.nick !== nick,
              ),
            },
          };
        }

        case "typingChanged": {
          const { channelId, nick, typing } = payload.value;
          const members = state.members[channelId] ?? [];
          if (!members.some((member) => member.nick === nick)) {
            return next;
          }
          return {
            ...next,
            members: {
              ...state.members,
              [channelId]: members.map((member) =>
                member.nick === nick ? { ...member, tag: typing ? "typing" : "" } : member,
              ),
            },
          };
        }

        case "presenceChanged": {
          const { nick, presence } = payload.value;
          const tag = presence === Presence.IDLE ? "idle" : "";
          const members: Record<string, MemberView[]> = {};
          for (const [channelId, roster] of Object.entries(state.members)) {
            members[channelId] = roster.map((member) =>
              member.nick === nick && member.tag !== "typing" ? { ...member, tag } : member,
            );
          }
          return { ...next, members };
        }

        case undefined:
          return next;
      }
    });
  },

  reset: () => {
    set({
      me: null,
      activeId: null,
      channels: [],
      messages: {},
      members: {},
      lastEventId: "",
    });
  },
}));
