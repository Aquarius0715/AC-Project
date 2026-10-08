// ESLint flat config of the shared package, as in the Next.js docs (Configuring › ESLint); `next lint` was removed in Next.js 16.
import { defineConfig, globalIgnores } from "eslint/config";
import nextVitals from "eslint-config-next/core-web-vitals";
import nextTs from "eslint-config-next/typescript";

export default defineConfig([
  ...nextVitals,
  ...nextTs,
  globalIgnores([".next/**", "out/**", "build/**", "next-env.d.ts"]),
  { rules: { "@next/next/no-html-link-for-pages": "off" } }, // a package without a pages/ directory
]);
