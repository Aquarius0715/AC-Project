import { describe, expect, it } from "vitest";
import { demoContracts, demoInvoice } from "../app/customer/payments/_lib/demo-contracts";

// the demo invoice page reads the invoice the demo list showed (R840): its own amount, due date and contract, the
// cooling restriction only on the invoice that causes it, and an unknown id is not found
describe("demoInvoice", () => {
  it("returns the listed invoice with its own contract", () => {
    expect(demoInvoice("invoice-sep-a")).toMatchObject({ invoice: { amount: "50.00", due: "Sep 30", paid: null, restricted: false }, contract: { id: "contract-gm-a" } });
    expect(demoInvoice("invoice-0198")).toMatchObject({ invoice: { amount: "80.00", paid: "Aug 8" }, contract: { id: "contract-rto-a" } });
  });

  it("marks only the overdue RTO invoice as causing the restriction", () => {
    const restricted = demoContracts.flatMap((c) => c.invoices).filter((i) => i.restricted).map((i) => i.id);
    expect(restricted).toStrictEqual(["invoice-overdue-a"]);
  });

  it("returns null for an unknown id", () => {
    expect(demoInvoice("invoice-unknown")).toBeNull();
    expect(demoInvoice("")).toBeNull();
  });
});
