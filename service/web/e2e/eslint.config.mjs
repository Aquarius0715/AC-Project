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
