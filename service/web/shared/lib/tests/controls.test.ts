// @vitest-environment node
import { readdirSync, readFileSync, statSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import ts from "typescript";
import { describe, expect, it } from "vitest";
import { lookup } from "@ac/web/lib/lookup";

// Phase 5 of the shadcn/ui adoption (DEC-75, IR404): the screens build their controls from the shadcn/ui components
// (through the product names in components/ui.tsx), not from raw <button>, <input>, <select> and <textarea> elements. A
// raw element that a screen still needs goes on the baseline below with its count; the baseline is exact, so a file that
// gains one fails, and a file that loses its last one must leave the list. It is empty since the phase closed.
const root = fileURLToPath(new URL("../../../", import.meta.url));
const DIRS = ["customer", "partner", "technician", "admin", "shared"];
const RAW = new Set(["button", "input", "select", "textarea"]);
// the shadcn components and the facade over them: they render the raw element themselves
const FACADE = fileURLToPath(new URL("../../components/ui", import.meta.url));

/** The raw form elements of one file, by tag. */
export function rawElements(file: string): Record<string, number> {
  const sf = ts.createSourceFile(file, readFileSync(file, "utf8"), ts.ScriptTarget.Latest, true, file.endsWith(".tsx") ? ts.ScriptKind.TSX : ts.ScriptKind.TS);
  const counts: Record<string, number> = {};
  const visit = (n: ts.Node) => {
    if (ts.isJsxOpeningElement(n) || ts.isJsxSelfClosingElement(n)) {
      const tag = n.tagName.getText(sf);
      if (RAW.has(tag)) Object.assign(counts, { [tag]: (lookup(counts, tag) ?? 0) + 1 });
    }
    ts.forEachChild(n, visit);
  };
  visit(sf);
  return counts;
}

function sources(): string[] {
  const out: string[] = [];
  const walk = (directory: string) => {
    for (const name of readdirSync(directory)) {
      if (["node_modules", ".next", "tests"].includes(name)) continue;
      const file = path.join(directory, name);
      if (statSync(file).isDirectory()) walk(file);
      else if (name.endsWith(".tsx") && !file.startsWith(FACADE) && file !== FACADE + ".tsx") out.push(file);
    }
  };
  for (const d of DIRS) walk(path.join(root, d));
  return out;
}

/** The baseline: the raw elements each file still has, relative to service/web (none since IR404). */
const BASELINE: Record<string, Record<string, number>> = {};

describe("the screens' controls are shadcn/ui components (DEC-75)", () => {
  it("every raw form element left in a screen is on the baseline, and the baseline has no spare entry", () => {
    const got: Record<string, Record<string, number>> = {};
    for (const f of sources()) {
      const c = rawElements(f);
      if (Object.keys(c).length > 0) got[path.relative(root, f)] = c;
    }
    expect(got).toStrictEqual(BASELINE);
  });
});
