export type MessageKind = "normal" | "system" | "action" | "dm";
export type MessageState = "sent" | "pending" | "failed";

export interface MessageView {
  id: string;
  clientId: string;
  channelId: string;
  nick: string;
  body: string;
  kind: MessageKind;
  state: MessageState;
  createdAt: number;
}

export interface ChannelView {
  id: string;
  name: string;
  memberCount: number;
}

export interface MemberView {
  nick: string;
  tag: string;
}

export const NICK_PALETTE = [
  "var(--nick-1)",
  "var(--nick-2)",
  "var(--nick-3)",
  "var(--nick-4)",
  "var(--nick-5)",
  "var(--nick-6)",
] as const;

export function nickHash(nick: string): number {
  let sum = 0;
  for (const character of nick) {
    sum += character.charCodeAt(0);
  }
  return sum % NICK_PALETTE.length;
}

export function nickColor(nick: string, me: string | null): string {
  if (me !== null && nick === me) {
    return "var(--accent)";
  }
  return NICK_PALETTE[nickHash(nick)] ?? NICK_PALETTE[0];
}

export interface RenderedLine {
  nick: string;
  body: string;
  variant: MessageKind;
  nickColor: string | undefined;
}

export function renderLine(message: MessageView, me: string | null): RenderedLine {
  switch (message.kind) {
    case "system":
      return { nick: "*", body: message.body, variant: "system", nickColor: undefined };
    case "action":
      return {
        nick: "*",
        body: `${message.nick} ${message.body}`,
        variant: "action",
        nickColor: undefined,
      };
    case "dm":
      return { nick: message.nick, body: message.body, variant: "dm", nickColor: undefined };
    case "normal":
      return {
        nick: message.nick,
        body: message.body,
        variant: "normal",
        nickColor: nickColor(message.nick, me),
      };
  }
}
