import type { ReactNode } from "react";
import { useEffect, useState } from "react";

import { usePrefersReducedMotion } from "./useReducedMotion";

import "./term.css";

export type LineVariant = "normal" | "system" | "action" | "dm";
export type LineState = "sent" | "pending" | "failed";

export interface PaneProps {
  children: ReactNode;
  scroll?: boolean;
  label?: string;
  live?: boolean;
  className?: string;
}

export function Pane({ children, scroll = false, label, live = false, className }: PaneProps) {
  return (
    <div
      className={className === undefined ? "term-pane" : `term-pane ${className}`}
      data-scroll={scroll}
      aria-label={label}
      aria-live={live ? "polite" : undefined}
    >
      {children}
    </div>
  );
}

export interface LineProps {
  nick: string;
  children: ReactNode;
  variant?: LineVariant;
  state?: LineState;
  nickColor?: string | undefined;
}

export function Line({ nick, children, variant = "normal", state = "sent", nickColor }: LineProps) {
  return (
    <div className="term-line" data-variant={variant} data-state={state}>
      <span
        className="term-line__nick"
        style={nickColor === undefined ? undefined : { color: nickColor }}
      >
        {nick}
      </span>
      <span className="term-line__body">{children}</span>
    </div>
  );
}

export function Prompt({ symbol = ">" }: { symbol?: string }) {
  return (
    <span className="term-prompt" aria-hidden="true">
      {symbol}
    </span>
  );
}

export function Caret() {
  const reduced = usePrefersReducedMotion();

  return (
    <span
      className="term-caret"
      data-blinking={!reduced}
      style={reduced ? { animation: "none" } : undefined}
      aria-hidden="true"
    >
      &nbsp;
    </span>
  );
}

export interface StatusBarProps {
  children: ReactNode;
  label?: string;
}

export function StatusBar({ children, label }: StatusBarProps) {
  return (
    <header className="term-status" aria-label={label}>
      {children}
    </header>
  );
}

const SPINNER_FRAMES = ["|", "/", "-", "\\"] as const;

export function Spinner({ label = "working" }: { label?: string }) {
  const reduced = usePrefersReducedMotion();
  const [frame, setFrame] = useState(0);

  useEffect(() => {
    if (reduced) {
      return;
    }
    const timer = setInterval(() => {
      setFrame((current) => (current + 1) % SPINNER_FRAMES.length);
    }, 120);
    return () => {
      clearInterval(timer);
    };
  }, [reduced]);

  return (
    <span className="term-spinner" role="status" aria-label={label}>
      {SPINNER_FRAMES[frame]}
    </span>
  );
}

export interface BadgeProps {
  count?: number;
  children?: ReactNode;
  onRaised?: boolean;
}

export function Badge({ count, children, onRaised = false }: BadgeProps) {
  const text = count === undefined ? children : count === 0 ? "" : String(count);

  return (
    <span className="term-badge" data-on-raised={onRaised}>
      {text}
    </span>
  );
}
