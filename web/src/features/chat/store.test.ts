import { create } from "@bufbuild/protobuf";
import { beforeEach, describe, expect, it } from "vitest";

import type { MessageView } from "./model";
import { useChatStore } from "./store";

import { EventSchema, MessageKind as ProtoKind, MessageSchema } from "@gen/chat/v1/chat_pb";
import { Presence } from "@gen/chat/v1/session_pb";

function messageEvent(overrides: {
  id: string;
  channelId?: string;
  clientMessageId?: string;
  authorNick?: string;
  body?: string;
  kind?: ProtoKind;
}) {
  return create(EventSchema, {
    eventId: overrides.id,
    payload: {
      case: "messageCreated",
      value: {
        message: create(MessageSchema, {
          id: overrides.id,
          clientMessageId: overrides.clientMessageId ?? "",
          channelId: overrides.channelId ?? "c1",
          authorNick: overrides.authorNick ?? "0x9c",
          body: overrides.body ?? "hello",
          kind: overrides.kind ?? ProtoKind.NORMAL,
        }),
      },
    },
  });
}

function pending(clientId: string): MessageView {
  return {
    id: "",
    clientId,
    channelId: "c1",
    nick: "rill",
    body: "in flight",
    kind: "normal",
    state: "pending",
    createdAt: 0,
  };
}

beforeEach(() => {
  useChatStore.getState().reset();
  useChatStore.getState().setActive("c1");
  useChatStore.getState().setMe("rill");
});

describe("event reduction", () => {
  it("appends a created message", () => {
    useChatStore.getState().applyEvent(messageEvent({ id: "m1", body: "the void, mostly" }));

    const lines = useChatStore.getState().messages.c1 ?? [];
    expect(lines).toHaveLength(1);
    expect(lines[0]?.body).toBe("the void, mostly");
    expect(useChatStore.getState().lastEventId).toBe("m1");
  });

  it("ignores a message it has already seen", () => {
    useChatStore.getState().applyEvent(messageEvent({ id: "m1" }));
    useChatStore.getState().applyEvent(messageEvent({ id: "m1" }));

    expect(useChatStore.getState().messages.c1).toHaveLength(1);
  });

  it("reconciles a pending line rather than duplicating it", () => {
    useChatStore.getState().appendMessage(pending("client-1"));
    expect(useChatStore.getState().messages.c1?.[0]?.state).toBe("pending");

    useChatStore
      .getState()
      .applyEvent(messageEvent({ id: "m1", clientMessageId: "client-1", authorNick: "rill" }));

    const lines = useChatStore.getState().messages.c1 ?? [];
    expect(lines).toHaveLength(1);
    expect(lines[0]?.id).toBe("m1");
    expect(lines[0]?.state).toBe("sent");
  });

  it("carries the system kind through", () => {
    useChatStore
      .getState()
      .applyEvent(messageEvent({ id: "m1", kind: ProtoKind.SYSTEM, authorNick: "*" }));

    expect(useChatStore.getState().messages.c1?.[0]?.kind).toBe("system");
  });

  it("tracks the roster as members join and leave", () => {
    useChatStore.getState().applyEvent(
      create(EventSchema, {
        eventId: "e1",
        payload: {
          case: "memberJoined",
          value: { channelId: "c1", member: { nick: "0x9c" } },
        },
      }),
    );
    expect(useChatStore.getState().members.c1).toHaveLength(1);

    useChatStore.getState().applyEvent(
      create(EventSchema, {
        eventId: "e2",
        payload: { case: "memberLeft", value: { channelId: "c1", nick: "0x9c" } },
      }),
    );
    expect(useChatStore.getState().members.c1).toHaveLength(0);
  });

  it("puts typing in the roster tag slot", () => {
    useChatStore.getState().setMembers("c1", [{ nick: "0x9c", tag: "" }]);

    useChatStore.getState().applyEvent(
      create(EventSchema, {
        eventId: "e1",
        payload: {
          case: "typingChanged",
          value: { channelId: "c1", nick: "0x9c", typing: true },
        },
      }),
    );
    expect(useChatStore.getState().members.c1?.[0]?.tag).toBe("typing");

    useChatStore.getState().applyEvent(
      create(EventSchema, {
        eventId: "e2",
        payload: {
          case: "typingChanged",
          value: { channelId: "c1", nick: "0x9c", typing: false },
        },
      }),
    );
    expect(useChatStore.getState().members.c1?.[0]?.tag).toBe("");
  });

  it("puts idle presence in the same tag slot, without clobbering typing", () => {
    useChatStore.getState().setMembers("c1", [
      { nick: "0x9c", tag: "" },
      { nick: "anon2", tag: "typing" },
    ]);

    useChatStore.getState().applyEvent(
      create(EventSchema, {
        eventId: "e1",
        payload: { case: "presenceChanged", value: { nick: "0x9c", presence: Presence.IDLE } },
      }),
    );
    expect(useChatStore.getState().members.c1?.[0]?.tag).toBe("idle");

    useChatStore.getState().applyEvent(
      create(EventSchema, {
        eventId: "e2",
        payload: { case: "presenceChanged", value: { nick: "anon2", presence: Presence.IDLE } },
      }),
    );
    expect(useChatStore.getState().members.c1?.[1]?.tag).toBe("typing");
  });
});

describe("optimistic send", () => {
  it("marks a line failed without losing it", () => {
    useChatStore.getState().appendMessage(pending("client-1"));
    useChatStore.getState().markFailed("client-1");

    const lines = useChatStore.getState().messages.c1 ?? [];
    expect(lines).toHaveLength(1);
    expect(lines[0]?.state).toBe("failed");
    expect(lines[0]?.body).toBe("in flight");
  });

  it("reconciles by client id even when the id arrives late", () => {
    useChatStore.getState().appendMessage(pending("client-1"));
    useChatStore.getState().reconcile("client-1", {
      ...pending("client-1"),
      id: "m1",
      state: "sent",
    });

    const lines = useChatStore.getState().messages.c1 ?? [];
    expect(lines).toHaveLength(1);
    expect(lines[0]?.id).toBe("m1");
  });
});

describe("clear", () => {
  it("empties only the named channel", () => {
    useChatStore.getState().applyEvent(messageEvent({ id: "m1", channelId: "c1" }));
    useChatStore.getState().applyEvent(messageEvent({ id: "m2", channelId: "c2" }));

    useChatStore.getState().clearChannel("c1");

    expect(useChatStore.getState().messages.c1).toHaveLength(0);
    expect(useChatStore.getState().messages.c2).toHaveLength(1);
  });
});
