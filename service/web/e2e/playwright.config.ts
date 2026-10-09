// End-to-end tests of the four web apps in API mode against the local stack (IR252): the compose backend and Keycloak,
// and the apps on ports 3000–3003 (or E2E_<APP>_URL). One project per app runs the shared specs and its own; each signs
// in once through the UI in its setup project. The specs restore what they change; they share the demo users, so they
// run one at a time. Run from service/web: `npm run e2e` (`-- --project=admin` for one app).
import { defineConfig, devices } from "@playwright/test";
import { APPS, type App } from "./fixtures/apps";

export default defineConfig<object, { app: App }>({
  testDir: ".",
  outputDir: "./test-results",
  reporter: [["list"], ["html", { outputFolder: "./playwright-report", open: "never" }]],
  workers: 1,
  timeout: 90_000,
  expect: { timeout: 10_000 },
  use: { ...devices["Desktop Chrome"], viewport: { width: 1920, height: 1080 }, trace: "retain-on-failure", screenshot: "only-on-failure" },
  projects: APPS.flatMap((app) => [
    { name: `${app.id}-setup`, testMatch: /setup\/auth\.setup\.ts$/, use: { app, baseURL: app.url } },
    {
      name: app.id,
      dependencies: [`${app.id}-setup`],
      testMatch: [/specs\/shared\/.*\.e2e\.ts$/, new RegExp(`specs/${app.id}/.*\\.e2e\\.ts$`)],
      use: { app, baseURL: app.url, storageState: app.state },
    },
  ]),
});
