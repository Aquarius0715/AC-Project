// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import type { ComponentProps } from "react";
import { recordRows, type ApiOffsetRecord, type ApiQuote } from "@ac/web/lib/offsets";
import type { ActionFailure } from "@ac/web/lib/actionMessage";

// The offset demo registry's screens (FR-A15): in API mode the filters and the selection go to the URL, a quote is
// checked before it is asked for, requested only once confirmed, and each demo step acts on the record's current
// attempt; the Phase 1A demo moves its fixture records locally.
const actions = vi.hoisted(() => {
  type Action = (...args: unknown[]) => Promise<unknown>;
  return { quoteOffset: vi.fn<Action>(), requestOffset: vi.fn<Action>(), simulateOffset: vi.fn<Action>(), retryOffset: vi.fn<Action>() };
});
const nav = vi.hoisted(() => ({ patch: vi.fn<(p: Record<string, string | null>) => void>(), toasts: [] as string[] }));
vi.mock("../app/admin/offsets/actions", () => actions);
vi.mock("@ac/web/lib/useUrlPatch", () => ({ useUrlPatch: () => nav.patch }));
type Result = { ok: true; value: unknown } | ({ ok: false } & ActionFailure);
vi.mock("@ac/web/lib/useAction", () => ({
  useAction: () => [false, (fn: () => Promise<Result>, ok: string, after?: (v: unknown) => void) => {
    void fn().then((r) => {
      if (!r.ok) return;
      nav.toasts.push(ok);
      after?.(r.value);
    });
  }] as const,
}));
vi.mock("@ac/web/components/ui", async (importOriginal) => ({ ...(await importOriginal<Record<string, unknown>>()), useToast: () => (m: string) => { nav.toasts.push(m); } }));
// the period pickers as one text field each: the view only passes their value through
vi.mock("@ac/web/components/TimeSelect", async (importOriginal) => ({
  ...(await importOriginal<Record<string, unknown>>()),
  DateTimeSelect: ({ value, onChange }: Readonly<{ value: string; onChange: (v: string) => void }>) => <input aria-label="date and time" value={value} onChange={(e) => { onChange(e.target.value); }} />,
}));

import { OffsetsView } from "../app/admin/offsets/_components/offsets-view";
import { OffsetsDemo } from "../app/admin/offsets/_components/offsets-demo";

type Live = ComponentProps<typeof OffsetsView>["live"];
const attempt = { id: "att-1", stage: "purchase" as const, status: "pending" as const, startedAt: "2026-09-20T01:30:00Z", completedAt: null };
const record = (id: string, over: Partial<ApiOffsetRecord> = {}): ApiOffsetRecord => ({
  id, version: 2, createdAt: "2026-09-20T01:00:00Z", quoteId: "quo-00001111", customerId: "cust-a", amountKg: 1.5, attempts: [attempt], currentAttemptId: "att-1",
  state: "demo_requested", previousState: null, purchaseRef: null, retirementRef: null, demoCertificateRef: null,
  eventHistory: [{ action: "offsets.request", occurredAt: "2026-09-20T01:00:00Z", actorId: "user-hq", actorName: "hq-operator", result: "success", reason: null }], ...over,
});
const customers = [{ id: "cust-b", name: "Beta Hotel" }, { id: "cust-a", name: "Demo Customer A" }];
function liveOf(records: ApiOffsetRecord[], over: Partial<Live> = {}): Live {
  const rows = recordRows(records, customers);
  return {
    tab: "records", rows, selectedId: rows[0]?.id ?? null, canWrite: true, recordMissing: false, scope: { created: "30d" }, customers,
    units: [{ id: "u-1", label: "Lobby AC", customerId: "cust-a" }, { id: "u-2", label: "Office AC", customerId: "cust-a" }, { id: "u-9", label: "Hotel AC", customerId: "cust-b" }], ...over,
  };
}
const quote: ApiQuote = {
  id: "quo-22223333", version: 1, customerId: "cust-a", purpose: "August", amountKg: 1.25, period: { from: "2026-07-31T16:00:00Z", to: "2026-08-31T16:00:00Z" }, unitIds: ["u-1"],
  expiresAt: "2026-09-22T01:15:00Z", provider: "unselected", scheme: "demo", marketConcept: { stage: "future_concept", providerLabel: "unselected", verificationStatus: "unverified", ledgerStatus: "not_connected" },
};
const button = (name: string | RegExp) => screen.getByRole("button", { name });
// inside act, so the state updates the settled action makes (a closed dialog, a toast) render before the checks
const flush = () => act(() => new Promise<void>((r) => { setTimeout(r, 0); }));

beforeEach(() => {
  nav.patch.mockReset(); nav.toasts = [];
  history.replaceState(null, "", "/admin/offsets"); // the demo keeps its tab in the URL
  for (const fn of Object.values(actions)) fn.mockReset();
});
afterEach(cleanup);

describe("the offsets view in API mode", () => {
  it("puts the filters and the selection in the URL, and offers to clear filters that match nothing", () => {
    const { rerender } = render(<OffsetsView live={liveOf([record("rec-11112222"), record("rec-33334444", { state: "demo_retired", currentAttemptId: null, demoCertificateRef: "DEMO-0007" })])} />);
    expect(within(screen.getByLabelText("Customer")).getAllByRole("option").map((o) => o.textContent)).toStrictEqual(["All customers", "Beta Hotel", "Demo Customer A"]);
    fireEvent.change(screen.getByLabelText("Customer"), { target: { value: "cust-a" } });
    expect(nav.patch).toHaveBeenLastCalledWith({ customerId: "cust-a", recordId: null });
    fireEvent.change(screen.getByLabelText("Status"), { target: { value: "failed" } });
    expect(nav.patch).toHaveBeenLastCalledWith({ status: "failed", recordId: null });
    fireEvent.change(screen.getByLabelText("Created"), { target: { value: "30d" } });
    expect(nav.patch).toHaveBeenLastCalledWith({ created: null, recordId: null }); // the default is no key
    fireEvent.change(screen.getByLabelText("Created"), { target: { value: "all" } });
    expect(nav.patch).toHaveBeenLastCalledWith({ created: "all", recordId: null });
    fireEvent.click(button(/rec-3333/));
    expect(nav.patch).toHaveBeenLastCalledWith({ recordId: "rec-33334444" });
    fireEvent.click(screen.getByRole("tab", { name: /Market concept/ }));
    expect(nav.patch).toHaveBeenLastCalledWith({ tab: "market" });
    rerender(<OffsetsView live={liveOf([record("rec-11112222")], { scope: { customerId: "cust-a", status: "failed", created: "30d" } })} />);
    fireEvent.change(screen.getByLabelText("Customer"), { target: { value: "" } });
    expect(nav.patch).toHaveBeenLastCalledWith({ customerId: null, recordId: null }); // All customers is no key
    fireEvent.change(screen.getByLabelText("Status"), { target: { value: "" } });
    expect(nav.patch).toHaveBeenLastCalledWith({ status: null, recordId: null });
    rerender(<OffsetsView live={liveOf([], { scope: { customerId: "cust-a", created: "30d" } })} />);
    fireEvent.click(button("Clear the filters"));
    expect(nav.patch).toHaveBeenLastCalledWith({ customerId: null, status: null, created: "all", recordId: null });
    rerender(<OffsetsView live={liveOf([], { scope: { created: "all" } })} />);
    expect(screen.queryByRole("button", { name: "Clear the filters" })).toBeNull();
    expect(screen.getByText("Get a demo quote and request it to create a record.")).toBeDefined();
  });

  it("shows the market concept read only, and the record panel's empty states", () => {
    const { rerender } = render(<OffsetsView live={liveOf([], { tab: "market" })} />);
    expect(screen.getByText("Carbon market — future concept")).toBeDefined();
    fireEvent.click(screen.getByRole("tab", { name: /Demo records/ }));
    expect(nav.patch).toHaveBeenLastCalledWith({ tab: null });
    rerender(<OffsetsView live={liveOf([], { recordMissing: true })} />);
    expect(screen.getByText("That record is not in this list")).toBeDefined();
    rerender(<OffsetsView live={liveOf([])} />);
    expect(screen.getByText("Nothing selected")).toBeDefined();
  });

  it("confirms a requested record's simulated purchase or a failure on its current attempt", async () => {
    actions.simulateOffset.mockResolvedValue({ ok: true, value: null });
    render(<OffsetsView live={liveOf([record("rec-11112222")])} />);
    expect(screen.getByText("purchase · pending · started 20 Sept 2026, 9:30 am MYT")).toBeDefined();
    fireEvent.click(button("Confirm simulated purchase"));
    fireEvent.click(button("Simulate a failure"));
    await flush();
    expect(actions.simulateOffset.mock.calls).toStrictEqual([["rec-11112222", 2, "att-1", "purchase_confirm"], ["rec-11112222", 2, "att-1", "fail"]]);
    expect(nav.toasts).toStrictEqual(["Demo purchase confirmed", "Failure recorded — the previous stage is kept"]);
  });

  it("retires a purchased record only once the demo is acknowledged", async () => {
    actions.simulateOffset.mockResolvedValue({ ok: true, value: null });
    render(<OffsetsView live={liveOf([record("rec-11112222", { state: "demo_purchased", purchaseRef: "PUR-1" })])} />);
    const retire = screen.getByRole<HTMLButtonElement>("button", { name: "Retire whole amount (demo)" });
    expect(retire.disabled).toBe(true);
    fireEvent.click(screen.getByRole("checkbox", { name: "I understand this is a demo retirement. It does not retire a real credit." }));
    expect(retire.disabled).toBe(false);
    fireEvent.click(retire);
    await flush();
    expect(actions.simulateOffset).toHaveBeenCalledWith("rec-11112222", 2, "att-1", "retire");
    fireEvent.click(button("Simulate a failure"));
    await flush();
    expect(actions.simulateOffset).toHaveBeenLastCalledWith("rec-11112222", 2, "att-1", "fail");
    expect(nav.toasts).toStrictEqual(["Demo retired", "Failure recorded — the previous stage is kept"]);
    expect(screen.getByText("PUR-1")).toBeDefined();
  });

  it("retries a failed record's attempt, and shows a retired one's demo certificate without steps", async () => {
    actions.retryOffset.mockResolvedValue({ ok: true, value: null });
    const { rerender } = render(<OffsetsView live={liveOf([record("rec-11112222", { state: "failed", previousState: "demo_requested" })])} />);
    expect(screen.getByText("Purchase failed · previous: Demo requested. A retry starts a new attempt of the failed stage.")).toBeDefined();
    fireEvent.click(button("Retry"));
    await flush();
    expect(actions.retryOffset).toHaveBeenCalledWith("rec-11112222", 2, "att-1");
    expect(nav.toasts).toStrictEqual(["Retry started — a new attempt"]);
    rerender(<OffsetsView live={liveOf([record("rec-11112222", { state: "demo_retired", currentAttemptId: null, demoCertificateRef: "DEMO-0007", eventHistory: [] })])} />);
    expect(screen.getByText("DEMO-0007 (demo — not a real certificate)")).toBeDefined();
    expect(screen.getByText("none")).toBeDefined(); // no current attempt
    expect(screen.getByText("No events yet.")).toBeDefined();
    for (const name of ["Retire whole amount (demo)", "Confirm simulated purchase", "Retry"]) expect(screen.queryByRole("button", { name })).toBeNull();
  });

  it("offers no writes to a role that may only read offsets", () => {
    render(<OffsetsView live={liveOf([record("rec-11112222", { state: "failed", previousState: "demo_purchased" })], { canWrite: false })} />);
    for (const name of [/New demo quote/, "Retry", "Confirm simulated purchase", "Retire whole amount (demo)"]) expect(screen.queryByRole("button", { name })).toBeNull();
    expect(screen.getByText("Retirement failed · previous: Demo purchased. A retry starts a new attempt of the failed stage.")).toBeDefined();
  });

  it("checks a quote before asking for it, then requests it only once no real transaction is confirmed, and opens the new record", async () => {
    actions.quoteOffset.mockResolvedValue({ ok: true, value: quote });
    actions.requestOffset.mockResolvedValue({ ok: true, value: "rec-new" });
    render(<OffsetsView live={liveOf([])} />);
    fireEvent.click(button(/New demo quote/));
    const dialog = within(screen.getByRole("dialog"));
    expect(dialog.getByText("Choose the customer first.")).toBeDefined();
    fireEvent.click(dialog.getByRole("button", { name: "Get demo quote" }));
    expect(dialog.getByText("Choose a customer")).toBeDefined();
    expect(dialog.getByText("Choose 1–100 units")).toBeDefined();
    expect(actions.quoteOffset).not.toHaveBeenCalled();
    fireEvent.change(dialog.getByLabelText(/^Customer/), { target: { value: "cust-a" } });
    expect(dialog.queryByRole("checkbox", { name: "Hotel AC" })).toBeNull(); // only the chosen customer's units
    fireEvent.click(dialog.getByRole("checkbox", { name: "Lobby AC" }));
    fireEvent.click(dialog.getByRole("checkbox", { name: "Office AC" }));
    fireEvent.click(dialog.getByRole("checkbox", { name: "Office AC" }));
    fireEvent.change(dialog.getByLabelText(/Amount/), { target: { value: "1.25" } });
    const [from, to] = dialog.getAllByLabelText("date and time");
    if (!from || !to) throw new Error("no period pickers");
    fireEvent.change(from, { target: { value: "2026-08-01T00:00" } });
    fireEvent.change(to, { target: { value: "2026-09-01T00:00" } });
    fireEvent.change(dialog.getByLabelText(/^Purpose/), { target: { value: " August " } });
    fireEvent.click(dialog.getByRole("button", { name: "Get demo quote" }));
    await flush();
    expect(actions.quoteOffset).toHaveBeenCalledWith({ customerId: "cust-a", purpose: "August", amountKg: 1.25, period: { from: "2026-07-31T16:00:00.000Z", to: "2026-08-31T16:00:00.000Z" }, unitIds: ["u-1"] });
    expect(dialog.getByText("Demo quote quo-2222 · v1")).toBeDefined();
    const requestButton = dialog.getByRole<HTMLButtonElement>("button", { name: "Request (demo)" });
    expect(requestButton.disabled).toBe(true);
    fireEvent.click(requestButton);
    expect(actions.requestOffset).not.toHaveBeenCalled();
    fireEvent.click(dialog.getByRole("checkbox", { name: "I confirm no real transaction takes place." }));
    fireEvent.click(requestButton);
    await flush();
    expect(actions.requestOffset).toHaveBeenCalledWith("quo-22223333", 1);
    expect(nav.patch).toHaveBeenLastCalledWith({ recordId: "rec-new" });
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(nav.toasts).toStrictEqual(["Demo quote ready — valid for 15 minutes", "Demo request recorded"]);
  });

  it("drops a quote when the draft changes, and closes with Cancel", async () => {
    actions.quoteOffset.mockResolvedValue({ ok: true, value: quote });
    render(<OffsetsView live={liveOf([])} />);
    fireEvent.click(button(/New demo quote/));
    const dialog = within(screen.getByRole("dialog"));
    fireEvent.change(dialog.getByLabelText(/^Customer/), { target: { value: "cust-a" } });
    fireEvent.click(dialog.getByRole("checkbox", { name: "Lobby AC" }));
    fireEvent.change(dialog.getByLabelText(/Amount/), { target: { value: "2" } });
    const [from, to] = dialog.getAllByLabelText("date and time");
    if (!from || !to) throw new Error("no period pickers");
    fireEvent.change(from, { target: { value: "2026-08-01T00:00" } });
    fireEvent.change(to, { target: { value: "2026-09-01T00:00" } });
    fireEvent.change(dialog.getByLabelText(/^Purpose/), { target: { value: "August" } });
    fireEvent.click(dialog.getByRole("button", { name: "Get demo quote" }));
    await flush();
    expect(dialog.getByRole("button", { name: "Request (demo)" })).toBeDefined();
    fireEvent.change(dialog.getByLabelText(/Amount/), { target: { value: "3" } });
    expect(dialog.queryByRole("button", { name: "Request (demo)" })).toBeNull(); // the old quote no longer matches
    fireEvent.click(dialog.getByRole("button", { name: "Cancel" }));
    expect(screen.queryByRole("dialog")).toBeNull();
  });
});

describe("the offsets demo", () => {
  it("moves a fixture record from purchase to retirement, retries a failed one and adds a quote", () => {
    render(<OffsetsDemo />);
    // offset-0198 opens at the retirement step
    const retire = screen.getByRole<HTMLButtonElement>("button", { name: "Retire (demo)" });
    expect(retire.disabled).toBe(true);
    fireEvent.click(screen.getByRole("checkbox", { name: "I understand this is a demo retirement. It does not retire a real credit." }));
    fireEvent.click(retire);
    expect(nav.toasts).toStrictEqual(["Demo retired"]);
    expect(screen.getByText("DEMO-CERT-0231 (demo — not a real certificate)")).toBeDefined();
    fireEvent.click(button(/offset-0212/));
    fireEvent.click(button("Confirm simulated purchase"));
    expect(screen.getByRole("button", { name: "Retire (demo)" })).toBeDefined();
    fireEvent.click(button(/offset-0175/));
    fireEvent.click(button("Retry purchase"));
    expect(nav.toasts.at(-1)).toBe("Retry started (attempt-02)");
    expect(screen.getAllByText("Retry sent").length).toBeGreaterThan(0);
    fireEvent.click(button(/New demo quote/));
    const dialog = within(screen.getByRole("dialog"));
    fireEvent.change(dialog.getByLabelText(/Amount/), { target: { value: "0" } });
    expect(dialog.getByText("Positive numbers only")).toBeDefined();
    expect(dialog.getByRole<HTMLButtonElement>("button", { name: "Create quote" }).disabled).toBe(true);
    fireEvent.change(dialog.getByLabelText(/Amount/), { target: { value: "2.5" } });
    fireEvent.change(dialog.getByLabelText(/^Customer/), { target: { value: "customer-c" } });
    fireEvent.click(dialog.getByRole("button", { name: "Create quote" }));
    expect(screen.getByText("customer-c · Demo offset · 2.500 kgCO₂e")).toBeDefined();
    expect(screen.getByRole("tab", { name: /Demo records/ }).textContent).toBe("Demo records5");
    fireEvent.click(screen.getByRole("tab", { name: /Market concept/ }));
    expect(screen.getByText("A future carbon market is a concept only. Nothing here is tradable, priced or balance-carrying.")).toBeDefined();
  });

  it("closes the quote dialog with Cancel or Escape and keeps the list", () => {
    render(<OffsetsDemo />);
    fireEvent.click(button(/New demo quote/));
    fireEvent.click(within(screen.getByRole("dialog")).getByRole("button", { name: "Cancel" }));
    expect(screen.queryByRole("dialog")).toBeNull();
    fireEvent.click(button(/New demo quote/));
    fireEvent.keyDown(screen.getByRole("dialog"), { key: "Escape" });
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(screen.getByRole("tab", { name: /Demo records/ }).textContent).toBe("Demo records4");
  });
});
