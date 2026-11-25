// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import type { ComponentProps } from "react";
import {
  contractOptions, inquiryRows, invoiceDetail, invoiceRows, statementDetail, statementRows,
  type ApiContract, type ApiCustomer, type ApiInquiry, type ApiInvoice, type ApiOrganization, type ApiPayment, type ApiStatement,
} from "@ac/web/lib/billing";
import type { ActionFailure } from "@ac/web/lib/actionMessage";

// The HQ billing screen (FR-A08, FR-A23): the invoice scope and selection in the URL, manual and card payments at the
// invoice version, reminders previewed before they are sent, new invoices, the customers' inquiries and their replies,
// and the contractor payouts (generate, approve, mark paid, answer a query). The Phase 1A demo moves locally.
const actions = vi.hoisted(() => {
  type Action = (...args: unknown[]) => Promise<unknown>;
  return {
    answerInquiry: vi.fn<Action>(), confirmCardPayment: vi.fn<Action>(), createInvoice: vi.fn<Action>(), generatePayouts: vi.fn<Action>(), previewReminder: vi.fn<Action>(),
    recordManualPayment: vi.fn<Action>(), resolvePayoutQuery: vi.fn<Action>(), sendReminder: vi.fn<Action>(), transitionStatement: vi.fn<Action>(),
  };
});
const nav = vi.hoisted(() => ({ replace: vi.fn<(href: string, options?: unknown) => void>(), toasts: [] as string[] }));
vi.mock("../app/admin/billing/actions", () => actions);
vi.mock("next/navigation", async (importOriginal) => ({ ...(await importOriginal<Record<string, unknown>>()), useRouter: () => ({ replace: nav.replace, refresh: vi.fn() }), usePathname: () => "/admin/billing" }));
vi.mock("@ac/web/lib/useRefreshThen", () => ({ useRefreshThen: () => (then: () => void) => { then(); } }));
vi.mock("@ac/web/components/ui", async (importOriginal) => ({ ...(await importOriginal<Record<string, unknown>>()), useToast: () => (m: string) => { nav.toasts.push(m); } }));
vi.mock("@ac/web/components/Features", () => ({ ContractorPayouts: () => <div>demo payouts</div> }));

import { BillingView } from "../app/admin/billing/_components/billing-view";
import { I18nProvider } from "@ac/web/components/I18n";

type Live = NonNullable<ComponentProps<typeof BillingView>["live"]>;
const NOW = new Date("2026-09-14T01:00:00Z");
const customers: ApiCustomer[] = [{ id: "cust-a", name: "Demo Customer A", organizationId: "org-a", status: "active" }, { id: "cust-b", name: "Beta Hotel", organizationId: "org-b", status: "active" }];
const contracts: ApiContract[] = [{ id: "k-1", version: 2, customerId: "cust-a", customerOrgId: "org-a", unitIds: ["u-1"], planType: "rto", startAt: "2026-01-01T00:00:00Z", endAt: "2027-01-01T00:00:00Z", priceMinor: 12_000, currency: "MYR", restrictionEligible: true }];
const invoice = (id: string, over: Partial<ApiInvoice> = {}): ApiInvoice => ({
  id, version: 3, number: `INV-${id}`, amountMinor: 12_000, currency: "MYR", contractId: "k-1", contractVersion: 2, period: { from: "2026-08-01T00:00:00+08:00", to: "2026-09-01T00:00:00+08:00" },
  dueAt: "2026-09-10T00:00:00+08:00", status: "unpaid", paymentMethod: null, paymentStatus: null, paidAt: null, ...over,
});
const payment = (over: Partial<ApiPayment> = {}): ApiPayment => ({ id: "pay-1", version: 2, amountMinor: 12_000, currency: "MYR", invoiceId: "inv-p", method: "credit_card", channel: "demo", status: "processing", paymentReference: null, confirmedAt: null, confirmationReason: null, ...over });
const inquiry = (id: string, over: Partial<ApiInquiry> = {}): ApiInquiry => ({ id, version: 1, customerId: "cust-a", invoiceId: "inv-o", restrictionId: "11112222-3333-4444-5555-666677778888", subjectType: "payment", message: "Why is cooling limited?", state: "received", reply: null, createdAt: "2026-09-13T01:00:00Z", ...over });
const orgs: ApiOrganization[] = [{ id: "org-p", name: "Partner P", kind: "contractor" }];
const statement = (id: string, over: Partial<ApiStatement> = {}): ApiStatement => ({
  id, version: 2, contractorOrgId: "org-p", period: "2026-08", status: "draft", currency: "MYR", grossMinor: 50_000, deductionsMinor: 1000, netMinor: 49_000, payDate: "2026-09-15",
  lines: [{ id: "l-1", jobId: "job-11112222", workType: "preventive", acceptedAt: "2026-08-20T00:00:00Z", amountMinor: 50_000, kind: "charge", note: null }],
  queries: [], approvedByMembershipId: null, paidAt: null, ...over,
});
const invoices = [invoice("inv-u", { dueAt: "2026-09-30T00:00:00+08:00" }), invoice("inv-o"), invoice("inv-p", { status: "processing" }), invoice("inv-paid", { status: "paid", paidAt: "2026-09-01T00:00:00Z" })];
/** A live screen; `selected.inv` is the opened invoice (the overdue one by default), null for none loaded. */
function liveOf(over: Partial<Live> = {}, selected: { inv?: ApiInvoice | null; payments?: ApiPayment[]; restrictionIds?: string[] } = {}): Live {
  const inv = selected.inv === null ? null : selected.inv ?? invoices[1];
  return {
    tab: "invoices", canOpenRestrictions: true, scope: { overdueOnly: false }, months: null, rows: invoiceRows(invoices, contracts, customers, NOW),
    contracts: contractOptions(contracts, customers), inquiries: inquiryRows([inquiry("iq-1"), inquiry("iq-2", { state: "answered", reply: "Paid now", invoiceId: null, restrictionId: null })], customers),
    customers: [{ id: "cust-a", name: "Demo Customer A" }, { id: "cust-b", name: "Beta Hotel" }], properties: [{ id: "p-1", name: "Home A", customerId: "cust-a" }, { id: "p-2", name: "Beach", customerId: "cust-b" }],
    contractsOfCustomer: [{ id: "k-1", customerId: "cust-a" }, { id: "k-x", customerId: "cust-b" }],
    ...(inv && { selectedInvoice: invoiceDetail({ ...inv, paymentRefs: selected.payments ?? [], restrictionIds: selected.restrictionIds ?? ["11112222-3333-4444-5555-666677778888"] }) }),
    recipients: [{ id: "m-a", role: "client", displayLabel: "Ana", allowedChannels: ["email", "whatsapp"] }, { id: "m-b", role: "admin", displayLabel: "Ben", allowedChannels: ["inApp"] }],
    ...over,
  };
}
const button = (name: string | RegExp) => screen.getByRole<HTMLButtonElement>("button", { name });
const flush = () => act(() => new Promise<void>((r) => { setTimeout(r, 0); }));
const refusal = (messageKey = "error.conflict") => ({ messageKey, ok: false, code: "CONFLICT", fieldErrors: {} }) satisfies { ok: false } & ActionFailure;
const lastHref = () => nav.replace.mock.lastCall?.[0];

beforeEach(() => {
  nav.replace.mockReset(); nav.toasts = [];
  for (const fn of Object.values(actions)) { fn.mockReset(); fn.mockResolvedValue({ ok: true, value: null }); }
  history.replaceState(null, "", "/admin/billing");
});
afterEach(cleanup);

describe("invoices in API mode", () => {
  it("puts the scope, months and selection in the URL and filters the list by status", () => {
    render(<BillingView live={liveOf({ scope: { customerId: "cust-a", overdueOnly: true, from: "2026-07", to: "2026-08" }, months: "Jul – Aug 2026" })} />);
    expect([...screen.getByLabelText<HTMLSelectElement>("Property").options].map((o) => o.textContent)).toStrictEqual(["All properties", "Home A"]); // the customer's
    expect([...screen.getByLabelText<HTMLSelectElement>("Contract").options].map((o) => o.textContent)).toStrictEqual(["All contracts", "Demo Customer A · RTO · 120.00 MYR"]);
    fireEvent.change(screen.getByLabelText("Customer"), { target: { value: "cust-b" } });
    expect(lastHref()).toBe("/admin/billing?customerId=cust-b");
    fireEvent.change(screen.getByLabelText("Property"), { target: { value: "p-1" } });
    expect(lastHref()).toBe("/admin/billing?propertyId=p-1");
    fireEvent.change(screen.getByLabelText("Contract"), { target: { value: "k-1" } });
    expect(lastHref()).toBe("/admin/billing?contractId=k-1");
    fireEvent.click(button("Overdue only — show all invoices"));
    expect(nav.replace).toHaveBeenLastCalledWith("/admin/billing", { scroll: false });
    fireEvent.change(screen.getByLabelText(/^Billing months from/), { target: { value: "2026-09" } }); // after the last: not searched
    expect(screen.getByText("The first month must not be after the last")).toBeDefined();
    fireEvent.change(screen.getByLabelText(/^Billing months from/), { target: { value: "2026-06" } });
    expect(lastHref()).toBe("/admin/billing?from=2026-06&to=2026-08");
    fireEvent.click(button(/All months/));
    expect(lastHref()).toBe("/admin/billing");
    expect(screen.getByText("Jul – Aug 2026")).toBeDefined();
    fireEvent.click(screen.getByRole("tab", { name: /^Paid/ }));
    expect(screen.getAllByText("INV-inv-paid").length).toBeGreaterThan(0);
    fireEvent.click(screen.getByRole("tab", { name: /^Unpaid/ }));
    fireEvent.click(button(/INV-inv-u/));
    expect(lastHref()).toBe("/admin/billing?invoiceId=inv-u");
    fireEvent.click(screen.getByRole("tab", { name: /^Inquiries/ }));
    expect(lastHref()).toBe("/admin/billing?tab=inquiries");
  });

  it("records a manual payment of the full amount with a reference and reason, and says when the invoice changed first", async () => {
    actions.recordManualPayment.mockResolvedValueOnce(refusal("error.versionConflict")).mockResolvedValueOnce({ ok: true, value: null });
    render(<BillingView live={liveOf()} />);
    expect(screen.getByText("The customer has not started a payment. Record a manual payment if it was received outside the app (e.g. bank transfer).")).toBeDefined();
    fireEvent.click(button("Record manual payment…"));
    const dialog = within(screen.getByRole("dialog"));
    fireEvent.change(dialog.getByLabelText(/^Amount/), { target: { value: "100" } });
    fireEvent.click(dialog.getByRole("button", { name: "Record payment" }));
    for (const text of ["Must equal 120.00 MYR", "Reference is required", "A reason is required"]) expect(dialog.getByText(text)).toBeDefined();
    fireEvent.change(dialog.getByLabelText(/^Amount/), { target: { value: "120.00" } });
    fireEvent.change(dialog.getByLabelText(/^Reference/), { target: { value: " BANK-7 " } });
    fireEvent.change(dialog.getByLabelText(/^Reason/), { target: { value: " Transfer received " } });
    fireEvent.click(dialog.getByRole("button", { name: "Record payment" }));
    await flush();
    expect(screen.getByText(/The invoice changed first/)).toBeDefined();
    fireEvent.click(within(screen.getByRole("dialog")).getByRole("button", { name: "Record payment" }));
    await flush();
    expect(actions.recordManualPayment).toHaveBeenLastCalledWith("inv-o", 3, "BANK-7", 12_000, "MYR", "Transfer received");
    expect(nav.toasts.at(-1)).toBe("Manual payment recorded");
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("confirms a processing card payment, and waits while a card payment is under way", async () => {
    const processing = invoices[2];
    const { rerender } = render(<BillingView live={liveOf({}, { inv: processing, payments: [payment()] })} />);
    fireEvent.click(button("Confirm card payment"));
    const dialog = within(screen.getByRole("dialog"));
    expect(dialog.getByLabelText<HTMLInputElement>(/^Amount/).disabled).toBe(true);
    fireEvent.change(dialog.getByLabelText(/^Reference/), { target: { value: "ch_1" } });
    fireEvent.click(dialog.getByRole("button", { name: "Confirm payment" }));
    expect(actions.confirmCardPayment).not.toHaveBeenCalled(); // a reason is required too
    fireEvent.change(dialog.getByLabelText(/^Reason/), { target: { value: "Settled" } });
    fireEvent.click(dialog.getByRole("button", { name: "Confirm payment" }));
    await flush();
    expect(actions.confirmCardPayment).toHaveBeenCalledWith("pay-1", 2, "ch_1", 12_000, "MYR", "Settled");
    rerender(<BillingView live={liveOf({}, { inv: processing, payments: [payment({ channel: "stripe_checkout" })] })} />);
    expect(button("Confirm card payment").disabled).toBe(true); // a Stripe payment is confirmed by its webhook
    rerender(<BillingView live={liveOf({}, { inv: invoices[0], payments: [payment({ status: "initiated", invoiceId: "inv-u" })] })} />);
    fireEvent.click(button(/INV-inv-u/));
    expect(screen.getByText(/Manual recording waits while a payment is started/)).toBeDefined();
    rerender(<BillingView live={liveOf({}, { inv: invoices[0], payments: [payment({ status: "failed", invoiceId: "inv-u" })] })} />);
    expect(screen.getByText(/The customer's card payment did not go through/)).toBeDefined();
  });

  it("previews a reminder for a recipient and channel before sending it, and says why it cannot be sent yet", async () => {
    actions.previewReminder.mockResolvedValue({ ok: true, value: "INV-inv-o" });
    render(<BillingView live={liveOf()} />);
    expect(screen.getByText("Enter a reason to preview and send the reminder.")).toBeDefined();
    expect(button("Send reminder").disabled).toBe(true);
    fireEvent.change(screen.getByLabelText(/^Reason · e\.g\./), { target: { value: "Second notice" } });
    expect(screen.getByText("Preview the message first: Send uses the previewed text.")).toBeDefined();
    fireEvent.change(screen.getByLabelText("Channel"), { target: { value: "whatsapp" } });
    fireEvent.click(button("Preview"));
    await flush();
    expect(actions.previewReminder).toHaveBeenCalledWith("inv-o", "m-a", "whatsapp", "Second notice");
    expect(screen.getByText(/^Preview · WhatsApp/)).toBeDefined();
    fireEvent.click(button("Send reminder"));
    await flush();
    expect(actions.sendReminder).toHaveBeenCalledWith("inv-o", 3, "m-a", "whatsapp", "Second notice");
    expect(nav.toasts.at(-1)).toBe("Reminder recorded (delivery simulated)");
    fireEvent.change(screen.getByLabelText("Recipient"), { target: { value: "m-b" } });
    expect([...screen.getByLabelText<HTMLSelectElement>("Channel").options].map((o) => o.textContent)).toStrictEqual(["In-app"]);
  });

  it("says when no client can be reminded, names the related restrictions, and the inquiries about the invoice", () => {
    const { rerender } = render(<BillingView live={liveOf({ recipients: [] })} />);
    expect(screen.getByText("No client of this customer can receive the reminder.")).toBeDefined();
    expect(screen.getByRole("link", { name: /Open restriction/ }).getAttribute("href")).toBe("/admin/restrictions?restrictionId=11112222-3333-4444-5555-666677778888");
    fireEvent.click(button(/Open in Inquiries/));
    expect(lastHref()).toBe("/admin/billing?tab=inquiries&inquiryId=iq-1");
    rerender(<BillingView live={liveOf({ canOpenRestrictions: false }, { restrictionIds: ["r-1", "r-2"] })} />);
    expect(screen.getByText("Related restrictions")).toBeDefined();
    expect(screen.getAllByText("Your role can’t open Restrictions")).toHaveLength(2);
    rerender(<BillingView live={liveOf({}, { inv: invoices[3], restrictionIds: [] })} />);
    fireEvent.click(button(/INV-inv-paid/));
    expect(screen.getByText("No restriction cites this invoice.")).toBeDefined();
  });

  it("creates an invoice for a contract version with a valid period and due date", async () => {
    render(<BillingView live={liveOf()} />);
    fireEvent.click(button(/Create invoice/));
    const dialog = within(screen.getByRole("dialog"));
    expect(dialog.getByText("Amount from the contract version: 120.00 MYR")).toBeDefined();
    fireEvent.click(dialog.getByRole("button", { name: "Create" }));
    expect(dialog.getAllByText("Required")).toHaveLength(2);
    fireEvent.change(dialog.getByLabelText(/^Billing period start/), { target: { value: "2026-09-01" } });
    fireEvent.change(dialog.getByLabelText(/^Billing period end/), { target: { value: "2026-08-01" } });
    fireEvent.change(dialog.getByLabelText(/^Due date/), { target: { value: "2026-10-10" } });
    fireEvent.click(dialog.getByRole("button", { name: "Create" }));
    expect(dialog.getByText("After the start")).toBeDefined();
    fireEvent.change(dialog.getByLabelText(/^Billing period end/), { target: { value: "2026-10-01" } });
    fireEvent.change(dialog.getByLabelText("Contract"), { target: { value: "k-1" } });
    fireEvent.click(dialog.getByRole("button", { name: "Create" }));
    await flush();
    expect(actions.createInvoice).toHaveBeenCalledWith({ contractId: "k-1", contractVersion: 2, from: "2026-09-01", to: "2026-10-01", dueAt: "2026-10-10", amountMinor: 12_000, currency: "MYR" });
    expect(nav.toasts).toStrictEqual(["Invoice created"]);
  });

  it("says why no invoice is shown", () => {
    const { rerender } = render(<BillingView live={liveOf({ rows: [], scope: { overdueOnly: true } }, { inv: null })} />);
    expect(screen.getByText("No invoice matches this scope.")).toBeDefined();
    rerender(<BillingView live={liveOf({ rows: [] }, { inv: null })} />);
    expect(screen.getByText("No invoice has been issued yet.")).toBeDefined();
  });
});

describe("inquiries", () => {
  it("filters by state, opens an inquiry's invoice, and sends a reply in-app", async () => {
    actions.answerInquiry.mockResolvedValue({ ok: true, value: null });
    render(<BillingView live={liveOf({ tab: "inquiries", selectedInquiryId: "iq-1" })} />);
    fireEvent.click(button("Send reply"));
    expect(screen.getByText("A reply is required")).toBeDefined();
    fireEvent.change(screen.getByLabelText(/^Reply \(in-app only\)/), { target: { value: " We received it " } });
    fireEvent.click(button("Send reply"));
    await flush();
    expect(actions.answerInquiry).toHaveBeenCalledWith("iq-1", 1, "We received it");
    expect(screen.getByRole("link", { name: "11112222" }).getAttribute("href")).toBe("/admin/restrictions?restrictionId=11112222-3333-4444-5555-666677778888");
    fireEvent.click(button(/INV-inv-o/));
    expect(lastHref()).toBe("/admin/billing?invoiceId=inv-o");
    fireEvent.click(screen.getByRole("tab", { name: /^Answered/ }));
    fireEvent.click(button(/Why is cooling limited\?.*Answered|Answered/));
    expect(lastHref()).toBe("/admin/billing?tab=inquiries&inquiryId=iq-2");
  });

  it("says when there are none, or none in a state", () => {
    const { rerender } = render(<BillingView live={liveOf({ tab: "inquiries", inquiries: [] })} />);
    expect(screen.getByText("No inquiries")).toBeDefined();
    rerender(<BillingView live={liveOf({ tab: "inquiries", inquiries: inquiryRows([inquiry("iq-1")], customers) })} />);
    fireEvent.click(screen.getByRole("tab", { name: /^Answered/ }));
    expect(screen.getByText("No answered inquiries")).toBeDefined();
  });
});

const payoutsLive = (sts: ApiStatement[], selected: ApiStatement | null, over: Partial<Live> = {}) => liveOf({
  tab: "payouts", payouts: { lastClosed: "2026-08", contractors: [{ id: "org-p", name: "Partner P" }] }, statements: statementRows(sts, orgs),
  ...(selected && { statement: statementDetail(selected, orgs, NOW, undefined, new Map([["m-1", "HQ Finance"]])) }), ...over,
});

describe("contractor payouts", () => {
  it("filters by period, contractor and status in the URL, and generates the drafts of a month that has ended", async () => {
    actions.generatePayouts.mockResolvedValue({ ok: true, value: 2 });
    render(<BillingView live={payoutsLive([statement("st-1")], statement("st-1"), { payouts: { period: "2026-08", status: "draft", lastClosed: "2026-08", contractors: [{ id: "org-p", name: "Partner P" }] } })} />);
    fireEvent.change(screen.getByLabelText("Contractor"), { target: { value: "org-p" } });
    expect(lastHref()).toBe("/admin/billing?contractorOrgId=org-p");
    fireEvent.change(screen.getByLabelText("Status"), { target: { value: "paid" } });
    expect(lastHref()).toBe("/admin/billing?status=paid");
    fireEvent.change(screen.getByLabelText("Period"), { target: { value: "" } });
    expect(lastHref()).toBe("/admin/billing");
    fireEvent.click(button("Clear the filters"));
    fireEvent.click(button("Generate drafts"));
    const dialog = within(screen.getByRole("dialog"));
    fireEvent.change(dialog.getByLabelText(/^Month/), { target: { value: "2026-09" } });
    expect(dialog.getByText("Only a month that has ended (Kuala Lumpur) can be generated")).toBeDefined();
    fireEvent.change(dialog.getByLabelText(/^Month/), { target: { value: "2026-07" } });
    fireEvent.click(dialog.getByRole("button", { name: "Generate drafts" }));
    await flush();
    expect(actions.generatePayouts).toHaveBeenCalledWith("2026-07");
    expect(nav.toasts).toStrictEqual(["2 draft statements generated for 2026-07"]);
    expect(lastHref()).toBe("/admin/billing?period=2026-07");
  });

  it("approves a draft, marks an approved statement paid only from its pay date, and answers a query", async () => {
    const { rerender } = render(<BillingView live={payoutsLive([statement("st-1")], statement("st-1"))} />);
    fireEvent.click(button("Approve"));
    await flush();
    expect(actions.transitionStatement).toHaveBeenLastCalledWith("st-1", 2, "approve");
    const approved = statement("st-1", { status: "approved", approvedByMembershipId: "m-1", payDate: "2026-09-20", queries: [{ id: "q-1", lineId: "l-1", topic: "amount", message: "Rate is wrong", state: "open", reply: null, adjustmentMinor: null }, { id: "q-2", lineId: "l-1", topic: "other", message: "Thanks", state: "adjusted", reply: "Fixed", adjustmentMinor: 500 }] });
    rerender(<BillingView live={payoutsLive([approved], approved)} />);
    expect(button("Mark paid").disabled).toBe(true);
    expect(screen.getByText("HQ Finance")).toBeDefined();
    fireEvent.change(screen.getByLabelText("Reply to the contractor"), { target: { value: " Corrected " } });
    fireEvent.change(screen.getByLabelText("Adjustment"), { target: { value: "-5" } });
    fireEvent.click(button("Answer"));
    await flush();
    expect(actions.resolvePayoutQuery).toHaveBeenCalledWith("st-1", 2, "q-1", "Corrected", -500);
    const payable = statement("st-1", { status: "approved", payDate: "2026-09-14" });
    rerender(<BillingView live={payoutsLive([payable], payable)} />);
    fireEvent.click(button("Mark paid"));
    await flush();
    expect(actions.transitionStatement).toHaveBeenLastCalledWith("st-1", 2, "mark_paid");
    fireEvent.click(screen.getByText("Partner P", { selector: "b" })); // the statements table's row
    expect(lastHref()).toBe("/admin/billing?statementId=st-1");
  });

  it("says when no statement matches, or the URL's one is hidden", () => {
    const { rerender } = render(<BillingView live={payoutsLive([], null)} />);
    expect(screen.getByText("Generate the drafts of a month that has ended.")).toBeDefined();
    rerender(<BillingView live={payoutsLive([], null, { statementMissing: true, payouts: { status: "paid", lastClosed: "2026-08", contractors: [] } })} />);
    expect(screen.getByText("No statement matches these filters.")).toBeDefined();
    expect(screen.getByText("That statement is not in this list")).toBeDefined();
  });
});

describe("the billing demo", () => {
  it("records a manual payment, confirms a card payment, reminds, replies and creates locally", () => {
    render(<BillingView />);
    fireEvent.click(button("Send reminder"));
    expect(button("Send reminder").disabled).toBe(true); // a reason first
    fireEvent.change(screen.getByLabelText(/^Reason · e\.g\./), { target: { value: "Second notice" } });
    fireEvent.click(button("Send reminder"));
    expect(nav.toasts).toStrictEqual(["Reminder queued (simulated)"]);
    fireEvent.click(button("Record manual payment…"));
    const dialog = within(screen.getByRole("dialog"));
    fireEvent.change(dialog.getByLabelText(/^Reference/), { target: { value: "manual-1" } });
    fireEvent.change(dialog.getByLabelText(/^Paid on/), { target: { value: "2026-09-13" } });
    fireEvent.click(dialog.getByRole("button", { name: "Record payment" }));
    expect(nav.toasts.at(-1)).toBe("Manual payment recorded");
    expect(screen.getByText(/The invoice changed first/)).toBeDefined(); // the demo's reminder raced the payment
    fireEvent.click(button(/invoice-overdue-b/));
    fireEvent.click(button("Confirm card payment"));
    expect(nav.toasts.at(-1)).toBe("Card payment confirmed");
    fireEvent.click(button(/Create invoice/));
    const create = within(screen.getByRole("dialog"));
    fireEvent.change(create.getByLabelText(/^Billing period start/), { target: { value: "2026-10-01" } });
    fireEvent.change(create.getByLabelText(/^Billing period end/), { target: { value: "2026-11-01" } });
    fireEvent.change(create.getByLabelText(/^Due date/), { target: { value: "2026-11-10" } });
    fireEvent.click(create.getByRole("button", { name: "Create" }));
    expect(nav.toasts.at(-1)).toBe("Invoice created");
    fireEvent.click(screen.getByRole("tab", { name: /^Inquiries/ }));
    fireEvent.change(screen.getByLabelText(/^Reply \(in-app only\)/), { target: { value: "Received" } });
    fireEvent.click(button("Send reply"));
    expect(nav.toasts.at(-1)).toBe("Reply recorded in-app");
    fireEvent.click(button(/invoice-overdue-a/));
    expect(screen.getAllByText("invoice-overdue-a").length).toBeGreaterThan(0);
    fireEvent.click(screen.getByRole("tab", { name: /^Contractor payouts/ }));
    expect(screen.getByText("demo payouts")).toBeDefined();
    expect(nav.replace).not.toHaveBeenCalled();
  });
});

describe("billing edge cases", () => {
  it("clears each scope filter to its all-option, and follows new months from the URL", () => {
    const { rerender } = render(<BillingView live={liveOf({ scope: { customerId: "cust-a", propertyId: "p-1", contractId: "k-1", overdueOnly: false, from: "2026-07", to: "2026-08" } })} />);
    fireEvent.change(screen.getByLabelText("Customer"), { target: { value: "" } });
    expect(lastHref()).toBe("/admin/billing");
    fireEvent.change(screen.getByLabelText("Property"), { target: { value: "" } });
    fireEvent.change(screen.getByLabelText("Contract"), { target: { value: "" } });
    expect(lastHref()).toBe("/admin/billing");
    fireEvent.change(screen.getByLabelText(/^Billing months to/), { target: { value: "2026-09" } });
    expect(lastHref()).toBe("/admin/billing?from=2026-07&to=2026-09");
    rerender(<BillingView live={liveOf({ scope: { overdueOnly: false, from: "2026-01" } })} />);
    expect(screen.getByLabelText<HTMLInputElement>(/^Billing months from/).value).toBe("2026-01"); // the fields restart from the URL
    expect(screen.getByLabelText<HTMLInputElement>(/^Billing months to/).value).toBe("");
    rerender(<BillingView live={liveOf({ scope: { overdueOnly: false, to: "2026-03" } })} />);
    expect(screen.getByLabelText<HTMLInputElement>(/^Billing months from/).value).toBe("");
  });

  it("names Kuala Lumpur dates in another time zone, an unknown role, and an empty status tab", () => {
    render(<I18nProvider display={{ locale: "en", timeZone: "Asia/Tokyo" }}><BillingView live={liveOf({ rows: invoiceRows([invoice("inv-o"), invoice("inv-paid", { status: "paid", paidAt: "2026-09-01T00:00:00Z" })], contracts, customers, NOW), recipients: [{ id: "m-x", role: "auditor", displayLabel: "Xu", allowedChannels: ["email"] }] })} /></I18nProvider>);
    expect(screen.getByText(/Billing periods and due dates are Kuala Lumpur dates/)).toBeDefined();
    expect(screen.getByRole("option", { name: "Xu · auditor" })).toBeDefined();
    fireEvent.click(screen.getByRole("tab", { name: /^Processing/ }));
    expect(screen.getByText("No processing invoices")).toBeDefined();
  });

  it("highlights the inquiry from the link, labels an invoice the list does not hold, and goes back to the invoices tab", () => {
    const iqs = inquiryRows([inquiry("iq-1"), inquiry("iq-gone", { invoiceId: "inv-gone" })], customers);
    const { rerender } = render(<BillingView live={liveOf({ selectedInquiryId: "iq-1", inquiries: iqs })} />);
    const linkedRow = screen.getAllByText(/Why is cooling limited\?/).find((el) => el.tagName === "P" && el.parentElement?.className.includes("rounded-xl"));
    expect(linkedRow?.parentElement?.className).toContain("bg-primary-soft"); // the inquiry the link named
    rerender(<BillingView live={liveOf({ tab: "inquiries", selectedInquiryId: "iq-gone", inquiries: iqs })} />);
    expect(screen.getByRole("button", { name: /inv-gone/ })).toBeDefined(); // by its ID
    fireEvent.click(screen.getByRole("tab", { name: /^Invoices/ }));
    expect(lastHref()).toBe("/admin/billing");
    rerender(<BillingView live={liveOf({ tab: "inquiries", selectedInquiryId: "iq-2" })} />);
    expect(screen.getByText("Paid now")).toBeDefined(); // the reply, without the form
    expect(screen.queryByRole("button", { name: "Send reply" })).toBeNull();
    fireEvent.click(screen.getByRole("tab", { name: /^Received/ }));
    expect(screen.getAllByText(/Why is cooling limited\?/).length).toBeGreaterThan(0);
    rerender(<BillingView live={liveOf({ tab: "inquiries", inquiries: inquiryRows([inquiry("iq-2", { state: "answered", reply: "Done" })], customers) })} />);
    expect(screen.getByText("No received inquiries")).toBeDefined();
  });

  it("toasts a refusal that is not a version conflict without the conflict note, and opens an invoice whose detail has not loaded", async () => {
    actions.answerInquiry.mockResolvedValue(refusal("error.forbidden"));
    const { rerender } = render(<BillingView live={liveOf({ tab: "inquiries", selectedInquiryId: "iq-1" })} />);
    fireEvent.change(screen.getByLabelText(/^Reply \(in-app only\)/), { target: { value: "Ok" } });
    fireEvent.click(button("Send reply"));
    await flush();
    expect(nav.toasts).toHaveLength(1);
    rerender(<BillingView live={liveOf({ contracts: [] }, { inv: null })} />);
    expect(screen.getByText("No restriction cites this invoice.")).toBeDefined();
    fireEvent.click(button(/Create invoice/));
    expect(within(screen.getByRole("dialog")).queryByText(/Amount from the contract version/)).toBeNull(); // no contract to bill
    fireEvent.click(within(screen.getByRole("dialog")).getByRole("button", { name: "Create" }));
    expect(actions.createInvoice).not.toHaveBeenCalled();
    fireEvent.click(within(screen.getByRole("dialog")).getByRole("button", { name: "Cancel" }));
  });

  it("generates one draft, clears payout filters to all, words lines and answers without an adjustment, and closes the dialog", async () => {
    actions.generatePayouts.mockResolvedValue({ ok: true, value: 1 });
    const two = statement("st-1", {
      lines: [statement("x").lines[0] ?? { id: "l-1", jobId: "job-1", workType: "preventive", acceptedAt: null, amountMinor: 1, kind: "charge", note: null }, { id: "l-2", jobId: "job-2", workType: "repair", acceptedAt: null, amountMinor: 1000, kind: "deduction", note: "late" }],
      queries: [{ id: "q-1", lineId: "l-1", topic: "amount", message: "Why?", state: "open", reply: null, adjustmentMinor: null }, { id: "q-2", lineId: "l-2", topic: "other", message: "Ok", state: "answered", reply: "Because", adjustmentMinor: null }],
    });
    render(<BillingView live={liveOf({ tab: "payouts", payouts: { contractorOrgId: "org-p", status: "draft", lastClosed: "2026-08", contractors: [{ id: "org-p", name: "Partner P" }] }, statements: statementRows([two], orgs), statement: statementDetail(two, orgs, NOW) })} />);
    expect(screen.getByText(/2 jobs/)).toBeDefined();
    expect(screen.getByText("Reply: Because")).toBeDefined();
    fireEvent.change(screen.getByLabelText("Contractor"), { target: { value: "" } });
    fireEvent.change(screen.getByLabelText("Status"), { target: { value: "" } });
    expect(lastHref()).toBe("/admin/billing");
    fireEvent.change(screen.getByLabelText("Reply to the contractor"), { target: { value: "Rate card" } });
    fireEvent.click(button("Answer"));
    await flush();
    expect(actions.resolvePayoutQuery).toHaveBeenCalledWith("st-1", 2, "q-1", "Rate card", null);
    fireEvent.click(button("Generate drafts"));
    fireEvent.change(within(screen.getByRole("dialog")).getByLabelText(/^Month/), { target: { value: "" } });
    expect(within(screen.getByRole("dialog")).getByRole<HTMLButtonElement>("button", { name: "Generate drafts" }).disabled).toBe(true);
    fireEvent.click(within(screen.getByRole("dialog")).getByRole("button", { name: "Cancel" }));
    fireEvent.click(button("Generate drafts"));
    fireEvent.keyDown(screen.getByRole("dialog"), { key: "Escape" });
    fireEvent.click(button("Generate drafts"));
    fireEvent.click(within(screen.getByRole("dialog")).getByRole("button", { name: "Generate drafts" }));
    await flush();
    expect(nav.toasts.at(-1)).toBe("1 draft statement generated for 2026-08");
    cleanup();
    render(<BillingView live={liveOf({ tab: "payouts" })} />);
    expect(screen.getByText("Generate the drafts of a month that has ended.")).toBeDefined();
  });

  it("records a demo payment without a reminder, and opens an inquiry from the demo invoice", () => {
    render(<BillingView />);
    fireEvent.click(button("Record manual payment…"));
    const dialog = within(screen.getByRole("dialog"));
    fireEvent.change(dialog.getByLabelText(/^Reference/), { target: { value: "manual-2" } });
    fireEvent.click(dialog.getByRole("button", { name: "Record payment" }));
    expect(screen.queryByText(/The invoice changed first/)).toBeNull();
    fireEvent.click(button(/invoice-overdue-a/));
    fireEvent.click(button(/Open in Inquiries/));
    expect(screen.getByRole("tab", { name: /^Inquiries/ }).getAttribute("aria-selected")).toBe("true");
  });
});
