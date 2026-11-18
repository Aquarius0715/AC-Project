// @vitest-environment node
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";

// The palette in globals.css keeps the contrast that the UI/UX specification states (WCAG 2.2 AA 1.4.3, IR359, IR843):
// every text colour reads at 4.5:1 or more on every light surface, and each "on" colour on the fill it is made for. A
// new colour token has to be named in one of the lists below, so a text colour is never added without this check.
const css = readFileSync(fileURLToPath(new URL("../../globals.css", import.meta.url)), "utf8");
const tokens = new Map([...css.matchAll(/--([a-z0-9-]+):\s*(#[0-9a-f]{6})\s*;/giu)].map((m) => [m[1] ?? "", m[2] ?? ""]));

const TEXT = [
  "foreground", "card-foreground", "popover-foreground", "secondary-foreground", "muted-foreground", "accent-foreground",
  "sidebar-foreground", "sidebar-accent-foreground", "primary", "destructive",
  "color-ink", "color-ok", "color-warn", "color-crit", "color-unknown",
];
const SURFACES = [
  "background", "card", "popover", "secondary", "muted", "accent", "sidebar", "sidebar-accent",
  "color-primary-soft", "color-surface", "color-surface2", "color-bg",
];
/** [text, the fill it sits on] */
const ON = [
  ["primary-foreground", "primary"], ["sidebar-primary-foreground", "sidebar-primary"],
  ["color-ok", "color-ok-soft"], ["color-warn", "color-warn-soft"], ["color-crit", "color-crit-soft"],
  ["color-unknown", "color-unknown-soft"],
] as const;
// lines, focus rings and chart series: no text is drawn in them
const NO_TEXT = ["border", "input", "ring", "sidebar-border", "sidebar-ring", "color-line", "chart-1", "chart-2", "chart-3", "chart-4", "chart-5"];

function luminance(hex: string): number {
  const [r = 0, g = 0, b = 0] = [1, 3, 5].map((i) => {
    const v = Number.parseInt(hex.slice(i, i + 2), 16) / 255;
    return v <= 0.039_28 ? v / 12.92 : ((v + 0.055) / 1.055) ** 2.4;
  });
  return 0.2126 * r + 0.7152 * g + 0.0722 * b;
}

function ratio(a: string, b: string): number {
  const [hi, lo] = [luminance(a), luminance(b)].sort((x, y) => y - x);
  return ((hi ?? 0) + 0.05) / ((lo ?? 0) + 0.05);
}

function colour(name: string): string {
  const hex = tokens.get(name);
  if (hex === undefined) throw new Error(`globals.css declares no colour --${name}`);
  return hex;
}

describe("theme contrast (globals.css)", () => {
  it("names every colour token in one of the lists", () => {
    const named = new Set([...TEXT, ...SURFACES, ...ON.flat(), ...NO_TEXT]);
    expect([...tokens.keys()].filter((name) => !named.has(name))).toStrictEqual([]);
  });

  it("reads every text colour at 4.5:1 or more on every light surface", () => {
    const low = TEXT.flatMap((text) => SURFACES.filter((surface) => ratio(colour(text), colour(surface)) < 4.5).map((surface) => `${text} on ${surface}`));
    expect(low).toStrictEqual([]);
  });

  it("reads each on-colour at 4.5:1 or more on its own fill", () => {
    const low = ON.filter(([text, fill]) => ratio(colour(text), colour(fill)) < 4.5).map(([text, fill]) => `${text} on ${fill}`);
    expect(low).toStrictEqual([]);
  });

  it("measures the WCAG ratio", () => {
    expect(ratio("#000000", "#ffffff")).toBeCloseTo(21, 5);
    expect(ratio("#5b6e84", "#e6f0ff")).toBeGreaterThan(4.5);
    expect(ratio("#64788e", "#edf6ff")).toBeLessThan(4.5);
  });
});
