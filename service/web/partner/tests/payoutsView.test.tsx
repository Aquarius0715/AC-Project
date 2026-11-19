// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import type { ActionFailure } from "@ac/web/lib/actionMessage";
import type { ApiPayoutStatement, LineRow } from "@ac/web/lib/partnerPayouts";

// The contractor's payout statements (FR-P10): the first-statement and filtered empty states, the filters, a question
// to HQ on a line (the 1–2000 check, the refusals) and the PDF download
const actions = vi.hoisted(() => ({ askHq: vi.fn() }));
vi.mock("../app/partner/payouts/actions", () => actions);
const nav = vi.hoisted(() => vi.fn());
vi.mock("@ac/web/lib/useUrlPatch", () => ({ useUrlPatch: () => nav }));
type Result = { ok: true; value: unknown } | ({ ok: false } & ActionFailure);
vi.mock("@ac/web/lib/useAction", () => ({
  useAction: () => [false, (fn: () => Promise<Result>, _ok: unknown, after?: () => void, failed?: (f: ActionFailure) => void) => {
    void fn().then((r) => { if (r.ok) after?.(); else failed?.(r); });
  }] as const,
}));

import { PayoutsView, type PayoutsLive } from "../app/partner/payouts/_components/payouts-view";

beforeEach(() => { actions.askHq.mockReset(); nav.mockReset(); });
afterEach(() => { cleanup(); vi.restoreAllMocks(); vi.unstubAllGlobals(); });

const statement: ApiPayoutStatement = {
  id: "0193a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a5b", version: 4, contractorOrgId: "org-p", period: "2026-10", status: "approved", currency: "MYR",
  grossMinor: 30_000, deductionsMinor: 5000, netMinor: 25_000, payDate: "2026-11-15T00:00:00Z", lines: [], queries: [], approvedByMembershipId: null, paidAt: null,
};
const line = (over: Partial<LineRow>): LineRow => ({
  key: "l1", lineId: "line-1", jobId: "job-1", job: "job-1", work: "Periodic inspection", sub: null, accepted: "3 Oct", status: "accepted",
  statusText: "Accepted", tone: "ok", amount: "300.00", question: null, askable: true, ...over,
});
const rows: LineRow[] = [
  line({}),
  line({ key: "l2", lineId: "line-2", jobId: "job-2", job: "job-2", work: "Rework deduction", status: "deduction", statusText: "Deduction", amount: "− 50.00" }),
  line({ key: "l3", lineId: "line-3", jobId: "job-3", job: "job-3", question: { id: "q1", lineId: "line-3", topic: "amount", message: "Why?", state: "open", reply: null, adjustmentMinor: null } }),
  line({ key: "review-job-4", lineId: null, jobId: "job-4", job: "job-4", status: "not_included", statusText: "Not included", amount: "—", askable: false }),
];
const live = (over: Partial<PayoutsLive> = {}): PayoutsLive => ({
  periods: [{ value: "2026-10", text: "Oct 2026" }], period: "", status: "", company: "Cool Air Sdn Bhd",
  topics: [{ id: "amount", text: "Amount" }, { id: "deduction", text: "Deduction" }, { id: "other", text: "Other" }],
  statements: [{ id: statement.id, period: "2026-10", label: "Oct 2026 · 0193a1b2", sub: "MYR 250.00", badge: { label: "Approved", tone: "primary" } }],
  selected: {
    statement, rows, kpis: [{ label: "Net payable", value: "MYR 250.00", sub: "", badge: { label: "Approved", tone: "primary" } }, { label: "Gross", value: "MYR 300.00", sub: "rate card v2" }],
    questions: [{ id: "q1", title: "Amount · job-3", message: "Why?", state: "Waiting for HQ", reply: "Checked", tone: "warn" }],
    title: "Statement Oct 2026", sub: "Cool Air", totals: [["Net", "MYR 250.00"]],
  },
  ...over,
});
const question = () => screen.getByRole("textbox");
/** Opens the question dialog from the i-th "Ask HQ" button of the lines table. */
const openAsk = (i: number) => {
  const button = screen.getAllByRole("button", { name: "Ask HQ" }).at(i);
  if (!button) throw new Error(`no Ask HQ button ${String(i)}`);
  fireEvent.click(button);
};

describe("Payouts view", () => {
  it("shows one empty state before the first statement, and the filtered one with a way to clear the filters", () => {
    const view = render(<PayoutsView live={live({ statements: [], selected: null })} />);
    expect(screen.getAllByText("No statements yet")).toHaveLength(1);
    expect(screen.queryByRole("combobox", { name: "Period" })).toBeNull();
    view.rerender(<PayoutsView live={live({ statements: [], selected: null, status: "paid" })} />);
    expect(screen.getByText("No statement matches these filters.")).toBeTruthy();
    expect(screen.getByText("No statement to show")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Clear the filters" }));
    expect(nav).toHaveBeenCalledWith({ period: null, status: null, statementId: null });
  });

  it("moves the filters and the selection through the URL, clearing the open statement on a filter change", () => {
    render(<PayoutsView live={live()} />);
    expect(screen.queryByRole("button", { name: "Clear the filters" })).toBeNull();
    fireEvent.change(screen.getByRole("combobox", { name: "Period" }), { target: { value: "2026-10" } });
    expect(nav).toHaveBeenLastCalledWith({ period: "2026-10", statementId: null });
    fireEvent.change(screen.getByRole("combobox", { name: "Status" }), { target: { value: "" } });
    expect(nav).toHaveBeenLastCalledWith({ status: null, statementId: null });
    fireEvent.click(screen.getByText("Oct 2026 · 0193a1b2"));
    expect(nav).toHaveBeenLastCalledWith({ statementId: statement.id });
  });

  it("offers Ask HQ only on approved lines without a question, and names the others' state", () => {
    render(<PayoutsView live={live()} />);
    expect(screen.getAllByRole("button", { name: "Ask HQ" })).toHaveLength(2);
    expect(screen.getByText("Question open")).toBeTruthy();
    expect(screen.getByText("next statement")).toBeTruthy();
    expect(screen.getByText(/HQ: Checked/)).toBeTruthy();
  });

  it("refuses an empty question in the browser, then sends the trimmed text with the statement version and closes", async () => {
    actions.askHq.mockResolvedValue({ ok: true, value: null });
    render(<PayoutsView live={live()} />);
    openAsk(1); // the deduction line: topic deduction
    fireEvent.change(question(), { target: { value: " ".repeat(3) } });
    fireEvent.click(screen.getByRole("button", { name: "Send" }));
    expect(screen.getByText("Enter your question (1–2000 characters)")).toBeTruthy();
    expect(actions.askHq).not.toHaveBeenCalled();
    fireEvent.change(question(), { target: { value: "  Part came from HQ stock  " } });
    fireEvent.click(screen.getByRole("button", { name: "Send" }));
    expect(actions.askHq).toHaveBeenCalledWith(statement.id, 4, "line-2", "deduction", "Part came from HQ stock");
    await vi.waitFor(() => { expect(screen.queryByRole("textbox")).toBeNull(); });
  });

  it("shows the refusal of a changed statement and the message field's error from the Core API", async () => {
    actions.askHq.mockResolvedValue({ ok: false, code: "CONFLICT", messageKey: "error.conflict", fieldErrors: { message: "too long" } });
    render(<PayoutsView live={live()} />);
    openAsk(0);
    fireEvent.change(screen.getByRole("combobox", { name: "Topic" }), { target: { value: "other" } });
    fireEvent.change(question(), { target: { value: "Rate?" } });
    fireEvent.click(screen.getByRole("button", { name: "Send" }));
    expect(actions.askHq).toHaveBeenCalledWith(statement.id, 4, "line-1", "other", "Rate?");
    expect(await screen.findByText("The statement changed (or is no longer approved); the page shows the latest state.")).toBeTruthy();
    expect(screen.getByText("Enter your question (1–2000 characters)")).toBeTruthy();
  });

  it("names a statement or line of another company, and shows no banner for other refusals", async () => {
    actions.askHq.mockResolvedValueOnce({ ok: false, code: "NOT_FOUND", messageKey: "error.notFound", fieldErrors: {} });
    render(<PayoutsView live={live()} />);
    openAsk(0);
    fireEvent.change(question(), { target: { value: "Rate?" } });
    fireEvent.click(screen.getByRole("button", { name: "Send" }));
    expect(await screen.findByText("This statement or line is not available to your company.")).toBeTruthy();
    actions.askHq.mockResolvedValueOnce({ ok: false, code: "INTERNAL", messageKey: "error.internal", fieldErrors: {} });
    fireEvent.click(screen.getByRole("button", { name: "Send" }));
    await vi.waitFor(() => { expect(screen.queryByText("This statement or line is not available to your company.")).toBeNull(); });
  });

  it("lets the user switch the line in the dialog, and cancel it without sending", async () => {
    actions.askHq.mockResolvedValue({ ok: true, value: null });
    render(<PayoutsView live={live()} />);
    openAsk(0);
    fireEvent.change(screen.getByRole("combobox", { name: "Line" }), { target: { value: "line-2" } });
    fireEvent.change(question(), { target: { value: "Which part?" } });
    fireEvent.click(screen.getByRole("button", { name: "Send" }));
    expect(actions.askHq).toHaveBeenCalledWith(statement.id, 4, "line-2", "amount", "Which part?");
    await vi.waitFor(() => { expect(screen.queryByRole("textbox")).toBeNull(); });
    openAsk(0);
    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    expect(screen.queryByRole("textbox")).toBeNull();
  });

  it("marks a paid statement Paid and shows a line's second row", () => {
    const paid = { ...statement, status: "paid" as const, paidAt: "2026-11-15T02:00:00Z" };
    const sel = live().selected;
    if (!sel) throw new Error("fixture without a statement");
    render(<PayoutsView live={live({ selected: { ...sel, statement: paid, questions: [], rows: [line({ askable: false, sub: "Living room unit" })] } })} />);
    expect(screen.getByText("Paid")).toBeTruthy();
    expect(screen.getByText("Living room unit")).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Ask HQ" })).toBeNull();
    expect(screen.queryByText("Questions to HQ")).toBeNull();
  });

  it("names an answered question, a question without a reply, an empty period, and closes the dialog on Escape", async () => {
    const sel = live().selected;
    if (!sel) throw new Error("fixture without a statement");
    const answered = line({ key: "l5", lineId: "line-5", question: { id: "q2", lineId: "line-5", topic: "other", message: "Ok?", state: "answered", reply: "Yes", adjustmentMinor: null } });
    render(<PayoutsView live={live({ period: "2026-10", selected: { ...sel, rows: [line({}), answered], questions: [{ id: "q2", title: "Other · job-1", message: "Ok?", state: "Answered", reply: null, tone: "ok" }] } })} />);
    expect(screen.getAllByText("Answered")).toHaveLength(2); // the line's state and the question's badge
    expect(screen.queryByText(/HQ:/)).toBeNull();
    fireEvent.change(screen.getByRole("combobox", { name: "Period" }), { target: { value: "" } });
    expect(nav).toHaveBeenLastCalledWith({ period: null, statementId: null });
    fireEvent.click(screen.getByRole("button", { name: "Ask HQ" }));
    fireEvent.keyDown(document.activeElement ?? document.body, { key: "Escape" });
    await vi.waitFor(() => { expect(screen.queryByRole("textbox")).toBeNull(); });
  });

  it("sends nothing when the statement went away while the dialog was open", () => {
    const view = render(<PayoutsView live={live()} />);
    openAsk(0);
    fireEvent.change(question(), { target: { value: "Rate?" } });
    view.rerender(<PayoutsView live={live({ selected: null })} />);
    fireEvent.click(screen.getByRole("button", { name: "Send" }));
    expect(actions.askHq).not.toHaveBeenCalled();
  });

  it("downloads the statement as a PDF named by period and id, and frees the object URL", () => {
    const create = vi.fn(() => "blob:pdf");
    const revoke = vi.fn();
    const click = vi.spyOn(HTMLAnchorElement.prototype, "click").mockImplementation(() => { /* jsdom does not follow the download */ });
    // a subclass, so the real URL stays untouched; afterEach restores the global and the click spy
    vi.stubGlobal("URL", class extends URL { static override readonly createObjectURL = create; static override readonly revokeObjectURL = revoke; });
    render(<PayoutsView live={live()} />);
    fireEvent.click(screen.getByRole("button", { name: "Download PDF" }));
    expect(create).toHaveBeenCalledOnce();
    expect(click).toHaveBeenCalledOnce();
    expect(click.mock.contexts[0]).toMatchObject({ download: "statement-2026-10-0193a1b2.pdf" });
    expect(revoke).toHaveBeenCalledWith("blob:pdf");
  });
});
