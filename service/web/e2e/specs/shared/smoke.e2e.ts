// Every page an app links to renders for its signed-in role (API mode): no error boundary or not-found text, no page
// error, no failed document (the R222 smoke crawl as a test), nothing the page CSP refuses (IR354), and no critical or
// serious axe-core violation of WCAG 2.x A / AA (NFR-01, deterministic contracts: zero unresolved axe critical /
// serious issues, IR359). Follows the app's own links only, navigations only. axe runs through page.evaluate, which the
// page CSP does not govern, so the policy itself stays as strict as in production.
import { test, expect, FAILED_PAGE } from "../../fixtures/test";
import { axeViolations } from "../../fixtures/a11y";

test("every linked page of the app renders", async ({ page, app }) => {
  test.setTimeout(240_000);
  const errors: string[] = [];
  page.on("pageerror", (e) => errors.push(`${page.url()}: ${e.message}`));
  const inaccessible: string[] = [];
  const refused: string[] = []; // Chromium logs each CSP violation as a console error
  page.on("console", (m) => { if (m.type() === "error" && /Content Security Policy/i.test(m.text())) refused.push(`${page.url()}: ${m.text().slice(0, 200)}`); });
  const seen = new Set<string>();
  const queue = [app.home];
  const failed: string[] = [];
  while (queue.length && seen.size < 40) {
    const path = queue.shift()!;
    if (seen.has(path)) continue;
    seen.add(path);
    const res = await page.goto(path, { waitUntil: "networkidle" });
    const text = await page.locator("body").innerText();
    if (!res || res.status() >= 400 || FAILED_PAGE.test(text)) failed.push(`${path} (${res?.status() ?? "no response"})`);
    for (const v of await axeViolations(page)) inaccessible.push(`${path}: ${v}`);
    const links = await page.locator("a[href^='/']").evaluateAll((as) => as.map((a) => a.getAttribute("href") ?? ""));
    for (const href of links) {
      const p = href.split("#")[0];
      if (!p || p.startsWith("/bff/") || p.startsWith("/login") || seen.has(p)) continue;
      if (p === app.home || p.startsWith(app.home + "/") || p.startsWith(app.home + "?") || ["/notifications", "/settings/preferences", "/demo"].includes(p)) queue.push(p);
    }
  }
  expect(failed, "pages that did not render").toEqual([]);
  expect(errors, "page errors").toEqual([]);
  expect(refused, "CSP violations").toEqual([]);
  expect(inaccessible, "critical or serious accessibility violations").toEqual([]);
  expect(seen.size).toBeGreaterThan(3);
});
