import { beforeEach, describe, expect, it, vi } from "vitest";

import { clearCache, queued, queuedCount } from "./db";
import type { OutboxEntry } from "./db";
import { flush, isOnline, onOnline, queue } from "./outbox";

function entry(id: string, body: string): OutboxEntry {
  return {
    clientMessageId: id,
    channelId: "c1",
    body,
    kind: "normal",
    queuedAt: Number(id.replace(/\D/g, "")),
  };
}

beforeEach(async () => {
  await clearCache();
  vi.restoreAllMocks();
});

describe("queueing", () => {
  it("survives Background Sync being unavailable", async () => {
    await expect(queue(entry("q1", "hello"))).resolves.toBeUndefined();
    expect(await queuedCount()).toBe(1);
  });
});

describe("flush", () => {
  it("delivers every queued message once, in order", async () => {
    await queue(entry("q1", "one"));
    await queue(entry("q2", "two"));
    await queue(entry("q3", "three"));

    const sent: string[] = [];
    const delivered = await flush((item) => {
      sent.push(item.body);
      return Promise.resolve();
    });

    expect(delivered).toBe(3);
    expect(sent).toEqual(["one", "two", "three"]);
    expect(await queuedCount()).toBe(0);
  });

  it("keeps the queue when a send fails, so nothing is lost", async () => {
    await queue(entry("q1", "one"));
    await queue(entry("q2", "two"));

    const delivered = await flush((item) => {
      if (item.body === "two") {
        return Promise.reject(new Error("still offline"));
      }
      return Promise.resolve();
    });

    expect(delivered).toBe(1);
    const left = await queued();
    expect(left.map((item) => item.body)).toEqual(["two"]);
  });

  it("stops at the first failure rather than reordering the queue", async () => {
    await queue(entry("q1", "one"));
    await queue(entry("q2", "two"));
    await queue(entry("q3", "three"));

    const attempted: string[] = [];
    await flush((item) => {
      attempted.push(item.body);
      return Promise.reject(new Error("offline"));
    });

    expect(attempted).toEqual(["one"]);
    expect(await queuedCount()).toBe(3);
  });

  it("is a no-op on an empty queue", async () => {
    const send = vi.fn(() => Promise.resolve());
    expect(await flush(send)).toBe(0);
    expect(send).not.toHaveBeenCalled();
  });

  it("re-flushing after a partial delivery does not resend what landed", async () => {
    await queue(entry("q1", "one"));
    await queue(entry("q2", "two"));

    const sent: string[] = [];
    let stillOffline = true;
    const attempt = (item: OutboxEntry) => {
      if (item.body === "two" && stillOffline) {
        stillOffline = false;
        return Promise.reject(new Error("offline"));
      }
      sent.push(item.body);
      return Promise.resolve();
    };

    await flush(attempt);
    await flush(attempt);

    expect(sent).toEqual(["one", "two"]);
    expect(await queuedCount()).toBe(0);
  });
});

describe("connectivity", () => {
  it("reports the browser's own view", () => {
    expect(typeof isOnline()).toBe("boolean");
  });

  it("subscribes and unsubscribes from the online event", () => {
    const listener = vi.fn();
    const stop = onOnline(listener);

    globalThis.dispatchEvent(new Event("online"));
    expect(listener).toHaveBeenCalledOnce();

    stop();
    globalThis.dispatchEvent(new Event("online"));
    expect(listener).toHaveBeenCalledOnce();
  });
});
