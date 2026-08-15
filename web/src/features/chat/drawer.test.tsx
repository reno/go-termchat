import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { SessionClients } from "@/lib/clients";

import { Chat } from "./Chat";
import { useChatStore } from "./store";

function resize(width: number): void {
  Object.defineProperty(globalThis, "innerWidth", { value: width, configurable: true });
  globalThis.dispatchEvent(new Event("resize"));
}

function emptyStream(): AsyncIterable<never> {
  return {
    [Symbol.asyncIterator]: () => ({
      next: () => Promise.resolve({ done: true as const, value: undefined as never }),
    }),
  };
}

function stubClients(): SessionClients {
  return {
    sessions: { setNick: vi.fn(), leave: vi.fn() },
    chat: {
      joinChannel: () =>
        Promise.resolve({ channel: { id: "c1", name: "?teste", memberCount: 2 }, created: false }),
      listMessages: () => Promise.resolve({ messages: [], windowTruncated: false }),
      listMembers: () => Promise.resolve({ members: [{ nick: "0x9c", typing: false }] }),
      setTyping: () => Promise.resolve({}),
      sendMessage: () => Promise.resolve({ message: undefined, deduplicated: false }),
      streamEvents: () => emptyStream(),
    },
  } as unknown as SessionClients;
}

beforeEach(() => {
  useChatStore.getState().reset();
});

afterEach(() => {
  resize(1280);
});

describe("narrow drawer", () => {
  it("is closed by default at 375px", async () => {
    resize(375);
    render(<Chat clients={stubClients()} me="rill" onLeave={vi.fn()} />);

    const toggle = await screen.findByRole("button", { name: "channels and roster" });
    expect(toggle).toHaveAttribute("aria-expanded", "false");
    expect(screen.queryByRole("complementary", { name: "channels and roster" })).toBeNull();
  });

  it("opens on the toggle and closes on the scrim", async () => {
    resize(375);
    const user = userEvent.setup();
    const { container } = render(<Chat clients={stubClients()} me="rill" onLeave={vi.fn()} />);

    const toggle = await screen.findByRole("button", { name: "channels and roster" });
    await user.click(toggle);

    expect(toggle).toHaveAttribute("aria-expanded", "true");
    expect(screen.getByRole("complementary", { name: "channels and roster" })).toBeVisible();

    const scrim = container.querySelector(".chat__scrim");
    if (scrim === null) {
      throw new Error("the drawer opened without a scrim");
    }
    await user.click(scrim);

    expect(toggle).toHaveAttribute("aria-expanded", "false");
  });

  it("closes when a channel is chosen", async () => {
    resize(375);
    const user = userEvent.setup();
    render(<Chat clients={stubClients()} me="rill" onLeave={vi.fn()} />);

    const toggle = await screen.findByRole("button", { name: "channels and roster" });
    await user.click(toggle);

    const row = await screen.findByRole("button", { name: /\?teste/ });
    await user.click(row);

    expect(toggle).toHaveAttribute("aria-expanded", "false");
  });

  it("traps focus inside the open drawer", async () => {
    resize(375);
    const user = userEvent.setup();
    render(<Chat clients={stubClients()} me="rill" onLeave={vi.fn()} />);

    const toggle = await screen.findByRole("button", { name: "channels and roster" });
    await user.click(toggle);

    const panel = screen.getByRole("complementary", { name: "channels and roster" });
    await waitFor(() => {
      expect(panel.contains(document.activeElement)).toBe(true);
    });

    for (let i = 0; i < 8; i++) {
      await user.tab();
      expect(panel.contains(document.activeElement)).toBe(true);
    }
  });

  it("returns focus to the toggle when it closes", async () => {
    resize(375);
    const user = userEvent.setup();
    render(<Chat clients={stubClients()} me="rill" onLeave={vi.fn()} />);

    const toggle = await screen.findByRole("button", { name: "channels and roster" });
    await user.click(toggle);
    await user.keyboard("{Escape}");

    await waitFor(() => {
      expect(toggle).toHaveFocus();
    });
  });

  it("closes on Escape", async () => {
    resize(375);
    const user = userEvent.setup();
    render(<Chat clients={stubClients()} me="rill" onLeave={vi.fn()} />);

    const toggle = await screen.findByRole("button", { name: "channels and roster" });
    await user.click(toggle);
    expect(toggle).toHaveAttribute("aria-expanded", "true");

    await user.keyboard("{Escape}");
    expect(toggle).toHaveAttribute("aria-expanded", "false");
  });

  it("shows the panel and hides the toggle when wide", async () => {
    resize(1280);
    render(<Chat clients={stubClients()} me="rill" onLeave={vi.fn()} />);

    await waitFor(() => {
      expect(screen.getByRole("complementary", { name: "channels and roster" })).toBeVisible();
    });
    expect(screen.queryByRole("button", { name: "channels and roster" })).not.toBeInTheDocument();
  });

  it("hides the hint bar on narrow and shows it on wide", async () => {
    resize(375);
    const { container, rerender } = render(
      <Chat clients={stubClients()} me="rill" onLeave={vi.fn()} />,
    );
    await screen.findByRole("log", { name: "transcript" });
    expect(container.querySelector(".chat__hints")).toBeNull();

    resize(1280);
    rerender(<Chat clients={stubClients()} me="rill" onLeave={vi.fn()} />);
    await waitFor(() => {
      expect(container.querySelector(".chat__hints")).not.toBeNull();
    });
  });

  it("counts you in the online total", async () => {
    resize(1280);
    render(<Chat clients={stubClients()} me="rill" onLeave={vi.fn()} />);

    await waitFor(() => {
      expect(screen.getByText("2 online")).toBeInTheDocument();
    });
  });
});
