// The strict ESLint of the shared package, being brought to zero findings (the user's "全部入り", 2026-11-03): the web's
// shared rule set (../eslint.base.mjs). It replaces eslint.config.mjs once nothing is left.
import { webConfig } from "../eslint.base.mjs";

const config = [
  ...webConfig(import.meta.dirname),
  { rules: { "@next/next/no-html-link-for-pages": "off" } }, // a package without a pages/ directory
];

export default config;
