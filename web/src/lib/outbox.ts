import type { OutboxEntry } from "./db";
import { dequeue, enqueue, queued } from "./db";

export const SYNC_TAG = "termchat-outbox";

export type SendAttempt = (entry: OutboxEntry) => Promise<void>;

export async function queue(entry: OutboxEntry): Promise<void> {
  await enqueue(entry);
  await requestBackgroundSync();
}

export async function flush(send: SendAttempt): Promise<number> {
  const pending = await queued();
  let delivered = 0;

  for (const entry of pending) {
    try {
      await send(entry);
      await dequeue(entry.clientMessageId);
      delivered += 1;
    } catch {
      break;
    }
  }
  return delivered;
}

interface SyncRegistration extends ServiceWorkerRegistration {
  sync?: { register: (tag: string) => Promise<void> };
}

export async function requestBackgroundSync(): Promise<boolean> {
  if (!("serviceWorker" in navigator)) {
    return false;
  }
  try {
    const registration = (await navigator.serviceWorker.ready) as SyncRegistration;
    if (registration.sync === undefined) {
      return false;
    }
    await registration.sync.register(SYNC_TAG);
    return true;
  } catch {
    return false;
  }
}

export function onOnline(listener: () => void): () => void {
  globalThis.addEventListener("online", listener);
  return () => {
    globalThis.removeEventListener("online", listener);
  };
}

export function isOnline(): boolean {
  return navigator.onLine;
}
