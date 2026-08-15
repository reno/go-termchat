import { readdirSync, readFileSync, statSync } from "node:fs";
import { join, resolve } from "node:path";

import { describe, expect, it } from "vitest";

const webRoot = process.cwd();
const repoRoot = resolve(webRoot, "..");

const terminalCss = readFileSync(join(webRoot, "src/styles/terminal.css"), "utf8");
const designSpec = readFileSync(join(repoRoot, "design/README.md"), "utf8");

const HEX = /#[0-9a-fA-F]{3,8}\b/g;
const PX_FONT_SIZE = /font-size:\s*\d+(\.\d+)?px/g;

function sourceFiles(): string[] {
  const roots = [join(webRoot, "src")];
  const found: string[] = [];

  while (roots.length > 0) {
    const dir = roots.pop();
    if (dir === undefined) {
      break;
    }
    for (const entry of readdirSync(dir)) {
      const path = join(dir, entry);
      if (statSync(path).isDirectory()) {
        if (entry !== "gen") {
          roots.push(path);
        }
        continue;
      }
      if (/\.(ts|tsx|css)$/.test(entry)) {
        found.push(path);
      }
    }
  }
  return found;
}

describe("design tokens", () => {
  it("invents no colour: every hex in terminal.css appears in design/README.md", () => {
    const hexes: string[] = [...new Set<string>(terminalCss.match(HEX) ?? [])];
    expect(hexes.length).toBeGreaterThan(0);

    const missing = hexes.filter((hex) => !designSpec.toLowerCase().includes(hex.toLowerCase()));
    expect(missing).toEqual([]);
  });

  it("defines the four theme accents the export declares", () => {
    for (const accent of ["#7fb8a4", "#e0b080", "#8fb4dc", "#c8a2d8"]) {
      expect(terminalCss).toContain(accent);
    }
  });

  it("defines the six nick palette entries", () => {
    for (let i = 1; i <= 6; i++) {
      expect(terminalCss).toContain(`--nick-${String(i)}:`);
    }
  });

  it("keeps the exact named spacing values, unrounded", () => {
    const spacing: Record<string, string> = {
      "--pad-header": "10px 14px",
      "--pad-transcript": "16px 16px 20px",
      "--gap-message": "9px",
      "--gap-msg-cols": "4px 10px",
      "--gap-hint": "6px 14px",
      "--pad-panel": "14px 14px 18px",
      "--gap-roster": "7px",
      "--pad-roster": "2px 0",
      "--pad-btn-menu": "6px 10px",
    };

    for (const [token, value] of Object.entries(spacing)) {
      expect(terminalCss).toContain(`${token}: ${value};`);
    }
  });

  it("promotes every failing grey to a passing token for content text", () => {
    const promotions = [
      "--fg-system-content: var(--fg-muted)",
      "--fg-dim-content: var(--fg-muted)",
      "--fg-faint-content: var(--fg-muted)",
      "--fg-placeholder-content: var(--fg-muted)",
      "--fg-faint-content-raised: var(--fg-channel)",
    ];

    for (const promotion of promotions) {
      expect(terminalCss).toContain(promotion);
    }
  });

  it("keeps every raw hex out of components, so only terminal.css names colours", () => {
    const offenders: string[] = [];

    for (const path of sourceFiles()) {
      if (path.endsWith(join("styles", "terminal.css"))) {
        continue;
      }
      if (path.endsWith(".test.ts") || path.endsWith(".test.tsx")) {
        continue;
      }
      const hexes = readFileSync(path, "utf8").match(HEX);
      if (hexes !== null) {
        offenders.push(`${path.slice(webRoot.length)}: ${hexes.join(", ")}`);
      }
    }

    expect(offenders).toEqual([]);
  });

  it("keeps every px font size out of components", () => {
    const offenders: string[] = [];

    for (const path of sourceFiles()) {
      if (path.endsWith(join("styles", "terminal.css"))) {
        continue;
      }
      const sizes = readFileSync(path, "utf8").match(PX_FONT_SIZE);
      if (sizes !== null) {
        offenders.push(`${path.slice(webRoot.length)}: ${sizes.join(", ")}`);
      }
    }

    expect(offenders).toEqual([]);
  });
});
