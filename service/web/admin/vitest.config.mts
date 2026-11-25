// Vitest of the admin app, as in the Next.js docs (Testing › Vitest): the app's own server code (loaders) with the
// DAL mocked, in Node, and its client views in jsdom (a `// @vitest-environment jsdom` line in the .tsx test) with the
// React plugin. `@ac/web/*` resolves to the shared package, as the app's imports do through the workspace link.
import { defineConfig } from "vitest/config";
import react from "@vitejs/plugin-react";

const shared = `${import.meta.dirname.replace(/\/$/, "")}/../shared`;

export default defineConfig({
  plugins: [react()],
  resolve: { alias: { "@ac/web": shared } },
  // 30 s per test, as the shared package: a jsdom walk through a whole screen passes 5 s when check.sh runs every
  // workspace's lint, typecheck and tests at the same time (partner usersDemo / usersLive did, 2026-11-25)
  test: { environment: "node", include: ["tests/**/*.test.{ts,tsx}"], setupFiles: [`${shared}/lib/tests/setup-dom.ts`], testTimeout: 30_000 },
});
