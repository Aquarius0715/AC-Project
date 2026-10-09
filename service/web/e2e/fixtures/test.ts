// The test object of the suite: every project passes its app (fixtures/apps.ts) as the `app` option.
import { test as base, expect } from "@playwright/test";
import { APPS, type App } from "./apps";

export const test = base.extend<object, { app: App }>({
  app: [APPS[0], { option: true, scope: "worker" }],
});
export { expect };

/** Text that means a page failed instead of rendering (error boundary, not-found, framework error). */
export const FAILED_PAGE = /This page isn’t available|Application error|Something went wrong|Unhandled Runtime Error/;
