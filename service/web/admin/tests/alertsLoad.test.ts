import { describe, expect, it, vi } from "vitest";

// the HQ alerts screen reads its alerts through the DAL (FR-A05); alerts.list sorts the most severe first across the
// statuses and serves pages of at most 100
const dal = vi.hoisted(() => ({ coreOp: vi.fn(), coreAll: vi.fn() }));
vi.mock("server-only", () => ({}));
vi.mock("@ac/web/lib/dal", () => dal);

import { scopeAlerts } from "../app/admin/alerts/_lib/load";
import type { ApiAdminAlert } from "@ac/web/lib/adminAlerts";

const alert = (id: string, status: ApiAdminAlert["status"], severity: ApiAdminAlert["severity"]): ApiAdminAlert => ({
  id, severity, status, version: 1, unitId: "unit-1", policyId: null, type: "high_temperature", causeCode: "threshold", evidenceKind: "measurement",
  evidenceText: "", observedAt: "2026-11-10T01:00:00Z", evidenceIds: [], detectedAt: "2026-11-10T01:00:00Z", acknowledgedAt: null, resolvedAt: null,
  resolutionReason: null,
});
interface Query { limit?: number; filters?: { status?: string } }
// the server's list: 100 resolved critical alerts sort ahead of the open warning
const server = [...Array.from({ length: 100 }, (_, n) => alert(`resolved-${String(n)}`, "resolved", "critical")), alert("open-1", "open", "warning")];
const matching = (q: Query = {}) => server.filter((a) => !q.filters?.status || a.status === q.filters.status);

describe("HQ alerts", () => {
  it("keeps an open alert that a page of every status cuts behind 100 resolved ones (IR710)", async () => {
    dal.coreOp.mockImplementation((_: string, q: Query) => Promise.resolve({ items: matching(q).slice(0, q.limit ?? 25) }));
    dal.coreAll.mockImplementation((_: string, q: Query) => Promise.resolve(matching(q)));
    const { items } = await scopeAlerts({ customerId: "cust-a" });
    expect(items.filter((a) => a.status === "open").map((a) => a.id)).toStrictEqual(["open-1"]);
    expect(items.filter((a) => a.status === "resolved")).toHaveLength(100);
  });
});
