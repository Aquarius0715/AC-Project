// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, fireEvent, render, screen, within } from "@testing-library/react";

// The Phase 1A demo of the customer invoice (FR-C11, FR-C12; DATA_SOURCE≠API): the restriction tab only for the
// invoice that causes one, a demo card payment that is processing first and then confirmed (the debit card fails and
// can be retried), the instructions that leave the invoice unpaid, a paid invoice, and the reminders with a preview.
const toast = vi.hoisted(() => vi.fn());
vi.mock("@ac/web/components/ui", async (importOriginal) => ({ ...(await importOriginal<Record<string, unknown>>()), useToast: () => toast }));

import { InvoiceDemo } from "../app/customer/payments/[id]/_components/invoice-demo";
import { demoInvoice } from "../app/customer/payments/_lib/demo-contracts";

const show = (id: string, tab?: string) => {
  const demo = demoInvoice(id);
  if (!demo) throw new Error(`no demo invoice ${id}`);
  return render(<InvoiceDemo invoice={demo.invoice} contract={demo.contract} tab={tab} />);
};
const button = (name: string | RegExp) => screen.getByRole("button", { name });

beforeEach(() => { toast.mockReset(); });
afterEach(() => {
  cleanup();
  vi.useRealTimers();
});

describe("InvoiceDemo", () => {
  it("the restriction tab of the restricted invoice shows the restriction; another invoice shows its payment", () => {
    const view = show("invoice-overdue-a", "restriction");
    expect(screen.getByText("Cooling restriction · restriction-limited-a")).toBeDefined();
    expect(screen.getByRole("link", { name: /Invoice invoice-overdue-a/ }).getAttribute("href")).toBe("/customer/payments/invoice-overdue-a");
    expect(screen.queryByText("Amount due")).toBeNull();
    view.unmount();
    show("invoice-0198", "restriction");
    expect(screen.queryByText(/Cooling restriction ·/)).toBeNull();
    expect(screen.getByText("Amount due")).toBeDefined();
  });

  it("an unpaid restricted invoice: unpaid badge, contract scope, the banner linking to the restriction tab", () => {
    show("invoice-overdue-a");
    expect(screen.getByText("Unpaid")).toBeDefined();
    expect(screen.getByText("3 units (Bedroom AC, Meeting room AC, Lobby AC)")).toBeDefined();
    expect(screen.getByText("No payments yet")).toBeDefined();
    expect(screen.getByRole("link", { name: /View restriction/ }).getAttribute("href")).toBe("/customer/payments/invoice-overdue-a?tab=restriction");
  });

  it("a one-unit contract's scope says unit", () => {
    show("invoice-sep-a");
    expect(screen.getByText("1 unit (Study AC)")).toBeDefined();
  });

  it("a credit card payment is processing first, then confirmed with a history line", () => {
    vi.useFakeTimers();
    show("invoice-overdue-a");
    fireEvent.click(button("Pay now"));
    expect(screen.getByText("Processing")).toBeDefined();
    expect(screen.getByText(/“Processing” is not paid/)).toBeDefined();
    expect(screen.queryByRole("button", { name: "Pay now" })).toBeNull();
    act(() => { vi.advanceTimersByTime(1800); });
    expect(screen.getByText("Confirmed")).toBeDefined();
    expect(screen.getByText(/Payment confirmed \(demo\)/)).toBeDefined();
    expect(screen.getByText("Demo card payment · just now · 120.00 MYR")).toBeDefined();
  });

  it("the debit card fails; Retry goes back to the choice with the credit card", () => {
    vi.useFakeTimers();
    show("invoice-overdue-a");
    fireEvent.click(button(/Debit card \(demo\)/));
    expect(button(/Debit card \(demo\)/).getAttribute("aria-pressed")).toBe("true");
    fireEvent.click(button("Pay now"));
    act(() => { vi.advanceTimersByTime(1800); });
    expect(screen.getByText(/Payment failed — invoice stays unpaid/)).toBeDefined();
    expect(screen.getByText("Unpaid")).toBeDefined();
    fireEvent.click(button("Retry"));
    expect(button(/Credit card \(demo\)/).getAttribute("aria-pressed")).toBe("true");
    expect(button("Pay now")).toBeDefined();
  });

  it("payment instructions only tell the user, and the invoice stays unpaid", () => {
    show("invoice-overdue-a");
    fireEvent.click(button(/Payment instructions/));
    expect(screen.queryByRole("button", { name: "Pay now" })).toBeNull();
    fireEvent.click(button("View instructions"));
    expect(toast).toHaveBeenCalledWith("Bank instructions shown — invoice stays unpaid", "warn");
    expect(screen.getByText("Unpaid")).toBeDefined();
  });

  it("a paid invoice: paid badge, its payment in the history, no payment form and no restriction banner", () => {
    show("invoice-0198");
    expect(screen.getByText("Paid")).toBeDefined();
    expect(screen.getByText("Demo card payment · Aug 8 · 80.00 MYR")).toBeDefined();
    expect(screen.queryByRole("button", { name: "Pay now" })).toBeNull();
    expect(screen.queryByText(/Cooling restriction active/)).toBeNull();
  });

  it("the reminders open and close; each has a preview of the message", () => {
    show("invoice-overdue-a");
    expect(screen.queryByText(/Email · Demo record/)).toBeNull();
    fireEvent.click(button("Show reminders"));
    expect(button("Hide reminders").getAttribute("aria-expanded")).toBe("true");
    expect(screen.getByText("Sent Sep 12 (demo record)")).toBeDefined();
    const previews = screen.getAllByRole("button", { name: "View preview" });
    expect(previews).toHaveLength(2);
    fireEvent.click(previews[1] ?? document.body);
    const dialog = within(screen.getByRole("dialog"));
    expect(dialog.getByText("WhatsApp preview")).toBeDefined();
    expect(dialog.getByText(/Your invoice invoice-overdue-a \(120.00 MYR\) is due on Sep 10, 2026/)).toBeDefined();
    fireEvent.click(dialog.getAllByRole("button", { name: "Close" }).at(-1) ?? document.body);
    expect(screen.queryByRole("dialog")).toBeNull();
    // the dialog's corner button closes it as well
    fireEvent.click(previews[0] ?? document.body);
    expect(within(screen.getByRole("dialog")).getByText("Email preview")).toBeDefined();
    fireEvent.click(within(screen.getByRole("dialog")).getAllByRole("button", { name: "Close" })[0] ?? document.body);
    expect(screen.queryByRole("dialog")).toBeNull();
    fireEvent.click(button("Hide reminders"));
    expect(screen.queryByRole("button", { name: "View preview" })).toBeNull();
  });
});
