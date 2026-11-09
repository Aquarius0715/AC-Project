// @vitest-environment node
import { readdirSync, readFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";

// A Server Action that tells a refused Core API call from other errors must answer the other errors too (IR675, IR676):
// a call that got no answer (the Core API restarting or unreachable) throws a plain error, and thrown from an action it
// sends the page to its error boundary, where what the user typed is lost. So a Server Action file that looks at
// `CoreError` words every other error through `actionFailure` (or as UNAVAILABLE itself).
const root = fileURLToPath(new URL("../../../", import.meta.url));
const APPS = ["customer", "partner", "technician", "admin"];

/** Every `actions.ts` of the apps' `app` folders. */
function actionFiles(directory: string): string[] {
  const found: string[] = [];
  const entries = readdirSync(directory, { withFileTypes: true });
  for (const entry of entries) {
    const full = path.join(directory, entry.name);
    if (entry.isDirectory() && entry.name !== "node_modules" && entry.name !== ".next") found.push(...actionFiles(full));
    else if (entry.name === "actions.ts") found.push(full);
  }
  return found;
}

describe("the Server Actions of the apps", () => {
  const files = APPS.flatMap((app) => actionFiles(path.join(root, app, "app")));

  it("find the action files", () => {
    expect(files.length).toBeGreaterThan(20);
  });

  it.each(files.map((file) => [path.relative(root, file), file] as const))("%s answers an error that is not a refused call", (_name, file) => {
    const source = readFileSync(file, "utf8");
    if (!source.includes("instanceof CoreError") && !source.includes("CoreError)")) return;
    expect(source.includes("actionFailure") || source.includes("error.unavailable")).toBe(true);
  });
});
