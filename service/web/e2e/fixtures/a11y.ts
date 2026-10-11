// Automated accessibility checks (NFR-01, deterministic contracts: zero unresolved axe critical / serious issues, IR359,
// IR360): axe-core over the page or one part of it — an open dialog — for WCAG 2.0 / 2.1 / 2.2 A and AA. axe is
// injected with page.evaluate, which the page CSP does not govern, so the policy stays as strict as in production.
import { readFileSync } from "node:fs";
import { expect, type Page } from "@playwright/test";

const AXE = readFileSync(require.resolve("axe-core/axe.min.js"), "utf8");
const TAGS = ["wcag2a", "wcag2aa", "wcag21a", "wcag21aa", "wcag22aa"];
type Violation = { id: string; impact: string | null; help: string; nodes: { target: string[] }[] };

/** The critical and serious violations of the page, or of the elements `scope` selects, as readable lines. */
export async function axeViolations(page: Page, scope?: string): Promise<string[]> {
  if (scope) await page.locator(scope).first().waitFor({ state: "visible" });
  await page.evaluate(AXE);
  const found = await page.evaluate(async ([s, tags]) => {
    const axe = (window as unknown as { axe: { run: (c: unknown, o: object) => Promise<{ violations: Violation[] }> } }).axe;
    return (await axe.run(s ? { include: [s] } : document, { runOnly: { type: "tag", values: tags } })).violations;
  }, [scope ?? null, TAGS] as const);
  return found.filter((v) => v.impact === "critical" || v.impact === "serious")
    .map((v) => `${v.id} (${v.impact}) ×${v.nodes.length} — ${v.help} — ${v.nodes[0]?.target.join(" ") ?? ""}`);
}

/** Fails the test on any critical or serious violation of the page or of `scope` (a dialog: `[role="dialog"]`). */
export async function expectAccessible(page: Page, scope?: string): Promise<void> {
  expect(await axeViolations(page, scope), `critical or serious accessibility violations${scope ? ` in ${scope}` : ""} at ${new URL(page.url()).pathname}`).toEqual([]);
}
