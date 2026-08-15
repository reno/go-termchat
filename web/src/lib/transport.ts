import { Code, ConnectError, createClient } from "@connectrpc/connect";
import type { Interceptor, Transport } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";

import { ChatService, ErrorDetailSchema, ErrorReason } from "@gen/chat/v1/chat_pb";
import { SessionService } from "@gen/chat/v1/session_pb";

const NICK_KEY = "termchat.nick";
const JOIN_PROCEDURE = "Join";

export function rememberNick(nick: string): void {
  try {
    globalThis.sessionStorage.setItem(NICK_KEY, nick);
  } catch {
    // A browser with storage disabled simply cannot resume; the nick prompt handles it.
  }
}

export function rememberedNick(): string | null {
  try {
    return globalThis.sessionStorage.getItem(NICK_KEY);
  } catch {
    return null;
  }
}

export function forgetNick(): void {
  try {
    globalThis.sessionStorage.removeItem(NICK_KEY);
  } catch {
    // Nothing to forget.
  }
}

export function reasonOf(error: unknown): ErrorReason {
  if (!(error instanceof ConnectError)) {
    return ErrorReason.UNSPECIFIED;
  }
  for (const detail of error.findDetails(ErrorDetailSchema)) {
    return detail.reason;
  }
  return ErrorReason.UNSPECIFIED;
}

export function isUnauthenticated(error: unknown): boolean {
  return error instanceof ConnectError && error.code === Code.Unauthenticated;
}

export interface TransportOptions {
  baseUrl?: string;
  onSessionLost?: () => void;
}

export function createTransport(options: TransportOptions = {}): Transport {
  const { baseUrl = "/", onSessionLost } = options;

  let rejoining: Promise<boolean> | null = null;

  const rejoinOnce = async (transport: Transport): Promise<boolean> => {
    rejoining ??= (async () => {
      const nick = rememberedNick();
      if (nick === null) {
        return false;
      }
      try {
        await createClient(SessionService, transport).join({ nick });
        return true;
      } catch {
        forgetNick();
        return false;
      } finally {
        rejoining = null;
      }
    })();

    return rejoining;
  };

  const recover: Interceptor = (next) => async (request) => {
    try {
      return await next(request);
    } catch (error) {
      if (!isUnauthenticated(error) || request.method.name === JOIN_PROCEDURE) {
        throw error;
      }

      const recovered = await rejoinOnce(transport);
      if (!recovered) {
        onSessionLost?.();
        throw error;
      }
      return next(request);
    }
  };

  const transport: Transport = createConnectTransport({
    baseUrl,
    fetch: (input, init) => globalThis.fetch(input, { ...init, credentials: "include" }),
    interceptors: [recover],
  });

  return transport;
}

export function createClients(transport: Transport) {
  return {
    sessions: createClient(SessionService, transport),
    chat: createClient(ChatService, transport),
  };
}
