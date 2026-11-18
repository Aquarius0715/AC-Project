// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";

// The contractor's unit view (FR-P04, FR-P08): the history snapshot after the delegation, and the live view — the
// register, the job context with its technician and access window, past work, alert evidence, readings and charts —
// with their empty states; and the Phase 1A demo view's delegation-ended preview
import { PartnerUnitDetail } from "../app/partner/units/[id]/_components/unit-detail";
import { MockPartnerUnit } from "../app/partner/units/[id]/_components/mock-unit";
import type { UnitLive } from "../app/partner/units/[id]/_lib/load";

type Live = Extract<UnitLive, { kind: "live" }>;

beforeEach(() => {
  // recharts' ResponsiveContainer measures its box
  vi.stubGlobal("ResizeObserver", class { observe() { /* jsdom has no layout */ } unobserve() { /* none */ } disconnect() { /* none */ } });
});
afterEach(() => { cleanup(); vi.unstubAllGlobals(); });

const jobId = "0193a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a5b";
const context = (over: Partial<NonNullable<Live["context"]>> = {}): NonNullable<Live["context"]> => ({
  id: jobId, rows: [["Job", "0193a1b2 · Reactive"], ["Technician", "Aina"], ["Access window", "14 Sep 01:00 → 22 Sep 00:00"]],
  techMissing: false, pct: 40, note: "Access ends in 14 h", ...over,
});
const live = (over: Partial<Live> = {}): Live => ({
  kind: "live", name: "Rooftop unit", id: "0193ffff-0000-7000-8000-000000000001",
  banner: "Diagnosis-scoped view for 0193a1b2 — no billing, payments, or other contracts.",
  register: { connection: "online", rows: [["Model", "cap-split-std v2"]], lastSeen: "14 Sep 09:35" },
  context: context(),
  past: [{ id: "job-old", title: "job-old · Preventive", sub: "12 Jun 2026", badge: { text: "Completed", tone: "ok" } }],
  alerts: [
    { id: "a1", title: "Vibration anomaly", severity: "critical", resolved: false, sub: "Suspected — amplitude 2.1x · 13 Sep 22:40" },
    { id: "a2", title: "Pressure low", severity: "warning", resolved: true, sub: "Inspection record · 12 Sep 08:10" },
  ],
  readings: {
    rows: [{ label: "Supply air", value: "14.2 °C · good" }, { label: "Pressure", value: "3.1 bar · suspect", tone: "warn" }, { label: "Vibration", value: "2.1 · bad", tone: "crit" }],
    observed: "14 Sep 09:35",
  },
  charts: [{ metric: "vibration", title: "Vibration amplitude", now: "now 2.1", labels: ["a", "b"], values: [1, 2.1] }],
  window: "13 Sep 09:35 → 14 Sep 09:35",
  ...over,
});
/** The live view with the access window used up to pct (null: no window). */
const renderAt = (pct: number | null) => render(<PartnerUnitDetail live={live({ context: context({ pct }) })} />);
/** The card whose heading is the title. */
const card = (title: string) => {
  const heading = screen.getByText(title);
  const box = heading.closest("[data-slot=card]");
  if (!(box instanceof HTMLElement)) throw new TypeError(`no card ${title}`);
  return within(box);
};

describe("Partner unit view", () => {
  it("shows the history snapshot with its banner, rows and the way back to Jobs, and nothing live", () => {
    type Snapshot = Extract<UnitLive, { kind: "snapshot" }>;
    const snapshot: Snapshot = { jobId, kind: "snapshot", title: "Job history snapshot — 0193a1b2", rows: [["Status", "Completed"]], banner: "Access to this unit ended 22 Sep 00:00", id: "u1" };
    render(<PartnerUnitDetail live={snapshot} />);
    expect(screen.getByText("Access to this unit ended 22 Sep 00:00")).toBeDefined();
    expect(screen.getByText("Job history snapshot — 0193a1b2")).toBeDefined();
    expect(screen.getByText("Completed")).toBeDefined();
    expect(screen.getByRole("link", { name: "Back to Jobs" }).getAttribute("href")).toBe("/partner/jobs");
    expect(screen.queryByText("Unit register")).toBeNull();
  });

  it("links back to the job of the context, shows the register with its connection and the short id", () => {
    render(<PartnerUnitDetail live={live()} />);
    expect(screen.getByRole("link", { name: "Back" }).getAttribute("href")).toBe(`/partner/jobs/${jobId}`);
    expect(screen.getByText("· 0193ffff")).toBeDefined();
    const register = card("Unit register");
    expect(register.getByText("cap-split-std v2")).toBeDefined();
    expect(register.getByText("Online")).toBeDefined();
    expect(register.getByText("14 Sep 09:35")).toBeDefined();
    expect(card("Job context").getByRole("link", { name: "Job" }).getAttribute("href")).toBe(`/partner/jobs/${jobId}`);
  });

  it("highlights the technician row only when no technician is assigned", () => {
    const view = render(<PartnerUnitDetail live={live()} />);
    expect(screen.getByText("Aina").classList.contains("text-crit")).toBe(false);
    view.unmount();
    render(<PartnerUnitDetail live={live({ context: context({ techMissing: true, rows: [["Job", "0193a1b2 · Reactive"], ["Technician", "Not assigned yet"], ["Access window", "—"]] }) })} />);
    expect(screen.getByText("Not assigned yet").classList.contains("text-crit")).toBe(true);
    expect(screen.getByText("0193a1b2 · Reactive").classList.contains("text-crit")).toBe(false);
  });

  it("shows the access bar in the primary tone below 80 %, crit from 80 %, and none without a window", () => {
    const bar = () => screen.queryByRole("progressbar", { name: "Access ends in 14 h" });
    let view = renderAt(79);
    expect(bar()?.className.includes("bg-crit")).toBe(false);
    view.unmount();
    view = renderAt(80);
    expect(bar()?.className.includes("bg-crit")).toBe(true);
    view.unmount();
    renderAt(null);
    expect(bar()).toBeNull();
    expect(screen.getByText("Access ends in 14 h")).toBeDefined();
  });

  it("without a job context links back to Jobs and says so, with no Job link", () => {
    render(<PartnerUnitDetail live={live({ context: null })} />);
    expect(screen.getByRole("link", { name: "Back" }).getAttribute("href")).toBe("/partner/jobs");
    const job = card("Job context");
    expect(job.getByText("No current job of your company on this unit.")).toBeDefined();
    expect(job.queryByRole("link")).toBeNull();
  });

  it("lists past work, alert evidence with the resolved one dimmed, readings with their tones and the charts", () => {
    render(<PartnerUnitDetail live={live()} />);
    const past = card("Our past work on this unit");
    expect(past.getByText("job-old · Preventive")).toBeDefined();
    expect(past.getByText("Completed")).toBeDefined();
    const alerts = card("Alert evidence");
    expect(alerts.getByText("Vibration anomaly").closest(".opacity-70")).toBeNull();
    expect(alerts.getByText("Pressure low").closest(".opacity-70")).not.toBeNull();
    expect(alerts.getByText("Critical")).toBeDefined();
    const readings = card("Readings (diagnosis only)");
    expect(readings.getByText("observed 14 Sep 09:35")).toBeDefined();
    expect(readings.getByText("14.2 °C · good").className).toBe("font-semibold");
    expect(readings.getByText("3.1 bar · suspect").classList.contains("text-warn")).toBe(true);
    expect(readings.getByText("2.1 · bad").classList.contains("text-crit")).toBe(true);
    const charts = card("Readings — last 24 h");
    expect(charts.getByText("13 Sep 09:35 → 14 Sep 09:35")).toBeDefined();
    expect(charts.getByText("Vibration amplitude")).toBeDefined();
    expect(charts.getByText("now 2.1")).toBeDefined();
  });

  it("shows the empty states of past work, alerts, readings and charts", () => {
    render(<PartnerUnitDetail live={live({ past: [], alerts: [], readings: { rows: [], observed: "—" }, charts: [] })} />);
    expect(screen.getByText("No past jobs.")).toBeDefined();
    expect(screen.getByText("No alerts on this unit.")).toBeDefined();
    expect(card("Readings (diagnosis only)").getByText("No measurements yet")).toBeDefined();
    expect(screen.getByText("No diagnosis readings for this unit.")).toBeDefined();
  });
});

describe("Partner unit demo view", () => {
  it("hides the readings and shows the banner while previewing an ended delegation, and shows them again", () => {
    render(<MockPartnerUnit id="unit-p02" />);
    expect(screen.getByText("Rooftop unit · unit-p02")).toBeDefined();
    expect(screen.getByText("Readings (diagnosis only)")).toBeDefined();
    fireEvent.click(screen.getByRole("button", { name: "preview: delegation ended" }));
    expect(screen.getByText(/Delegation ended while open/)).toBeDefined();
    expect(screen.queryByText("Readings (diagnosis only)")).toBeNull();
    expect(screen.queryByText("Readings — last 24 h")).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "show live" }));
    expect(screen.queryByText(/Delegation ended while open/)).toBeNull();
    expect(screen.getByText("Readings — last 24 h")).toBeDefined();
  });
});
