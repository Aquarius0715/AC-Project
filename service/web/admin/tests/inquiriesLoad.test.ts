import { describe, expect, it, vi } from "vitest";

// HQ billing reads the customers' inquiries through the DAL (FR-A08); inquiries.list sorts the newest first across the
// states and serves pages of at most 100
const dal = vi.hoisted(() => ({ coreOp: vi.fn(), coreAll: vi.fn() }));
vi.mock("server-only", () => ({}));
vi.mock("@ac/web/lib/dal", () => dal);

import { billingInquiries } from "../app/admin/billing/_lib/inquiries";
import type { ApiInquiry } from "@ac/web/lib/billing";

const inquiry = (id: string, state: ApiInquiry["state"], createdAt: string): ApiInquiry => ({
  id, state, createdAt, version: 1, customerId: "cust-a", invoiceId: null, restrictionId: null, subjectType: "payment", message: "", reply: null,
});
interface Query { limit?: number; filters?: { status?: string } }
// the server's list, newest first: 100 answered inquiries after an older one still waiting for a reply
const server = [...Array.from({ length: 100 }, (_, n) => inquiry(`answered-${String(n)}`, "answered", "2026-11-09T00:00:00Z")), inquiry("received-1", "received", "2026-10-01T00:00:00Z")];
const matching = (q: Query = {}) => server.filter((x) => !q.filters?.status || x.state === q.filters.status);

describe("HQ inquiries", () => {
  it("keeps an inquiry waiting for a reply that 100 newer answered ones push off the first page (IR713)", async () => {
    dal.coreOp.mockImplementation((_: string, q: Query) => Promise.resolve({ items: matching(q).slice(0, q.limit ?? 25) }));
    dal.coreAll.mockImplementation((_: string, q: Query) => Promise.resolve(matching(q)));
    const rows = await billingInquiries();
    expect(rows.filter((x) => x.state === "received").map((x) => x.id)).toStrictEqual(["received-1"]);
    expect(rows.filter((x) => x.state === "answered")).toHaveLength(100);
  });
});
