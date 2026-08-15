import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";

import type { SessionClients } from "@/lib/clients";
import { clearCache, saveChannels } from "@/lib/db";

import { Chat } from "./Chat";
import { useChatStore } from "./store";

const ids: Record<string, string> = { "?teste": "c1", "?general": "c2", "?other": "c3" };

function stubClients(known: string[] = Object.keys(ids)): SessionClients {
  const joinChannel = vi.fn(({ name }: { name: string }) => {
    if (!known.includes(name)) {
      return Promise.reject(new Error("no such channel"));
    }
    return Promise.resolve({
      channel: { id: ids[name] ?? name, name, memberCount: 1 },
      created: false,
    });
  });

  return {
    sessions: { setNick: vi.fn(), leave: vi.fn() },
    chat: {
      joinChannel,
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

function joinedNames(clients: SessionClients): string[] {
  const joinChannel = clients.chat.joinChannel as unknown as {
    mock: { calls: [{ name: string }][] };
  };
  return joinChannel.mock.calls.map(([request]) => request.name);
}

function visit(search: string) {
  globalThis.history.replaceState({}, "", `/${search}`);
}

beforeEach(async () => {
  useChatStore.getState().reset();
  await clearCache();
  visit("");
  Object.defineProperty(globalThis, "innerWidth", { value: 1280, configurable: true });
});

describe("channel in the query string", () => {
  it("opens the channel named by ?keyword instead of the default", async () => {
    visit("?general");
    const clients = stubClients();

    render(<Chat clients={clients} me="rill" onLeave={vi.fn()} />);

    await waitFor(() => {
      expect(joinedNames(clients)).toContain("?general");
    });
    expect(joinedNames(clients)).not.toContain("?teste");
    await waitFor(() => {
      expect(useChatStore.getState().activeId).toBe("c2");
    });
    expect(globalThis.location.search).toBe("?general");
  });

  it("writes the default channel into the query string when there is none", async () => {
    render(<Chat clients={stubClients()} me="rill" onLeave={vi.fn()} />);

    await waitFor(() => {
      expect(globalThis.location.search).toBe("?teste");
    });
  });

  it("falls back to the default channel when the keyword is not joinable", async () => {
    visit("?nope");
    const clients = stubClients(["?teste"]);

    render(<Chat clients={clients} me="rill" onLeave={vi.fn()} />);

    await waitFor(() => {
      expect(useChatStore.getState().activeId).toBe("c1");
    });
    expect(joinedNames(clients)).toEqual(["?nope", "?teste"]);
    await waitFor(() => {
      expect(globalThis.location.search).toBe("?teste");
    });
  });

  it("reflects a channel switch in the query string", async () => {
    await saveChannels([
      { id: "c1", name: "?teste", memberCount: 1 },
      { id: "c3", name: "?other", memberCount: 1 },
    ]);
    const user = userEvent.setup();

    render(<Chat clients={stubClients()} me="rill" onLeave={vi.fn()} />);

    await waitFor(() => {
      expect(globalThis.location.search).toBe("?teste");
    });

    await user.click(screen.getByText("?other"));

    await waitFor(() => {
      expect(globalThis.location.search).toBe("?other");
    });
    expect(useChatStore.getState().activeId).toBe("c3");
  });

  it("follows the back button to the previous channel", async () => {
    await saveChannels([
      { id: "c1", name: "?teste", memberCount: 1 },
      { id: "c3", name: "?other", memberCount: 1 },
    ]);
    const user = userEvent.setup();

    render(<Chat clients={stubClients()} me="rill" onLeave={vi.fn()} />);

    await waitFor(() => {
      expect(globalThis.location.search).toBe("?teste");
    });
    await user.click(screen.getByText("?other"));
    await waitFor(() => {
      expect(useChatStore.getState().activeId).toBe("c3");
    });

    globalThis.history.back();

    await waitFor(() => {
      expect(globalThis.location.search).toBe("?teste");
    });
    await waitFor(() => {
      expect(useChatStore.getState().activeId).toBe("c1");
    });
  });
});
