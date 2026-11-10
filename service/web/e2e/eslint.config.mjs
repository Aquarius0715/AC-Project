// ESLint of the E2E specs: the web's shared rule set (../eslint.base.mjs) and playwright's recommended rules.
import playwright from "eslint-plugin-playwright";

import { webConfig } from "../eslint.base.mjs";

const config = [
  ...webConfig(import.meta.dirname),
  { files: ["**/*.ts"], ...playwright.configs["flat/recommended"] },
  // a Playwright fixture hands its value on with use(), which is not React's use hook; the specs are no Next.js app
  { rules: { "react-hooks/rules-of-hooks": "off", "@next/next/no-html-link-for-pages": "off" } },
  // a spec runs against a live stack, so it branches on what the stack and the project offer (a role, a viewport, a
  // feature the stack lacks) and skips with a reason instead of failing; its patterns come from fixture ids
  {
    rules: {
      "playwright/no-conditional-expect": "off",
      "playwright/no-conditional-in-test": "off",
      "playwright/no-skipped-test": "off",
      "sonarjs/no-skipped-tests": "off",
      "security/detect-non-literal-regexp": "off",
    },
  },
  // the fixtures read Node's CommonJS require and __dirname (the web packages are CommonJS, as in the Next.js configs)
  { files: ["**/fixtures/**/*.ts"], rules: { "unicorn/prefer-module": "off" } },
  // Each of these is a deliberate case, with its file:
  // - waits that follow the clock (the one-time code's 30 s step, the 3 s the autosave spec lets pass) are fixed by nature
  // - the smoke crawl waits for the network to go idle once per page, to count what a page asks for
  // - the fixture ID of the seed is a UUIDv5, which is defined over SHA-1
  // - alert-policies restores state in a second afterEach that only the tests after it need; merging or moving it would
  //   run it for the tests before it too
  // - axe-core's guard has to live inside the function Playwright sends to the page
  {
    files: ["**/fixtures/auth.ts", "**/specs/technician/report-autosave.e2e.ts"],
    rules: { "playwright/no-wait-for-timeout": "off", "sonarjs/no-fixed-wait-in-tests": "off" },
  },
  { files: ["**/specs/shared/smoke.e2e.ts"], rules: { "playwright/no-networkidle": "off", "sonarjs/no-networkidle-wait": "off" } },
  { files: ["**/specs/customer/iot-demo.e2e.ts"], rules: { "sonarjs/hashing": "off" } },
  { files: ["**/specs/admin/alert-policies.e2e.ts"], rules: { "playwright/no-duplicate-hooks": "off", "playwright/prefer-hooks-on-top": "off" } },
  { files: ["**/fixtures/a11y.ts"], rules: { "unicorn/consistent-function-scoping": "off" } },
  // the specs, fixtures and set-up run on Node, which has the newer built-ins the browser code may not use
  {
    rules: {
      "unicorn/prefer-dispose": "error",
      "unicorn/prefer-iterator-to-array": "error",
      "unicorn/prefer-regexp-escape": "error",
      "unicorn/prefer-uint8array-base64": "error",
      "unicorn/prefer-uint8array-hex": "error",
    },
  },
];

export default config;
