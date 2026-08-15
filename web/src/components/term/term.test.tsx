import { render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { mockReducedMotion } from "@/test/setup";

import { Badge, Caret, Line, Pane, Prompt, Spinner, StatusBar } from "./index";

afterEach(() => {
  vi.unstubAllGlobals();
  vi.useRealTimers();
});

describe("Pane", () => {
  it("renders its children", () => {
    render(<Pane>transcript</Pane>);
    expect(screen.getByText("transcript")).toBeInTheDocument();
  });

  it("becomes a scroll region only when asked", () => {
    const { rerender } = render(<Pane label="static">body</Pane>);
    expect(screen.getByLabelText("static")).toHaveAttribute("data-scroll", "false");

    rerender(
      <Pane label="static" scroll>
        body
      </Pane>,
    );
    expect(screen.getByLabelText("static")).toHaveAttribute("data-scroll", "true");
  });

  it("announces politely when it is the transcript", () => {
    render(
      <Pane label="transcript" live>
        body
      </Pane>,
    );
    expect(screen.getByLabelText("transcript")).toHaveAttribute("aria-live", "polite");
  });
});

describe("Line", () => {
  it("renders the four design variants", () => {
    const variants = [
      { variant: "normal", nick: "rill", body: "the void, mostly" },
      { variant: "system", nick: "*", body: "teste — messages are not stored." },
      { variant: "action", nick: "*", body: "rill waves" },
      { variant: "dm", nick: "→ 0x9c", body: "psst" },
    ] as const;

    for (const { variant, nick, body } of variants) {
      const { unmount } = render(
        <Line nick={nick} variant={variant}>
          {body}
        </Line>,
      );
      const line = screen.getByText(body).closest(".term-line");
      expect(line).toHaveAttribute("data-variant", variant);
      expect(screen.getByText(nick)).toBeInTheDocument();
      unmount();
    }
  });

  it("defaults to the normal variant in the sent state", () => {
    render(<Line nick="rill">hello</Line>);
    const line = screen.getByText("hello").closest(".term-line");
    expect(line).toHaveAttribute("data-variant", "normal");
    expect(line).toHaveAttribute("data-state", "sent");
  });

  it("carries the pending and failed states from the design gaps", () => {
    const { rerender } = render(
      <Line nick="rill" state="pending">
        in flight
      </Line>,
    );
    expect(screen.getByText("in flight").closest(".term-line")).toHaveAttribute(
      "data-state",
      "pending",
    );

    rerender(
      <Line nick="rill" state="failed">
        in flight
      </Line>,
    );
    expect(screen.getByText("in flight").closest(".term-line")).toHaveAttribute(
      "data-state",
      "failed",
    );
  });

  it("applies the nick colour it is handed", () => {
    render(
      <Line nick="rill" nickColor="var(--nick-3)">
        hello
      </Line>,
    );
    expect(screen.getByText("rill")).toHaveStyle({ color: "var(--nick-3)" });
  });

  it("preserves whitespace in the body", () => {
    render(<Line nick="rill">{"one\ntwo"}</Line>);
    const body = screen.getByText(/one/).closest(".term-line__body");
    expect(body).toBeInTheDocument();
  });
});

describe("Prompt", () => {
  it("renders the design's caret symbol and hides it from screen readers", () => {
    const { container } = render(<Prompt />);
    const prompt = container.querySelector(".term-prompt");
    expect(prompt).toHaveTextContent(">");
    expect(prompt).toHaveAttribute("aria-hidden", "true");
  });
});

describe("Caret", () => {
  it("blinks by default", () => {
    mockReducedMotion(false);
    const { container } = render(<Caret />);
    expect(container.querySelector(".term-caret")).toHaveAttribute("data-blinking", "true");
  });

  it("does not blink when the reader prefers reduced motion", () => {
    mockReducedMotion(true);
    const { container } = render(<Caret />);
    const caret = container.querySelector(".term-caret");
    expect(caret).toHaveAttribute("data-blinking", "false");
    expect(caret).toHaveStyle({ animation: "none" });
  });
});

describe("StatusBar", () => {
  it("renders as a labelled header", () => {
    render(<StatusBar label="channel status">?teste</StatusBar>);
    const header = screen.getByLabelText("channel status");
    expect(header.tagName).toBe("HEADER");
    expect(header).toHaveTextContent("?teste");
  });
});

describe("Spinner", () => {
  it("exposes a status role", () => {
    mockReducedMotion(false);
    render(<Spinner label="connecting" />);
    expect(screen.getByRole("status", { name: "connecting" })).toBeInTheDocument();
  });

  it("holds one frame when the reader prefers reduced motion", () => {
    mockReducedMotion(true);
    vi.useFakeTimers();

    render(<Spinner />);
    const first = screen.getByRole("status").textContent;

    vi.advanceTimersByTime(1000);
    expect(screen.getByRole("status").textContent).toBe(first);
  });
});

describe("Badge", () => {
  it("renders a count", () => {
    render(<Badge count={6} />);
    expect(screen.getByText("6")).toBeInTheDocument();
  });

  it("renders zero as an empty string, per the design", () => {
    const { container } = render(<Badge count={0} />);
    expect(container.querySelector(".term-badge")).toHaveTextContent("");
  });

  it("promotes its colour when it sits on a raised row", () => {
    const { container } = render(<Badge count={3} onRaised />);
    expect(container.querySelector(".term-badge")).toHaveAttribute("data-on-raised", "true");
  });

  it("renders arbitrary children such as a roster tag", () => {
    render(<Badge>idle</Badge>);
    expect(screen.getByText("idle")).toBeInTheDocument();
  });
});
