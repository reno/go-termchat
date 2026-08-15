import { StrictMode } from "react";
import { createRoot } from "react-dom/client";

import { App } from "@/app/App";
import { createClients, createTransport } from "@/lib/transport";

import "@/styles/terminal.css";
import "@/app/app.css";

const container = document.getElementById("root");
if (container === null) {
  throw new Error("termchat: #root is missing from index.html");
}

const clients = createClients(createTransport({ baseUrl: window.location.origin }));

if ("serviceWorker" in navigator && import.meta.env.PROD) {
  globalThis.addEventListener("load", () => {
    void navigator.serviceWorker.register("/sw.js", { type: "module" });
  });
}

createRoot(container).render(
  <StrictMode>
    <App clients={clients} />
  </StrictMode>,
);
