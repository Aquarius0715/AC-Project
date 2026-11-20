// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import type { ActionFailure } from "@ac/web/lib/actionMessage";
import type { InvoiceLive } from "../app/customer/payments/[id]/_components/invoice-view";

// The customer invoice (FR-C11, FR-C12) in API mode: the tabs, the demo card and instructions payment, the demo PSP
// events of an open payment, Stripe Checkout (DEC-68: the hosted page, the wait for the webhook, a cancel), the
// confirmed and failed results, the reminders and their previews (WhatsApp only with a phone number, IR578), the
// restriction banner and tab, and the inquiry form with its 1–2000 character check.
type Done<T> = { ok: true; value: T } | ({ ok: false } & ActionFailure);
const actions = vi.hoisted(() => ({
  startPayment: vi.fn<(invoiceId: string, version: number, method: string) => Promise<Done<string>>>(),
  startCheckout: vi.fn<(invoiceId: string, version: number) => Promise<Done<string>>>(),
  demoOutcome: vi.fn<(paymentId: string, version: number, event: string) => Promise<Done<string>>>(),
  paymentInstructions: vi.fn<(invoiceId: string, version: number) => Promise<Done<{ targetName: string }>>>(),
  previewMessage: vi.fn<(invoiceId: string, channel: string) => Promise<Done<null>>>(),
  sendInquiry: vi.fn<(input: Record<string, string>) => Promise<Done<string>>>(),
}));
vi.mock("../app/customer/payments/[id]/actions", () => actions);
const runs = vi.hoisted((): { toasts: string[] } => ({ toasts: [] }));
vi.mock("@ac/web/lib/useAction", () => ({
  useAction: () => [false, <T,>(fn: () => Promise<Done<T>>, ok: string, after?: (v: T) => void) => {
    void fn().then((r) => {
      if (!r.ok) return;
      runs.toasts.push(ok);
      after?.(r.value);
    });
  }] as const,
}));
const nav = vi.hoisted(() => ({ patch: vi.fn(), refresh: vi.fn() }));
vi.mock("@ac/web/lib/useUrlPatch", () => ({ useUrlPatch: () => nav.patch }));
vi.mock("next/navigation", () => ({ useRouter: () => ({ refresh: nav.refresh }) }));

import { InvoiceView } from "../app/customer/payments/[id]/_components/invoice-view";

type Live = InvoiceLive;
const notice = (state: NonNullable<Live["restriction"]>["notice"]["state"], live = true) => ({ id: "restr-123456789", label: state, policy: "minimum cooling setpoint 24 °C", units: "1 unit in scope · Lobby AC", unitIds: ["unit-1"], state, live });
const restriction = (state: Parameters<typeof notice>[0] = "applied", live = true): NonNullable<Live["restriction"]> => ({
  notice: notice(state, live), units: [{ unitId: "unit-1", name: "Lobby AC", apply: "Applied", release: "Release sent", releaseTone: "primary", note: "the unit is offline", tone: "warn" }, { unitId: "unit-2", name: "Study AC", apply: "Not sent yet", release: "", releaseTone: "ok", note: "", tone: "primary" }],
  timeline: [{ time: "13 Sept", title: "Applied", tone: "warn" }], inquiries: [], reason: "Unpaid invoice INV-0042", noticeAt: "12 Sept 2026 · in-app (demo)", start: "13 Sept 08:00 — policy", released: "not yet",
});
const live = (over: Partial<Live> = {}): Live => ({
  tab: "payment", whatsapp: true, stripe: false, checkout: null, otherZone: false,
  invoice: { id: "inv-1", version: 3, number: "INV-0042", amount: "120.00 MYR", due: "Due 10 Sept 2026", dueDate: "10 Sept 2026", status: "Overdue", payable: true },
  contract: { name: "RTO Plan", id: "contract-12345678", scope: "2 units (Lobby AC, Study AC)" },
  payments: [], open: null, last: null, restriction: null, inquiries: [], inquiryId: null, reminders: [], ...over,
});
const show = (over: Partial<Live> = {}) => render(<InvoiceView live={live(over)} />);
const button = (name: string | RegExp) => screen.getByRole("button", { name });
// the dialog's footer Close (its corner button has the same name)
const closeDialog = () => {
  const close = within(screen.getByRole("dialog")).getAllByRole("button", { name: "Close" }).at(-1);
  if (!close) throw new Error("no Close button");
  fireEvent.click(close);
};
const flush = () => act(async () => { await Promise.resolve(); });

beforeEach(() => {
  runs.toasts = [];
  nav.patch.mockReset();
  nav.refresh.mockReset();
  for (const fn of Object.values(actions)) fn.mockReset();
  actions.startPayment.mockResolvedValue({ ok: true, value: "pay-1" });
  actions.demoOutcome.mockResolvedValue({ ok: true, value: "processing" });
  actions.paymentInstructions.mockResolvedValue({ ok: true, value: { targetName: "Invoice INV-0042" } });
  actions.previewMessage.mockResolvedValue({ ok: true, value: null });
  actions.sendInquiry.mockResolvedValue({ ok: true, value: "q-1" });
});
afterEach(() => {
  cleanup();
  vi.useRealTimers();
  vi.unstubAllGlobals();
});

describe("InvoiceView payment", () => {
  it("pays with the chosen demo card at the invoice version; a debit card last time is chosen again", async () => {
    show({ last: { id: "pay-0", status: "failed", reference: null, method: "demo_debit_card", channel: "demo" } });
    expect(button(/Debit card \(demo\)/).getAttribute("aria-pressed")).toBe("true");
    expect(screen.getByText("Payment failed")).toBeDefined();
    expect(screen.getByText(/Your method is kept/)).toBeDefined();
    fireEvent.click(button("Try again — Demo"));
    await flush();
    expect(actions.startPayment.mock.lastCall).toStrictEqual(["inv-1", 3, "demo_debit_card"]);
    fireEvent.click(button(/Credit card \(demo\)/));
    fireEvent.click(button("Try again — Demo"));
    await flush();
    expect(actions.startPayment.mock.lastCall).toStrictEqual(["inv-1", 3, "demo_credit_card"]);
    expect(runs.toasts).toStrictEqual(["Demo payment started — processing", "Demo payment started — processing"]);
  });

  it("instructions show the transfer details in a dialog and do not start a payment", async () => {
    show({ otherZone: true });
    expect(screen.getByText(/Kuala Lumpur date \(Asia\/Kuala_Lumpur\)/)).toBeDefined();
    fireEvent.click(button(/Payment instructions/));
    fireEvent.click(button("View instructions"));
    await flush();
    expect(actions.paymentInstructions.mock.lastCall).toStrictEqual(["inv-1", 3]);
    expect(actions.startPayment).not.toHaveBeenCalled();
    const dialog = within(screen.getByRole("dialog"));
    expect(dialog.getByText(/Transfer 120.00 MYR for Invoice INV-0042/)).toBeDefined();
    closeDialog();
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("an open demo payment offers its next PSP event and Decline at the payment version, and no new payment", async () => {
    const open = { id: "pay-12345678", version: 2, status: "initiated", method: "demo_credit_card", channel: "demo" };
    const view = show({ open });
    expect(screen.queryByRole("button", { name: /Continue to payment/ })).toBeNull();
    expect(screen.getByText(/pay-1234 · Credit card \(demo\)/)).toBeDefined();
    fireEvent.click(button("Processing"));
    await flush();
    expect(actions.demoOutcome.mock.lastCall).toStrictEqual(["pay-12345678", 2, "processing"]);
    view.rerender(<InvoiceView live={live({ open: { ...open, status: "processing", version: 3 } })} />);
    expect(screen.queryByRole("button", { name: "Processing" })).toBeNull();
    fireEvent.click(button("Confirm receipt"));
    fireEvent.click(button("Decline"));
    await flush();
    expect(actions.demoOutcome.mock.calls.slice(1)).toStrictEqual([["pay-12345678", 3, "confirm"], ["pay-12345678", 3, "fail"]]);
    expect(runs.toasts).toStrictEqual(["Payment is processing", "Payment confirmed — the invoice is paid", "Payment declined — the invoice stays unpaid"]);
  });

  it("a confirmed payment shows its reference and way; a transfer HQ recorded and a live restriction are worded", () => {
    const paid = { status: "Paid" as const, payable: false };
    const view = show({ invoice: { ...live().invoice, ...paid }, last: { id: "pay-1", status: "confirmed", reference: "DEMO-PAY-1", method: "demo_credit_card", channel: "demo" } });
    expect(screen.getByText("Amount paid")).toBeDefined();
    expect(screen.getByText(/Reference DEMO-PAY-1 · Credit card \(demo\)\. The invoice is now paid\.$/)).toBeDefined();
    expect(screen.queryByRole("button", { name: /Continue/ })).toBeNull();
    view.rerender(<InvoiceView live={live({ invoice: { ...live().invoice, ...paid }, restriction: restriction("release_requested"), last: { id: "pay-1", status: "confirmed", reference: null, method: null, channel: "manual" } })} />);
    expect(screen.getByText(/Reference — · recorded by HQ\. The invoice is now paid\. A cooling-restriction release is requested/)).toBeDefined();
    expect(screen.getByText("Cooling restriction — release requested")).toBeDefined();
  });

  it("payment history lists each payment with its status; reminders show their channel", () => {
    show({
      payments: [{ id: "p-1", version: 1, label: "p-1 · Credit card (demo)", at: "20 Sept", status: "failed", reference: "REF-9" }, { id: "p-2", version: 1, label: "p-2 · Stripe Checkout", at: "21 Sept", status: "expired", reference: null }],
      reminders: [{ id: "n-1", channel: "whatsapp", at: "10 Sept 2026" }, { id: "n-2", channel: "sms", at: "11 Sept 2026" }],
    });
    expect(screen.getByText("20 Sept · REF-9")).toBeDefined();
    expect(screen.getByText("Failed")).toBeDefined();
    expect(screen.getByText("Expired")).toBeDefined();
    expect(screen.getByText("WhatsApp · sent 10 Sept 2026 (demo record)")).toBeDefined();
    expect(screen.getByText("sms · sent 11 Sept 2026 (demo record)")).toBeDefined();
    expect(screen.queryByText("No payments yet")).toBeNull();
  });

  it("previews a message per channel and switches channel in the dialog; without a phone number WhatsApp is off", async () => {
    const view = show();
    fireEvent.click(button("Preview email"));
    await flush();
    expect(actions.previewMessage.mock.lastCall).toStrictEqual(["inv-1", "email"]);
    let dialog = within(screen.getByRole("dialog"));
    expect(dialog.getByText("Subject: Payment reminder — INV-0042")).toBeDefined();
    fireEvent.click(dialog.getByRole("button", { name: "WhatsApp" }));
    await flush();
    expect(actions.previewMessage.mock.lastCall).toStrictEqual(["inv-1", "whatsapp"]);
    dialog = within(screen.getByRole("dialog"));
    expect(dialog.queryByText(/Subject:/)).toBeNull();
    closeDialog();
    expect(screen.queryByRole("dialog")).toBeNull();
    view.rerender(<InvoiceView live={live({ whatsapp: false })} />);
    expect(button("Preview WhatsApp").hasAttribute("disabled")).toBe(true);
    expect(screen.getByText(/WhatsApp reminders need a phone number/)).toBeDefined();
    fireEvent.click(button("Preview email"));
    await flush();
    expect(within(screen.getByRole("dialog")).queryByRole("button", { name: "WhatsApp" })).toBeNull();
  });

  it("Preview WhatsApp previews that channel; each dialog's corner button closes it too", async () => {
    show();
    fireEvent.click(button("Preview WhatsApp"));
    await flush();
    expect(actions.previewMessage.mock.lastCall).toStrictEqual(["inv-1", "whatsapp"]);
    expect(within(screen.getByRole("dialog")).getByText("WhatsApp preview")).toBeDefined();
    fireEvent.click(within(screen.getByRole("dialog")).getAllByRole("button", { name: "Close" })[0] ?? document.body);
    expect(screen.queryByRole("dialog")).toBeNull();
    fireEvent.click(button(/Payment instructions/));
    fireEvent.click(button("View instructions"));
    await flush();
    fireEvent.click(within(screen.getByRole("dialog")).getAllByRole("button", { name: "Close" })[0] ?? document.body);
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("an invoice whose contract is not readable shows dashes for the contract and its scope", () => {
    show({ contract: null });
    expect(screen.getAllByText("—")).toHaveLength(2);
  });
});

describe("InvoiceView Stripe Checkout", () => {
  it("goes to the hosted page; no demo banner, no method choice", async () => {
    const assign = vi.fn();
    vi.stubGlobal("location", { href: location.href, origin: location.origin, pathname: location.pathname, search: location.search, assign });
    actions.startCheckout.mockResolvedValue({ ok: true, value: "https://checkout.stripe.com/c/pay/cs_1" });
    show({ stripe: true });
    expect(screen.queryByText(/Demo payment — no real money moves/)).toBeNull();
    expect(screen.queryByRole("button", { name: /Credit card/ })).toBeNull();
    fireEvent.click(button("Continue to secure payment"));
    await flush();
    expect(actions.startCheckout.mock.lastCall).toStrictEqual(["inv-1", 3]);
    expect(assign).toHaveBeenCalledWith("https://checkout.stripe.com/c/pay/cs_1");
  });

  it("waits for Stripe: re-reads every 5 seconds and offers to start again; a processing one only waits", () => {
    vi.useFakeTimers();
    const open = { id: "pay-stripe-1", version: 1, status: "initiated", method: null, channel: "stripe_checkout" };
    const view = show({ stripe: true, open });
    expect(screen.getByText("Waiting for Stripe")).toBeDefined();
    expect(button("Continue to secure payment")).toBeDefined();
    act(() => { vi.advanceTimersByTime(5000); });
    expect(nav.refresh).toHaveBeenCalledOnce();
    view.rerender(<InvoiceView live={live({ stripe: true, open: { ...open, status: "processing" } })} />);
    expect(screen.getByText(/Stripe is processing payment pay-stri/)).toBeDefined();
    expect(screen.queryByRole("button", { name: "Continue to secure payment" })).toBeNull();
    expect(screen.queryByText(/Demo payment provider/)).toBeNull();
  });

  it("back from a cancel: says nothing was charged and stops re-reading while the attempt is open", () => {
    vi.useFakeTimers();
    show({ stripe: true, checkout: "cancelled", open: { id: "pay-stripe-1", version: 1, status: "initiated", method: null, channel: "stripe_checkout" } });
    expect(screen.getByText(/Payment cancelled on Stripe's page/)).toBeDefined();
    expect(screen.queryByText("Waiting for Stripe")).toBeNull();
    act(() => { vi.advanceTimersByTime(10_000); });
    expect(nav.refresh).not.toHaveBeenCalled();
  });

  it("a failed Stripe payment is worded for Stripe; a demo payment open under Stripe shows no demo events", () => {
    const view = show({ stripe: true, checkout: "cancelled", last: { id: "pay-stripe-1", status: "failed", reference: null, method: null, channel: "stripe_checkout" } });
    expect(screen.getByText(/failed on Stripe/)).toBeDefined();
    expect(screen.getByText(/Payment cancelled on Stripe's page/)).toBeDefined();
    view.rerender(<InvoiceView live={live({ stripe: true, open: { id: "pay-demo-1", version: 1, status: "processing", method: "demo_credit_card", channel: "demo" } })} />);
    expect(screen.getByText("Processing payment")).toBeDefined();
    expect(screen.queryByRole("button", { name: "Confirm receipt" })).toBeNull();
  });
});

describe("InvoiceView tabs, restriction and inquiries", () => {
  it("the tabs patch the URL; the restriction tab exists only with a restriction", () => {
    const view = show({ inquiries: [] });
    expect(screen.getAllByRole("tab").map((x) => x.textContent)).toStrictEqual(["Payment", "Inquiries0"]);
    view.rerender(<InvoiceView live={live({ restriction: restriction() })} />);
    expect(screen.getAllByRole("tab").map((x) => x.textContent)).toStrictEqual(["Payment", "Cooling restriction", "Inquiries0"]);
    fireEvent.click(screen.getByRole("tab", { name: "Cooling restriction" }));
    expect(nav.patch.mock.lastCall).toStrictEqual([{ tab: "restriction" }]);
    view.rerender(<InvoiceView live={live({ tab: "inquiry", restriction: restriction() })} />);
    fireEvent.click(screen.getByRole("tab", { name: "Payment" }));
    expect(nav.patch.mock.lastCall).toStrictEqual([{ tab: null }]);
  });

  it("the payment tab's banner links to the live restriction; an ended one has no banner", () => {
    const view = show({ restriction: restriction() });
    expect(screen.getByText("Cooling restriction active")).toBeDefined();
    expect(screen.getByRole("link", { name: /View restriction details/ }).getAttribute("href")).toBe("/customer/payments/inv-1?tab=restriction&restrictionId=restr-123456789");
    view.rerender(<InvoiceView live={live({ restriction: restriction("released", false) })} />);
    expect(screen.queryByText(/Cooling restriction active/)).toBeNull();
  });

  it("the restriction tab: the summary, each unit's state and note, the keep-plugged-in and never-released notes", () => {
    const view = show({ tab: "restriction", restriction: restriction("release_requested") });
    expect(screen.getByText("Cooling restriction · restr-12")).toBeDefined();
    expect(screen.getByText("RTO Plan · contract")).toBeDefined();
    expect(screen.getByText("Release sent")).toBeDefined();
    expect(screen.getByText("the unit is offline")).toBeDefined();
    expect(screen.getByText(/Keep the AC's control device plugged in/)).toBeDefined();
    expect(screen.getByText(/Status stays “Release requested”/)).toBeDefined();
    expect(screen.queryByText("Payment history")).toBeNull();
    view.rerender(<InvoiceView live={live({ tab: "restriction", contract: null, restriction: restriction("scheduled") })} />);
    expect(screen.queryByText(/Keep the AC's control device plugged in/)).toBeNull();
    expect(screen.queryByText(/Status stays/)).toBeNull();
  });

  it("the restriction tab without a restriction falls back to the payment tab", () => {
    show({ tab: "restriction" });
    expect(screen.getByText("Payment history")).toBeDefined();
  });

  it("an inquiry needs 1–2000 characters; it is sent about the restriction and the form clears", async () => {
    show({ tab: "restriction", restriction: restriction() });
    const box = screen.getByRole("textbox", { name: "Your message" });
    fireEvent.click(button("Send inquiry"));
    expect(screen.getByText("1–2000 characters")).toBeDefined();
    fireEvent.change(box, { target: { value: "x".repeat(2001) } });
    fireEvent.click(button("Send inquiry"));
    expect(actions.sendInquiry).not.toHaveBeenCalled();
    fireEvent.change(box, { target: { value: "x".repeat(2000) } });
    expect(screen.queryByText("1–2000 characters")).toBeNull();
    fireEvent.change(box, { target: { value: " why? " } });
    fireEvent.click(button("Send inquiry"));
    await flush();
    expect(actions.sendInquiry.mock.lastCall).toStrictEqual([{ subjectType: "restriction", restrictionId: "restr-123456789", message: " why? " }]);
    expect(runs.toasts).toStrictEqual(["Inquiry sent to HQ"]);
    expect(screen.getByRole("textbox", { name: "Your message" })).toHaveProperty("value", "");
    expect(screen.queryByText("1–2000 characters")).toBeNull();
  });

  it("the inquiry tab lists the invoice's inquiries, marks the asked one and shows HQ's reply", async () => {
    show({
      tab: "inquiry", inquiryId: "q-2",
      inquiries: [{ id: "q-1", subject: "Payment", at: "12 Sept", state: "received", message: "When?", reply: null }, { id: "q-2", subject: "Payment", at: "13 Sept", state: "answered", message: "Paid?", reply: "Yes" }],
    });
    expect(screen.getByText("Inquiries about invoice INV-0042")).toBeDefined();
    expect(screen.getByText("Received")).toBeDefined();
    expect(screen.getByText("Answered")).toBeDefined();
    expect(screen.getByText("Yes")).toBeDefined();
    const asked = screen.getByText("Paid?").parentElement;
    expect(asked?.className).toContain("border-primary");
    expect(screen.getByText("When?").parentElement?.className).not.toContain("border-primary");
    fireEvent.change(screen.getByRole("textbox", { name: "Your message" }), { target: { value: "Again" } });
    fireEvent.click(button("Send inquiry"));
    await flush();
    expect(actions.sendInquiry.mock.lastCall).toStrictEqual([{ subjectType: "payment", invoiceId: "inv-1", message: "Again" }]);
  });

  it("an empty inquiry list says so", () => {
    show({ tab: "inquiry" });
    expect(screen.getByText("No inquiry yet")).toBeDefined();
  });
});
