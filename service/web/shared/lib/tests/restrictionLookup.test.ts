// @vitest-environment node
import { describe, expect, it, vi } from "vitest";

const dal = vi.hoisted(() => ({ coreOp: vi.fn<(op: string, input: { invoiceId: string }) => Promise<{ items: { id: string }[] }>>() }));
vi.mock("@ac/web/lib/dal", () => dal);

const { restrictionFor } = await import("@ac/web/lib/restrictionLookup");

interface Invoice { id: string; status: string; dueAt: string }
// a contract's invoices as invoices.list answers them, the latest due first: three newer unpaid invoices the customer has
// not paid either, the overdue one the restriction was scheduled for, and an older paid one
const invoices: Invoice[] = [
  { id: "inv-nov", status: "unpaid", dueAt: "2026-11-15T00:00:00Z" },
  { id: "inv-oct", status: "unpaid", dueAt: "2026-10-15T00:00:00Z" },
  { id: "inv-sep", status: "unpaid", dueAt: "2026-09-15T00:00:00Z" },
  { id: "inv-aug", status: "unpaid", dueAt: "2026-08-15T00:00:00Z" },
  { id: "inv-jul", status: "paid", dueAt: "2026-07-15T00:00:00Z" },
];
// restrictions.forInvoice: the restriction cites only the August invoice
const causes = new Map([["inv-aug", [{ id: "restriction-1" }]]]);

// the customer has no restriction list: its active restriction is found through the invoices it cites (IR715)
describe("restrictionFor", () => {
  it("finds the restriction of an older overdue invoice behind newer unpaid ones", async () => {
    dal.coreOp.mockImplementation((_: string, input: { invoiceId: string }) => Promise.resolve({ items: causes.get(input.invoiceId) ?? [] }));
    const found = await restrictionFor(invoices, (r) => r.id === "restriction-1");
    expect(found?.invoice.id).toBe("inv-aug");
    // the unpaid invoices oldest due first, so the cause is asked first
    expect(dal.coreOp.mock.calls.map((c) => c[1])).toStrictEqual([{ invoiceId: "inv-aug", query: { limit: 10 } }]);
  });

  it("looks at the paid invoices last, the latest due first (a restriction released after payment)", async () => {
    dal.coreOp.mockReset();
    dal.coreOp.mockResolvedValue({ items: [] });
    expect(await restrictionFor(invoices, () => true)).toBeNull();
    expect(dal.coreOp.mock.calls.map((c) => c[1].invoiceId)).toStrictEqual(["inv-aug", "inv-sep", "inv-oct", "inv-nov", "inv-jul"]);
  });
});
