// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import type { ComponentProps } from "react";
import { baselineRows, summaryView, type ApiBaseline, type ApiEnergySummary } from "@ac/web/lib/energy";
import type { ActionFailure } from "@ac/web/lib/actionMessage";

// The HQ energy analysis screens (FR-A13): the analysis scope, baseline and period in the URL, the comparison and its
// quality notes; the baselines tab's scope, list and editor (a new baseline, the next version, a fixed or measured
// value, delete with a reason). The Phase 1A demo moves locally.
const actions = vi.hoisted(() => ({ saveBaseline: vi.fn<(...args: unknown[]) => Promise<unknown>>(), deleteBaseline: vi.fn<(...args: unknown[]) => Promise<unknown>>() }));
const nav = vi.hoisted(() => ({ patch: vi.fn<(p: Record<string, string | null>) => void>(), toasts: [] as string[] }));
vi.mock("../app/admin/energy/actions", () => actions);
vi.mock("@ac/web/lib/useUrlPatch", () => ({ useUrlPatch: () => nav.patch }));
type Result = { ok: true; value: unknown } | ({ ok: false } & ActionFailure);
vi.mock("@ac/web/lib/useAction", () => ({
  useAction: () => [false, (fn: () => Promise<Result>, ok: string | ((v: unknown) => string), after?: (v: unknown) => void) => {
    void fn().then((r) => {
      if (!r.ok) return;
      nav.toasts.push(typeof ok === "string" ? ok : ok(r.value));
      after?.(r.value);
    });
  }] as const,
}));
vi.mock("@ac/web/components/ui", async (importOriginal) => ({ ...(await importOriginal<Record<string, unknown>>()), useToast: () => (m: string) => { nav.toasts.push(m); } }));
vi.mock("@ac/web/components/TimeSelect", async (importOriginal) => ({
  ...(await importOriginal<Record<string, unknown>>()),
  DateTimeSelect: ({ value, onChange, id }: Readonly<{ value: string; onChange: (v: string) => void; id?: string }>) => <input id={id} aria-label="date and time" value={value} onChange={(e) => { onChange(e.target.value); }} />,
}));

import { EnergyView } from "../app/admin/energy/_components/energy-view";
import { EnergyDemo } from "../app/admin/energy/_components/energy-demo";

type Live = ComponentProps<typeof EnergyView>["live"];
const baseline = (id: string, over: Partial<ApiBaseline> = {}): ApiBaseline => ({
  id, version: 2, createdAt: "2026-08-01T00:00:00Z", unitIds: ["u-1"], period: { from: "2026-08-24T16:00:00Z", to: "2026-08-31T16:00:00Z" }, method: "demo_fixed", baselineKWh: 100,
  quality: { kind: "modeled", coverage: null }, boundaryId: "ac_input_electricity", boundary: "AC input", assumptions: "Same week", source: "demo", ...over,
});
const summary = (over: Partial<ApiEnergySummary> = {}): ApiEnergySummary => ({
  period: { from: "2026-09-13T16:00:00Z", to: "2026-09-14T01:00:00Z" }, unitIds: ["u-1"],
  totals: { kWh: 120, amountMinor: 6000, savedKWh: -20, deltaKWh: 20, savingPercentage: -20, savedAmountMinor: -1000, emissionsKg: 70.8, savedEmissionsKg: -11.8 },
  currency: "MYR", baselineRef: { id: "b-1", version: 2 }, factorRef: { id: "f-1", version: 1 },
  baselineSnapshot: { id: "b-1", version: 2, createdAt: "2026-08-01T00:00:00Z", unitIds: ["u-1"], period: { from: "", to: "" }, method: "demo_fixed", baselineKWh: 100, quality: { kind: "modeled", coverage: null }, boundaryId: "ac_input_electricity", boundary: "AC input", assumptions: "x", source: "y" },
  factorSnapshot: { id: "f-1", version: 1, region: "Demo", year: 2026, kgCO2ePerKWh: 0.59, source: "demo" }, tariffVersion: "tariff-demo-1", boundaryId: "ac_input_electricity", boundary: "AC input",
  coverage: 1, qualityWarnings: ["modeled_baseline"], ...over,
});
const baselines = baselineRows([baseline("b-11112222"), baseline("b-33334444", { method: "demo_period_comparison", baselineKWh: null, boundaryId: "whole_building_electricity" })]);
/** A live screen without analysis data, as the baselines tab gets it. */
function bareLive(over: Partial<Live> = {}): Live {
  return {
    tab: "analysis", canWrite: true, scope: {}, customers: [{ id: "cust-b", name: "Beta Hotel" }, { id: "cust-a", name: "Demo Customer A" }],
    properties: [{ id: "p-1", name: "Tower", customerId: "cust-a" }, { id: "p-2", name: "Beach", customerId: "cust-b" }],
    units: [{ id: "u-1", label: "Lobby AC", customerId: "cust-a", propertyId: "p-1" }, { id: "u-2", label: "Office AC", customerId: "cust-a", propertyId: "p-1" }, { id: "u-9", label: "Hotel AC", customerId: "cust-b", propertyId: "p-2" }],
    baselines, ...over,
  };
}
const liveOf = (over: Partial<Live> = {}): Live => bareLive({ analysis: { unitIds: ["u-1"], from: "2026-09-14T00:00", to: "2026-09-14T09:00", baselineId: "b-11112222", summary: summaryView(summary()) }, ...over });
const button = (name: string | RegExp) => screen.getByRole<HTMLButtonElement>("button", { name });
const flush = () => act(() => new Promise<void>((r) => { setTimeout(r, 0); }));

beforeEach(() => {
  nav.patch.mockReset(); nav.toasts = [];
  actions.saveBaseline.mockReset(); actions.deleteBaseline.mockReset();
  history.replaceState(null, "", "/admin/energy");
});
afterEach(cleanup);

describe("the energy analysis in API mode", () => {
  it("puts the customer, baseline, period and units in the URL", () => {
    render(<EnergyView live={liveOf({ scope: { customerId: "cust-a" } })} />);
    expect(screen.queryByRole("checkbox", { name: "Hotel AC" })).toBeNull(); // the customer's units
    fireEvent.change(screen.getByLabelText("Customer"), { target: { value: "cust-b" } });
    expect(nav.patch).toHaveBeenLastCalledWith({ customerId: "cust-b", unitIds: null, baselineId: null });
    fireEvent.change(screen.getByLabelText("Customer"), { target: { value: "" } });
    expect(nav.patch).toHaveBeenLastCalledWith({ customerId: null, unitIds: null, baselineId: null });
    fireEvent.change(screen.getByLabelText("Baseline"), { target: { value: "" } });
    expect(nav.patch).toHaveBeenLastCalledWith({ baselineId: null, unitIds: null, from: null, to: null });
    fireEvent.change(screen.getByLabelText("Baseline"), { target: { value: "b-33334444" } });
    expect(nav.patch).toHaveBeenLastCalledWith({ baselineId: "b-33334444", unitIds: null, from: null, to: null });
    const apply = button("Apply period");
    expect(apply.disabled).toBe(true);
    const [from, to] = screen.getAllByLabelText("date and time");
    fireEvent.change(from ?? document.body, { target: { value: "2026-09-13T00:00" } });
    fireEvent.change(to ?? document.body, { target: { value: "2026-09-14T10:00" } });
    fireEvent.click(apply);
    expect(nav.patch).toHaveBeenLastCalledWith({ from: "2026-09-13T00:00", to: "2026-09-14T10:00" });
    fireEvent.click(screen.getByRole("checkbox", { name: "Office AC" }));
    expect(nav.patch).toHaveBeenLastCalledWith({ unitIds: "u-1,u-2" });
    fireEvent.click(screen.getByRole("checkbox", { name: "Lobby AC" }));
    expect(nav.patch).toHaveBeenLastCalledWith({ unitIds: null }); // none left: the scope's units again
    fireEvent.click(screen.getByRole("tab", { name: /Baselines/ }));
    expect(nav.patch).toHaveBeenLastCalledWith({ tab: "baselines", baselineId: null });
  });

  it("shows the comparison with an increase, the cost and conditions, and says why there is none", () => {
    const { rerender } = render(<EnergyView live={liveOf()} />);
    expect(screen.getByText("comparable with the baseline")).toBeDefined();
    expect(screen.getByText(/^Increase/, { selector: "div.text-xl" }).className).toContain("text-crit");
    expect(screen.getByText("the selected baseline version")).toBeDefined();
    rerender(<EnergyView live={liveOf({ analysis: { unitIds: ["u-1"], from: "a", to: "b", baselineId: "b-11112222", summary: summaryView(summary({ totals: { ...summary().totals, kWh: 80, savedKWh: 20, deltaKWh: -20, savingPercentage: 20 } })) } })} />);
    expect(screen.getByText(/^Reduction/, { selector: "div.text-xl" }).className).toContain("text-ok"); // a saving
    rerender(<EnergyView live={liveOf({ analysis: { unitIds: ["u-1"], from: "a", to: "b", baselineId: "b-11112222", summary: summaryView(summary({ totals: { ...summary().totals, savedKWh: null, deltaKWh: null, savingPercentage: null } })) } })} />);
    expect(screen.getByText("No difference can be calculated — see the quality warnings.")).toBeDefined();
    rerender(<EnergyView live={liveOf({ analysis: { unitIds: ["u-1"], from: "a", to: "b", baselineId: null, summary: summaryView(summary({ baselineRef: null, baselineSnapshot: null, totals: { ...summary().totals, savedKWh: null } })) } })} />);
    expect(screen.getByText("Choose a baseline to compare.")).toBeDefined();
    expect(screen.getAllByText("none selected").length).toBeGreaterThan(0);
    rerender(<EnergyView live={liveOf({ analysis: { unitIds: [], from: "a", to: "b", baselineId: null, summary: null } })} />);
    expect(screen.getByText("Choose at least one unit.")).toBeDefined();
    rerender(<EnergyView live={liveOf({ analysis: { unitIds: ["u-1"], from: "a", to: "b", baselineId: null, summary: null, error: "The period is too long" } })} />);
    expect(screen.getByText("The period is too long")).toBeDefined();
    rerender(<EnergyView live={bareLive()} />);
    expect(screen.queryByText("Scope")).toBeNull(); // nothing for the analysis tab without its data
  });
});

const listLive = (over: Partial<Live> = {}) => bareLive({ tab: "baselines", list: { rows: baselines, from: "", to: "", selectedId: "b-11112222" }, ...over });

describe("the baselines tab", () => {
  it("narrows the scope in the URL, applies a period, and says when nothing matches", () => {
    const { rerender } = render(<EnergyView live={listLive({ scope: { customerId: "cust-a" } })} />);
    expect([...screen.getByLabelText<HTMLSelectElement>("Property").options].map((o) => o.textContent)).toStrictEqual(["All properties", "Tower"]);
    fireEvent.change(screen.getByLabelText("Customer"), { target: { value: "cust-b" } });
    expect(nav.patch).toHaveBeenLastCalledWith({ customerId: "cust-b", propertyId: null, unitId: null, baselineId: null });
    fireEvent.change(screen.getByLabelText("Customer"), { target: { value: "" } });
    fireEvent.change(screen.getByLabelText("Property"), { target: { value: "p-1" } });
    expect(nav.patch).toHaveBeenLastCalledWith({ propertyId: "p-1", unitId: null, baselineId: null });
    fireEvent.change(screen.getByLabelText("Property"), { target: { value: "" } });
    fireEvent.change(screen.getByLabelText("Unit"), { target: { value: "u-1" } });
    expect(nav.patch).toHaveBeenLastCalledWith({ unitId: "u-1", baselineId: null });
    fireEvent.change(screen.getByLabelText("Unit"), { target: { value: "" } });
    expect(nav.patch).toHaveBeenLastCalledWith({ unitId: null, baselineId: null });
    const [from, to] = screen.getAllByLabelText("date and time");
    fireEvent.change(from ?? document.body, { target: { value: "2026-08-01T00:00" } });
    fireEvent.click(button("Apply"));
    expect(nav.patch).toHaveBeenLastCalledWith({ from: "2026-08-01T00:00", to: null, baselineId: null });
    fireEvent.change(from ?? document.body, { target: { value: "" } });
    fireEvent.change(to ?? document.body, { target: { value: "2026-09-01T00:00" } });
    fireEvent.click(button("Apply"));
    expect(nav.patch).toHaveBeenLastCalledWith({ from: null, to: "2026-09-01T00:00", baselineId: null });
    fireEvent.click(screen.getByRole("tab", { name: /Analysis/ }));
    expect(nav.patch).toHaveBeenLastCalledWith({ tab: null, baselineId: null });
    expect(screen.getByText("2 baselines in scope")).toBeDefined();
    expect(screen.getByText("Baselines · Demo Customer A")).toBeDefined();
    fireEvent.click(button(/b-333344/));
    expect(nav.patch).toHaveBeenLastCalledWith({ baselineId: "b-33334444" });
    rerender(<EnergyView live={listLive({ scope: { customerId: "cust-a", propertyId: "p-1" } })} />);
    expect([...screen.getByLabelText<HTMLSelectElement>("Unit").options].map((o) => o.textContent)).toStrictEqual(["All units", "Lobby AC", "Office AC"]); // the property's units
    rerender(<EnergyView live={listLive({ list: { rows: [], from: "2026-08-01T00:00", to: "", selectedId: null } })} />);
    expect(screen.getByText("No baseline matches this scope.")).toBeDefined();
    expect(screen.getByText("Nothing selected")).toBeDefined();
    rerender(<EnergyView live={listLive({ list: { rows: baselineRows([baseline("b-11112222")]), from: "", to: "", selectedId: "b-11112222" } })} />);
    expect(screen.getByText("1 baseline in scope")).toBeDefined();
    expect(screen.getByText("Baselines · all customers")).toBeDefined();
    rerender(<EnergyView live={listLive({ list: { rows: [], from: "", to: "", selectedId: null } })} />);
    expect(screen.getByText("Create the first one with New baseline.")).toBeDefined();
  });

  it("saves the next version of a fixed baseline, discards an edit, and focuses the first field to fix", async () => {
    actions.saveBaseline.mockResolvedValue({ ok: true, value: { id: "b-11112222", version: 3 } });
    render(<EnergyView live={listLive()} />);
    expect(screen.getByText("Saving creates version 3 — the version number is assigned automatically")).toBeDefined();
    fireEvent.change(screen.getByLabelText(/^Baseline energy/), { target: { value: "-5" } });
    fireEvent.click(button("Save as version 3"));
    await act(() => new Promise<void>((r) => { requestAnimationFrame(() => { r(); }); })); // focusField waits a frame
    expect(document.activeElement?.id).toBe(screen.getByLabelText(/^Baseline energy/).id); // the field to fix
    expect(actions.saveBaseline).not.toHaveBeenCalled();
    fireEvent.click(button("Discard"));
    expect(screen.getByLabelText<HTMLInputElement>(/^Baseline energy/).value).toBe("100");
    fireEvent.change(screen.getByLabelText(/^Baseline energy/), { target: { value: "110" } });
    fireEvent.change(screen.getByLabelText(/^Boundary description/), { target: { value: " AC only " } });
    fireEvent.change(screen.getByLabelText(/^Assumptions/), { target: { value: "Same week last year" } });
    fireEvent.change(screen.getByLabelText(/^Source/), { target: { value: "Meter" } });
    fireEvent.click(button("Whole-building electricity"));
    fireEvent.click(screen.getByRole("checkbox", { name: "Office AC" }));
    fireEvent.click(screen.getByRole("checkbox", { name: "Hotel AC" }));
    fireEvent.click(screen.getByRole("checkbox", { name: "Hotel AC" })); // ticked and taken off again
    fireEvent.click(button("Save as version 3"));
    await flush();
    expect(actions.saveBaseline).toHaveBeenCalledWith(expect.objectContaining({ id: "b-11112222", method: "demo_fixed", baselineKWh: 110, boundary: "AC only", boundaryId: "whole_building_electricity", unitIds: ["u-1", "u-2"] }), 2);
    expect(nav.toasts).toStrictEqual(["Saved as baseline version 3"]);
    expect(nav.patch).toHaveBeenLastCalledWith({ baselineId: "b-11112222" });
  });

  it("creates a measured baseline, cancels a new one, and deletes one with a reason", async () => {
    actions.saveBaseline.mockResolvedValue({ ok: true, value: { id: "b-new", version: 1 } });
    actions.deleteBaseline.mockResolvedValue({ ok: true, value: null });
    render(<EnergyView live={listLive()} />);
    fireEvent.click(button(/New baseline/));
    expect(screen.getByText("Saving creates version 1")).toBeDefined();
    fireEvent.click(button("Cancel"));
    expect(screen.getByText(/^Saving creates version 3/)).toBeDefined();
    fireEvent.click(button(/New baseline/));
    fireEvent.click(screen.getByRole("checkbox", { name: "Lobby AC" }));
    fireEvent.click(screen.getByRole("checkbox", { name: "Lobby AC" }));
    fireEvent.click(screen.getByRole("checkbox", { name: "Lobby AC" }));
    const [from, to] = screen.getAllByLabelText("date and time").slice(-2);
    fireEvent.change(from ?? document.body, { target: { value: "2026-08-01T00:00" } });
    fireEvent.change(to ?? document.body, { target: { value: "2026-08-08T00:00" } });
    fireEvent.click(button("demo_period_comparison · measured"));
    expect(screen.getByText("Measured from the period’s readings when saved; no value is entered.")).toBeDefined();
    fireEvent.change(screen.getByLabelText(/^Boundary description/), { target: { value: "AC" } });
    fireEvent.change(screen.getByLabelText(/^Assumptions/), { target: { value: "x" } });
    fireEvent.change(screen.getByLabelText(/^Source/), { target: { value: "y" } });
    fireEvent.click(button("Create baseline"));
    await flush();
    expect(actions.saveBaseline).toHaveBeenCalledWith(expect.objectContaining({ method: "demo_period_comparison", unitIds: ["u-1"] }), undefined);
    expect(nav.toasts).toStrictEqual(["Baseline created"]);
    cleanup();
    render(<EnergyView live={listLive()} />);
    fireEvent.click(button("Delete baseline…"));
    const dialog = within(screen.getByRole("dialog"));
    fireEvent.change(dialog.getByLabelText(/^Reason/), { target: { value: "Wrong units" } });
    fireEvent.click(dialog.getByRole("button", { name: "Delete" }));
    await flush();
    expect(actions.deleteBaseline).toHaveBeenCalledWith("b-11112222", 2, "Wrong units");
    expect(nav.patch).toHaveBeenLastCalledWith({ baselineId: null });
    fireEvent.click(button("Delete baseline…"));
    fireEvent.click(within(screen.getByRole("dialog")).getByRole("button", { name: "Cancel" }));
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("offers no writes to a role that may only read energy", () => {
    render(<EnergyView live={listLive({ canWrite: false })} />);
    for (const name of [/New baseline/, "Delete baseline…", "Discard", /Save as version/]) expect(screen.queryByRole("button", { name })).toBeNull();
  });
});

describe("the energy demo", () => {
  it("compares its fixture baselines, keeps one unit chosen, and shows the baselines tab", () => {
    render(<EnergyDemo />);
    expect(screen.getByText("comparable")).toBeDefined();
    fireEvent.click(screen.getByRole("checkbox", { name: "unit-limited" }));
    expect(screen.getByText("Unit set mismatch → no difference can be calculated.")).toBeDefined();
    fireEvent.click(screen.getByRole("checkbox", { name: "unit-online-rto" }));
    fireEvent.click(screen.getByRole("checkbox", { name: "unit-limited" }));
    expect(screen.getByRole("checkbox", { name: "unit-limited" }).getAttribute("aria-checked")).toBe("true"); // the last one stays
    fireEvent.click(screen.getByRole("checkbox", { name: "unit-online-rto" }));
    fireEvent.change(screen.getByLabelText(/^Baseline/), { target: { value: "baseline-2units-100" } });
    expect(screen.getByText("comparable")).toBeDefined();
    fireEvent.change(screen.getByLabelText(/^Actual/), { target: { value: "120" } });
    expect(screen.getAllByText(/Increase/).length).toBeGreaterThan(0);
    fireEvent.click(screen.getByRole("tab", { name: /Baselines/ }));
    fireEvent.click(button(/baseline-2units-100/));
    fireEvent.change(screen.getByLabelText(/^New version value/), { target: { value: "90" } });
    fireEvent.click(button("Save as v2"));
    fireEvent.click(button(/New/));
    expect(nav.toasts).toStrictEqual(["Baseline saved as version 2", "Baseline draft created"]);
  });
});
