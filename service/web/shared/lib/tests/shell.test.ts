// The app shell in API mode (IR241): who is signed in and the sidebar badges per role — unread notifications for
// everyone, the customer's alerts, the contractor's offers, HQ's critical and warning alerts summed over open and
// acknowledged. A count that cannot be read shows no badge; mock mode has no live shell.
import { beforeEach, describe, expect, it, vi } from "vitest";

const NOTIFICATIONS_LIST = "notifications.list";

const coreOp = vi.fn();
const api = { on: true };
vi.mock("server-only", () => ({}));
vi.mock("next/server", () => ({ connection: () => Promise.resolve() }));
vi.mock("@ac/web/lib/dal", () => {
  return {
    apiMode: () => api.on,
    coreOp: (...a: unknown[]) => coreOp(...a),
    coreIdentity: () => Promise.resolve({ displayName: "Tan Wei", organizationName: "Demo Customer A" }),
    coreDisplay: () => Promise.resolve({ language: "en", timeZone: "Asia/Kuala_Lumpur" }),
  };
});

import { loadShell } from "@ac/web/lib/shell";

// HQ's alerts per filter: 2 open and 1 acknowledged critical, 5 warnings each
const alertTotal = (f: { severity?: string; status?: string }) => {
  if (f.severity !== "critical") return 5;
  return f.status === "open" ? 2 : 1;
};

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
    coreOp.mockImplementation((op: string) => Promise.resolve(op === NOTIFICATIONS_LIST ? { total: 2 } : { counts: { alertCount: 3 } }));
    const shell = await loadShell("client");
    expect(shell).toMatchObject({ user: "Tan Wei", organization: "Demo Customer A", badges: { "/notifications": 2, "/customer/alerts": 3 } });
  });

  it("shows the contractor's offers waiting for an answer", async () => {
    coreOp.mockImplementation((op: string) => Promise.resolve(op === NOTIFICATIONS_LIST ? { total: 0 } : { counts: { offerCount: 4 } }));
    const shell = await loadShell("contractor");
    expect(shell?.badges).toEqual({ "/notifications": 0, "/partner/jobs": 4 });
  });

  it("sums HQ's critical and warning alerts over open and acknowledged", async () => {
    coreOp.mockImplementation((op: string, input: { filters: { severity?: string; status?: string } }) =>
      Promise.resolve({ total: op === NOTIFICATIONS_LIST ? 1 : alertTotal(input.filters) }));
    const shell = await loadShell("admin");
    expect(shell?.badges).toEqual({ "/notifications": 1, "/admin/alerts": 2 + 1 + 5 + 5 });
    expect(coreOp.mock.calls.filter(([op]) => op === "alerts.list")).toHaveLength(4);
  });

  it("drops a badge whose count cannot be read", async () => {
    coreOp.mockImplementation((op: string) => (op === "summaries.get" ? Promise.reject(new Error("unavailable")) : Promise.resolve({ total: 6 })));
    const shell = await loadShell("client");
    expect(shell?.badges).toEqual({ "/notifications": 6 });
  });
});
