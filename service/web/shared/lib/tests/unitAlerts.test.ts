// @vitest-environment node
import { describe, expect, it, vi } from "vitest";

const dal = vi.hoisted(() => ({ coreAll: vi.fn(), coreItems: vi.fn() }));
vi.mock("@ac/web/lib/dal", () => dal);

const { unitAlerts } = await import("@ac/web/lib/unitAlerts");

interface Row { id: string; status: string }
interface Query { limit?: number; filters?: { unitId?: string; status?: string } }
// the server's alerts of the unit, most severe first across the statuses: 50 resolved critical ones ahead of an open
// warning and an acknowledged normal one
const server: Row[] = [...Array.from({ length: 50 }, (_, n) => ({ id: `resolved-${String(n)}`, status: "resolved" })), { id: "open-1", status: "open" }, { id: "ack-1", status: "acknowledged" }];
const matching = (q: Query = {}) => server.filter((a) => !q.filters?.status || a.status === q.filters.status);

// the unit pages (technician unit and alerts, contractor unit) list the unresolved alerts first (IR712)
describe("unitAlerts", () => {
  it("keeps every unresolved alert of the unit that one page of every status cuts behind resolved ones", async () => {
    dal.coreItems.mockImplementation((_: string, q: Query) => Promise.resolve(matching(q).slice(0, q.limit ?? 25)));
    dal.coreAll.mockImplementation((_: string, q: Query) => Promise.resolve(matching(q)));
    const rows = await unitAlerts<Row>("unit-1", 50);
    expect(rows.filter((a) => a.status !== "resolved").map((a) => a.id)).toStrictEqual(["open-1", "ack-1"]);
    expect(rows.filter((a) => a.status === "resolved")).toHaveLength(50);
    expect(dal.coreItems).toHaveBeenCalledWith("alerts.list", { limit: 50, filters: { unitId: "unit-1", status: "resolved" }, sort: { field: "updatedAt", direction: "desc" } });
  });
});
