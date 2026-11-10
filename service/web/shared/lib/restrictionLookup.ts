// The active restriction of a customer's contract, found through the invoices it cites (restrictions.forInvoice): the
// customer has no restriction list (IR583, IR610). Read by Contracts & payments, the overview's due notice and the unit
// page (IR715). The pages looked at the first three unpaid invoices, latest due first, so a customer who had not paid the
// newer invoices either lost the notice of a restriction scheduled for an older one.
import { coreOp } from "@ac/web/lib/dal";
import type { ApiRestriction } from "@ac/web/lib/restrictions";

interface Invoice { id: string; status: string; dueAt: string }

/** The order to look in: the unpaid invoices oldest due first (a restriction cites the invoices overdue when it was
scheduled), then the paid ones latest due first (a restriction being released once they are paid). */
function lookupOrder<T extends Invoice>(invoices: T[]): T[] {
  const unpaid = invoices.filter((x) => x.status !== "paid").toSorted((a, b) => a.dueAt.localeCompare(b.dueAt));
  const paid = invoices.filter((x) => x.status === "paid").toSorted((a, b) => b.dueAt.localeCompare(a.dueAt));
  return [...unpaid, ...paid];
}

/** The first restriction that `accept` takes, with the invoice it was found through, or null; it stops at the first. */
export async function restrictionFor<T extends Invoice>(invoices: T[], accept: (r: ApiRestriction) => boolean): Promise<{ r: ApiRestriction; invoice: T } | null> {
  for (const invoice of lookupOrder(invoices)) {
    const page = await coreOp<{ items: ApiRestriction[] }>("restrictions.forInvoice", { invoiceId: invoice.id, query: { limit: 10 } });
    const r = page.items.find((x) => accept(x));
    if (r) return { r, invoice };
  }
  return null;
}
