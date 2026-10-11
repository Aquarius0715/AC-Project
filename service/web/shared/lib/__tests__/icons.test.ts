// @vitest-environment node
import { readdirSync, readFileSync, statSync } from "node:fs";
import { join, relative } from "node:path";
import { fileURLToPath } from "node:url";
import ts from "typescript";
import { describe, expect, it } from "vitest";

// One icon library (DEC-74, IR393–IR399): every icon of the four apps is a Lucide icon drawn by shared/components/Icon.
// A character standing in for an icon comes back easily — an emoji, a symbol, an arrow at the end of a link, a "+"
// before "Add …" — so this test reads every UI string of the apps (string literals, template text and JSX text; not
// comments) and refuses:
// - symbol and pictograph characters: geometric shapes, dingbats, technical and miscellaneous symbols, emoji, ⋯ ∅ ℹ ‼;
// - an arrow, ‹ or › at the start or end of a text. A link or button affordance is <Icon name="arrowRight" />. Between
//   words an arrow is typography ("requested → offered", "Home A › 1F"), and so is a separator with spaces on both sides
//   on the same line (" › ", "${a} → ${b}");
// - "+ " before a label ("+ Add user"): that is <Icon name="add" />;
// - a button whose only content is one sign ("+", "−", "–", "×"): an icon button with a label.
// The Malay dictionary is a source file too, so its keys and texts are held to the same rules.
const root = fileURLToPath(new URL("../../../", import.meta.url));
const DIRS = ["customer", "partner", "technician", "admin", "shared"];
const SYMBOL = /[\u2139\u203C\u2049\u2205\u2295-\u22A1\u22EF\u2300-\u23FF\u2460-\u24FF\u25A0-\u25FF\u2600-\u26FF\u2700-\u27BF\u2B00-\u2BFF]/u;
const PICTOGRAPH = /\p{Extended_Pictographic}/u;
const ARROW = /[\u2190-\u21FF\u27F0-\u27FF\u2900-\u297F\u2039\u203A]/u;
const TYPOGRAPHY = new Set(["©", "®", "™"]); // pictographs to Unicode, plain marks in a text

const iconChar = (c: string) => SYMBOL.test(c) || (PICTOGRAPH.test(c) && !ARROW.test(c) && !TYPOGRAPHY.has(c));

/** Why a UI text uses a character as an icon, or null when it does not. */
export function iconProblem(text: string): string | null {
  const symbol = [...text].find(iconChar);
  if (symbol) return `the symbol ${symbol} (U+${symbol.codePointAt(0)!.toString(16).toUpperCase()})`;
  const chars = [...text.trim()];
  if (!chars.length) return null;
  const first = chars[0], last = chars[chars.length - 1];
  const lead = text.match(/^[ \t]*/)![0], trail = text.match(/[ \t]*$/)![0];
  const separatorFirst = lead.length > 0 && /^[ \t]/.test(text.slice(lead.length + first.length));
  const separatorLast = trail.length > 0 && /[ \t]$/.test(text.slice(0, text.length - trail.length - last.length));
  if (ARROW.test(first) && !separatorFirst) return `the arrow ${first} before the text`;
  if (ARROW.test(last) && !separatorLast) return `the arrow ${last} after the text`;
  if (/^\s*\+ \p{Lu}/u.test(text)) return `"+ " before the label`;
  return null;
}

function sources(): string[] {
  const out: string[] = [];
  const walk = (dir: string) => {
    for (const name of readdirSync(dir)) {
      if (["node_modules", ".next", "__tests__"].includes(name)) continue;
      const path = join(dir, name);
      if (statSync(path).isDirectory()) walk(path);
      else if (/\.tsx?$/.test(name) && !/\.(gen|d)\.ts$/.test(name)) out.push(path);
    }
  };
  for (const d of DIRS) walk(join(root, d));
  return out;
}

/** The texts of a source file a person can read: string literals, template text and JSX text, with their lines. */
const BUTTONS = new Set(["button", "Btn", "LinkBtn", "Button", "Link", "a"]);
/** A sign standing alone as a button's content is an icon: "+", "−", "–", "×" and the like. */
const signButton = (text: string) => /^[^\p{L}\p{N}\s]$/u.test(text.trim()) && !/^[%°#$€£¥()]$/.test(text.trim());

function texts(file: string): { line: number; text: string; button?: boolean }[] {
  const sf = ts.createSourceFile(file, readFileSync(file, "utf8"), ts.ScriptTarget.Latest, true, file.endsWith(".tsx") ? ts.ScriptKind.TSX : ts.ScriptKind.TS);
  const out: { line: number; text: string; button?: boolean }[] = [];
  const visit = (n: ts.Node) => {
    if (ts.isStringLiteral(n) || ts.isNoSubstitutionTemplateLiteral(n) || ts.isTemplateHead(n) || ts.isTemplateMiddle(n) || ts.isTemplateTail(n) || ts.isJsxText(n)) {
      // a JSX text that is a button's only content
      const parent = ts.isJsxText(n) && ts.isJsxElement(n.parent) ? n.parent : null;
      const button = !!parent && BUTTONS.has(parent.openingElement.tagName.getText(sf)) && parent.children.every((c) => c === n || (ts.isJsxText(c) && !c.text.trim()));
      out.push({ line: sf.getLineAndCharacterOfPosition(n.getStart()).line + 1, text: n.text, button });
    }
    ts.forEachChild(n, visit);
  };
  visit(sf);
  return out;
}

describe("one icon library (DEC-74)", () => {
  it("tells a character icon from typography", () => {
    const cases: [string, boolean][] = [
      ["View details →", true], ["← Back", true], ["Alerts ›", true], ["‹", true], ["→", true],
      ["⚠ Offline", true], ["🔔", true], ["✓ Saved", true], ["⋯", true], ["Sort: status ↑", true], ["+ Add user", true],
      ["requested → offered", false], ["10 → 12 kW", false], [" › Invoice ", false], ["Home A › 1F › ", false], [" → ", false],
      ["+2 reminders", false], ["≤ 350 kPa", false], ["© 2026", false], ["26 °C", false], ["", false], ["⊘ Offline", true],
    ];
    expect(cases.filter(([text, icon]) => (iconProblem(text) !== null) !== icon).map(([text]) => text)).toEqual([]);
    expect([["+", true], ["−", true], ["–", true], ["×", true], ["5", false], ["Add", false], ["%", false], ["°", false]].filter(([t, sign]) => signButton(t as string) !== sign)).toEqual([]);
  });

  it("no UI string of the four apps uses a character as an icon", () => {
    const files = sources();
    expect(files.length).toBeGreaterThan(400);
    const problems = files.flatMap((file) => texts(file).flatMap(({ line, text, button }) => {
      const why = iconProblem(text) ?? (button && signButton(text) ? `the sign ${text.trim()} as a button's only content` : null);
      return why ? [`${relative(root, file)}:${line} ${JSON.stringify(text.trim().slice(0, 60))} — ${why}; use <Icon name="…" /> (shared/components/Icon.tsx)`] : [];
    }));
    expect(problems).toEqual([]);
  });
});
