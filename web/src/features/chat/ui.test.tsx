import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";

import { Composer, PLACEHOLDER } from "./Composer";
import { Header, TAGLINE } from "./Header";
import { HintBar } from "./HintBar";
import { PANEL_FOOTER, SidePanel } from "./SidePanel";
import { Transcript } from "./Transcript";
import type { ChannelView, MemberView, MessageView } from "./model";

afterEach(() => {
  vi.restoreAllMocks();
});

function line(overrides: Partial<MessageView>): MessageView {
  return {
    id: `m${String(Math.random())}`,
    clientId: "",
    channelId: "c1",
    nick: "rill",
    body: "hello",
    kind: "normal",
    state: "sent",
    createdAt: 0,
    ...overrides,
  };
}

describe("header", () => {
  it("shows the channel, the separator and the online count", () => {
    render(
      <Header
        channel="?teste"
        online={6}
        narrow={false}
        panelOpen={false}
        onTogglePanel={vi.fn()}
      />,
    );

    expect(screen.getByText("?teste")).toBeInTheDocument();
    expect(screen.getByText("6 online")).toBeInTheDocument();
    expect(screen.getByText("·")).toHaveAttribute("aria-hidden", "true");
  });

  it("shows the tagline on wide only", () => {
    const { rerender } = render(
      <Header
        channel="?teste"
        online={1}
        narrow={false}
        panelOpen={false}
        onTogglePanel={vi.fn()}
      />,
    );
    expect(screen.getByText(TAGLINE)).toBeInTheDocument();

    rerender(
      <Header channel="?teste" online={1} narrow panelOpen={false} onTogglePanel={vi.fn()} />,
    );
    expect(screen.queryByText(TAGLINE)).not.toBeInTheDocument();
  });

  it("shows the drawer toggle on narrow only, wired for assistive tech", async () => {
    const toggle = vi.fn();
    const user = userEvent.setup();

    const { rerender } = render(
      <Header
        channel="?teste"
        online={1}
        narrow={false}
        panelOpen={false}
        onTogglePanel={toggle}
      />,
    );
    expect(screen.queryByRole("button", { name: "channels and roster" })).not.toBeInTheDocument();

    rerender(
      <Header channel="?teste" online={1} narrow panelOpen={false} onTogglePanel={toggle} />,
    );
    const button = screen.getByRole("button", { name: "channels and roster" });
    expect(button).toHaveAttribute("aria-expanded", "false");

    await user.click(button);
    expect(toggle).toHaveBeenCalledOnce();
  });
});

describe("hint bar", () => {
  it("renders exactly six entries", () => {
    const { container } = render(<HintBar />);
    expect(container.querySelectorAll(".chat__hint")).toHaveLength(6);
  });

  it("renders each command and its description", () => {
    render(<HintBar />);
    expect(screen.getByText("/nick")).toBeInTheDocument();
    expect(screen.getByText("name")).toBeInTheDocument();
    expect(screen.getByText("/clear")).toBeInTheDocument();
  });
});

describe("transcript", () => {
  it("renders each variant with its design treatment", () => {
    const { container } = render(
      <Transcript
        me="rill"
        messages={[
          line({ kind: "normal", nick: "0x9c", body: "the void, mostly" }),
          line({ kind: "system", nick: "*", body: "teste — messages are not stored." }),
          line({ kind: "action", nick: "rill", body: "waves" }),
          line({ kind: "dm", nick: "→ 0x9c", body: "psst" }),
        ]}
      />,
    );

    const variants = [...container.querySelectorAll(".term-line")].map((node) =>
      node.getAttribute("data-variant"),
    );
    expect(variants).toEqual(["normal", "system", "action", "dm"]);
    expect(screen.getByText("rill waves")).toBeInTheDocument();
  });

  it("is a polite live region", () => {
    render(<Transcript me="rill" messages={[]} />);
    const log = screen.getByRole("log", { name: "transcript" });
    expect(log).toHaveAttribute("aria-live", "polite");
  });

  it("marks a failed line so it can be retried", () => {
    const { container } = render(
      <Transcript me="rill" messages={[line({ state: "failed", clientId: "c" })]} />,
    );
    expect(container.querySelector(".term-line")).toHaveAttribute("data-state", "failed");
  });
});

describe("composer", () => {
  it("sends on Enter and inserts a newline on Shift+Enter", async () => {
    const onSend = vi.fn();
    const onDraft = vi.fn();
    const user = userEvent.setup();

    render(<Composer draft="hello" onDraft={onDraft} onSend={onSend} narrow={false} />);

    const field = screen.getByLabelText("message");
    await user.click(field);

    await user.keyboard("{Enter}");
    expect(onSend).toHaveBeenCalledOnce();

    onSend.mockClear();
    await user.keyboard("{Shift>}{Enter}{/Shift}");
    expect(onSend).not.toHaveBeenCalled();
  });

  it("carries the design's placeholder", () => {
    render(<Composer draft="" onDraft={vi.fn()} onSend={vi.fn()} narrow={false} />);
    expect(screen.getByPlaceholderText(PLACEHOLDER)).toBeInTheDocument();
  });

  it("completes a command on Tab", async () => {
    const onDraft = vi.fn();
    const user = userEvent.setup();

    render(<Composer draft="/ni" onDraft={onDraft} onSend={vi.fn()} narrow={false} />);

    await user.click(screen.getByLabelText("message"));
    await user.keyboard("{Tab}");

    expect(onDraft).toHaveBeenCalledWith("/nick ");
  });

  it("shows the send button on narrow only", () => {
    const { rerender } = render(
      <Composer draft="" onDraft={vi.fn()} onSend={vi.fn()} narrow={false} />,
    );
    expect(screen.queryByRole("button", { name: "send" })).not.toBeInTheDocument();

    rerender(<Composer draft="" onDraft={vi.fn()} onSend={vi.fn()} narrow />);
    expect(screen.getByRole("button", { name: "send" })).toBeInTheDocument();
  });
});

describe("side panel", () => {
  const channels: ChannelView[] = [
    { id: "c1", name: "?teste", memberCount: 6 },
    { id: "c2", name: "?quiet", memberCount: 0 },
  ];
  const members: MemberView[] = [
    { nick: "0x9c", tag: "" },
    { nick: "anon2", tag: "idle" },
  ];

  it("renders channel rows as real buttons", async () => {
    const onSelect = vi.fn();
    const user = userEvent.setup();

    render(
      <SidePanel
        channels={channels}
        activeId="c1"
        members={members}
        me="rill"
        onSelectChannel={onSelect}
        onMention={vi.fn()}
      />,
    );

    const row = screen.getByRole("button", { name: /\?quiet/ });
    await user.click(row);
    expect(onSelect).toHaveBeenCalledWith("c2");
  });

  it("marks the active channel", () => {
    render(
      <SidePanel
        channels={channels}
        activeId="c1"
        members={members}
        me="rill"
        onSelectChannel={vi.fn()}
        onMention={vi.fn()}
      />,
    );
    expect(screen.getByRole("button", { name: /\?teste/ })).toHaveAttribute("aria-current", "true");
  });

  it("renders a member count of zero as an empty string", () => {
    const { container } = render(
      <SidePanel
        channels={channels}
        activeId="c1"
        members={members}
        me="rill"
        onSelectChannel={vi.fn()}
        onMention={vi.fn()}
      />,
    );

    const rows = [...container.querySelectorAll(".chat__channel-row")];
    const quiet = rows.find((row) => row.textContent.includes("?quiet"));
    if (quiet === undefined) {
      throw new Error("the ?quiet row was not rendered");
    }
    expect(quiet.querySelector(".term-badge")).toHaveTextContent("");
  });

  it("appends your own nick last with the me tag", () => {
    render(
      <SidePanel
        channels={channels}
        activeId="c1"
        members={members}
        me="rill"
        onSelectChannel={vi.fn()}
        onMention={vi.fn()}
      />,
    );

    const entries = screen.getAllByRole("button", { name: /0x9c|anon2|rill/ });
    expect(entries.at(-1)).toHaveTextContent("rill");
    expect(entries.at(-1)).toHaveTextContent("me");
  });

  it("surfaces the roster tag", () => {
    render(
      <SidePanel
        channels={channels}
        activeId="c1"
        members={members}
        me="rill"
        onSelectChannel={vi.fn()}
        onMention={vi.fn()}
      />,
    );
    expect(screen.getByText("idle")).toBeInTheDocument();
  });

  it("mentions a member when its entry is clicked", async () => {
    const onMention = vi.fn();
    const user = userEvent.setup();

    render(
      <SidePanel
        channels={channels}
        activeId="c1"
        members={members}
        me="rill"
        onSelectChannel={vi.fn()}
        onMention={onMention}
      />,
    );

    await user.click(screen.getByRole("button", { name: /0x9c/ }));
    expect(onMention).toHaveBeenCalledWith("0x9c");
  });

  it("renders the panel footer", () => {
    render(
      <SidePanel
        channels={channels}
        activeId="c1"
        members={members}
        me="rill"
        onSelectChannel={vi.fn()}
        onMention={vi.fn()}
      />,
    );
    expect(screen.getByText(PANEL_FOOTER)).toBeInTheDocument();
  });
});
