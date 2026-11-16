// The Phase 1A demo fixtures of /customer/payments and /customer/payments/[id] (DATA_SOURCE≠API). Both screens read this
// one list, so an invoice opened from the list shows the amount, due date and contract the list showed.
import { nonEmpty } from "@ac/web/lib/lists";

/** A demo invoice: paid is the payment date (null = unpaid); restricted = it causes the cooling restriction. */
export interface DemoInvoice { id: string; due: string; issued: string; paid: string | null; amount: string; restricted: boolean }
/** A demo contract: its units (name, place) and invoices. */
export interface DemoContract { id: string; name: string; sub: string; period: string; plan: string; units: [string, string][]; invoices: DemoInvoice[] }

export const demoContracts = nonEmpty<DemoContract>([
  {
    id: "contract-rto-a", name: "RTO Plan", sub: "contract-rto-a · Jan–Dec 2026", period: "Jan 1 – Dec 31, 2026", plan: "Rent-to-own (RTO)",
    units: [["Bedroom AC", "Home A › 1F › Bedroom"], ["Meeting room AC", "Office A › Meeting room"], ["Lobby AC", "Office A · not in a room"]],
    invoices: [
      { id: "invoice-overdue-a", due: "Sep 10, 2026", issued: "Aug 27", paid: null, amount: "120.00", restricted: true },
      { id: "invoice-0198", due: "Aug 10", issued: "Jul 27", paid: "Aug 8", amount: "80.00", restricted: false },
      { id: "invoice-0175", due: "Jul 10", issued: "Jun 26", paid: "Jul 9", amount: "80.00", restricted: false },
    ],
  },
  {
    id: "contract-gm-a", name: "General maintenance", sub: "contract-gm-a · Mar 2026 – Feb 2027", period: "Mar 1, 2026 – Feb 28, 2027", plan: "General maintenance",
    units: [["Study AC", "Home A › 2F › Study"]],
    invoices: [{ id: "invoice-sep-a", due: "Sep 30", issued: "Sep 16", paid: null, amount: "50.00", restricted: false }],
  },
  { id: "contract-es-a", name: "Energy service", sub: "contract-es-a · 2025", period: "2025", plan: "Energy service", units: [["Living room AC", "Home A › 1F"]], invoices: [] },
]);

/** The demo invoice with this id and its contract, or null for an unknown id (the page then shows not found). */
export function demoInvoice(id: string): { invoice: DemoInvoice; contract: DemoContract } | null {
  for (const contract of demoContracts) {
    const invoice = contract.invoices.find((i) => i.id === id);
    if (invoice) return { invoice, contract };
  }
  return null;
}
