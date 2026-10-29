// Every page an app links to renders for its signed-in role (API mode): no error boundary or not-found text, no page
// error, no failed document (the R222 smoke crawl as a test), nothing the page CSP refuses (IR354), and no critical or
// serious axe-core violation of WCAG 2.x A / AA (NFR-01, deterministic contracts: zero unresolved axe critical /
// serious issues, IR359). Follows the app's own links only, navigations only. axe runs through page.evaluate, which the
// page CSP does not govern, so the policy itself stays as strict as in production. The crawl runs again at 360 px, the
// narrowest width the UIUX specification names, in Malay, the longest words (IR487, IR500): no axe violation there either, and no page scrolls sideways. Every page declares
// the display language as its language (IR489) and is titled with its name (IR490).
import type { Page } from "@playwright/test";
import { test, expect, FAILED_PAGE } from "../../fixtures/test";
import { axeViolations } from "../../fixtures/a11y";
import type { App } from "../../fixtures/apps";
import { displayZone, ENGLISH, MALAY, setDisplay } from "../../fixtures/display";

type Crawl = { seen: number; failed: string[]; inaccessible: string[]; wide: string[]; otherLang: string[]; untitled: string[] };

/** Visits up to 40 pages the app links to, from its home, and records what fails on each; `lang` is the page language
 * every page must declare (WCAG 3.1.1, IR489). `wideFont` sets a wide font on every page before it is measured, so a
 * Mac finds what the wider fonts of the Linux runners push sideways (IR501). */
async function crawl(page: Page, app: App, lang = "en", wideFont = false): Promise<Crawl> {
  const seen = new Set<string>();
  const queue = [app.home];
  const out: Crawl = { seen: 0, failed: [], inaccessible: [], wide: [], otherLang: [], untitled: [] };
  while (queue.length && seen.size < 40) {
    const path = queue.shift()!;
    if (seen.has(path)) continue;
    seen.add(path);
    const res = await page.goto(path, { waitUntil: "networkidle" });
    if (wideFont) await page.addStyleTag({ content: "*{font-family: Verdana, 'DejaVu Sans', sans-serif !important}" });
    const text = await page.locator("body").innerText();
    if (!res || res.status() >= 400 || FAILED_PAGE.test(text)) out.failed.push(`${path} (${res?.status() ?? "no response"})`);
    for (const v of await axeViolations(page)) out.inaccessible.push(`${path}: ${v}`);
    const sideways = await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth);
    if (sideways > 1) out.wide.push(`${path}: ${sideways}px`);
    const declared = await page.locator("html").getAttribute("lang");
    if (declared !== lang) out.otherLang.push(`${path}: ${declared}`);
    const title = await page.title(); // the page's name, then the product (WCAG 2.4.2, IR490)
    if (!/^.+ — AC Project$/.test(title)) out.untitled.push(`${path}: ${title}`);
    const links = await page.locator("a[href^='/']").evaluateAll((as) => as.map((a) => a.getAttribute("href") ?? ""));
    for (const href of links) {
      const p = href.split("#")[0];
      if (!p || p.startsWith("/bff/") || p.startsWith("/login") || seen.has(p)) continue;
      if (p === app.home || p.startsWith(app.home + "/") || p.startsWith(app.home + "?") || ["/notifications", "/settings/preferences", "/demo"].includes(p)) queue.push(p);
    }
  }
  out.seen = seen.size;
  return out;
}

test("every linked page of the app renders", async ({ page, app }) => {
  test.setTimeout(240_000);
  const errors: string[] = [];
  page.on("pageerror", (e) => errors.push(`${page.url()}: ${e.message}`));
  const refused: string[] = []; // Chromium logs each CSP violation as a console error
  page.on("console", (m) => { if (m.type() === "error" && /Content Security Policy/i.test(m.text())) refused.push(`${page.url()}: ${m.text().slice(0, 200)}`); });
  const r = await crawl(page, app);
  expect(r.failed, "pages that did not render").toEqual([]);
  expect(errors, "page errors").toEqual([]);
  expect(refused, "CSP violations").toEqual([]);
  expect(r.inaccessible, "critical or serious accessibility violations").toEqual([]);
  expect(r.otherLang, "pages that do not declare English").toEqual([]);
  expect(r.untitled, "pages whose title does not name them").toEqual([]);
  expect(r.seen).toBeGreaterThan(3);
  // a client-side navigation names the new page too
  await page.goto(app.home);
  const link = page.locator(`aside nav a[href^="${app.home}/"]`).first(); // a menu page below the home, without a query
  const name = (await link.locator("span.flex-1").innerText()).trim(); // the label without its badge
  await link.click();
  await expect(page).toHaveTitle(`${name} — AC Project`);
});

let zoneBefore: string | null = null;
test.afterEach(async ({ page }) => {
  if (zoneBefore) await setDisplay(page, ENGLISH, zoneBefore);
  zoneBefore = null;
});

test.describe("phone width", () => {
  test.use({ bypassCSP: true }); // the wide test font is an injected style; the first crawl checks the CSP itself

  test("every linked page at phone width in Malay is accessible and fits the screen", async ({ page, app }) => {
    test.setTimeout(300_000);
    const zone = await displayZone(page);
    zoneBefore = zone;
    await setDisplay(page, MALAY, zone);
    await page.setViewportSize({ width: 360, height: 800 }); // the UIUX specification's narrowest width and Figma's mobile frames
    const r = await crawl(page, app, "ms", true);
    expect(r.failed, "pages that did not render").toEqual([]);
    expect(r.otherLang, "pages that do not declare Malay").toEqual([]);
    expect(r.untitled, "pages whose title does not name them").toEqual([]);
    expect(r.inaccessible, "critical or serious accessibility violations at phone width").toEqual([]);
    expect(r.wide, "pages that scroll sideways at phone width").toEqual([]);
    expect(r.seen).toBeGreaterThan(3);
  });
});
