// Vitest of the technician app, as in the Next.js docs (Testing › Vitest): the app's own server code (loaders) with the
// DAL mocked, in Node. `@ac/web/*` resolves to the shared package, as the app's imports do through the workspace link.
import { defineConfig } from "vitest/config";

const shared = `${import.meta.dirname.replace(/\/$/, "")}/../shared`;

export default defineConfig({
  resolve: { alias: { "@ac/web": shared } },
  test: { environment: "node", include: ["tests/**/*.test.ts"] },
});
