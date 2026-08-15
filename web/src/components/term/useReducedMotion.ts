import { useSyncExternalStore } from "react";

const REDUCED_MOTION = "(prefers-reduced-motion: reduce)";

function supported(): boolean {
  return typeof globalThis.matchMedia === "function";
}

function subscribe(onChange: () => void): () => void {
  if (!supported()) {
    return () => undefined;
  }
  const query = globalThis.matchMedia(REDUCED_MOTION);
  query.addEventListener("change", onChange);
  return () => {
    query.removeEventListener("change", onChange);
  };
}

function snapshot(): boolean {
  return supported() && globalThis.matchMedia(REDUCED_MOTION).matches;
}

function serverSnapshot(): boolean {
  return false;
}

export function usePrefersReducedMotion(): boolean {
  return useSyncExternalStore(subscribe, snapshot, serverSnapshot);
}
