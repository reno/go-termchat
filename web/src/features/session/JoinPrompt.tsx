import type { SubmitEventHandler } from "react";
import { useEffect, useRef, useState } from "react";

import { Caret, Line, Pane, Prompt } from "@/components/term";

import "./join.css";

export interface JoinPromptProps {
  onJoin: (nick: string) => Promise<void>;
  notice?: string | null;
  busy?: boolean;
}

const BANNER = "termchat";
const ASK = "pick a nick to start.";

export function JoinPrompt({ onJoin, notice, busy = false }: JoinPromptProps) {
  const [nick, setNick] = useState("");
  const [error, setError] = useState<string | null>(null);
  const input = useRef<HTMLInputElement>(null);

  useEffect(() => {
    input.current?.focus();
  }, []);

  const submit: SubmitEventHandler<HTMLFormElement> = (event) => {
    event.preventDefault();
    const trimmed = nick.trim();
    if (trimmed === "" || busy) {
      return;
    }
    setError(null);
    void onJoin(trimmed).catch((reason: unknown) => {
      setError(reason instanceof Error ? reason.message : "could not join");
      input.current?.focus();
    });
  };

  return (
    <main className="join">
      <Pane label="join" live>
        <Line nick="*" variant="system">
          {BANNER} - {ASK}
        </Line>
        {notice !== null && notice !== undefined && (
          <Line nick="*" variant="system">
            {notice}
          </Line>
        )}
        {error !== null && (
          <Line nick="*" variant="system">
            {error}
          </Line>
        )}
      </Pane>

      <form className="join__form" onSubmit={submit}>
        <label className="join__label" htmlFor="join-nick">
          nick
        </label>
        <Prompt />
        <input
          id="join-nick"
          ref={input}
          className="join__input"
          value={nick}
          onChange={(event) => {
            setNick(event.target.value);
          }}
          autoComplete="off"
          autoCapitalize="off"
          spellCheck={false}
          disabled={busy}
          aria-describedby={error === null ? undefined : "join-error"}
          aria-invalid={error !== null}
        />
        {nick === "" && !busy && <Caret />}
        <button className="join__submit" type="submit" disabled={busy || nick.trim() === ""}>
          join
        </button>
      </form>

      {error !== null && (
        <p id="join-error" role="alert" className="join__error">
          {error}
        </p>
      )}
    </main>
  );
}
