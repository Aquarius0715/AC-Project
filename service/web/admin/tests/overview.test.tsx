// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { isValidElement, type ComponentProps, type ReactNode } from "react";
import { forecastView, type AdminSummary, type ApiForecast } from "@ac/web/lib/adminSummary";

// The HQ dashboard (FR-A01, DD-A01, IR244, IR611): the Server Component reads one admin.summary for the URL scope and
// period, the scope options only where the role may read them, and the requests waiting for HQ; the view keeps the
// scope and period in the URL; the Phase 1A demo shows its fixture KPIs and state toggles.
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
  const orAbsent = async <T,>(read: Promise<T>): Promise<T | null> => {
    try {
      return await read;
    } catch (error) {
      if (error instanceof CoreError && (error.error.code === "FORBIDDEN" || error.error.code === "NOT_FOUND")) return null;
      throw error;
    }
  };
  return {
    CoreError, orAbsent, apiMode: vi.fn(() => true), coreNow: vi.fn(() => Promise.resolve(new Date("2026-09-14T01:00:00Z"))),
    coreDisplay: vi.fn(() => Promise.resolve({ locale: "en", timeZone: "Asia/Kuala_Lumpur" })),
    coreOp: vi.fn<(op: string, input?: unknown) => Promise<unknown>>(), coreAll: vi.fn<(op: string, input?: unknown) => Promise<unknown>>(),
  };
});
const nav = vi.hoisted(() => ({ patch: vi.fn<(p: Record<string, string | null>) => void>(), refresh: vi.fn() }));
vi.mock("server-only", () => ({}));
vi.mock("next/server", () => ({ connection: () => Promise.resolve() }));
vi.mock("@ac/web/lib/dal", () => dal);
vi.mock("@ac/web/lib/useUrlPatch", () => ({ useUrlPatch: () => nav.patch }));
vi.mock("next/navigation", async (importOriginal) => ({ ...(await importOriginal<Record<string, unknown>>()), useRouter: () => ({ refresh: nav.refresh }) }));
// the period pickers as one text field each: the view only passes their value through
vi.mock("@ac/web/components/TimeSelect", async (importOriginal) => ({
  ...(await importOriginal<Record<string, unknown>>()),
  DateTimeSelect: ({ value, onChange, "aria-label": label }: Readonly<{ value: string; onChange: (v: string) => void; "aria-label"?: string }>) => <input aria-label={label} value={value} onChange={(e) => { onChange(e.target.value); }} />,
}));

import AdminOverviewPage from "../app/admin/(overview)/page";
import { AdminOverviewView } from "../app/admin/(overview)/_components/overview-view";
import { AdminOverviewDemo } from "../app/admin/(overview)/_components/overview-demo";
import OverviewLoading from "../app/admin/(overview)/loading";
import OverviewErrorState from "../app/admin/(overview)/error";
import { RouteError, RouteLoading } from "@ac/web/components/RouteStates";
import { NeedsAccess } from "@ac/web/components/NeedsAccess";

type Live = ComponentProps<typeof AdminOverviewView>["live"];
const refused = (code: string) => new dal.CoreError({ code, messageKey: `error.${code.toLowerCase()}`, fieldErrors: {}, retryAfterSeconds: null });
const forecast: ApiForecast = {
  period: { from: "2026-09-13T16:00:00Z", to: "2026-09-14T01:00:00Z" }, unitIds: ["u1"], baselineRef: { id: "baseline-demo-tenant-a", version: 1 },
  baselineSnapshot: { id: "baseline-demo-tenant-a", version: 1, method: "demo_fixed", baselineKWh: 2592, period: { from: "", to: "" } },
  expectedUnitMinutes: 2700, validUnitMinutes: 1080, actualKWhOnValidSlots: 9, predictedBaselineKWh: 32.4, predictedActualKWh: 22.5,
  forecastSavedKWh: 9.9, forecastSavingPercentage: 30.6, qualityWarnings: ["partial_coverage"],
};
/** A summary as an older Core API sends it, without asOf. */
const summaryWithoutAsOf = (): AdminSummary => ({
  customerCount: 2, total: 5, online: 4, offline: 1, unknown: 0, powerOn: 2, powerOff: 2, powerUnknown: 1, operatingRate: 50, alertCount: 1,
  jobCounts: { requested: 1, assigned: 1 }, overdueInvoiceCount: 1, amountsByCurrency: [{ amountMinor: 12_000, currency: "MYR" }], billingVisibility: "allowed",
  energySummary: { totals: { kWh: 9, amountMinor: 329 }, coverage: 0.4 }, energyForecast: forecast,
});
const summary = (over: Partial<AdminSummary> = {}): AdminSummary => ({ ...summaryWithoutAsOf(), asOf: "2026-09-14T00:30:00Z", ...over });
/** The Core API of one test: each operation answers with its value, or `answers` decides from the input. */
type Answer = (input: unknown) => unknown;
const is = (value: unknown): Answer => () => value;
const fails = (code: string): Answer => () => Promise.reject(refused(code));
function core(answers: Record<string, Answer>) {
  const table = new Map(Object.entries(answers));
  const answer = (op: string, input?: unknown) => Promise.try(() => table.get(op)?.(input));
  dal.coreOp.mockImplementation(answer);
  dal.coreAll.mockImplementation(answer);
}
const callsOf = (op: string) => [...dal.coreOp.mock.calls, ...dal.coreAll.mock.calls].filter(([name]) => name === op).map(([, input]) => input);
const props = (sp: Record<string, string> = {}) => ({ searchParams: Promise.resolve(sp), params: Promise.resolve({}) });
async function liveOf(sp: Record<string, string> = {}): Promise<Live> {
  const page: ReactNode = await AdminOverviewPage(props(sp));
  if (!isValidElement<ComponentProps<typeof AdminOverviewView>>(page) || page.type !== AdminOverviewView) throw new TypeError("not AdminOverviewView");
  return page.props.live;
}
const everything = () => ({
  "customers.list": is([{ id: "cust-b", name: "Beta Hotel" }, { id: "cust-a", name: "Alpha Office" }]),
  "properties.list": is([{ id: "prop-1", name: "Tower" }]),
  "jobs.list": is({ total: 3 }),
  "admin.summary": is(summary()),
});

beforeEach(() => {
  dal.apiMode.mockReturnValue(true);
  dal.coreOp.mockReset(); dal.coreAll.mockReset(); nav.patch.mockReset(); nav.refresh.mockReset();
});
afterEach(cleanup);

describe("the HQ overview page (Server Component)", () => {
  it("shows the demo outside API mode, and its loading and error states", async () => {
    dal.apiMode.mockReturnValue(false);
    const page: ReactNode = await AdminOverviewPage(props());
    expect(isValidElement(page) && page.type).toBe(AdminOverviewDemo);
    const loading = OverviewLoading();
    expect(loading.type === RouteLoading && loading.props).toStrictEqual({ what: "the dashboard", variant: "dashboard" });
    const error = new Error("down"), retry = vi.fn();
    const failed = OverviewErrorState({ error, retry });
    expect(failed.type === RouteError && failed.props).toStrictEqual({ what: "The dashboard", error, retry });
  });

  it("reads today for all customers, with the requests waiting for HQ", async () => {
    core(everything());
    const live = await liveOf();
    expect([live.period.kind, live.period.error, live.customerId, live.properties, live.propertyId, live.waiting]).toStrictEqual(["today", null, null, null, null, 3]);
    expect(callsOf("properties.list")).toStrictEqual([]); // no customer, no properties to choose
    expect(callsOf("jobs.list")).toStrictEqual([{ limit: 1, filters: { status: "requested", proposalPending: false } }]);
    expect(callsOf("admin.summary")).toStrictEqual([{ from: live.period.from, to: live.period.to }]);
    expect(live.asOf).toMatch(/^As of 8:30 am MYT · /); // the summary's own time
  });

  it("scopes every read to a known customer and property, and ignores unknown ones", async () => {
    core(everything());
    const live = await liveOf({ customerId: "cust-a", propertyId: "prop-1", period: "7d" });
    expect([live.customerId, live.propertyId, live.period.kind]).toStrictEqual(["cust-a", "prop-1", "7d"]);
    expect(callsOf("properties.list")).toStrictEqual([{ filters: { customerId: "cust-a" } }]);
    expect(callsOf("jobs.list")).toStrictEqual([{ limit: 1, filters: { status: "requested", proposalPending: false, customerId: "cust-a", propertyId: "prop-1" } }]);
    expect(callsOf("admin.summary")[0]).toMatchObject({ customerId: "cust-a", propertyId: "prop-1" });
    dal.coreOp.mockClear(); dal.coreAll.mockClear();
    const unknown = await liveOf({ customerId: "cust-x", propertyId: "prop-1", period: "yearly" });
    expect([unknown.customerId, unknown.propertyId, unknown.period.kind]).toStrictEqual([null, null, "today"]);
    dal.coreOp.mockClear(); dal.coreAll.mockClear();
    const noProperty = await liveOf({ customerId: "cust-a", propertyId: "prop-x" });
    expect([noProperty.customerId, noProperty.propertyId]).toStrictEqual(["cust-a", null]);
  });

  it("reads no summary for an invalid custom period, and keeps the typed ends", async () => {
    core(everything());
    const live = await liveOf({ period: "custom", from: "2026-09-14T10:00", to: "2026-09-14T09:00" });
    expect(live.period.error).not.toBeNull();
    expect([live.summary, live.asOf, live.custom]).toStrictEqual([null, null, { from: "2026-09-14T10:00", to: "2026-09-14T09:00" }]);
    expect(callsOf("admin.summary")).toStrictEqual([]);
    const blank = await liveOf({ period: "custom" });
    expect(blank.custom).toStrictEqual({ from: "", to: "" });
  });

  it("hides what the role may not read: no scope options, no waiting tile", async () => {
    core({ ...everything(), "customers.list": fails("FORBIDDEN"), "jobs.list": fails("FORBIDDEN"), "admin.summary": is(summaryWithoutAsOf()) });
    const live = await liveOf();
    expect([live.customers, live.waiting]).toStrictEqual([null, null]);
    expect(live.asOf).toMatch(/^As of 9:00 am MYT · /); // a summary without asOf: the scenario's now
  });

  it("names dashboard.read when the summary is refused, and fails on anything else", async () => {
    core({ ...everything(), "admin.summary": fails("FORBIDDEN") });
    const page: ReactNode = await AdminOverviewPage(props());
    expect(isValidElement(page) && page.type === NeedsAccess && page.props).toStrictEqual({ what: "Overview", permission: "dashboard.read" });
    core({ ...everything(), "admin.summary": fails("UNAVAILABLE") });
    await expect(AdminOverviewPage(props())).rejects.toThrow("UNAVAILABLE");
  });
});

function liveView(over: Partial<Live> = {}): Live {
  return {
    period: { kind: "today", from: "2026-09-13T16:00:00.000Z", to: "2026-09-14T01:00:00.000Z", error: null, label: "14 Sept (1 day)" }, custom: { from: "", to: "" },
    customers: [{ id: "cust-b", name: "Beta Hotel" }, { id: "cust-a", name: "Alpha Office" }], customerId: "cust-a", properties: [{ id: "prop-2", name: "Wing" }, { id: "prop-1", name: "Tower" }], propertyId: null,
    summary: summary(), asOf: "As of 8:30 am MYT · 14 Sept (1 day)", waiting: 3, ...over,
  };
}
const optionsOf = (label: string) => [...screen.getByLabelText<HTMLSelectElement>(label).options].map((o) => o.textContent);
const hrefOf = (name: string | RegExp) => screen.getAllByRole("link", { name }).map((a) => a.getAttribute("href"));

describe("the HQ overview view", () => {
  it("lists the scope options by name and puts the scope and period in the URL", () => {
    render(<AdminOverviewView live={liveView()} />);
    expect(optionsOf("Customer")).toStrictEqual(["Customer: All", "Customer: Alpha Office", "Customer: Beta Hotel"]);
    expect(optionsOf("Property")).toStrictEqual(["Property: All", "Property: Tower", "Property: Wing"]);
    fireEvent.change(screen.getByLabelText("Customer"), { target: { value: "cust-b" } });
    expect(nav.patch).toHaveBeenLastCalledWith({ customerId: "cust-b", propertyId: null }); // another customer drops the property
    fireEvent.change(screen.getByLabelText("Customer"), { target: { value: "" } });
    expect(nav.patch).toHaveBeenLastCalledWith({ customerId: null, propertyId: null });
    fireEvent.change(screen.getByLabelText("Property"), { target: { value: "prop-1" } });
    expect(nav.patch).toHaveBeenLastCalledWith({ propertyId: "prop-1" });
    fireEvent.change(screen.getByLabelText("Property"), { target: { value: "" } });
    expect(nav.patch).toHaveBeenLastCalledWith({ propertyId: null });
    fireEvent.change(screen.getByLabelText("Period"), { target: { value: "7d" } });
    expect(nav.patch).toHaveBeenLastCalledWith({ period: "7d", from: null, to: null });
    fireEvent.change(screen.getByLabelText("Period"), { target: { value: "today" } });
    expect(nav.patch).toHaveBeenLastCalledWith({ period: null, from: null, to: null }); // today is no key
    fireEvent.change(screen.getByLabelText("Period"), { target: { value: "custom" } });
    expect(nav.patch).toHaveBeenLastCalledWith({ period: "custom" }); // the custom ends are kept
    fireEvent.click(screen.getByRole("button", { name: /Refresh/ }));
    expect(nav.refresh).toHaveBeenCalledOnce();
  });

  it("keeps the property select closed until a customer is chosen, and hides both without asset.read", () => {
    const { rerender } = render(<AdminOverviewView live={liveView({ customerId: null, properties: null })} />);
    expect(screen.getByLabelText<HTMLSelectElement>("Property").disabled).toBe(true);
    expect(optionsOf("Property")).toStrictEqual(["Property: All"]);
    rerender(<AdminOverviewView live={liveView({ customers: null, customerId: null, properties: null })} />);
    expect(screen.queryByLabelText("Customer")).toBeNull();
    expect(screen.queryByLabelText("Property")).toBeNull();
  });

  it("edits the custom period's two ends in the URL, an emptied end as no key", () => {
    render(<AdminOverviewView live={liveView({ period: { ...liveView().period, kind: "custom" }, custom: { from: "2026-09-10T00:00", to: "2026-09-12T00:00" } })} />);
    const from = screen.getByLabelText<HTMLInputElement>("From"), to = screen.getByLabelText<HTMLInputElement>("To");
    expect([from.value, to.value]).toStrictEqual(["2026-09-10T00:00", "2026-09-12T00:00"]);
    fireEvent.change(from, { target: { value: "2026-09-11T00:00" } });
    expect(nav.patch).toHaveBeenLastCalledWith({ from: "2026-09-11T00:00" });
    expect(from.value).toBe("2026-09-11T00:00"); // the picker shows the choice while the page reads it
    fireEvent.change(to, { target: { value: "" } });
    expect(nav.patch).toHaveBeenLastCalledWith({ to: null });
    fireEvent.change(from, { target: { value: "" } });
    expect(nav.patch).toHaveBeenLastCalledWith({ from: null });
  });

  it("shows every section with links that keep the scope and period", () => {
    render(<AdminOverviewView live={liveView()} />);
    expect(screen.getByText("As of 8:30 am MYT · 14 Sept (1 day)")).toBeDefined();
    expect(hrefOf("Open requests")).toStrictEqual(["/admin/jobs?customerId=cust-a&stage=requested"]);
    expect(hrefOf("Units running")).toStrictEqual(["/admin/units?customerId=cust-a&powerState=on"]);
    expect(hrefOf(/^Open jobs/)[0]).toContain("customerId=cust-a");
    expect(hrefOf(/^Open jobs/)[0]).toContain("from=2026-09-13T16%3A00%3A00.000Z");
    expect(screen.getByText("Requested 1 · Assigned 1")).toBeDefined();
    expect(screen.getByText("120.00 MYR", { selector: "b.text-crit" })).toBeDefined();
    expect(hrefOf("View the units: Running")).toStrictEqual(["/admin/units?customerId=cust-a&powerState=on"]);
    expect(screen.getByRole("img", { name: /^Running 2, Stopped 2/ })).toBeDefined();
    expect(screen.getByText(/Coverage 1,080 \/ 2,700 unit-min/)).toBeDefined();
    expect(screen.getByText("Forecast savings").nextElementSibling?.className).toContain("text-ok");
  });

  it("words the empty, forbidden and unforecast cases", () => {
    const { rerender } = render(<AdminOverviewView live={liveView({
      waiting: 0, summary: summary({ total: 0, alertCount: 0, jobCounts: {}, billingVisibility: "forbidden", overdueInvoiceCount: null, amountsByCurrency: null, energyForecast: null }),
    })} />);
    expect(screen.getByText("Zero units in scope — nothing to show yet.")).toBeDefined();
    expect(screen.getByText("no jobs in the period")).toBeDefined();
    expect(screen.getByText("Hidden — your role cannot read billing.")).toBeDefined();
    expect(screen.queryByRole("link", { name: "Open overdue" })).toBeNull();
    expect(screen.queryByText("Forecast savings")).toBeNull();
    rerender(<AdminOverviewView live={liveView({ waiting: null, summary: summary({ overdueInvoiceCount: 0, amountsByCurrency: [] }) })} />);
    expect(screen.queryByText("Waiting for HQ")).toBeNull();
    expect(screen.getByText("Nothing unpaid past due.")).toBeDefined();
    rerender(<AdminOverviewView live={liveView({ summary: summary({ energyForecast: { ...forecast, forecastSavedKWh: -2, forecastSavingPercentage: -5 } }) })} />);
    expect(screen.getByText("Forecast savings").nextElementSibling?.className).toContain("text-crit");
    rerender(<AdminOverviewView live={liveView({ summary: summary({ energyForecast: { ...forecast, forecastSavedKWh: null, qualityWarnings: ["partial_coverage"] } }) })} />);
    expect(screen.queryByText("Forecast savings")).toBeNull();
    expect(screen.getByText(forecastView({ ...forecast, forecastSavedKWh: null }).warnings[0] ?? "")).toBeDefined(); // the reason it cannot forecast
    rerender(<AdminOverviewView live={liveView({ summary: null, asOf: null, period: { ...liveView().period, kind: "custom", error: "The end must be after the start" } })} />);
    expect(screen.getByText("The end must be after the start — choose another period.")).toBeDefined();
    expect(screen.queryByText("Active customers")).toBeNull();
    expect(screen.queryAllByRole("link", { name: /View the units/ })).toStrictEqual([]);
  });
});

describe("the HQ overview demo", () => {
  it("shows the fixture KPIs and switches between its zero, loading and error states", () => {
    render(<AdminOverviewDemo />);
    expect(screen.getByText("50.0%")).toBeDefined(); // the fixture operation rate
    fireEvent.click(screen.getByRole("button", { name: "zero units" }));
    expect(screen.getByText("Zero units in scope — nothing to show yet.")).toBeDefined();
    fireEvent.click(screen.getByRole("button", { name: "normal" }));
    expect(screen.queryByText("Zero units in scope — nothing to show yet.")).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "skeleton" }));
    fireEvent.click(screen.getByRole("button", { name: "Finish loading" }));
    fireEvent.click(screen.getByRole("button", { name: "error" }));
    expect(screen.getByText("Couldn’t load the dashboard")).toBeDefined();
    fireEvent.click(screen.getByRole("button", { name: /Try again|Retry/ }));
    expect(screen.getByText("Active customers")).toBeDefined();
  });
});
