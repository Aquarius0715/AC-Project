// The security headers of network architecture §6 (IR354): every page carries a Content-Security-Policy with a fresh
// nonce, which Next.js puts on its scripts, and nosniff, a referrer policy and no framing. Static assets carry the
// static headers too. Runs in each app's project, signed in as its user; nothing is changed.
import { test, expect } from "../../fixtures/test";

test("pages carry the CSP with a fresh nonce and the static security headers", async ({ page, app }) => {
  const res = await page.goto(app.home);
  const h = res!.headers();
  const csp = h["content-security-policy"] ?? "";
  const nonce = csp.match(/'nonce-([^']+)'/)?.[1] ?? "";
  expect(nonce, "a nonce in the CSP").not.toBe("");
  for (const d of ["default-src 'self'", "object-src 'none'", "base-uri 'self'", "frame-ancestors 'none'", "connect-src 'self'"]) expect(csp).toContain(d);
  expect(csp).not.toContain("unsafe-eval"); // a production build
  expect([h["x-content-type-options"], h["referrer-policy"], h["x-frame-options"]]).toEqual(["nosniff", "strict-origin-when-cross-origin", "DENY"]);
  const html = await res!.text();
  expect(html, "the framework's scripts carry the nonce").toContain(`nonce="${nonce}"`);
  expect(html.match(/<script(?![^>]*nonce=)[^>]*>/g) ?? [], "scripts without the nonce").toEqual([]);
  const again = await page.request.get(app.home);
  expect(again.headers()["content-security-policy"]).not.toContain(`'nonce-${nonce}'`); // a fresh nonce per request
  const css = await page.locator("link[rel=stylesheet]").first().getAttribute("href");
  expect(css, "the app's stylesheet").toBeTruthy();
  expect((await page.request.get(css!)).headers()["x-content-type-options"]).toBe("nosniff");
});
