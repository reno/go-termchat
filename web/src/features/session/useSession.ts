import { useCallback, useEffect, useRef, useState } from "react";

import type { SessionClients } from "@/lib/clients";
import { clearCache } from "@/lib/db";
import { forgetNick, isUnauthenticated, reasonOf, rememberNick } from "@/lib/transport";
import { ErrorReason } from "@gen/chat/v1/chat_pb";

export type SessionPhase = "resuming" | "anonymous" | "joined";

export interface SessionState {
  phase: SessionPhase;
  nick: string | null;
  notice: string | null;
  busy: boolean;
}

export const NICK_TAKEN_MESSAGE = "that nick is taken. pick another.";
export const NICK_INVALID_MESSAGE = "nicks are letters, digits, - and _ only.";
export const SESSION_LOST_MESSAGE = "session ended. pick a nick to start again.";

function joinFailureMessage(error: unknown): string {
  switch (reasonOf(error)) {
    case ErrorReason.NICK_TAKEN:
      return NICK_TAKEN_MESSAGE;
    case ErrorReason.NICK_INVALID:
      return NICK_INVALID_MESSAGE;
    default:
      return "could not join. try again.";
  }
}

export function useSession(clients: SessionClients) {
  const [state, setState] = useState<SessionState>({
    phase: "resuming",
    nick: null,
    notice: null,
    busy: false,
  });

  const live = useRef(true);
  useEffect(() => {
    live.current = true;
    return () => {
      live.current = false;
    };
  }, []);

  useEffect(() => {
    void (async () => {
      try {
        const current = await clients.sessions.getSession({});
        if (!live.current) {
          return;
        }
        const nick = current.session?.nick ?? null;
        if (nick === null) {
          setState({ phase: "anonymous", nick: null, notice: null, busy: false });
          return;
        }
        rememberNick(nick);
        setState({ phase: "joined", nick, notice: null, busy: false });
      } catch (error) {
        if (!live.current) {
          return;
        }
        if (isUnauthenticated(error)) {
          void clearCache().catch(() => undefined);
        }
        forgetNick();
        setState({
          phase: "anonymous",
          nick: null,
          notice: isUnauthenticated(error) ? null : "could not reach the server.",
          busy: false,
        });
      }
    })();
  }, [clients]);

  const join = useCallback(
    async (nick: string) => {
      setState((current) => ({ ...current, busy: true, notice: null }));
      try {
        const joined = await clients.sessions.join({ nick });
        const claimed = joined.session?.nick ?? nick;
        rememberNick(claimed);
        if (live.current) {
          setState({ phase: "joined", nick: claimed, notice: null, busy: false });
        }
      } catch (error) {
        if (live.current) {
          setState({ phase: "anonymous", nick: null, notice: null, busy: false });
        }
        throw new Error(joinFailureMessage(error), { cause: error });
      }
    },
    [clients],
  );

  const leave = useCallback(async () => {
    try {
      await clients.sessions.leave({});
    } finally {
      await clearCache().catch(() => undefined);
      forgetNick();
      if (live.current) {
        setState({ phase: "anonymous", nick: null, notice: null, busy: false });
      }
    }
  }, [clients]);

  const sessionLost = useCallback(() => {
    void clearCache().catch(() => undefined);
    forgetNick();
    if (live.current) {
      setState({ phase: "anonymous", nick: null, notice: SESSION_LOST_MESSAGE, busy: false });
    }
  }, []);

  return { state, join, leave, sessionLost };
}
