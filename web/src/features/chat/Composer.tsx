import type { KeyboardEventHandler } from "react";
import { useEffect, useRef } from "react";

import { Prompt } from "@/components/term";

import { completeCommand } from "./commands";

const MIN_HEIGHT = 44;
const MAX_HEIGHT = 120;

export interface ComposerProps {
  draft: string;
  onDraft: (draft: string) => void;
  onSend: () => void;
  onTyping?: (typing: boolean) => void;
  narrow: boolean;
}

export const PLACEHOLDER = "message or /command — enter sends";

export function Composer({ draft, onDraft, onSend, onTyping, narrow }: ComposerProps) {
  const field = useRef<HTMLTextAreaElement>(null);

  useEffect(() => {
    const element = field.current;
    if (element === null) {
      return;
    }
    element.style.height = "auto";
    element.style.height = `${String(Math.min(Math.max(element.scrollHeight, MIN_HEIGHT), MAX_HEIGHT))}px`;
  }, [draft]);

  const onKeyDown: KeyboardEventHandler<HTMLTextAreaElement> = (event) => {
    if (event.key === "Tab") {
      const completed = completeCommand(draft);
      if (completed !== null) {
        event.preventDefault();
        onDraft(completed);
      }
      return;
    }
    if (event.key === "Enter" && !event.shiftKey) {
      event.preventDefault();
      onSend();
    }
  };

  return (
    <div className="chat__composer">
      <Prompt />
      <label className="chat__composer-label" htmlFor="composer">
        message
      </label>
      <textarea
        id="composer"
        ref={field}
        className="chat__input"
        rows={1}
        value={draft}
        placeholder={PLACEHOLDER}
        onChange={(event) => {
          onDraft(event.target.value);
          onTyping?.(event.target.value !== "");
        }}
        onKeyDown={onKeyDown}
        onBlur={() => {
          onTyping?.(false);
        }}
        autoComplete="off"
        autoCapitalize="sentences"
        spellCheck
      />
      {narrow && (
        <button type="button" className="chat__send" onClick={onSend}>
          send
        </button>
      )}
    </div>
  );
}
