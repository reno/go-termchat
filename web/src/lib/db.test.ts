import { beforeEach, describe, expect, it } from "vitest";

import type { MessageView } from "@/features/chat/model";

import {
  MESSAGES_PER_CHANNEL,
  clearCache,
  clearChannelCache,
  dequeue,
  enqueue,
  loadChannels,
  loadMessages,
  loadSession,
  queued,
  queuedCount,
  saveChannels,
  saveMessages,
  saveSession,
} from "./db";

function message(index: number, channelId = "c1"): MessageView {
  return {
    id: `01M${String(index).padStart(20, "0")}`,
    clientId: "",
    channelId,
    nick: "rill",
    body: `line ${String(index)}`,
    kind: "normal",
    state: "sent",
    createdAt: index,
  };
}

beforeEach(async () => {
  await clearCache();
});

describe("session handle", () => {
  it("round-trips the nick", async () => {
    await saveSession("rill");
    expect((await loadSession())?.nick).toBe("rill");
  });
});

describe("channels", () => {
  it("round-trips the channel list", async () => {
    await saveChannels([
      { id: "c1", name: "?teste", memberCount: 2 },
      { id: "c2", name: "?lounge", memberCount: 0 },
    ]);

    const stored = await loadChannels();
    expect(stored).toHaveLength(2);
    expect(stored.map((channel) => channel.name).sort()).toEqual(["?lounge", "?teste"]);
  });
});

describe("messages", () => {
  it("round-trips a transcript in id order", async () => {
    await saveMessages("c1", [message(3), message(1), message(2)]);

    const stored = await loadMessages("c1");
    expect(stored.map((line) => line.body)).toEqual(["line 1", "line 2", "line 3"]);
  });

  it("keeps channels apart", async () => {
    await saveMessages("c1", [message(1, "c1")]);
    await saveMessages("c2", [message(2, "c2")]);

    expect(await loadMessages("c1")).toHaveLength(1);
    expect(await loadMessages("c2")).toHaveLength(1);
  });

  it("never stores an unsent line, which has no id yet", async () => {
    await saveMessages("c1", [{ ...message(1), id: "" }]);
    expect(await loadMessages("c1")).toHaveLength(0);
  });

  it("caps a channel at 200 messages, evicting oldest first", async () => {
    const lines = Array.from({ length: MESSAGES_PER_CHANNEL + 25 }, (_, index) =>
      message(index + 1),
    );
    await saveMessages("c1", lines);

    const stored = await loadMessages("c1");
    expect(stored).toHaveLength(MESSAGES_PER_CHANNEL);
    expect(stored[0]?.body).toBe("line 26");
    expect(stored.at(-1)?.body).toBe(`line ${String(MESSAGES_PER_CHANNEL + 25)}`);
  });
});

describe("cache lifecycle", () => {
  it("clears one channel on /clear and leaves the others", async () => {
    await saveMessages("c1", [message(1, "c1")]);
    await saveMessages("c2", [message(2, "c2")]);

    await clearChannelCache("c1");

    expect(await loadMessages("c1")).toHaveLength(0);
    expect(await loadMessages("c2")).toHaveLength(1);
  });

  it("empties every object store on leave", async () => {
    await saveSession("rill");
    await saveChannels([{ id: "c1", name: "?teste", memberCount: 1 }]);
    await saveMessages("c1", [message(1)]);
    await enqueue({
      clientMessageId: "q1",
      channelId: "c1",
      body: "queued",
      kind: "normal",
      queuedAt: 1,
    });

    await clearCache();

    expect(await loadSession()).toBeUndefined();
    expect(await loadChannels()).toHaveLength(0);
    expect(await loadMessages("c1")).toHaveLength(0);
    expect(await queued()).toHaveLength(0);
  });
});

describe("outbox", () => {
  it("queues in the order it was written", async () => {
    await enqueue({
      clientMessageId: "a",
      channelId: "c1",
      body: "1",
      kind: "normal",
      queuedAt: 3,
    });
    await enqueue({
      clientMessageId: "b",
      channelId: "c1",
      body: "2",
      kind: "normal",
      queuedAt: 1,
    });
    await enqueue({
      clientMessageId: "c",
      channelId: "c1",
      body: "3",
      kind: "normal",
      queuedAt: 2,
    });

    expect((await queued()).map((entry) => entry.clientMessageId)).toEqual(["b", "c", "a"]);
    expect(await queuedCount()).toBe(3);
  });

  it("is keyed on client_message_id, so a retry does not duplicate", async () => {
    const entry = {
      clientMessageId: "same",
      channelId: "c1",
      body: "once",
      kind: "normal" as const,
      queuedAt: 1,
    };
    await enqueue(entry);
    await enqueue(entry);

    expect(await queuedCount()).toBe(1);
  });

  it("drops an entry once it is delivered", async () => {
    await enqueue({
      clientMessageId: "a",
      channelId: "c1",
      body: "1",
      kind: "normal",
      queuedAt: 1,
    });
    await dequeue("a");
    expect(await queuedCount()).toBe(0);
  });
});
