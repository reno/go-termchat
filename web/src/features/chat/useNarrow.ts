import { useSyncExternalStore } from "react";

export const NARROW_BREAKPOINT = 640;

function subscribe(onChange: () => void): () => void {
  globalThis.addEventListener("resize", onChange);
  return () => {
    globalThis.removeEventListener("resize", onChange);
  };
}

function snapshot(): boolean {
  return globalThis.innerWidth < NARROW_BREAKPOINT;
}

function serverSnapshot(): boolean {
  return false;
}

export function useNarrow(): boolean {
  return useSyncExternalStore(subscribe, snapshot, serverSnapshot);
}
