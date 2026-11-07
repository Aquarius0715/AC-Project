// ESLint of the shared package: the web's shared rule set (../eslint.base.mjs), CLAUDE.md 1-5.
import { webConfig } from "../eslint.base.mjs";

const config = [
  ...webConfig(import.meta.dirname),
  { rules: { "@next/next/no-html-link-for-pages": "off" } }, // a package without a pages/ directory
];

export default config;
