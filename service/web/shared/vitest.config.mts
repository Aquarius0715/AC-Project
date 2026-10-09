// Vitest of the shared package, as in the Next.js docs (Testing › Vitest): the React plugin and a jsdom environment
// so component tests can follow the pure-function tests of lib/. `@ac/web/*` resolves to this package (the apps
// import it through the workspace link).
import { fileURLToPath } from "node:url";
import { defineConfig } from "vitest/config";
import react from "@vitejs/plugin-react";

const here = fileURLToPath(new URL(".", import.meta.url)).replace(/\/$/, "");

export default defineConfig({
  plugins: [react()],
  resolve: { alias: { "@ac/web": here } },
  test: { environment: "jsdom", include: ["**/*.test.{ts,tsx}"], exclude: ["**/node_modules/**", "**/.next/**"] },
});
