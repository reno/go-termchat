export const UPDATE_AVAILABLE = "[update available]";

export interface UpdateHandle {
  activate: () => void;
}

export function watchForUpdates(onAvailable: (handle: UpdateHandle) => void): () => void {
  if (!("serviceWorker" in navigator)) {
    return () => undefined;
  }

  let cancelled = false;

  void navigator.serviceWorker.register("/sw.js", { type: "module" }).then((registration) => {
    if (cancelled) {
      return;
    }

    const offer = (waiting: ServiceWorker) => {
      onAvailable({
        activate: () => {
          waiting.postMessage({ type: "SKIP_WAITING" });
        },
      });
    };

    if (registration.waiting !== null) {
      offer(registration.waiting);
    }

    registration.addEventListener("updatefound", () => {
      const installing = registration.installing;
      if (installing === null) {
        return;
      }
      installing.addEventListener("statechange", () => {
        if (installing.state === "installed" && navigator.serviceWorker.controller !== null) {
          offer(installing);
        }
      });
    });
  });

  return () => {
    cancelled = true;
  };
}
