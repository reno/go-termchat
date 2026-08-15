import { Code, ConnectError } from "@connectrpc/connect";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { App } from "@/app/App";
import type { SessionClients } from "@/lib/clients";
import { forgetNick, rememberedNick } from "@/lib/transport";
import { ErrorDetailSchema, ErrorReason } from "@gen/chat/v1/chat_pb";

import { NICK_TAKEN_MESSAGE } from "./useSession";

function quietStream(): AsyncIterable<never> {
  return {
    [Symbol.asyncIterator]: () => ({
      next: () => Promise.resolve({ done: true as const, value: undefined as never }),
    }),
  };
}

function sessionOf(nick: string) {
  return { session: { nick, $typeName: "chat.v1.Session" } } as never;
}

function domainError(code: Code, reason: ErrorReason): ConnectError {
  return new ConnectError("rejected", code, undefined, [
    { desc: ErrorDetailSchema, value: { reason } },
  ]);
}

function clientsWith(overrides: {
  getSession?: () => Promise<unknown>;
  join?: (request: { nick: string }) => Promise<unknown>;
}): SessionClients {
  return {
    sessions: {
      getSession: overrides.getSession ?? (() => Promise.resolve(sessionOf("rill"))),
      join:
        overrides.join ?? ((request: { nick: string }) => Promise.resolve(sessionOf(request.nick))),
      setNick: vi.fn(),
      leave: vi.fn(),
    },
    chat: {
      joinChannel: () =>
        Promise.resolve({ channel: { id: "c1", name: "?teste", memberCount: 1 }, created: false }),
      listMessages: () => Promise.resolve({ messages: [], windowTruncated: false }),
      listMembers: () => Promise.resolve({ members: [] }),
      listChannels: () => Promise.resolve({ channels: [] }),
      sendMessage: () => Promise.resolve({ message: undefined, deduplicated: false }),
      sendDirectMessage: () => Promise.resolve({ message: undefined, delivered: true }),
      setTyping: () => Promise.resolve({}),
      streamEvents: () => quietStream(),
    },
  } as unknown as SessionClients;
}

const unauthenticated = new ConnectError("no session", Code.Unauthenticated);

beforeEach(() => {
  forgetNick();
});

describe("boot", () => {
  it("resumes silently from the session cookie without a blank screen", async () => {
    render(<App clients={clientsWith({})} />);

    expect(screen.getByRole("status", { name: "connecting" })).toBeInTheDocument();

    await waitFor(() => {
      expect(screen.getByRole("log", { name: "transcript" })).toBeInTheDocument();
    });
    expect(screen.queryByLabelText("nick")).not.toBeInTheDocument();
    expect(rememberedNick()).toBe("rill");
  });

  it("drops back to the nick prompt when there is no session", async () => {
    render(<App clients={clientsWith({ getSession: () => Promise.reject(unauthenticated) })} />);

    await waitFor(() => {
      expect(screen.getByLabelText("nick")).toBeInTheDocument();
    });
    expect(screen.getByText(/pick a nick to start\./)).toBeInTheDocument();
  });

  it("reports a server it cannot reach rather than showing an empty prompt", async () => {
    render(
      <App clients={clientsWith({ getSession: () => Promise.reject(new Error("offline")) })} />,
    );

    await waitFor(() => {
      expect(screen.getByText("could not reach the server.")).toBeInTheDocument();
    });
  });
});

describe("join", () => {
  it("is a terminal dialogue, not a login form", async () => {
    render(<App clients={clientsWith({ getSession: () => Promise.reject(unauthenticated) })} />);

    await screen.findByLabelText("nick");

    expect(screen.getByText(/termchat/)).toBeInTheDocument();
    expect(screen.queryByLabelText(/password/i)).not.toBeInTheDocument();
    expect(screen.queryByText(/sign in/i)).not.toBeInTheDocument();
    expect(screen.queryByText(/register/i)).not.toBeInTheDocument();
  });

  it("focuses the nick field on arrival", async () => {
    render(<App clients={clientsWith({ getSession: () => Promise.reject(unauthenticated) })} />);

    const input = await screen.findByLabelText("nick");
    await waitFor(() => {
      expect(input).toHaveFocus();
    });
  });

  it("claims a nick and remembers it for the transport", async () => {
    const user = userEvent.setup();
    render(<App clients={clientsWith({ getSession: () => Promise.reject(unauthenticated) })} />);

    await user.type(await screen.findByLabelText("nick"), "rill");
    await user.click(screen.getByRole("button", { name: "join" }));

    await waitFor(() => {
      expect(screen.getByRole("log", { name: "transcript" })).toBeInTheDocument();
    });
    expect(rememberedNick()).toBe("rill");
  });

  it("handles a taken nick inline and keeps the prompt usable", async () => {
    const user = userEvent.setup();
    render(
      <App
        clients={clientsWith({
          getSession: () => Promise.reject(unauthenticated),
          join: () => Promise.reject(domainError(Code.AlreadyExists, ErrorReason.NICK_TAKEN)),
        })}
      />,
    );

    await user.type(await screen.findByLabelText("nick"), "rill");
    await user.click(screen.getByRole("button", { name: "join" }));

    expect(await screen.findByRole("alert")).toHaveTextContent(NICK_TAKEN_MESSAGE);
    expect(screen.getByLabelText("nick")).toBeInTheDocument();
    expect(screen.getByLabelText("nick")).toHaveAttribute("aria-invalid", "true");
    expect(rememberedNick()).toBeNull();
  });

  it("refuses an empty draft", async () => {
    const join = vi.fn(() => Promise.resolve(sessionOf("rill")));
    const user = userEvent.setup();

    render(
      <App clients={clientsWith({ getSession: () => Promise.reject(unauthenticated), join })} />,
    );

    await screen.findByLabelText("nick");
    expect(screen.getByRole("button", { name: "join" })).toBeDisabled();

    await user.type(screen.getByLabelText("nick"), "   ");
    expect(screen.getByRole("button", { name: "join" })).toBeDisabled();
    expect(join).not.toHaveBeenCalled();
  });

  it("submits on Enter", async () => {
    const user = userEvent.setup();
    render(<App clients={clientsWith({ getSession: () => Promise.reject(unauthenticated) })} />);

    await user.type(await screen.findByLabelText("nick"), "rill{Enter}");

    await waitFor(() => {
      expect(screen.getByRole("log", { name: "transcript" })).toBeInTheDocument();
    });
  });
});
