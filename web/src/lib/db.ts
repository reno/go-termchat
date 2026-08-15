import { openDB } from "idb";
import type { DBSchema, IDBPDatabase } from "idb";

import type { ChannelView, MessageView } from "@/features/chat/model";

export const DB_NAME = "termchat";
export const DB_VERSION = 1;
export const MESSAGES_PER_CHANNEL = 200;

export interface OutboxEntry {
  clientMessageId: string;
  channelId: string;
  body: string;
  kind: "normal" | "action";
  queuedAt: number;
}

export interface SessionHandle {
  nick: string;
  savedAt: number;
}

interface TermchatDB extends DBSchema {
  session: {
    key: string;
    value: SessionHandle;
  };
  channels: {
    key: string;
    value: ChannelView;
  };
  messages: {
    key: string;
    value: MessageView & { channelId: string };
    indexes: { "by-channel": string };
  };
  outbox: {
    key: string;
    value: OutboxEntry;
    indexes: { "by-queued": number };
  };
}

export type Database = IDBPDatabase<TermchatDB>;

let connection: Promise<Database> | null = null;

export function openCache(): Promise<Database> {
  connection ??= openDB<TermchatDB>(DB_NAME, DB_VERSION, {
    upgrade(db) {
      db.createObjectStore("session");
      db.createObjectStore("channels", { keyPath: "id" });

      const messages = db.createObjectStore("messages", { keyPath: "id" });
      messages.createIndex("by-channel", "channelId");

      const outbox = db.createObjectStore("outbox", { keyPath: "clientMessageId" });
      outbox.createIndex("by-queued", "queuedAt");
    },
  });
  return connection;
}

export function resetConnection(): void {
  connection = null;
}

export async function saveSession(nick: string): Promise<void> {
  const db = await openCache();
  await db.put("session", { nick, savedAt: Date.now() }, "current");
}

export async function loadSession(): Promise<SessionHandle | undefined> {
  const db = await openCache();
  return db.get("session", "current");
}

export async function saveChannels(channels: ChannelView[]): Promise<void> {
  const db = await openCache();
  const tx = db.transaction("channels", "readwrite");
  await Promise.all(channels.map((channel) => tx.store.put(channel)));
  await tx.done;
}

export async function loadChannels(): Promise<ChannelView[]> {
  const db = await openCache();
  return db.getAll("channels");
}

export async function saveMessages(channelId: string, messages: MessageView[]): Promise<void> {
  const db = await openCache();
  const tx = db.transaction("messages", "readwrite");

  for (const message of messages) {
    if (message.id === "") {
      continue;
    }
    await tx.store.put({ ...message, channelId });
  }
  await tx.done;
  await evict(channelId);
}

export async function loadMessages(channelId: string): Promise<MessageView[]> {
  const db = await openCache();
  const stored = await db.getAllFromIndex("messages", "by-channel", channelId);
  return stored.sort((a, b) => a.id.localeCompare(b.id));
}

export async function evict(channelId: string): Promise<number> {
  const db = await openCache();
  const stored = await db.getAllFromIndex("messages", "by-channel", channelId);
  if (stored.length <= MESSAGES_PER_CHANNEL) {
    return 0;
  }

  const oldest = stored
    .sort((a, b) => a.id.localeCompare(b.id))
    .slice(0, stored.length - MESSAGES_PER_CHANNEL);

  const tx = db.transaction("messages", "readwrite");
  for (const message of oldest) {
    await tx.store.delete(message.id);
  }
  await tx.done;
  return oldest.length;
}

export async function clearChannelCache(channelId: string): Promise<void> {
  const db = await openCache();
  const stored = await db.getAllFromIndex("messages", "by-channel", channelId);

  const tx = db.transaction("messages", "readwrite");
  for (const message of stored) {
    await tx.store.delete(message.id);
  }
  await tx.done;
}

export async function enqueue(entry: OutboxEntry): Promise<void> {
  const db = await openCache();
  await db.put("outbox", entry);
}

export async function queued(): Promise<OutboxEntry[]> {
  const db = await openCache();
  return db.getAllFromIndex("outbox", "by-queued");
}

export async function dequeue(clientMessageId: string): Promise<void> {
  const db = await openCache();
  await db.delete("outbox", clientMessageId);
}

export async function queuedCount(): Promise<number> {
  const db = await openCache();
  return db.count("outbox");
}

export async function clearCache(): Promise<void> {
  const db = await openCache();
  const tx = db.transaction(["session", "channels", "messages", "outbox"], "readwrite");
  await Promise.all([
    tx.objectStore("session").clear(),
    tx.objectStore("channels").clear(),
    tx.objectStore("messages").clear(),
    tx.objectStore("outbox").clear(),
  ]);
  await tx.done;
}
