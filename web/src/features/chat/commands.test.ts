import { describe, expect, it } from "vitest";

import {
  HIDDEN_COMMANDS,
  HINTS,
  USAGE_JOIN,
  USAGE_ME,
  USAGE_MSG,
  USAGE_NICK,
  completeCommand,
  helpText,
  mentionDraft,
  parseCommand,
  unknownCommand,
} from "./commands";

describe("the hint bar", () => {
  it("lists exactly the six commands the export declares", () => {
    expect(HINTS.map((hint) => `${hint.cmd} ${hint.desc}`.trim())).toEqual([
      "/nick name",
      "/me action",
      "/join ?channel",
      "/msg user text",
      "/clear",
      "/help",
    ]);
  });

  it("does not list the hidden commands, so it stays pixel-true", () => {
    const listed = HINTS.map((hint) => hint.cmd);
    for (const hidden of HIDDEN_COMMANDS) {
      expect(listed).not.toContain(hidden);
    }
  });
});

describe("usage strings", () => {
  it("matches the export character for character", () => {
    expect(USAGE_NICK).toBe("usage: /nick newname");
    expect(USAGE_ME).toBe("usage: /me does a thing");
    expect(USAGE_JOIN).toBe("usage: /join ?channel");
    expect(USAGE_MSG).toBe("usage: /msg user text");
  });

  it("reports an unknown command in the export's wording", () => {
    expect(unknownCommand("xyz")).toBe("unknown command: /xyz");
  });
});

describe("/help", () => {
  it("joins the hint list with three spaces", () => {
    const text = helpText();
    expect(
      text.startsWith("/nick name   /me action   /join ?channel   /msg user text   /clear   /help"),
    ).toBe(true);
  });

  it("is the only place the hidden commands appear", () => {
    const text = helpText();
    for (const hidden of HIDDEN_COMMANDS) {
      expect(text).toContain(hidden);
    }
  });
});

describe("parseCommand", () => {
  it("treats plain text as a message", () => {
    expect(parseCommand("anyone here")).toEqual({ kind: "say", body: "anyone here" });
  });

  it("ignores an empty or whitespace draft", () => {
    expect(parseCommand("")).toBeNull();
    expect(parseCommand("   ")).toBeNull();
  });

  it("is case-insensitive", () => {
    expect(parseCommand("/HELP")).toEqual({ kind: "help" });
    expect(parseCommand("/Clear")).toEqual({ kind: "clear" });
  });

  it("takes only the first word for /nick", () => {
    expect(parseCommand("/nick rill and more")).toEqual({ kind: "nick", nick: "rill" });
  });

  it("keeps the whole argument for /me", () => {
    expect(parseCommand("/me does a thing")).toEqual({ kind: "me", action: "does a thing" });
  });

  it("splits /msg into a recipient and a body", () => {
    expect(parseCommand("/msg 0x9c the void, mostly")).toEqual({
      kind: "msg",
      to: "0x9c",
      body: "the void, mostly",
    });
  });

  it("returns the exact usage string for each bad input", () => {
    expect(parseCommand("/nick")).toEqual({ kind: "usage", message: USAGE_NICK });
    expect(parseCommand("/me")).toEqual({ kind: "usage", message: USAGE_ME });
    expect(parseCommand("/join")).toEqual({ kind: "usage", message: USAGE_JOIN });
    expect(parseCommand("/msg")).toEqual({ kind: "usage", message: USAGE_MSG });
    expect(parseCommand("/msg 0x9c")).toEqual({ kind: "usage", message: USAGE_MSG });
  });

  it("rejects anything else with the unknown-command line", () => {
    expect(parseCommand("/xyz")).toEqual({ kind: "usage", message: "unknown command: /xyz" });
  });

  it("understands the two hidden commands", () => {
    expect(parseCommand("/leave")).toEqual({ kind: "leave" });
    expect(parseCommand("/theme amber")).toEqual({ kind: "theme", theme: "amber" });
    expect(parseCommand("/theme nonsense")).toMatchObject({ kind: "usage" });
  });
});

describe("tab completion", () => {
  it("completes a unique prefix", () => {
    expect(completeCommand("/ni")).toBe("/nick ");
    expect(completeCommand("/th")).toBe("/theme ");
  });

  it("leaves an ambiguous prefix alone", () => {
    expect(completeCommand("/")).toBeNull();
  });

  it("does nothing once the command has an argument", () => {
    expect(completeCommand("/nick ri")).toBeNull();
  });

  it("does nothing for plain text", () => {
    expect(completeCommand("hello")).toBeNull();
  });
});

describe("roster mention", () => {
  it("appends to an empty draft", () => {
    expect(mentionDraft("", "rill")).toBe("@rill ");
  });

  it("normalises trailing whitespace so it never double-spaces", () => {
    expect(mentionDraft("hello", "rill")).toBe("hello @rill ");
    expect(mentionDraft("hello ", "rill")).toBe("hello @rill ");
    expect(mentionDraft("hello   ", "rill")).toBe("hello @rill ");
    expect(mentionDraft("hello\n", "rill")).toBe("hello @rill ");
  });
});
