// ESLint of the E2E specs: the web's shared rule set (../eslint.base.mjs) and playwright's recommended rules.
import playwright from "eslint-plugin-playwright";

import { webConfig } from "../eslint.base.mjs";

export default [
  ...webConfig(import.meta.dirname),
  { files: ["**/*.ts"], ...playwright.configs["flat/recommended"] },
  // a Playwright fixture hands its value on with use(), which is not React's use hook; the specs are no Next.js app
  { rules: { "react-hooks/rules-of-hooks": "off", "@next/next/no-html-link-for-pages": "off" } },
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
