import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { isValidElement, type ComponentProps } from "react";
import type { ApiInquiry } from "@ac/web/lib/billing";
import type { ClientContract, ClientPayment } from "@ac/web/lib/clientBilling";
import type { ApiCommand, ApiRestriction } from "@ac/web/lib/restrictions";

// The customer invoice page (FR-C11, FR-C12, SCR-C11): the demo outside API mode, another customer's invoice reading as
// absent, and in API mode what the view gets: the tab, the restriction it shows (the asked one, else the live one, else
// the first), the per-unit commands only on the restriction tab, the reason and release lines, the contract scope, the
// open and last payments, the WhatsApp check (IR578), the Stripe return (DEC-68) and the Kuala Lumpur date note. Its
// Server Actions send the demo PSP events at the right versions, check the Stripe page address (IR840), trim the
// inquiry and re-read the page whatever happens.
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
  return { coreOp: vi.fn(), apiMode: vi.fn(), coreAll: vi.fn(), coreItems: vi.fn(), coreNow: vi.fn(), corePrincipal: vi.fn(), coreDisplay: vi.fn(), CoreError };
});
const cache = vi.hoisted(() => ({ refresh: vi.fn() }));
const views = vi.hoisted(() => ({ InvoiceView: () => null, InvoiceDemo: () => null }));
vi.mock("server-only", () => ({}));
vi.mock("next/cache", () => cache);
vi.mock("next/server", () => ({ connection: () => Promise.resolve() }));
vi.mock("next/navigation", async (importOriginal) => ({
  ...(await importOriginal<Record<string, unknown>>()),
  notFound: () => { throw new Error("notFound"); },
}));
vi.mock("@ac/web/lib/dal", () => dal);
vi.mock("../app/customer/payments/[id]/_components/invoice-demo", () => ({ InvoiceDemo: views.InvoiceDemo }));
vi.mock("../app/customer/payments/[id]/_components/invoice-view", () => ({ InvoiceView: views.InvoiceView }));

import { demoOutcome, paymentInstructions, previewMessage, sendInquiry, startCheckout, startPayment } from "../app/customer/payments/[id]/actions";
import CustomerInvoicePage from "../app/customer/payments/[id]/page";
import { InvoiceView } from "../app/customer/payments/[id]/_components/invoice-view";
import { InvoiceDemo } from "../app/customer/payments/[id]/_components/invoice-demo";
import InvoiceLoading from "../app/customer/payments/[id]/loading";
import InvoiceErrorState from "../app/customer/payments/[id]/error";
import { RouteError, RouteLoading } from "@ac/web/components/RouteStates";

const refusal = (code: string, fieldErrors: Record<string, string> = {}) => new dal.CoreError({ code, fieldErrors, messageKey: `error.${code}`, retryAfterSeconds: null });
const payment = (id: string, over: Partial<ClientPayment> = {}): ClientPayment => ({
  id, version: 1, amountMinor: 12_000, currency: "MYR", invoiceId: "inv-1", method: "demo_credit_card", channel: "demo", status: "failed", paymentReference: null,
  confirmedAt: null, confirmationReason: null, createdAt: "2026-09-20T02:00:00Z", updatedAt: "2026-09-20T02:00:00Z", ...over,
});
const invoice = (over: Record<string, unknown> = {}) => ({
  id: "inv-1", version: 3, number: "INV-0042", amountMinor: 12_000, currency: "MYR", contractId: "contract-1", contractVersion: 1,
  period: { from: "2026-08-31T16:00:00Z", to: "2026-09-30T16:00:00Z" }, dueAt: "2026-09-09T16:00:00Z", status: "unpaid", paymentMethod: null, paymentStatus: null,
  paidAt: null, paymentRefs: [], restrictionIds: [], createdAt: "2026-08-27T00:00:00Z", ...over,
});
const contract = (id: string, unitIds: string[]): ClientContract => ({
  id, unitIds, version: 1, customerId: "cust-a", customerOrgId: "org-a", planType: "rto", startAt: "2026-01-01T00:00:00Z", endAt: "2026-12-31T00:00:00Z",
  priceMinor: 12_000, currency: "MYR", restrictionEligible: true,
});
const restriction = (id: string, state: ApiRestriction["state"], over: Partial<ApiRestriction> = {}): ApiRestriction => ({
  id, version: 1, createdAt: "2026-09-11T00:00:00Z", unitIds: ["unit-1"], rulesVersion: "v1", policy: { kind: "temperature_limit", minimumCoolingSetpoint: 24 }, state,
  perUnit: [{ unitId: "unit-1", applyState: "applied", releaseState: "none", applyCommandIds: ["cmd-0", "cmd-1"], releaseCommandIds: [], observedRestriction: null, observedAt: null, pendingReason: null }],
  recoveryCases: [], noticeAt: "2026-09-11T16:30:00Z", executeAfter: "2026-09-13T00:00:00Z", releaseIntent: null, ...over,
});
const inquiry = (id: string, over: Partial<ApiInquiry> = {}): ApiInquiry => ({
  id, version: 1, customerId: "cust-a", invoiceId: "inv-1", restrictionId: null, subjectType: "payment", message: "When?", state: "received", reply: null, createdAt: "2026-09-12T00:00:00Z", ...over,
});
const reminder = (id: string, targetId: string, kind = "invoice") => ({ id, channel: "email", occurredAt: "2026-09-10T01:00:00Z", target: { kind, id: targetId } });

interface Data { all: Map<string, unknown[]>; restrictions: ApiRestriction[]; invoice: ReturnType<typeof invoice>; recipients: { id: string }[] | Error; commands: Map<string, ApiCommand> }
let data: Data;
const EVENT_ID = "00000000-0000-4000-8000-000000000001";
const call = (fn: typeof dal.coreAll, op: string) => fn.mock.calls.filter((c) => c[0] === op);
const render = async (sp: Record<string, string> = {}) => {
  const element: unknown = await CustomerInvoicePage({ params: Promise.resolve({ id: "inv-1" }), searchParams: Promise.resolve(sp) });
  if (!isValidElement<ComponentProps<typeof InvoiceView>>(element) || element.type !== InvoiceView) throw new TypeError("not InvoiceView");
  return element.props.live;
};

beforeEach(() => {
  vi.resetAllMocks();
  vi.spyOn(crypto, "randomUUID").mockReturnValue(EVENT_ID);
  data = {
    all: new Map<string, unknown[]>([
      ["contracts.list", [contract("contract-other", ["unit-1"]), contract("contract-1", ["unit-1", "unit-2"])]],
      ["units.list", [{ id: "unit-1", displayName: "Lobby AC", propertyId: "prop-1", spaceId: null }, { id: "unit-2", displayName: "Study AC", propertyId: "prop-1", spaceId: "space-1" }]],
      ["properties.list", [{ id: "prop-1", name: "Office A" }]],
      ["spaces.list", [{ id: "space-1", name: "Study", parentSpaceId: null }]],
      ["notifications.list", [reminder("n-1", "inv-1"), reminder("n-2", "inv-other"), reminder("n-3", "inv-1", "restriction")]],
    ]),
    restrictions: [], invoice: invoice(), recipients: [{ id: "m-me" }], commands: new Map(),
  };
  dal.apiMode.mockReturnValue(true);
  dal.coreNow.mockResolvedValue(new Date("2026-09-20T00:00:00Z"));
  dal.corePrincipal.mockResolvedValue({ membershipId: "m-me" });
  dal.coreDisplay.mockResolvedValue({ locale: "en", timeZone: "Asia/Kuala_Lumpur" });
  // the inquiries about a restriction answer with one; every other list answers with its rows of `data`
  dal.coreAll.mockImplementation((op: string, input?: { filters?: { restrictionId?: string } }) => Promise.resolve(input?.filters?.restrictionId
    ? [inquiry("q-r", { subjectType: "restriction", restrictionId: input.filters.restrictionId })]
    : data.all.get(op) ?? []));
  dal.coreItems.mockImplementation(() => Promise.resolve(data.restrictions));
  dal.coreOp.mockImplementation((op: string, input: { id?: string }) => {
    if (op === "invoices.get") return Promise.resolve(data.invoice);
    if (op === "notifications.recipients") return Error.isError(data.recipients) ? Promise.reject(data.recipients) : Promise.resolve({ items: data.recipients });
    if (op === "commands.get") {
      const found = data.commands.get(input.id ?? "");
      return found === undefined ? Promise.reject(refusal("NOT_FOUND")) : Promise.resolve(found);
    }
    return Promise.resolve({ id: "x", version: 1, status: "processing", params: { targetName: "INV-0042" }, templateKey: "payment", channel: "email" });
  });
});
afterEach(() => {
  vi.unstubAllEnvs();
  vi.restoreAllMocks();
});

describe("invoice actions", () => {
  it("startPayment initiates with the card at the invoice version, then sends processing at the payment's version", async () => {
    dal.coreOp.mockResolvedValueOnce({ id: "pay-12345678-abcd", version: 1 }).mockResolvedValueOnce({ id: "pay-12345678-abcd", version: 2 });
    await expect(startPayment("inv-1", 3, "demo_debit_card")).resolves.toStrictEqual({ ok: true, value: "pay-12345678-abcd" });
    expect(dal.coreOp.mock.calls[0]).toStrictEqual(["payments.simulate", { event: "initiate", demoConfirmed: true, invoiceId: "inv-1", method: "demo_debit_card" }, { write: true, expectedVersion: 3 }]);
    expect(dal.coreOp.mock.calls[1]).toStrictEqual(["payments.simulate", { event: "processing", paymentId: "pay-12345678-abcd", eventId: EVENT_ID, paymentReference: "DEMO-PAY-5678ABCD" }, { write: true, expectedVersion: 1 }]);
    expect(cache.refresh).toHaveBeenCalledOnce();
  });

  it("startCheckout returns only an HTTPS Stripe page; any other address fails as unavailable (IR840)", async () => {
    dal.coreOp.mockResolvedValueOnce({ paymentId: "pay-1", checkoutUrl: "https://checkout.stripe.com/c/pay/cs_1" });
    await expect(startCheckout("inv-1", 3)).resolves.toStrictEqual({ ok: true, value: "https://checkout.stripe.com/c/pay/cs_1" });
    expect(dal.coreOp.mock.lastCall).toStrictEqual(["payments.startCheckout", { invoiceId: "inv-1" }, { write: true, expectedVersion: 3 }]);
    for (const checkoutUrl of ["http://evil.example/x", "javascript:alert(1)"]) {
      dal.coreOp.mockResolvedValueOnce({ paymentId: "pay-1", checkoutUrl });
      await expect(startCheckout("inv-1", 3)).resolves.toStrictEqual({ ok: false, code: "UNAVAILABLE", messageKey: "error.unavailable", fieldErrors: {} });
    }
    expect(cache.refresh).toHaveBeenCalledTimes(3);
  });

  it("demoOutcome sends the event at the payment version and returns its status; a refusal keeps its code", async () => {
    dal.coreOp.mockResolvedValueOnce({ status: "confirmed" });
    await expect(demoOutcome("pay-1", 2, "confirm")).resolves.toStrictEqual({ ok: true, value: "confirmed" });
    expect(dal.coreOp.mock.lastCall).toStrictEqual(["payments.simulate", { event: "confirm", paymentId: "pay-1", eventId: EVENT_ID, paymentReference: "DEMO-PAY-PAY1" }, { write: true, expectedVersion: 2 }]);
    dal.coreOp.mockRejectedValueOnce(refusal("VERSION_CONFLICT"));
    await expect(demoOutcome("pay-1", 2, "fail")).resolves.toStrictEqual({ ok: false, code: "VERSION_CONFLICT", messageKey: "error.VERSION_CONFLICT", fieldErrors: {}, retryAfterSeconds: null });
    expect(cache.refresh).toHaveBeenCalledTimes(2);
  });

  it("paymentInstructions returns the message params; previewMessage previews for the signed-in member", async () => {
    dal.coreOp.mockResolvedValueOnce({ params: { targetName: "INV-0042" } });
    await expect(paymentInstructions("inv-1", 3)).resolves.toStrictEqual({ ok: true, value: { targetName: "INV-0042" } });
    expect(dal.coreOp.mock.lastCall).toStrictEqual(["payments.simulate", { event: "instructions", demoConfirmed: true, invoiceId: "inv-1" }, { write: true, expectedVersion: 3 }]);
    dal.coreOp.mockResolvedValueOnce({ params: { targetName: "INV-0042" }, templateKey: "payment", channel: "whatsapp", extra: true });
    await expect(previewMessage("inv-1", "whatsapp")).resolves.toStrictEqual({ ok: true, value: { params: { targetName: "INV-0042" }, templateKey: "payment", channel: "whatsapp" } });
    expect(dal.coreOp.mock.lastCall).toStrictEqual(["notifications.preview", { target: { kind: "invoice", id: "inv-1" }, templateKey: "payment", recipientMembershipId: "m-me", channel: "whatsapp" }]);
  });

  it("sendInquiry trims the message and is a write without a version", async () => {
    dal.coreOp.mockResolvedValueOnce({ id: "q-9" });
    await expect(sendInquiry({ subjectType: "restriction", restrictionId: "r-1", message: "  why?  " })).resolves.toStrictEqual({ ok: true, value: "q-9" });
    expect(dal.coreOp.mock.lastCall).toStrictEqual(["inquiries.create", { subjectType: "restriction", restrictionId: "r-1", message: "why?" }, { write: true }]);
  });
});

describe("invoice page", () => {
  it("outside API mode shows the demo invoice with the asked tab, and an unknown demo invoice is not found", async () => {
    dal.apiMode.mockReturnValue(false);
    const element: unknown = await CustomerInvoicePage({ params: Promise.resolve({ id: "invoice-overdue-a" }), searchParams: Promise.resolve({ tab: "restriction" }) });
    if (!isValidElement<ComponentProps<typeof InvoiceDemo>>(element) || element.type !== InvoiceDemo) throw new TypeError("not InvoiceDemo");
    expect(element.props.invoice.id).toBe("invoice-overdue-a");
    expect(element.props.contract.id).toBe("contract-rto-a");
    expect(element.props.tab).toBe("restriction");
    await expect(CustomerInvoicePage({ params: Promise.resolve({ id: "nope" }), searchParams: Promise.resolve({}) })).rejects.toThrow("notFound");
    expect(dal.coreOp).not.toHaveBeenCalled();
  });

  it("another customer's invoice (NOT_FOUND, or a refused id) is not found; any other error goes to the error boundary", async () => {
    for (const error of [refusal("NOT_FOUND"), refusal("VALIDATION", { id: "invalid" })]) {
      dal.coreOp.mockRejectedValueOnce(error);
      await expect(render()).rejects.toThrow("notFound");
    }
    dal.coreOp.mockRejectedValueOnce(refusal("FORBIDDEN"));
    await expect(render()).rejects.toThrow("FORBIDDEN");
  });

  it("an unpaid invoice with no restriction: the payment tab, its contract and scope, its own reminders only, no commands read", async () => {
    const live = await render({ tab: "restriction", checkout: "bogus" });
    expect(live.tab).toBe("payment");
    expect(live.restriction).toBeNull();
    expect(live.invoice).toStrictEqual({ id: "inv-1", version: 3, number: "INV-0042", amount: "120.00 MYR", due: "Due 10 Sept 2026 · billing period September 2026", dueDate: "10 Sept 2026", status: "Overdue", payable: true });
    expect(live.contract).toMatchObject({ id: "contract-1", scope: "2 units (Lobby AC, Study AC)" });
    expect(live.reminders).toStrictEqual([{ id: "n-1", channel: "email", at: "10 Sept 2026" }]);
    expect(live).toMatchObject({ open: null, last: null, payments: [], otherZone: false, whatsapp: true, stripe: false, checkout: null, inquiryId: null });
    expect(call(dal.coreOp, "commands.get")).toStrictEqual([]);
    expect(dal.coreItems.mock.lastCall).toStrictEqual(["restrictions.forInvoice", { invoiceId: "inv-1", query: { limit: 20 } }]);
    expect(call(dal.coreAll, "inquiries.list")).toStrictEqual([["inquiries.list", { filters: { invoiceId: "inv-1" } }]]);
  });

  it("the open payment is the newest initiated or processing one, the last one is the newest; a paid invoice is not payable", async () => {
    data.invoice = invoice({ status: "processing", paymentRefs: [
      payment("pay-old", { createdAt: "2026-09-18T00:00:00Z", status: "processing", version: 4, channel: "stripe_checkout", method: null }),
      payment("pay-new", { createdAt: "2026-09-19T00:00:00Z", status: "failed", paymentReference: "REF-1" }),
    ] });
    const live = await render({ checkout: "cancelled", inquiryId: "q-1" });
    expect(live.open).toStrictEqual({ id: "pay-old", version: 4, status: "processing", method: null, channel: "stripe_checkout" });
    expect(live.last).toStrictEqual({ id: "pay-new", status: "failed", reference: "REF-1", method: "demo_credit_card", channel: "demo" });
    expect(live.invoice).toMatchObject({ status: "Processing", payable: false });
    expect(live.payments.map((p) => p.id)).toStrictEqual(["pay-new", "pay-old"]);
    expect(live).toMatchObject({ checkout: "cancelled", inquiryId: "q-1" });
    data.invoice = invoice({ status: "paid" });
    await expect(render()).resolves.toMatchObject({ invoice: { status: "Paid", payable: false } });
  });

  it("no contract when it is not the customer's; one unit and no units are worded without a list", async () => {
    data.all.set("contracts.list", [contract("contract-other", [])]);
    let live = await render();
    expect(live.contract).toBeNull();
    data.all.set("contracts.list", [contract("contract-1", ["unit-1"])]);
    live = await render();
    expect(live.contract?.scope).toBe("1 unit (Lobby AC)");
    data.all.set("contracts.list", [contract("contract-1", [])]);
    live = await render();
    expect(live.contract?.scope).toBe("0 units");
  });

  it("WhatsApp is ready only when the member is a WhatsApp recipient; a failed check keeps the button", async () => {
    data.recipients = [{ id: "m-other" }];
    let live = await render();
    expect(live.whatsapp).toBe(false);
    expect(call(dal.coreOp, "notifications.recipients")[0]?.[1]).toStrictEqual({ target: { kind: "invoice", id: "inv-1" }, templateKey: "payment", channel: "whatsapp", query: { limit: 100 } });
    data.recipients = new Error("down");
    live = await render();
    expect(live.whatsapp).toBe(true);
  });

  it("outside Kuala Lumpur the due date is marked as a Kuala Lumpur day, and event times are in the user's zone", async () => {
    dal.coreDisplay.mockResolvedValue({ locale: "en", timeZone: "Asia/Tokyo" });
    const live = await render();
    expect(live.otherZone).toBe(true);
    expect(live.invoice.dueDate).toBe("10 Sept 2026");
  });

  it("the restriction tab: the asked restriction, its newest commands (a missing one left out), its inquiries and lines", async () => {
    data.restrictions = [
      restriction("r-live", "applied"),
      restriction("r-asked", "release_requested", {
        causeInvoiceIds: ["inv-1", "inv-2"], releaseIntent: { source: "payment", at: "2026-09-24T01:41:00Z" },
        perUnit: [{ unitId: "unit-1", applyState: "applied", releaseState: "requested", applyCommandIds: ["cmd-a1"], releaseCommandIds: ["cmd-r0", "cmd-r1"], observedRestriction: null, observedAt: null, pendingReason: null }],
      }),
    ];
    data.commands = new Map([["cmd-r1", { id: "cmd-r1", status: "sent", delivery: "mqtt", requestedAt: "2026-09-24T01:41:00Z", failureCode: null }]]);
    const live = await render({ tab: "restriction", restrictionId: "r-asked" });
    expect(live.tab).toBe("restriction");
    expect(call(dal.coreOp, "commands.get").map((c): unknown => c[1])).toStrictEqual([{ id: "cmd-a1" }, { id: "cmd-r1" }]);
    expect(call(dal.coreAll, "inquiries.list")[1]).toStrictEqual(["inquiries.list", { filters: { restrictionId: "r-asked" } }]);
    const r = live.restriction;
    if (!r) throw new Error("no restriction");
    expect(r.notice.id).toBe("r-asked");
    expect(r.inquiries.map((q) => q.id)).toStrictEqual(["q-r"]);
    expect(r.reason).toBe("2 unpaid invoices, including INV-0042");
    expect(r.noticeAt).toBe("12 Sept 2026 · in-app (demo)");
    expect(r.released).toMatch(/— automatically after payment confirmation$/);
    expect(r.start).toMatch(/— .*24/);
    expect(r.units).toHaveLength(1);
  });

  it("without an asked restriction the live one is shown, else the first; HQ's release and none yet are worded", async () => {
    data.restrictions = [restriction("r-done", "released", { releaseIntent: { source: "manual", at: "2026-09-24T01:41:00Z" } }), restriction("r-live", "scheduled")];
    let live = await render({ restrictionId: "r-unknown" });
    expect(live.tab).toBe("payment");
    expect(live.restriction?.notice.id).toBe("r-live");
    expect(live.restriction?.reason).toBe("Unpaid invoice INV-0042 (120.00 MYR, due 10 Sept 2026)");
    expect(live.restriction?.released).toBe("not yet — all cause invoices must be paid");
    expect(call(dal.coreOp, "commands.get")).toStrictEqual([]);
    data.restrictions = [restriction("r-done", "released", { releaseIntent: { source: "manual", at: "2026-09-24T01:41:00Z" } })];
    live = await render({ tab: "inquiry" });
    expect(live.tab).toBe("inquiry");
    expect(live.restriction?.notice.id).toBe("r-done");
    expect(live.restriction?.released).toMatch(/— by HQ$/);
  });

  it("with PAYMENT_PROVIDER=Stripe the view takes payments on Stripe; an unknown provider fails at start", async () => {
    vi.resetModules();
    vi.stubEnv("PAYMENT_PROVIDER", "Stripe");
    const page = await import("../app/customer/payments/[id]/page");
    const element: unknown = await page.default({ params: Promise.resolve({ id: "inv-1" }), searchParams: Promise.resolve({ checkout: "return" }) });
    if (!isValidElement<ComponentProps<typeof InvoiceView>>(element)) throw new TypeError("not an element");
    expect(element.props.live).toMatchObject({ stripe: true, checkout: "return" });
    vi.resetModules();
    vi.stubEnv("PAYMENT_PROVIDER", "paypal");
    await expect(import("../app/customer/payments/[id]/page")).rejects.toThrow(/invalid_value|Invalid option/);
  });

  it("loading and error states name the invoice", () => {
    const loading: unknown = InvoiceLoading();
    expect(isValidElement<ComponentProps<typeof RouteLoading>>(loading) && loading.type === RouteLoading && loading.props).toStrictEqual({ what: "the invoice", variant: "detail" });
    const error = new Error("boom");
    const retry = vi.fn();
    const failed: unknown = InvoiceErrorState({ error, retry });
    expect(isValidElement<ComponentProps<typeof RouteError>>(failed) && failed.type === RouteError && failed.props).toStrictEqual({ what: "Invoice", error, retry });
  });
});
