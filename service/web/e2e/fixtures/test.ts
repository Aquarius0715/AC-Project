// The test object of the suite: every project passes its app (fixtures/apps.ts) as the `app` option.
import { test as base, expect } from "@playwright/test";
import { APPS, type App } from "./apps";

export const test = base.extend<object, { app: App }>({
  app: [APPS[0], { option: true, scope: "worker" }],
  // page.goto waits until the app's page has hydrated (UIProviders sets <html data-hydrated>): a click or a keystroke
  // before that is lost, which made specs flaky on a busy CI runner. Only the app's own HTML pages carry the mark.
  page: async ({ page, baseURL }, use) => {
    const goto = page.goto.bind(page);
    page.goto = async (url, options) => {
      const response = await goto(url, options);
      const own = !baseURL || new URL(page.url()).origin === new URL(baseURL).origin; // not the identity provider's pages
      if (own && response?.headers()["content-type"]?.includes("text/html")) {
        await page.waitForFunction(() => document.documentElement.dataset.hydrated === "1", null, { timeout: 15_000 }).catch(() => undefined);
      }
      return response;
    };
    await use(page);
  },
});
export { expect };

/** Text that means a page failed instead of rendering (error boundary, not-found, framework error). */
export const FAILED_PAGE = /This page isn’t available|Application error|Something went wrong|Unhandled Runtime Error/;
