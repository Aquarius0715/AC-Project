// Vitest of the customer app, as in the Next.js docs (Testing › Vitest): the app's own server actions with the DAL
// mocked, in Node, and its client hooks in jsdom (a `// @vitest-environment jsdom` line in the .tsx test) with the React
// plugin. `@ac/web/*` resolves to the shared package, as the app's imports do through the workspace link.
import { defineConfig } from "vitest/config";
import react from "@vitejs/plugin-react";

const shared = `${import.meta.dirname.replace(/\/$/, "")}/../shared`;

export default defineConfig({
  plugins: [react()],
  resolve: { alias: { "@ac/web": shared } },
  test: { environment: "node", include: ["tests/**/*.test.{ts,tsx}"], setupFiles: [`${shared}/lib/tests/setup-dom.ts`] },
});
