import { describe, expect, it, vi } from "vitest";

// the HQ restrictions screen counts each state in its scope (FR-A09); restrictions.list answers a page and the total
const dal = vi.hoisted(() => ({ coreOp: vi.fn() }));
vi.mock("server-only", () => ({}));
vi.mock("@ac/web/lib/dal", () => dal);

import { stateCounts } from "../app/admin/restrictions/_lib/counts";

interface Query { limit?: number; filters?: { contractId?: string; status?: string } }
// the server's restrictions of the scope: 120 released ones and one applied, the oldest
const server = [...Array.from({ length: 120 }, () => "released"), "applied"];

describe("HQ restriction counts", () => {
  it("counts every state of the scope, also past the first page and while a state is picked (IR714)", async () => {
    dal.coreOp.mockImplementation((_: string, q: Query) => Promise.resolve({ items: [], total: server.filter((s) => !q.filters?.status || s === q.filters.status).length }));
    const countOf = await stateCounts({ contractId: "contract-a" });
    expect([countOf("applied"), countOf("released"), countOf("scheduled")]).toStrictEqual([1, 120, 0]);
    expect(dal.coreOp).toHaveBeenCalledWith("restrictions.list", { limit: 1, filters: { contractId: "contract-a", status: "applied" } });
  });
});
