import { describe, expect, it } from "vitest";

import type { MessageView } from "./model";
import { NICK_PALETTE, nickColor, nickHash, renderLine } from "./model";

function message(overrides: Partial<MessageView>): MessageView {
  return {
    id: "m1",
    clientId: "",
    channelId: "c1",
    nick: "rill",
    body: "the void, mostly",
    kind: "normal",
    state: "sent",
    createdAt: 0,
    ...overrides,
  };
}

function referenceHash(nick: string): number {
  let total = 0;
  for (const character of nick) {
    total += character.charCodeAt(0);
  }
  return total % 6;
}

describe("nick hash", () => {
  it("matches the export's formula for known nicks", () => {
    const nicks = ["rill", "0x9c", "anon2", "ferris", "nib", "tape", "moth", "teste"];

    for (const nick of nicks) {
      expect(nickHash(nick)).toBe(referenceHash(nick));
    }
  });

  it("pins known nick to colour pairs", () => {
    const pairs: [string, number][] = [
      ["rill", referenceHash("rill")],
      ["0x9c", referenceHash("0x9c")],
      ["anon2", referenceHash("anon2")],
      ["nib", referenceHash("nib")],
    ];

    for (const [nick, index] of pairs) {
      expect(nickColor(nick, null)).toBe(NICK_PALETTE[index]);
    }
  });

  it("is stable and inside the six-colour palette", () => {
    for (const nick of ["a", "zzzzzz", "ünï", "0", "😀"]) {
      const index = nickHash(nick);
      expect(index).toBeGreaterThanOrEqual(0);
      expect(index).toBeLessThan(6);
      expect(nickColor(nick, null)).toBe(NICK_PALETTE[index]);
    }
  });

  it("renders your own nick in the accent, overriding the hash", () => {
    expect(nickColor("rill", "rill")).toBe("var(--accent)");
    expect(nickColor("rill", "0x9c")).not.toBe("var(--accent)");
  });
});

describe("renderLine", () => {
  it("renders a normal message with its hashed nick", () => {
    const line = renderLine(message({}), "0x9c");
    expect(line).toEqual({
      nick: "rill",
      body: "the void, mostly",
      variant: "normal",
      nickColor: NICK_PALETTE[referenceHash("rill")],
    });
  });

  it("renders a system message under a bare asterisk", () => {
    const line = renderLine(message({ kind: "system", nick: "*", body: "joined" }), "rill");
    expect(line.nick).toBe("*");
    expect(line.variant).toBe("system");
    expect(line.nickColor).toBeUndefined();
  });

  it("folds the nick into the body for an action, as the export does", () => {
    const line = renderLine(message({ kind: "action", nick: "rill", body: "waves" }), "0x9c");
    expect(line.nick).toBe("*");
    expect(line.body).toBe("rill waves");
    expect(line.variant).toBe("action");
  });

  it("keeps the arrow prefix on a direct message", () => {
    const line = renderLine(message({ kind: "dm", nick: "→ 0x9c", body: "psst" }), "rill");
    expect(line.nick).toBe("→ 0x9c");
    expect(line.body).toBe("psst");
    expect(line.variant).toBe("dm");
  });
});
