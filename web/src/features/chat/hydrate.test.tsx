import { create } from "@bufbuild/protobuf";
import { render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import type { SessionClients } from "@/lib/clients";
import { clearCache, saveChannels, saveMessages } from "@/lib/db";

import { Chat } from "./Chat";
import type { MessageView } from "./model";
import { useChatStore } from "./store";

import { EventSchema, MessageSchema } from "@gen/chat/v1/chat_pb";

function cached(id: string, body: string): MessageView {
  return {
    id,
    clientId: "",
    channelId: "c1",
    nick: "0x9c",
    body,
    kind: "normal",
    state: "sent",
    createdAt: 1,
  };
}

function stubClients(overrides: { hold?: Promise<void> } = {}): SessionClients {
  return {
    sessions: { setNick: vi.fn(), leave: vi.fn() },
    chat: {
      joinChannel: async () => {
        await (overrides.hold ?? Promise.resolve());
        return { channel: { id: "c1", name: "?teste", memberCount: 1 }, created: false };
      },
      listMessages: () => Promise.resolve({ messages: [], windowTruncated: false }),
      listMembers: () => Promise.resolve({ members: [] }),
      setTyping: () => Promise.resolve({}),
      sendMessage: () => Promise.resolve({ message: undefined, deduplicated: false }),
      streamEvents: () => ({
        [Symbol.asyncIterator]: () => ({
          next: () => Promise.resolve({ done: true as const, value: undefined as never }),
        }),
      }),
    },
  } as unknown as SessionClients;
}

beforeEach(async () => {
  useChatStore.getState().reset();
  await clearCache();
  Object.defineProperty(globalThis, "innerWidth", { value: 1280, configurable: true });
});

describe("boot from cache", () => {
  it("paints the cached transcript before the server answers", async () => {
    await saveChannels([{ id: "c1", name: "?teste", memberCount: 1 }]);
    await saveMessages("c1", [cached("01M001", "cached line")]);

    let release: () => void = () => {
      // replaced below, before the promise is awaited
    };
    const hold = new Promise<void>((resolve) => {
      release = resolve;
    });

    render(<Chat clients={stubClients({ hold })} me="rill" onLeave={vi.fn()} />);

    await waitFor(() => {
      expect(screen.getByText("cached line")).toBeInTheDocument();
    });

    release();
  });

  it("does not duplicate a cached message when it arrives again on the stream", async () => {
    await saveChannels([{ id: "c1", name: "?teste", memberCount: 1 }]);
    await saveMessages("c1", [cached("01M001", "the void, mostly")]);

    render(<Chat clients={stubClients()} me="rill" onLeave={vi.fn()} />);

    await waitFor(() => {
      expect(screen.getByText("the void, mostly")).toBeInTheDocument();
    });

    useChatStore.getState().applyEvent(
      create(EventSchema, {
        eventId: "01M001",
        payload: {
          case: "messageCreated",
          value: {
            message: create(MessageSchema, {
              id: "01M001",
              channelId: "c1",
              authorNick: "0x9c",
              body: "the void, mostly",
            }),
          },
        },
      }),
    );

    await waitFor(() => {
      expect(screen.getAllByText("the void, mostly")).toHaveLength(1);
    });
  });

  it("survives an empty cache without blocking the join", async () => {
    render(<Chat clients={stubClients()} me="rill" onLeave={vi.fn()} />);

    await waitFor(() => {
      expect(screen.getAllByText("?teste").length).toBeGreaterThan(0);
    });
  });
});
