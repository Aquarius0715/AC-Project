// @vitest-environment node
import { readdirSync, readFileSync, statSync } from "node:fs";
import { join, relative } from "node:path";
import { fileURLToPath } from "node:url";
import ts from "typescript";
import { describe, expect, it } from "vitest";
import { characters } from "@ac/web/lib/text";
import { must } from "./must";

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
const SYMBOL = /[\u{2139}\u{203C}\u{2049}\u{2205}\u{2295}-\u{22A1}\u{22EF}\u{2300}-\u{23FF}\u{2460}-\u{24FF}\u{25A0}-\u{25FF}\u{2600}-\u{26FF}\u{2700}-\u{27BF}\u{2B00}-\u{2BFF}]/u;
const PICTOGRAPH = /\p{Extended_Pictographic}/u;
const ARROW = /[\u{2190}-\u{21FF}\u{27F0}-\u{27FF}\u{2900}-\u{297F}\u{2039}\u{203A}]/u;
const TYPOGRAPHY = new Set(["©", "®", "™"]); // pictographs to Unicode, plain marks in a text

const iconChar = (c: string) => SYMBOL.test(c) || (PICTOGRAPH.test(c) && !ARROW.test(c) && !TYPOGRAPHY.has(c));

/** Why a UI text uses a character as an icon, or null when it does not. */
export function iconProblem(text: string): string | null {
  const symbol = characters(text).find((item) => iconChar(item));
  if (symbol) return `the symbol ${symbol} (U+${must(symbol.codePointAt(0)).toString(16).toUpperCase()})`;
  const chars = characters(text.trim());
  if (chars.length === 0) return null;
  const first = must(chars[0]), last = must(chars[chars.length - 1]);
  const lead = must(/^[ \t]*/.exec(text))[0], trail = must(/[ \t]*$/.exec(text))[0];
  const isSeparatorFirst = lead.length > 0 && /^[ \t]/.test(text.slice(lead.length + first.length));
  if (ARROW.test(first) && !isSeparatorFirst) return `the arrow ${first} before the text`;
  const isSeparatorLast = trail.length > 0 && /[ \t]$/.test(text.slice(0, text.length - trail.length - last.length));
  if (ARROW.test(last) && !isSeparatorLast) return `the arrow ${last} after the text`;
  if (/^\s*\+ \p{Lu}/u.test(text)) return `"+ " before the label`;
  return null;
}

function sources(): string[] {
  const out: string[] = [];
  const walk = (directory: string) => {
    for (const name of readdirSync(directory)) {
      if (["node_modules", ".next", "tests"].includes(name)) continue;
      const path = join(directory, name);
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
      const isButton = !!parent && BUTTONS.has(parent.openingElement.tagName.getText(sf)) && parent.children.every((c) => c === n || (ts.isJsxText(c) && !c.text.trim()));
      out.push({ line: sf.getLineAndCharacterOfPosition(n.getStart()).line + 1, text: n.text, button: isButton });
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
    const problems = files.flatMap((file) => {
      return texts(file).flatMap(({ line, text, button }) => {
        const why = iconProblem(text) ?? (button && signButton(text) ? `the sign ${text.trim()} as a button's only content` : null);
        return why ? [`${relative(root, file)}:${String(line)} ${JSON.stringify(text.trim().slice(0, 60))} — ${why}; use <Icon name="…" /> (shared/components/Icon.tsx)`] : [];
      });
    });
    expect(problems).toEqual([]);
  });
});
