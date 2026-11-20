// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import type { ComponentProps } from "react";
import type { ActionFailure } from "@ac/web/lib/actionMessage";
import type { ApiOffsetRecord, ApiQuote } from "@ac/web/lib/offsets";

// Carbon offsets (FR-C13) in API mode: the estimate from Energy & cost, the amount and purpose checks, the demo quote
// (offsets.preview), its confirmation and the demo request (offsets.simulate request), an expired quote (CONFLICT) that
// starts again with a new quote to confirm, the demo records and the market concept in the display language.
type Quote = ApiQuote & { estimatedAmountMinor: number | null; currency: string | null };
type Done<T> = { ok: true; value: T } | ({ ok: false } & ActionFailure);
const actions = vi.hoisted(() => ({
  getQuote: vi.fn<(input: unknown) => Promise<Done<Quote>>>(),
  requestOffset: vi.fn<(quoteId: string, quoteVersion: number) => Promise<Done<ApiOffsetRecord>>>(),
}));
vi.mock("../app/customer/energy/offsets/actions", () => actions);
const runs = vi.hoisted((): { toasts: string[] } => ({ toasts: [] }));
vi.mock("@ac/web/lib/useAction", () => ({
  useAction: () => [false, <T,>(fn: () => Promise<Done<T>>, ok: string, after?: (v: T) => void, failed?: (f: ActionFailure) => void) => {
    void fn().then((r) => {
      if (r.ok) { runs.toasts.push(ok); after?.(r.value); } else failed?.(r);
    });
  }] as const,
}));

import { OffsetsView } from "../app/customer/energy/offsets/_components/offsets-view";
import { I18nProvider } from "@ac/web/components/I18n";

type Live = ComponentProps<typeof OffsetsView>["live"];
const live = (over: Partial<Live> = {}): Live => ({
  unitIds: ["u-1"], subject: "Bedroom AC", period: { from: "2026-09-08T16:00:00.000Z", to: "2026-09-15T01:12:00.000Z", label: "Sep 9 – Sep 15 (7 days)" },
  estimate: { emissions: "10.0 kgCO₂e", savedEmissions: "Reduction 2.0 kgCO₂e", factor: "MY 2026", comparable: true }, back: "unitIds=u-1&period=7d&baselineId=none",
  records: [
    { id: "off-1-aaaaaaaa", amountKg: 1.5, state: "demo_retired", proof: "DEMO-CERT-1", createdAt: "2026-09-14T02:00:00Z" },
    { id: "off-2-bbbbbbbb", amountKg: 2, state: "failed", proof: null, createdAt: "2026-09-13T02:00:00Z" },
  ],
  ...over,
});
const quote = (over: Partial<Quote> = {}): Quote => ({
  id: "quote-123456789", version: 2, customerId: "org-a", purpose: "Demo offset", amountKg: 1.5, period: { from: "2026-09-08T16:00:00.000Z", to: "2026-09-15T01:12:00.000Z" }, unitIds: ["u-1"],
  expiresAt: "2026-09-15T01:27:00Z", provider: "unselected", scheme: "demo", marketConcept: { stage: "future_concept", providerLabel: "Provider X", verificationStatus: "unverified", ledgerStatus: "not_connected" },
  estimatedAmountMinor: null, currency: null, ...over,
});
const record = (over: Partial<ApiOffsetRecord> = {}): ApiOffsetRecord => ({
  id: "off-9-zzzzzzzz", version: 1, createdAt: "2026-09-15T01:14:00Z", quoteId: "quote-123456789", customerId: "org-a", amountKg: 1.5, attempts: [], currentAttemptId: null, state: "demo_requested",
  previousState: null, purchaseRef: null, retirementRef: null, demoCertificateRef: null, eventHistory: [], ...over,
});
const amountBox = () => screen.getByRole("textbox", { name: /Amount/ });
const button = (name: string) => screen.getByRole("button", { name });
const isOff = (name: string) => button(name).hasAttribute("disabled");
const concept = () => screen.getByText(/^Concept only|^コンセプトのみ/).textContent;
/** the amount typed, the quote asked for, and Continue to the confirmation */
async function toConfirm() {
  fireEvent.change(amountBox(), { target: { value: "1.5" } });
  fireEvent.click(button("Get demo quote"));
  await waitFor(() => { expect(screen.getByText("Demo quote")).toBeDefined(); });
  fireEvent.click(button("Continue"));
}

beforeEach(() => {
  runs.toasts = [];
  actions.getQuote.mockReset().mockResolvedValue({ ok: true, value: quote() });
  actions.requestOffset.mockReset().mockResolvedValue({ ok: true, value: record() });
});
afterEach(() => { cleanup(); });

describe("OffsetsView", () => {
  it("shows the estimate, the way back with the selection, the explanation and the records", () => {
    render(<OffsetsView live={live()} />);
    expect(screen.getByRole("link", { name: /Energy & cost/ }).getAttribute("href")).toBe("/customer/energy?unitIds=u-1&period=7d&baselineId=none");
    expect(screen.getByText("Bedroom AC · Sep 9 – Sep 15 (7 days) · MY 2026")).toBeDefined();
    expect(screen.getByText("Reduction 2.0 kgCO₂e").className).toBe("text-ok");
    const what = button("What is a carbon offset?");
    expect(what.getAttribute("aria-expanded")).toBe("false");
    fireEvent.click(what);
    expect(screen.getByText(/An offset retires a carbon credit/)).toBeDefined();
    fireEvent.click(what);
    expect(screen.queryByText(/An offset retires a carbon credit/)).toBeNull();
    const records = within(screen.getByText("Demo offset records").closest("[data-slot=card]") ?? document.body);
    const rows = records.getAllByRole("listitem").map((li) => li.textContent);
    expect(rows).toHaveLength(2);
    expect(rows.at(0)).toMatch(/^off-1-aa · 1\.500 kgCO₂e · .*Proof: DEMO-CERT-1Demo retired$/);
    expect(rows.at(1)).toMatch(/Proof: Not issuedFailed$/);
    expect(concept()).toBe("Concept only — provider not selected, verification unverified, ledger not connected. Integration partners and verification conditions are undecided; nothing here is tradable.");
  });

  it("without an estimate or records, says so; a savings figure that is not comparable is muted", () => {
    const { rerender } = render(<OffsetsView live={live({ estimate: null, records: [] })} />);
    expect(screen.getByText(/No estimate for this selection/)).toBeDefined();
    expect(screen.getByText("Bedroom AC · Sep 9 – Sep 15 (7 days)")).toBeDefined();
    expect(screen.getByText("No demo record yet.")).toBeDefined();
    rerender(<OffsetsView live={live({ estimate: { emissions: "10.0 kgCO₂e", savedEmissions: "Cannot calculate", factor: "MY 2026", comparable: false } })} />);
    expect(screen.getByText("Cannot calculate").className).toBe("text-muted-foreground");
  });

  it("checks the amount (above 0, at most 100000, up to 3 decimals) and the purpose (1–1000 characters)", () => {
    render(<OffsetsView live={live()} />);
    const check = "Above 0, at most 100000, up to 3 decimals";
    for (const bad of ["0", "abc", "1.2345", "100000.001", "-1", ""]) {
      fireEvent.change(amountBox(), { target: { value: bad } });
      expect(screen.getByText(check)).toBeDefined();
      expect(isOff("Get demo quote")).toBe(true);
    }
    for (const good of ["0.001", " 100000 ", "12.5"]) {
      fireEvent.change(amountBox(), { target: { value: good } });
      expect(screen.queryByText(check)).toBeNull();
    }
    const purpose = screen.getByRole("textbox", { name: "Purpose" });
    expect(purpose).toHaveProperty("value", "Demo offset");
    fireEvent.change(purpose, { target: { value: " ".repeat(3) } });
    expect(screen.getByText("1–1000 characters")).toBeDefined();
    expect(isOff("Get demo quote")).toBe(true);
    fireEvent.change(purpose, { target: { value: "x".repeat(1001) } });
    expect(screen.getByText("1–1000 characters")).toBeDefined();
    fireEvent.change(purpose, { target: { value: "x".repeat(1000) } });
    expect(screen.queryByText("1–1000 characters")).toBeNull();
    expect(isOff("Get demo quote")).toBe(false);
  });

  it("asks a quote for the selection, shows it with its market concept, and records the confirmed demo request", async () => {
    actions.getQuote.mockResolvedValueOnce({ ok: true, value: quote({ estimatedAmountMinor: 1250, currency: "MYR", marketConcept: { stage: "future_concept", providerLabel: "Provider X", verificationStatus: "pending_review", ledgerStatus: "not_connected" } }) });
    render(<OffsetsView live={live()} />);
    fireEvent.change(screen.getByRole("textbox", { name: "Purpose" }), { target: { value: "  Office cooling  " } });
    fireEvent.change(amountBox(), { target: { value: "1.5" } });
    fireEvent.click(button("Get demo quote"));
    await waitFor(() => { expect(screen.getByText("Demo quote")).toBeDefined(); });
    expect(actions.getQuote).toHaveBeenCalledWith({ purpose: "Office cooling", amountKg: 1.5, period: { from: "2026-09-08T16:00:00.000Z", to: "2026-09-15T01:12:00.000Z" }, unitIds: ["u-1"] });
    expect(runs.toasts).toStrictEqual(["Demo quote ready — no record was created"]);
    expect(screen.getByText("1.500 kgCO₂e")).toBeDefined();
    expect(screen.getByText("2026-09-09 00:00 → 2026-09-15 09:12 (Asia/Kuala_Lumpur) · Bedroom AC")).toBeDefined();
    expect(screen.getByText("quote-12 (demo)")).toBeDefined();
    expect(screen.getByText(/^MYR\s?12\.50 \(demo\)$|^12\.50 MYR \(demo\)$/)).toBeDefined();
    // a code without a word stays as it is; a known one is worded
    expect(concept()).toBe("Concept only — provider Provider X, verification pending_review, ledger not connected. Integration partners and verification conditions are undecided; nothing here is tradable.");
    fireEvent.click(button("Back"));
    expect(screen.getByText("How much would you like to offset?")).toBeDefined();
    fireEvent.click(button("Get demo quote"));
    await waitFor(() => { expect(screen.getByText("Demo quote")).toBeDefined(); });
    expect(screen.getByText("No price — no offset provider is selected in this demo")).toBeDefined();
    fireEvent.click(button("Continue"));
    expect(screen.getByText("Confirm demo request")).toBeDefined();
    expect(isOff("Submit demo request")).toBe(true);
    fireEvent.click(button("Back"));
    fireEvent.click(button("Continue"));
    fireEvent.click(screen.getByRole("checkbox"));
    fireEvent.click(button("Submit demo request"));
    await waitFor(() => { expect(screen.getByText(/off-9-zz created · status Demo requested · proof Not issued/)).toBeDefined(); });
    expect(actions.requestOffset).toHaveBeenCalledWith("quote-123456789", 2);
    expect(screen.queryByText("How much would you like to offset?")).toBeNull();
    fireEvent.click(button("Start another"));
    expect(screen.getByText("How much would you like to offset?")).toBeDefined();
    expect(screen.queryByText("Demo request recorded")).toBeNull();
  });

  it("a recorded request names its demo certificate", async () => {
    actions.requestOffset.mockResolvedValueOnce({ ok: true, value: record({ demoCertificateRef: "DEMO-CERT-9", state: "demo_retired" }) });
    render(<OffsetsView live={live()} />);
    await toConfirm();
    fireEvent.click(screen.getByRole("checkbox"));
    fireEvent.click(button("Submit demo request"));
    await waitFor(() => { expect(screen.getByText(/status Demo retired · proof DEMO-CERT-9/)).toBeDefined(); });
  });

  it("an expired quote (CONFLICT) starts again, and the new quote is confirmed again; another refusal stays on the confirmation", async () => {
    actions.requestOffset.mockResolvedValueOnce({ ok: false, code: "VALIDATION", messageKey: "error.validation", fieldErrors: {} });
    actions.requestOffset.mockResolvedValueOnce({ ok: false, code: "CONFLICT", messageKey: "error.conflict", fieldErrors: {} });
    render(<OffsetsView live={live()} />);
    await toConfirm();
    fireEvent.click(screen.getByRole("checkbox"));
    fireEvent.click(button("Submit demo request"));
    await waitFor(() => { expect(actions.requestOffset).toHaveBeenCalledOnce(); });
    expect(screen.getByText("Confirm demo request")).toBeDefined();
    fireEvent.click(button("Submit demo request"));
    await waitFor(() => { expect(screen.getByText("How much would you like to offset?")).toBeDefined(); });
    expect(screen.queryByText("Demo quote")).toBeNull();
    await toConfirm();
    expect(within(screen.getByText("Confirm demo request").closest("[data-slot=card]") ?? document.body).getByRole("checkbox").getAttribute("aria-checked")).toBe("false");
    expect(isOff("Submit demo request")).toBe(true);
  });

  it("the quote's market concept codes are worded in the display language, as before a quote", async () => {
    render(<I18nProvider display={{ locale: "ja", timeZone: "Asia/Kuala_Lumpur" }}><OffsetsView live={live()} /></I18nProvider>);
    expect(concept()).toContain("検証 未検証、台帳 未接続");
    fireEvent.click(button("デモの見積もりを取得")); // Get demo quote
    await waitFor(() => { expect(actions.getQuote).toHaveBeenCalledOnce(); });
    await waitFor(() => { expect(concept()).toContain("提供事業者 Provider X、検証 未検証、台帳 未接続"); });
  });

  it("a refused quote stays on the amount", async () => {
    actions.getQuote.mockResolvedValueOnce({ ok: false, code: "VALIDATION", messageKey: "error.validation", fieldErrors: { amountKg: "out_of_range" } });
    render(<OffsetsView live={live()} />);
    fireEvent.click(button("Get demo quote"));
    await waitFor(() => { expect(actions.getQuote).toHaveBeenCalledOnce(); });
    expect(screen.getByText("How much would you like to offset?")).toBeDefined();
    expect(runs.toasts).toStrictEqual([]);
  });
});
