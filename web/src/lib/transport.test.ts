import { Code, ConnectError } from "@connectrpc/connect";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { ErrorDetailSchema, ErrorReason } from "@gen/chat/v1/chat_pb";

import {
  createTransport,
  forgetNick,
  isUnauthenticated,
  reasonOf,
  rememberNick,
  rememberedNick,
} from "./transport";

beforeEach(() => {
  forgetNick();
  vi.restoreAllMocks();
});

describe("remembered nick", () => {
  it("round-trips through session storage", () => {
    expect(rememberedNick()).toBeNull();
    rememberNick("rill");
    expect(rememberedNick()).toBe("rill");
    forgetNick();
    expect(rememberedNick()).toBeNull();
  });

  it("survives storage being unavailable", () => {
    const denied = () => {
      throw new Error("storage is disabled");
    };
    vi.stubGlobal("sessionStorage", {
      getItem: denied,
      setItem: denied,
      removeItem: denied,
    });

    expect(() => {
      rememberNick("rill");
    }).not.toThrow();
    expect(rememberedNick()).toBeNull();
    expect(() => {
      forgetNick();
    }).not.toThrow();

    vi.unstubAllGlobals();
  });
});

describe("error inspection", () => {
  it("recognises an unauthenticated failure", () => {
    expect(isUnauthenticated(new ConnectError("no", Code.Unauthenticated))).toBe(true);
    expect(isUnauthenticated(new ConnectError("no", Code.NotFound))).toBe(false);
    expect(isUnauthenticated(new Error("no"))).toBe(false);
  });

  it("reads the typed reason out of an error detail", () => {
    const error = new ConnectError("taken", Code.AlreadyExists, undefined, [
      { desc: ErrorDetailSchema, value: { reason: ErrorReason.NICK_TAKEN } },
    ]);

    expect(reasonOf(error)).toBe(ErrorReason.NICK_TAKEN);
  });

  it("falls back to unspecified for anything else", () => {
    expect(reasonOf(new Error("boom"))).toBe(ErrorReason.UNSPECIFIED);
    expect(reasonOf(new ConnectError("plain", Code.Internal))).toBe(ErrorReason.UNSPECIFIED);
  });
});

describe("credentials", () => {
  it("sends cookies on every request", async () => {
    const seen: RequestInit[] = [];
    const fetchMock = vi.fn((_input: unknown, init?: RequestInit) => {
      if (init !== undefined) {
        seen.push(init);
      }
      return Promise.resolve(
        new Response(JSON.stringify({}), {
          status: 200,
          headers: { "content-type": "application/json" },
        }),
      );
    });
    vi.stubGlobal("fetch", fetchMock);

    const transport = createTransport({ baseUrl: "http://localhost" });
    const { createClients } = await import("./transport");
    const clients = createClients(transport);

    await clients.sessions.getSession({}).catch(() => undefined);

    expect(fetchMock).toHaveBeenCalled();
    expect(seen[0]?.credentials).toBe("include");

    vi.unstubAllGlobals();
  });
});
