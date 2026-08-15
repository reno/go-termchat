export interface Hint {
  cmd: string;
  desc: string;
}

export const HINTS: readonly Hint[] = [
  { cmd: "/nick", desc: "name" },
  { cmd: "/me", desc: "action" },
  { cmd: "/join", desc: "?channel" },
  { cmd: "/msg", desc: "user text" },
  { cmd: "/clear", desc: "" },
  { cmd: "/help", desc: "" },
] as const;

export const HIDDEN_COMMANDS = ["/theme", "/leave"] as const;

export const USAGE_NICK = "usage: /nick newname";
export const USAGE_ME = "usage: /me does a thing";
export const USAGE_JOIN = "usage: /join ?channel";
export const USAGE_MSG = "usage: /msg user text";
export const USAGE_THEME = "usage: /theme green|amber|blue|violet";

export const THEMES = ["green", "amber", "blue", "violet"] as const;
export type Theme = (typeof THEMES)[number];

export function helpText(): string {
  const visible = HINTS.map((hint) => `${hint.cmd} ${hint.desc}`.trim());
  const hidden = HIDDEN_COMMANDS.map((cmd) => cmd);
  return [...visible, ...hidden].join("   ");
}

export function unknownCommand(name: string): string {
  return `unknown command: /${name}`;
}

export type Command =
  | { kind: "say"; body: string }
  | { kind: "nick"; nick: string }
  | { kind: "me"; action: string }
  | { kind: "join"; channel: string }
  | { kind: "msg"; to: string; body: string }
  | { kind: "clear" }
  | { kind: "help" }
  | { kind: "leave" }
  | { kind: "theme"; theme: Theme }
  | { kind: "usage"; message: string };

export function parseCommand(raw: string): Command | null {
  const text = raw.trim();
  if (text === "") {
    return null;
  }
  if (!text.startsWith("/")) {
    return { kind: "say", body: text };
  }

  const [name, ...rest] = text.slice(1).split(" ");
  const arg = rest.join(" ").trim();

  switch ((name ?? "").toLowerCase()) {
    case "nick": {
      const first = arg.split(" ")[0] ?? "";
      return first === "" ? { kind: "usage", message: USAGE_NICK } : { kind: "nick", nick: first };
    }
    case "me":
      return arg === "" ? { kind: "usage", message: USAGE_ME } : { kind: "me", action: arg };
    case "join": {
      const first = arg.split(" ")[0] ?? "";
      return first === ""
        ? { kind: "usage", message: USAGE_JOIN }
        : { kind: "join", channel: first };
    }
    case "msg": {
      const to = rest[0] ?? "";
      const body = rest.slice(1).join(" ").trim();
      return to === "" || body === ""
        ? { kind: "usage", message: USAGE_MSG }
        : { kind: "msg", to, body };
    }
    case "clear":
      return { kind: "clear" };
    case "help":
      return { kind: "help" };
    case "leave":
      return { kind: "leave" };
    case "theme": {
      const wanted = (arg.split(" ")[0] ?? "").toLowerCase();
      return (THEMES as readonly string[]).includes(wanted)
        ? { kind: "theme", theme: wanted as Theme }
        : { kind: "usage", message: USAGE_THEME };
    }
    default:
      return { kind: "usage", message: unknownCommand(name ?? "") };
  }
}

const COMPLETABLE = [...HINTS.map((hint) => hint.cmd), ...HIDDEN_COMMANDS];

export function completeCommand(draft: string): string | null {
  if (!draft.startsWith("/") || draft.includes(" ")) {
    return null;
  }
  const matches = COMPLETABLE.filter((command) => command.startsWith(draft.toLowerCase()));
  if (matches.length !== 1) {
    return null;
  }
  return `${matches[0] ?? draft} `;
}

export function mentionDraft(draft: string, nick: string): string {
  const prefix = draft === "" ? "" : draft.replace(/\s*$/, " ");
  return `${prefix}@${nick} `;
}

export function applyTheme(theme: Theme, root: HTMLElement): void {
  if (theme === "green") {
    root.removeAttribute("data-theme");
    return;
  }
  root.setAttribute("data-theme", theme);
}
