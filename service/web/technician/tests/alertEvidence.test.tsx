// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import type { EvidenceRow } from "@ac/web/lib/alertEvidence";
import type { AlertView } from "@ac/web/lib/techAlerts";

// the technician's alert evidence in API mode (FR-T07) and its Phase 1A demo (review round 1083)
type Answer = { ok: true } | { ok: false; messageKey: string; code: string; fieldErrors: Record<string, string> };
const store = vi.hoisted((): { next: Answer | Error; toasts: string[]; refresh: ReturnType<typeof vi.fn>; replace: ReturnType<typeof vi.fn> } => ({ next: { ok: true }, toasts: [], refresh: vi.fn(), replace: vi.fn() }));
vi.mock("next/navigation", async (importOriginal) => ({
  ...(await importOriginal<Record<string, unknown>>()),
  useRouter: () => ({ refresh: store.refresh, replace: store.replace }),
  usePathname: () => "/technician/units/unit-1/alerts",
}));
vi.mock("@ac/web/components/ui", async (importOriginal) => ({ ...(await importOriginal<Record<string, unknown>>()), useToast: () => (m: string) => { store.toasts.push(m); } }));
vi.mock("../app/technician/units/[id]/alerts/actions", () => {
  const answer = () => (Error.isError(store.next) ? Promise.reject(store.next) : Promise.resolve(store.next));
  return { acknowledgeAlert: vi.fn(answer), resolveAlert: vi.fn(answer) };
});

import { acknowledgeAlert, resolveAlert } from "../app/technician/units/[id]/alerts/actions";
import { AlertEvidenceView } from "../app/technician/units/[id]/alerts/_components/alert-evidence";
import type { AlertsLive } from "../app/technician/units/[id]/alerts/_lib/load";
import { MockAlertEvidence } from "../app/technician/units/[id]/alerts/_components/mock-evidence";

const view = (over: Partial<AlertView> = {}, resolution: Partial<AlertView["resolution"]> = {}): AlertView => ({
  id: "alert-1", version: 4, status: "open", severity: "warning", policyless: false,
  banner: { title: "Bedroom too hot", sub: "≥ 30 °C for 60 s", tone: "warn", state: "Acknowledged" },
  cards: [{ kind: "Measured", title: "Sensor reading", value: "30.4 °C", sub: "observed 09:12" }],
  resolution: { info: "Automatic resolution needs a remeasurement", done: null, note: "Recurrence opens a new alert.", forbidden: "You can resolve with a remeasurement only.", ...resolution },
  history: [{ id: "h-1", time: "09:12", title: "Raised", sub: "30.4 °C", badge: { text: "Open", tone: "warn" } }, { id: "h-2", time: "09:14", title: "Notified", sub: "", badge: { text: "Sent", tone: "ok" } }],
  related: [["Job", "job-t07 · Preventive"]],
  ...over,
});
const EVIDENCE: EvidenceRow[] = [
  { id: "ev-1", kind: "remeasurement", title: "Remeasurement · Temperature 27 °C", sub: "measured · valid" },
  { id: "ev-2", kind: "detection", title: "Evidence attached to this alert", sub: "ev-2 · observed" },
];
const live = (over: Partial<AlertsLive> = {}, alert: Partial<AlertView> = {}, resolution: Partial<AlertView["resolution"]> = {}): AlertsLive => ({
  kind: "live", unitName: "Bedroom AC", unitId: "unit-1", unitHref: "/technician/units/unit-1?jobId=job-1",
  choices: [{ id: "alert-1", title: "Bedroom too hot", sub: "Open · 5 min ago", severity: "warning" }, { id: "alert-2", title: "Sensor offline", sub: "Resolved · 2 d ago", severity: "critical" }],
  count: 2, alert: view(alert, resolution), evidence: EVIDENCE, ...over,
});
const button = (name: string | RegExp) => screen.getByRole<HTMLButtonElement>("button", { name });
const REASON = "e.g. Confirmed on site that airflow is restored after filter cleaning.";
const typeReason = (text: string) => { fireEvent.change(screen.getByPlaceholderText(REASON), { target: { value: text } }); };
const refused = (messageKey: string, code: string, fieldErrors: Record<string, string> = {}): Answer => ({ ok: false, messageKey, code, fieldErrors });

beforeEach(() => {
  store.next = { ok: true }; store.toasts = []; store.refresh.mockReset(); store.replace.mockReset();
  vi.mocked(acknowledgeAlert).mockClear(); vi.mocked(resolveAlert).mockClear();
});
afterEach(() => { cleanup(); });

describe("AlertEvidenceView", () => {
  it("shows the alert, its cards, history, related rows and the unit's other alerts", () => {
    render(<AlertEvidenceView live={live()} />);
    expect(screen.getByRole("link", { name: "Back" }).getAttribute("href")).toBe("/technician/units/unit-1?jobId=job-1");
    expect(screen.getByText("· Bedroom AC")).toBeDefined();
    expect(screen.getByText("≥ 30 °C for 60 s")).toBeDefined();
    expect(screen.getByText("30.4 °C", { selector: "div" })).toBeDefined();
    expect(screen.getByText("09:12 · Raised")).toBeDefined();
    expect(screen.getByText("job-t07 · Preventive")).toBeDefined();
    expect(screen.getByText("Alerts on this unit (2)")).toBeDefined();
    expect(button(/Bedroom too hot/).className).toContain("font-bold");
    expect(button(/Sensor offline/).className).not.toContain("font-bold");
    expect(screen.getByText("Recurrence opens a new alert.")).toBeDefined();
  });

  it("shows the empty state without an alert, and no switcher for a single alert", () => {
    render(<AlertEvidenceView live={live({ alert: null, count: 0, choices: [] })} />);
    expect(screen.getByText("No alerts on this unit")).toBeDefined();
    cleanup();
    render(<AlertEvidenceView live={live({ count: 1 })} />);
    expect(screen.queryByText(/Alerts on this unit/)).toBeNull();
  });

  it("acknowledges an open alert with its version, then toasts after the new read", async () => {
    render(<AlertEvidenceView live={live()} />);
    fireEvent.click(button("Acknowledge"));
    await waitFor(() => { expect(store.toasts).toStrictEqual(["Alert acknowledged"]); });
    expect(acknowledgeAlert).toHaveBeenCalledWith("alert-1", 4);
    expect(store.refresh).toHaveBeenCalledOnce();
  });

  it("shows the state instead of Acknowledge once the alert is no longer open", () => {
    render(<AlertEvidenceView live={live({}, { status: "acknowledged" })} />);
    expect(screen.queryByRole("button", { name: "Acknowledge" })).toBeNull();
    expect(screen.getByText("Acknowledged", { selector: "b" })).toBeDefined();
  });

  it("refuses an empty reason without calling the API", () => {
    render(<AlertEvidenceView live={live()} />);
    typeReason(" ".repeat(3));
    fireEvent.click(button("Resolve alert"));
    expect(screen.getByText("A resolution reason is required (1–1000 characters).")).toBeDefined();
    expect(resolveAlert).not.toHaveBeenCalled();
  });

  it("asks for evidence on an alert without a policy, then resolves with the trimmed reason and the picked evidence", async () => {
    render(<AlertEvidenceView live={live({}, { policyless: true })} />);
    typeReason("  Airflow restored  ");
    fireEvent.click(button("Resolve alert"));
    expect(screen.getByText("Pick at least one evidence record — this alert has no policy")).toBeDefined();
    expect(resolveAlert).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("checkbox", { name: /Remeasurement/ }));
    fireEvent.click(button("Resolve alert"));
    expect(screen.queryByText("Pick at least one evidence record — this alert has no policy")).toBeNull();
    await waitFor(() => { expect(store.toasts).toStrictEqual(["Alert resolved"]); });
    expect(resolveAlert).toHaveBeenCalledWith("alert-1", 4, "Airflow restored", ["ev-1"]);
    expect(screen.getByPlaceholderText<HTMLTextAreaElement>(REASON).value).toBe("");
    expect(screen.getByRole("checkbox", { name: /Remeasurement/ }).getAttribute("aria-checked")).toBe("false");
  });

  it("resolves a policy alert by hand without evidence", async () => {
    render(<AlertEvidenceView live={live()} />);
    typeReason("Checked on site");
    fireEvent.click(button("Resolve alert"));
    await waitFor(() => { expect(store.toasts).toStrictEqual(["Alert resolved"]); });
    expect(resolveAlert).toHaveBeenCalledWith("alert-1", 4, "Checked on site", []);
  });

  it("sends no evidence when none can be cited (no alert.resolve), and shows the permission's refusal", async () => {
    store.next = refused("error.forbidden", "FORBIDDEN");
    render(<AlertEvidenceView live={live({ evidence: null }, { policyless: true })} />);
    expect(screen.queryByRole("checkbox")).toBeNull();
    typeReason("Fixed");
    fireEvent.click(button("Resolve alert"));
    expect(await screen.findByText("You can resolve with a remeasurement only.")).toBeDefined();
    expect(resolveAlert).toHaveBeenCalledWith("alert-1", 4, "Fixed", []);
    expect(store.toasts).toStrictEqual([]);
    expect(screen.getByPlaceholderText<HTMLTextAreaElement>(REASON).value).toBe("Fixed");
  });

  it("puts the evidence refusal in words before the general one", async () => {
    store.next = refused("error.validation", "VALIDATION", { resolutionEvidenceIds: "errors.evidence_unknown" });
    render(<AlertEvidenceView live={live()} />);
    typeReason("Fixed");
    fireEvent.click(button("Resolve alert"));
    expect(await screen.findByText("An evidence record is no longer a candidate of this alert — pick again from the list")).toBeDefined();
  });

  it("reads a call without an answer as not sent, keeping the reason", async () => {
    store.next = new Error("offline");
    render(<AlertEvidenceView live={live()} />);
    typeReason("Fixed");
    fireEvent.click(button("Resolve alert"));
    expect(await screen.findByText("Not sent; your reason is kept. Try again.")).toBeDefined();
    expect(screen.getByPlaceholderText<HTMLTextAreaElement>(REASON).value).toBe("Fixed");
  });

  it("shows a refused acknowledge in words", async () => {
    store.next = refused("error.versionConflict", "CONFLICT");
    render(<AlertEvidenceView live={live()} />);
    fireEvent.click(button("Acknowledge"));
    expect(await screen.findByText("The alert changed elsewhere — the latest state is shown.")).toBeDefined();
  });

  it("drops a picked record the new read no longer lists", async () => {
    const { rerender } = render(<AlertEvidenceView live={live({}, { policyless: true })} />);
    fireEvent.click(screen.getByRole("checkbox", { name: /Remeasurement/ }));
    rerender(<AlertEvidenceView live={live({ evidence: EVIDENCE.slice(1) }, { policyless: true })} />);
    typeReason("Fixed");
    fireEvent.click(button("Resolve alert"));
    expect(screen.getByText("Pick at least one evidence record — this alert has no policy")).toBeDefined();
    fireEvent.click(screen.getByRole("checkbox", { name: /Evidence attached/ }));
    fireEvent.click(button("Resolve alert"));
    await waitFor(() => { expect(resolveAlert).toHaveBeenCalledWith("alert-1", 4, "Fixed", ["ev-2"]); });
  });

  it("shows a resolved alert's outcome without the resolve form", () => {
    render(<AlertEvidenceView live={live({ evidence: null }, { status: "resolved" }, { done: "Resolved 09:30 · Airflow restored", info: null })} />);
    expect(screen.getByText("Resolved 09:30 · Airflow restored")).toBeDefined();
    expect(screen.queryByRole("button", { name: "Resolve alert" })).toBeNull();
    expect(screen.queryByText("Automatic resolution needs a remeasurement")).toBeNull();
  });

  it("switches to another alert through the URL, clearing the message", () => {
    render(<AlertEvidenceView live={live()} />);
    fireEvent.click(button("Resolve alert"));
    expect(screen.getByText("A resolution reason is required (1–1000 characters).")).toBeDefined();
    fireEvent.click(button(/Sensor offline/));
    expect(screen.queryByText("A resolution reason is required (1–1000 characters).")).toBeNull();
    expect(store.replace).toHaveBeenCalledWith("/technician/units/unit-1/alerts?alertId=alert-2", { scroll: false });
  });
});

describe("MockAlertEvidence", () => {
  it("links back to the unit and shows the fixture evidence", () => {
    render(<MockAlertEvidence id="unit-online-rto" />);
    expect(screen.getByRole("link", { name: /Unit/ }).getAttribute("href")).toBe("/technician/units/unit-online-rto");
    expect(screen.getByText("Open")).toBeDefined();
    expect(screen.getByText("Sensor reading")).toBeDefined();
  });

  it("acknowledges once, then refuses to resolve without the permission or a reason, then resolves", () => {
    render(<MockAlertEvidence id="unit-1" />);
    fireEvent.click(button("Acknowledge"));
    expect(store.toasts).toStrictEqual(["Alert acknowledged"]);
    expect(button("Acknowledge").disabled).toBe(true);
    expect(within(screen.getByRole("heading", { level: 1 }).parentElement ?? document.body).getByText("Acknowledged")).toBeDefined();
    fireEvent.click(button("Resolve alert"));
    expect(screen.getByText(/^FORBIDDEN/)).toBeDefined();
    fireEvent.click(screen.getByRole("checkbox", { name: /demo: I have alert.resolve/ }));
    fireEvent.click(button("Resolve alert"));
    expect(screen.getByText("A resolution reason is required (1–1000 characters).")).toBeDefined();
    fireEvent.change(screen.getByPlaceholderText(/Confirmed on-site/), { target: { value: "Filter cleaned" } });
    fireEvent.click(button("Resolve alert"));
    expect(screen.queryByText("A resolution reason is required (1–1000 characters).")).toBeNull();
    expect(screen.getByText("Resolved")).toBeDefined();
    expect(button("Resolve alert").disabled).toBe(true);
    expect(store.toasts).toStrictEqual(["Alert acknowledged", "Alert resolved after remeasurement"]);
  });
});
