// Every page an app links to renders for its signed-in role (API mode): no error boundary or not-found text, no page
// error, no failed document (the R222 smoke crawl as a test). Follows the app's own links only, navigations only.
import { test, expect, FAILED_PAGE } from "../../fixtures/test";

test("every linked page of the app renders", async ({ page, app }) => {
  test.setTimeout(240_000);
  const errors: string[] = [];
  page.on("pageerror", (e) => errors.push(`${page.url()}: ${e.message}`));
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
    const links = await page.locator("a[href^='/']").evaluateAll((as) => as.map((a) => a.getAttribute("href") ?? ""));
    for (const href of links) {
      const p = href.split("#")[0];
      if (!p || p.startsWith("/bff/") || p.startsWith("/login") || seen.has(p)) continue;
      if (p === app.home || p.startsWith(app.home + "/") || p.startsWith(app.home + "?") || ["/notifications", "/settings/preferences", "/demo"].includes(p)) queue.push(p);
    }
  }
  expect(failed, "pages that did not render").toEqual([]);
  expect(errors, "page errors").toEqual([]);
  expect(seen.size).toBeGreaterThan(3);
});
