// Vitest of the portal, as in the Next.js docs (Testing › Vitest): the portal's own server code in Node.
// `@ac/web/*` resolves to the shared package, as the app's imports do through the workspace link.
import { defineConfig } from "vitest/config";

const shared = `${import.meta.dirname.replace(/\/$/, "")}/../shared`;

export default defineConfig({
  resolve: { alias: { "@ac/web": shared, "@": import.meta.dirname } },
  test: { environment: "node", include: ["tests/**/*.test.ts"] },
});
