import { beforeEach, describe, expect, it, vi } from "vitest";
import { isValidElement, type ComponentProps, type ReactNode } from "react";
import type { ApiInvoice, ApiStatement } from "@ac/web/lib/billing";

// The HQ billing screen (FR-A08, FR-A23, SCR-A08): the Server Actions (invoices, payments, reminders, inquiries and
// contractor payouts, each at its version) and the Server Component (the URL scope and months, the tab, the selected
// invoice with its reminder recipients, and the payouts tab's filters, statements and approver names).
const dal = vi.hoisted(() => {
  interface DomainError { code: string; messageKey: string; fieldErrors: Record<string, string>; retryAfterSeconds: number | null }
  class CoreError extends Error {
    readonly error: DomainError;
    constructor(error: DomainError, options?: ErrorOptions) {
      super(error.code, options);
      this.name = "CoreError";
      this.error = error;
    }
  }
  const orWhenRefused = async <T, F>(read: Promise<T>, fallback: F): Promise<T | F> => {
    try {
      return await read;
    } catch (error) {
      if (error instanceof CoreError && error.error.code !== "UNAVAILABLE") return fallback;
      throw error;
    }
  };
  return {
    CoreError, orWhenRefused, apiMode: vi.fn(() => true), coreNow: vi.fn(() => Promise.resolve(new Date("2026-09-14T01:00:00Z"))),
    coreDisplay: vi.fn(() => Promise.resolve({ locale: "en", timeZone: "Asia/Kuala_Lumpur" })),
    corePermissions: vi.fn(() => Promise.resolve(new Set(["billing.read", "asset.read", "restriction.read"]))),
    coreOp: vi.fn<(op: string, input?: unknown, options?: unknown) => Promise<unknown>>(),
    coreAll: vi.fn<(op: string, input?: unknown) => Promise<unknown>>(),
    coreAllPage: vi.fn<(op: string, input?: unknown) => Promise<unknown>>(),
  };
});
const cache = vi.hoisted(() => ({ refresh: vi.fn() }));
vi.mock("server-only", () => ({}));
vi.mock("next/cache", () => cache);
vi.mock("next/server", () => ({ connection: () => Promise.resolve() }));
vi.mock("@ac/web/lib/dal", () => dal);
vi.mock("../app/admin/billing/_components/billing-view", () => ({ BillingView: () => null }));

import {
  answerInquiry, confirmCardPayment, createInvoice, generatePayouts, previewReminder, recordManualPayment, resolvePayoutQuery, sendReminder, transitionStatement,
} from "../app/admin/billing/actions";
import AdminBillingPage from "../app/admin/billing/page";
import { BillingView } from "../app/admin/billing/_components/billing-view";
import BillingLoading from "../app/admin/billing/loading";
import BillingErrorState from "../app/admin/billing/error";
import { RouteError, RouteLoading } from "@ac/web/components/RouteStates";
import { NeedsAccess } from "@ac/web/components/NeedsAccess";

const refused = (code: string) => new dal.CoreError({ code, messageKey: `error.${code.toLowerCase()}`, fieldErrors: {}, retryAfterSeconds: null });
const lastCall = () => dal.coreOp.mock.lastCall;

beforeEach(() => {
  dal.apiMode.mockReturnValue(true);
  dal.coreOp.mockReset(); dal.coreAll.mockReset(); dal.coreAllPage.mockReset(); cache.refresh.mockReset();
});

describe("billing Server Actions", () => {
  it("creates an invoice for a contract version with Kuala Lumpur dates", async () => {
    dal.coreOp.mockResolvedValue({});
    expect(await createInvoice({ contractId: "k-1", contractVersion: 2, from: "2026-08-01", to: "2026-09-01", dueAt: "2026-09-10", amountMinor: 12_000, currency: "MYR" })).toStrictEqual({ ok: true, value: null });
    expect(lastCall()).toStrictEqual(["invoices.create", {
      contractId: "k-1", contractVersion: 2, amountMinor: 12_000, currency: "MYR", period: { from: "2026-08-01T00:00:00+08:00", to: "2026-09-01T00:00:00+08:00" }, dueAt: "2026-09-10T00:00:00+08:00",
    }, { write: true }]);
    expect(cache.refresh).toHaveBeenCalledOnce();
  });

  it("records payments, reminders, answers and payout steps at their versions", async () => {
    dal.coreOp.mockResolvedValue({});
    const cases: [() => Promise<unknown>, string, unknown, unknown][] = [
      [() => recordManualPayment("inv-1", 3, "BANK-7", 12_000, "MYR", "Paid at the counter"), "payments.recordManual", { confirmedAmountMinor: 12_000, invoiceId: "inv-1", paymentReference: "BANK-7", currency: "MYR", reason: "Paid at the counter" }, { write: true, expectedVersion: 3 }],
      [() => confirmCardPayment("pay-1", 2, "ch_1", 12_000, "MYR", "Stripe settled"), "payments.confirm", { confirmedAmountMinor: 12_000, paymentId: "pay-1", paymentReference: "ch_1", currency: "MYR", reason: "Stripe settled" }, { write: true, expectedVersion: 2 }],
      [() => sendReminder("inv-1", 3, "m-1", "email", "Second notice"), "invoices.remind", { invoiceId: "inv-1", recipientMembershipId: "m-1", channel: "email", reason: "Second notice" }, { write: true, expectedVersion: 3 }],
      [() => answerInquiry("iq-1", 1, "We fixed the amount"), "inquiries.answer", { inquiryId: "iq-1", reply: "We fixed the amount" }, { write: true, expectedVersion: 1 }],
      [() => transitionStatement("st-1", 4, "approve"), "payouts.transition", { statementId: "st-1", action: "approve" }, { write: true, expectedVersion: 4 }],
      [() => resolvePayoutQuery("st-1", 4, "q-1", "Adjusted", -500), "payouts.resolveQuery", { statementId: "st-1", queryId: "q-1", reply: "Adjusted", adjustmentMinor: -500 }, { write: true, expectedVersion: 4 }],
      [() => resolvePayoutQuery("st-1", 5, "q-2", "No change", null), "payouts.resolveQuery", { statementId: "st-1", queryId: "q-2", reply: "No change" }, { write: true, expectedVersion: 5 }],
    ];
    for (const [call, op, body, options] of cases) {
      expect(await call()).toStrictEqual({ ok: true, value: null });
      expect(lastCall()).toStrictEqual([op, body, options]);
    }
    expect(cache.refresh).toHaveBeenCalledTimes(cases.length);
  });

  it("generates the payouts of a month and counts them, and previews a reminder without re-rendering", async () => {
    dal.coreOp.mockResolvedValueOnce([{}, {}, {}]);
    expect(await generatePayouts("2026-08")).toStrictEqual({ ok: true, value: 3 });
    expect(lastCall()).toStrictEqual(["payouts.generate", { period: "2026-08" }, { write: true }]);
    cache.refresh.mockReset();
    dal.coreOp.mockResolvedValueOnce({ params: { targetName: "INV-0001" } });
    expect(await previewReminder("inv-1", "m-1", "email", "Second notice")).toStrictEqual({ ok: true, value: "INV-0001" });
    expect(lastCall()).toStrictEqual(["notifications.preview", { target: { kind: "invoice", id: "inv-1" }, templateKey: "payment_reminder", channel: "email", recipientMembershipId: "m-1", reason: "Second notice" }]);
    expect(cache.refresh).not.toHaveBeenCalled();
    dal.coreOp.mockRejectedValueOnce(refused("CONFLICT"));
    expect(await transitionStatement("st-1", 4, "mark_paid")).toMatchObject({ ok: false, code: "CONFLICT" });
  });
});

const invoice = (id: string, over: Partial<ApiInvoice> = {}): ApiInvoice => ({
  id, version: 1, number: `INV-${id}`, amountMinor: 12_000, currency: "MYR", contractId: "k-1", contractVersion: 2, period: { from: "2026-08-01T00:00:00+08:00", to: "2026-09-01T00:00:00+08:00" },
  dueAt: "2026-09-20T00:00:00+08:00", status: "unpaid", paymentMethod: null, paymentStatus: null, paidAt: null, ...over,
});
const statement = (id: string, over: Partial<ApiStatement> = {}): ApiStatement => ({
  id, version: 1, contractorOrgId: "org-p", period: "2026-08", status: "approved", currency: "MYR", grossMinor: 50_000, deductionsMinor: 0, netMinor: 50_000, payDate: "2026-09-15",
  lines: [], queries: [], approvedByMembershipId: "m-1", paidAt: null, ...over,
});
/** The Core API of one page read; `ops` answer coreOp by operation, the lists answer coreAll and coreAllPage. */
function core(ops: Record<string, (input: unknown) => unknown>, lists: Record<string, unknown[] | Error> = {}) {
  const op = new Map(Object.entries(ops));
  const list = new Map<string, unknown[] | Error>(Object.entries({
    "contracts.list": [{ id: "k-1", version: 2, customerId: "cust-a", customerOrgId: "org-a", unitIds: ["u-1"], planType: "rto", startAt: "2026-01-01T00:00:00Z", endAt: "2027-01-01T00:00:00Z", priceMinor: 12_000, currency: "MYR", restrictionEligible: true }],
    "customers.list": [{ id: "cust-a", name: "Demo Customer A", organizationId: "org-a", status: "active" }],
    "properties.list": [{ id: "p-1", name: "Home A", customerOrgId: "org-a", archived: false }, { id: "p-old", name: "Old", customerOrgId: "org-a", archived: true }, { id: "p-z", name: "Stray", customerOrgId: "org-z", archived: false }],
    "inquiries.list": [], "organizations.list": [{ id: "org-p", name: "Partner P", kind: "contractor" }, { id: "org-b", name: "Alpha Air", kind: "contractor" }, { id: "org-a", name: "Demo Customer A", kind: "customer" }],
    "members.list": [{ id: "m-1", displayName: "HQ Finance" }],
    ...lists,
  }));
  dal.coreOp.mockImplementation((name, input) => Promise.try(() => (op.get(name) ?? (() => ({ items: [] })))(input)));
  dal.coreAll.mockImplementation((name) => {
    const v = list.get(name);
    return Error.isError(v) ? Promise.reject(v) : Promise.resolve(v ?? []);
  });
  dal.coreAllPage.mockImplementation((name) => {
    const v = list.get(name);
    return Promise.resolve({ items: Error.isError(v) ? [] : v ?? [], total: 0 });
  });
}
const props = (sp: Record<string, string> = {}) => ({ searchParams: Promise.resolve(sp), params: Promise.resolve({}) });
type Live = NonNullable<ComponentProps<typeof BillingView>["live"]>;
async function liveOf(sp: Record<string, string> = {}): Promise<Live> {
  const page: ReactNode = await AdminBillingPage(props(sp));
  if (!isValidElement<ComponentProps<typeof BillingView>>(page) || page.type !== BillingView || !page.props.live) throw new TypeError("not the live BillingView");
  return page.props.live;
}
const callsOf = (op: string) => dal.coreOp.mock.calls.filter(([name]) => name === op).map(([, input]) => input);
const detail = (inv: ApiInvoice) => ({ ...inv, paymentRefs: [], restrictionIds: [] });

describe("the billing page (Server Component)", () => {
  it("shows the demo outside API mode, and its loading and error states", async () => {
    dal.apiMode.mockReturnValue(false);
    const page: ReactNode = await AdminBillingPage(props());
    expect(isValidElement<ComponentProps<typeof BillingView>>(page) && page.type === BillingView && page.props.live).toBeUndefined();
    expect(BillingLoading().type).toBe(RouteLoading);
    const error = new Error("down"), retry = vi.fn();
    const failed = BillingErrorState({ error, retry });
    expect(failed.type === RouteError && failed.props).toMatchObject({ error, retry });
  });

  it("reads the invoices of the URL scope and months, and opens the first overdue one with its reminder recipients", async () => {
    core({
      "invoices.list": () => ({ items: [invoice("inv-1"), invoice("inv-due", { dueAt: "2026-09-10T00:00:00+08:00" })] }),
      "invoices.get": (q) => detail(invoice(JSON.stringify(q).includes("inv-due") ? "inv-due" : "inv-1", { dueAt: "2026-09-10T00:00:00+08:00" })),
      "notifications.recipients": () => ({ items: [{ id: "m-c", role: "client", displayLabel: "Ana", allowedChannels: ["email"] }] }),
    });
    const live = await liveOf({ customerId: "cust-a", propertyId: "p-1", contractId: "k-1", overdueOnly: "true", from: "2026-07", to: "2026-08" });
    expect(callsOf("invoices.list")).toStrictEqual([{ limit: 100, filters: { customerId: "cust-a", propertyId: "p-1", contractId: "k-1", overdueOnly: true, from: "2026-06-30T16:00:00.000Z", to: "2026-08-31T16:00:00.000Z" } }]);
    expect([live.tab, live.scope]).toStrictEqual(["invoices", { customerId: "cust-a", propertyId: "p-1", contractId: "k-1", overdueOnly: true, from: "2026-07", to: "2026-08" }]);
    expect(callsOf("invoices.get")).toStrictEqual([{ id: "inv-due" }]);
    expect(live.recipients).toStrictEqual([{ id: "m-c", role: "client", displayLabel: "Ana", allowedChannels: ["email"] }]);
    expect(live.properties).toStrictEqual([{ id: "p-1", name: "Home A", customerId: "cust-a" }, { id: "p-z", name: "Stray", customerId: "" }]); // archived left out
    expect([live.canOpenRestrictions, live.contractsOfCustomer]).toStrictEqual([true, [{ id: "k-1", customerId: "cust-a" }]]);
  });

  it("opens the URL's invoice without recipients when it is not overdue, and none when the list is empty", async () => {
    core({ "invoices.list": () => ({ items: [invoice("inv-1"), invoice("inv-2")] }), "invoices.get": () => detail(invoice("inv-2")) });
    const live = await liveOf({ invoiceId: "inv-2", from: "2026-09", to: "2026-07" }); // a reversed range reads every month
    expect(callsOf("invoices.list")[0]).toStrictEqual({ limit: 100, filters: {} });
    expect([callsOf("invoices.get"), live.recipients, live.selectedInvoice?.id]).toStrictEqual([[{ id: "inv-2" }], undefined, "inv-2"]);
    dal.coreOp.mockClear();
    core({ "invoices.list": () => ({ items: [] }) });
    const empty = await liveOf();
    expect([empty.selectedInvoice, callsOf("invoices.get")]).toStrictEqual([undefined, []]);
  });

  it("opens the inquiries tab for a link that names only an inquiry, or when asked", async () => {
    core({ "invoices.list": () => ({ items: [invoice("inv-1")] }), "invoices.get": () => detail(invoice("inv-1")), "inquiries.list": () => ({ items: [] }) });
    const linked = await liveOf({ inquiryId: "iq-1" });
    expect(linked.tab).toBe("inquiries");
    const asked = await liveOf({ tab: "inquiries" });
    expect(asked.tab).toBe("inquiries");
    const both = await liveOf({ inquiryId: "iq-1", invoiceId: "inv-1" });
    expect([both.tab, both.selectedInquiryId]).toStrictEqual(["invoices", "iq-1"]);
    expect(callsOf("invoices.get")).toStrictEqual([{ id: "inv-1" }]); // only for the invoices tab
  });

  it("lists the payouts of the URL filters, contractors by name, and the selected statement with its approver", async () => {
    core({ "invoices.list": () => ({ items: [] }), "payouts.get": () => statement("st-2") }, { "payouts.list": [statement("st-1"), statement("st-2", { status: "draft", approvedByMembershipId: null })] });
    const live = await liveOf({ tab: "payouts", period: "2026-08", contractorOrgId: "org-p", status: "approved", statementId: "st-2" });
    expect(dal.coreAll.mock.calls.find(([op]) => op === "payouts.list")?.[1]).toStrictEqual({ filters: { period: "2026-08", contractorOrgId: "org-p", status: "approved" } });
    expect(live.payouts).toStrictEqual({ period: "2026-08", contractorOrgId: "org-p", status: "approved", lastClosed: "2026-08", contractors: [{ id: "org-b", name: "Alpha Air" }, { id: "org-p", name: "Partner P" }] });
    expect([live.statements?.map((s) => s.id), live.statementMissing, live.statement?.id]).toStrictEqual([["st-1", "st-2"], false, "st-2"]);
    expect(callsOf("invoices.get")).toStrictEqual([]);
  });

  it("ignores a malformed period or status, flags a statement that is not listed, and reads no approver names it may not", async () => {
    core({ "invoices.list": () => ({ items: [] }), "payouts.get": () => statement("st-1") }, { "payouts.list": [statement("st-1")], "members.list": refused("FORBIDDEN") });
    const live = await liveOf({ tab: "payouts", period: "2026-13", status: "lost", statementId: "st-gone" });
    expect(dal.coreAll.mock.calls.find(([op]) => op === "payouts.list")?.[1]).toStrictEqual({ filters: {} });
    expect([live.payouts?.period, live.payouts?.status, live.statementMissing, live.statement]).toStrictEqual([undefined, undefined, true, undefined]);
    core({ "invoices.list": () => ({ items: [] }) }, { "payouts.list": [] });
    const none = await liveOf({ tab: "payouts" });
    expect([none.statements, none.statementMissing, none.statement]).toStrictEqual([[], false, undefined]);
  });

  it("names billing.read when the invoices are refused (IR582)", async () => {
    core({ "invoices.list": () => Promise.reject(refused("FORBIDDEN")) });
    dal.corePermissions.mockResolvedValueOnce(new Set(["billing.read", "asset.read"])).mockResolvedValueOnce(new Set());
    const page: ReactNode = await AdminBillingPage(props());
    expect(isValidElement(page) && page.type === NeedsAccess && page.props).toStrictEqual({ what: "Billing", permission: "billing.read" });
  });
});
