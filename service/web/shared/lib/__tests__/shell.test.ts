// The app shell in API mode (IR241): who is signed in and the sidebar badges per role — unread notifications for
// everyone, the customer's alerts, the contractor's offers, HQ's critical and warning alerts summed over open and
// acknowledged. A count that cannot be read shows no badge; mock mode has no live shell.
import { beforeEach, describe, expect, it, vi } from "vitest";

const coreOp = vi.fn();
const api = { on: true };
vi.mock("server-only", () => ({}));
vi.mock("next/server", () => ({ connection: async () => undefined }));
vi.mock("@ac/web/lib/dal", () => ({
  apiMode: () => api.on,
  coreOp: (...a: unknown[]) => coreOp(...a),
  coreIdentity: async () => ({ displayName: "Tan Wei", organizationName: "Demo Customer A" }),
  coreDisplay: async () => ({ language: "en", timeZone: "Asia/Kuala_Lumpur" }),
}));

import { loadShell } from "@ac/web/lib/shell";

beforeEach(() => {
  api.on = true;
  coreOp.mockReset();
});

describe("loadShell", () => {
  it("has no live shell in mock mode", async () => {
    api.on = false;
    expect(await loadShell("client")).toBeUndefined();
    expect(coreOp).not.toHaveBeenCalled();
  });

  it("shows the customer's unread notifications and alerts", async () => {
    coreOp.mockImplementation(async (op: string) => (op === "notifications.list" ? { total: 2 } : { counts: { alertCount: 3 } }));
    const shell = await loadShell("client");
    expect(shell).toMatchObject({ user: "Tan Wei", organization: "Demo Customer A", badges: { "/notifications": 2, "/customer/alerts": 3 } });
  });

  it("shows the contractor's offers waiting for an answer", async () => {
    coreOp.mockImplementation(async (op: string) => (op === "notifications.list" ? { total: 0 } : { counts: { offerCount: 4 } }));
    expect((await loadShell("contractor"))?.badges).toEqual({ "/notifications": 0, "/partner/jobs": 4 });
  });

  it("sums HQ's critical and warning alerts over open and acknowledged", async () => {
    coreOp.mockImplementation(async (op: string, input: { filters: { severity?: string; status?: string } }) =>
      op === "notifications.list" ? { total: 1 } : { total: input.filters.severity === "critical" ? (input.filters.status === "open" ? 2 : 1) : 5 });
    expect((await loadShell("admin"))?.badges).toEqual({ "/notifications": 1, "/admin/alerts": 2 + 1 + 5 + 5 });
    expect(coreOp.mock.calls.filter(([op]) => op === "alerts.list")).toHaveLength(4);
  });

  it("drops a badge whose count cannot be read", async () => {
    coreOp.mockImplementation(async (op: string) => {
      if (op === "summaries.get") throw new Error("unavailable");
      return { total: 6 };
    });
    expect((await loadShell("client"))?.badges).toEqual({ "/notifications": 6 });
  });
});
