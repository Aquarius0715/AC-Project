// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { coverageKpis, registerKpis, type ApiImportPreview, type ApiUnitImport, type ClaimCandidate, type CoverageRow, type CustomerRow } from "@ac/web/lib/assets";
import type { ActionFailure } from "@ac/web/lib/actionMessage";

// The HQ customer register (FR-A02) and its top-level tabs: the register's search, status and contract filters, the
// standing columns, a new customer; Warranty & coverage (FR-A19): the filters, the KPIs, the CSV export, the claimable
// jobs and a filed claim; the CSV import wizard (FR-A18): file, mapping, preview, import and undo within 24 h.
const actions = vi.hoisted(() => {
  const fn = () => vi.fn<(...args: unknown[]) => Promise<unknown>>();
  return { createCustomer: fn(), recordWarrantyClaim: fn(), importPreview: fn(), importCommit: fn(), importUndo: fn() };
});
const nav = vi.hoisted(() => ({ patch: vi.fn<(p: Record<string, string | null>) => void>(), toasts: [] as string[], downloads: [] as string[] }));
vi.mock("server-only", () => ({}));
vi.mock("next/navigation", () => ({ useRouter: () => ({ push: vi.fn(), replace: vi.fn() }), usePathname: () => "/admin/units", useSearchParams: () => new URLSearchParams() }));
vi.mock("../app/admin/units/actions", () => actions);
vi.mock("../app/admin/units/_components/unit-device", () => ({ NewUnitDeviceSection: () => null, UnitDevicePanel: () => null }));
vi.mock("@ac/web/components/LocationPicker", () => ({ LocationPicker: () => null, PinMap: () => null }));
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

import { UnitsView, type UnitsLive } from "../app/admin/units/_components/units-view";

const row = (id: string, over: Partial<CustomerRow> = {}): CustomerRow => ({
  id, version: 1, name: `Customer ${id}`, billingName: `Customer ${id}`, status: "active", orgId: `org-${id}`, orgVersion: 1, orgStatus: "active", profile: "general", since: "Jan 2026",
  properties: 1, propertyNames: ["Home"], units: 2, operation: "50%", running: 1, known: 2, alerts: 0, standing: null, marks: [], ...over,
});
const rows = [
  row("a", { billingName: "Alpha Sdn Bhd", marks: [{ label: "Overdue", tone: "crit" }], standing: { plans: ["rto"], contracts: 1, contractUnits: 2, overdue: 1, overdueAmount: "MYR 100", restriction: null }, alerts: 2 }),
  row("b", { marks: [{ label: "Good standing", tone: "ok" }], standing: { plans: ["general"], contracts: 1, contractUnits: 1, overdue: 0, overdueAmount: null, restriction: null } }),
  row("old", { status: "inactive", marks: [{ label: "Inactive", tone: "muted" }] }),
];
const coverage = (unitId: string, over: Partial<CoverageRow> = {}): CoverageRow => ({
  unitId, unit: `Unit ${unitId}`, sub: "Home", customerId: "a", customer: "Customer a", model: "AC-Co m1", ends: "1 Oct 2026", endsDate: "2026-10-01", endsAt: "2026-10-01T00:00:00Z", days: 17,
  contractIds: [], contracts: "—", status: "expiring", statusText: "Ends in 17 days", ...over,
});
const coverageRowsList = [coverage("u-1"), coverage("u-2", { customerId: "b", customer: "Customer b", status: "no_coverage", statusText: "No coverage", days: -3 }), coverage("u-3", { status: "contract", statusText: "Maintenance contract", contractIds: ["k-1"], contracts: "RTO until 1 Jan 2027", days: null })];
const claim: ClaimCandidate = { jobId: "job-12345678", version: 2, unit: "Unit u-1", type: "repair", completed: "1 Sep 2026", parts: "Compressor ×1", partLabel: "Compressor", amountMinor: 12_000, currency: "MYR" };
function liveOf(over: Partial<UnitsLive> = {}): UnitsLive {
  return {
    now: "2026-09-14T01:00:00Z", todayKL: "2026-09-14", canWrite: true, canForce: false, standingKnown: true, rows, kpis: registerKpis(rows, []), search: "", missing: false, top: "customers",
    activeCustomers: [{ id: "a", name: "Customer a" }, { id: "b", name: "Customer b" }], coverageAttention: 2, map: { styleUrl: null, search: false }, ...over,
  };
}
const warrantyLive = (over: Partial<NonNullable<UnitsLive["warranty"]>> = {}) => liveOf({ top: "warranty", warranty: { rows: coverageRowsList, kpis: coverageKpis(coverageRowsList), customers: [{ id: "a", name: "Customer a" }, { id: "b", name: "Customer b" }], claims: [claim], canClaim: true, ...over } });
const button = (name: string | RegExp) => screen.getByRole<HTMLButtonElement>("button", { name });
const dialog = () => within(screen.getByRole("dialog"));
const flush = () => act(() => new Promise<void>((r) => { setTimeout(r, 0); }));
const bodyRows = () => screen.getAllByRole("row").slice(1);

beforeEach(() => {
  nav.patch.mockReset(); nav.toasts = []; nav.downloads = [];
  for (const f of Object.values(actions)) f.mockReset();
  URL.createObjectURL = vi.fn(() => "blob:csv");
  URL.revokeObjectURL = vi.fn();
  vi.spyOn(HTMLAnchorElement.prototype, "click").mockImplementation(function record(this: HTMLAnchorElement) { nav.downloads.push(this.download); }); // a download, not a navigation
});
afterEach(cleanup);

describe("the customer register", () => {
  it("needs asset.read", () => {
    render(<UnitsView live={null} />);
    expect(screen.getByText(/asset\.read|View customers/)).toBeDefined();
  });

  it("searches, filters by status and contract, opens a customer, and moves to the warranty tab", () => {
    render(<UnitsView live={liveOf({ search: "" })} />);
    expect(bodyRows()).toHaveLength(2); // active only
    expect(screen.getByText(/^billed as Alpha Sdn Bhd ·/)).toBeDefined();
    fireEvent.change(screen.getByLabelText("Status"), { target: { value: "all" } });
    expect(bodyRows()).toHaveLength(3);
    expect(screen.getAllByText("—").length).toBeGreaterThan(0); // an inactive customer's alerts
    fireEvent.change(screen.getByLabelText("Contract"), { target: { value: "overdue" } });
    expect(bodyRows().map((r) => r.textContent)).toStrictEqual([expect.stringContaining("Customer a")]);
    fireEvent.change(screen.getByLabelText("Contract"), { target: { value: "all" } });
    fireEvent.change(screen.getByPlaceholderText("Search customer name, ID or property…"), { target: { value: "customer b" } });
    expect(bodyRows()).toHaveLength(1);
    fireEvent.click(bodyRows()[0] ?? document.body);
    expect(nav.patch).toHaveBeenLastCalledWith({ customerId: "b", tab: null, locationId: null, unitId: null, powerState: null, connections: null, search: null });
    fireEvent.click(screen.getByRole("tab", { name: /Warranty & coverage/ }));
    expect(nav.patch).toHaveBeenLastCalledWith({ tab: "warranty" });
  });

  it("filters the customers needing attention from their KPI, and says when one asked for is gone", () => {
    render(<UnitsView live={liveOf({ missing: true })} />);
    expect(screen.getByText("This customer no longer exists or is outside your scope — showing the register.")).toBeDefined();
    fireEvent.click(button("Show these"));
    expect(bodyRows()).toHaveLength(1);
    fireEvent.click(button("Clear filter"));
    expect(bodyRows()).toHaveLength(2);
  });

  it("hides the contract column and filter without billing, and invites the first customer", () => {
    const { rerender } = render(<UnitsView live={liveOf({ standingKnown: false, coverageAttention: null })} />);
    expect(screen.queryByLabelText("Contract")).toBeNull();
    expect(screen.getByText("open alerts (billing needs “View billing”)")).toBeDefined();
    expect(screen.getByRole("tab", { name: /Warranty & coverage/ }).textContent).toContain("—");
    rerender(<UnitsView live={liveOf({ rows: [], kpis: registerKpis([], []) })} />);
    expect(screen.getByText("Create the first customer, then add its properties and units.")).toBeDefined();
    rerender(<UnitsView live={liveOf({ rows: [], kpis: registerKpis([], []), canWrite: false })} />);
    expect(screen.getByText("Customers appear here once HQ registers them.")).toBeDefined();
    expect(screen.queryByRole("button", { name: /New customer/ })).toBeNull();
    expect(screen.queryByRole("button", { name: /Import CSV/ })).toBeNull();
  });

  it("checks and creates a customer, then opens it", async () => {
    actions.createCustomer.mockResolvedValue({ ok: true, value: "cust-new" });
    render(<UnitsView live={liveOf()} />);
    fireEvent.click(button(/New customer/));
    fireEvent.click(dialog().getByRole("button", { name: "Create customer" }));
    expect(dialog().getAllByText("1–120 characters")).toHaveLength(2);
    fireEvent.change(dialog().getByLabelText(/^Billing name/), { target: { value: "Gamma Sdn Bhd" } });
    fireEvent.change(dialog().getByLabelText(/^Customer display name/), { target: { value: " Gamma " } });
    fireEvent.click(dialog().getByRole("button", { name: "Rent-to-own (RTO)" }));
    fireEvent.change(dialog().getByLabelText(/^Status/), { target: { value: "inactive" } });
    fireEvent.click(dialog().getByRole("button", { name: "Create customer" }));
    await flush();
    expect(actions.createCustomer).toHaveBeenCalledWith({ billingName: "Gamma Sdn Bhd", name: " Gamma ", serviceProfile: "rto", status: "inactive" });
    expect(nav.toasts).toStrictEqual(["Customer “Gamma” created — add a property next"]);
    expect(nav.patch).toHaveBeenLastCalledWith({ customerId: "cust-new", tab: null, search: null });
    fireEvent.click(button(/New customer/));
    fireEvent.click(dialog().getByRole("button", { name: "Cancel" }));
    expect(screen.queryByRole("dialog")).toBeNull();
  });
});

describe("warranty & coverage", () => {
  it("needs a read permission for the coverage", () => {
    render(<UnitsView live={liveOf({ top: "warranty", warranty: null })} />);
    expect(screen.getByText("Coverage needs “View customers & units” or “View contracts”")).toBeDefined();
    fireEvent.click(screen.getByRole("tab", { name: /Customers & units/ }));
    expect(nav.patch).toHaveBeenLastCalledWith({ tab: null });
  });

  it("filters the units by customer, coverage and end, exports them, and links each to its contract or an offer", () => {
    render(<UnitsView live={warrantyLive()} />);
    expect(screen.queryByRole("button", { name: /Import CSV/ })).toBeNull(); // the register's
    expect(screen.getByRole("link", { name: "Open contract" }).getAttribute("href")).toBe("/admin/billing/contracts?contractId=k-1");
    expect(screen.getAllByRole("link", { name: "Renewal offer" })[0]?.getAttribute("href")).toBe("/admin/billing/contracts?customerId=a&unitId=u-1");
    fireEvent.click(button("Offer maintenance"));
    expect(screen.getByText("1 of 3")).toBeDefined();
    fireEvent.click(button("Clear filter"));
    fireEvent.change(screen.getByLabelText("Customer"), { target: { value: "a" } });
    expect(screen.getByText("2 of 3")).toBeDefined();
    fireEvent.change(screen.getByLabelText("Coverage"), { target: { value: "expiring" } });
    fireEvent.change(screen.getByLabelText("Ends within"), { target: { value: "30" } });
    expect(screen.getByText("1 of 3")).toBeDefined();
    fireEvent.click(button("Export CSV"));
    expect(nav.downloads).toStrictEqual(["warranty-coverage.csv"]);
    fireEvent.change(screen.getByLabelText("Coverage"), { target: { value: "contract" } });
    fireEvent.change(screen.getByLabelText("Ends within"), { target: { value: "" } });
    fireEvent.change(screen.getByLabelText("Customer"), { target: { value: "b" } });
    expect(button("Export CSV").disabled).toBe(true);
  });

  it("files a warranty claim with its part, amount and reason", async () => {
    actions.recordWarrantyClaim.mockResolvedValue({ ok: true, value: null });
    render(<UnitsView live={warrantyLive()} />);
    expect(screen.getByText(/Claim 120\.00 MYR · not filed/)).toBeDefined();
    fireEvent.click(button("Mark claim filed"));
    expect(dialog().getByLabelText<HTMLInputElement>(/^Claim amount/).value).toBe("120.00");
    fireEvent.change(dialog().getByLabelText(/^Part/), { target: { value: "" } });
    fireEvent.change(dialog().getByLabelText(/^Claim amount/), { target: { value: "1.234" } });
    fireEvent.click(dialog().getByRole("button", { name: "Mark claim filed" }));
    expect(dialog().getByText("An amount above 0 with at most 2 decimals")).toBeDefined();
    expect(actions.recordWarrantyClaim).not.toHaveBeenCalled();
    fireEvent.change(dialog().getByLabelText(/^Part/), { target: { value: "Compressor kit" } });
    fireEvent.change(dialog().getByLabelText(/^Claim amount/), { target: { value: "99.5" } });
    fireEvent.change(dialog().getByLabelText(/^Reason/), { target: { value: " WC-77 " } });
    fireEvent.click(dialog().getByRole("button", { name: "Mark claim filed" }));
    await flush();
    expect(actions.recordWarrantyClaim).toHaveBeenCalledWith("job-12345678", 2, "Compressor kit", 9950, "MYR", "WC-77");
    expect(nav.toasts).toStrictEqual(["Warranty claim filed for Unit u-1"]);
    fireEvent.click(button("Mark claim filed"));
    fireEvent.click(dialog().getByRole("button", { name: "Cancel" }));
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("says when claims cannot be read or there are none, and offers no filing without job.write", () => {
    const { rerender } = render(<UnitsView live={warrantyLive({ claims: null })} />);
    expect(screen.getByText("Claimable jobs need “View maintenance jobs”.")).toBeDefined();
    rerender(<UnitsView live={warrantyLive({ claims: [] })} />);
    expect(screen.getByText("No claimable job right now.")).toBeDefined();
    const covered = [coverage("u-3", { status: "contract", contractIds: ["k-1"], days: null })];
    rerender(<UnitsView live={warrantyLive({ rows: covered, kpis: coverageKpis(covered), claims: [{ ...claim, amountMinor: null }] })} />);
    expect(screen.queryByRole("button", { name: "Offer maintenance" })).toBeNull(); // nothing uncovered to filter
    fireEvent.click(button("Mark claim filed"));
    expect(dialog().getByLabelText<HTMLInputElement>(/^Claim amount/).value).toBe(""); // no cost recorded: typed by HQ
    fireEvent.click(dialog().getByRole("button", { name: "Cancel" }));
    rerender(<UnitsView live={warrantyLive({ claims: [{ ...claim, amountMinor: null }], canClaim: false })} />);
    expect(screen.getByText(/Claim amount to enter · not filed/)).toBeDefined();
    expect(screen.queryByRole("button", { name: "Mark claim filed" })).toBeNull();
  });
});

const csv = "property,floor,room,unit_name,model_code\nOffice A,2F,Room 1,AC 1,SPL-100\n";
const fileOf = (text: string, name = "units.csv") => new File([text], name, { type: "text/csv" });
const previewOf = (over: Partial<ApiImportPreview> = {}): ApiImportPreview => ({
  previewId: "prev-1", customerId: "a", fileName: "units.csv", readyCount: 1, warningCount: 1, errorCount: 1, expiresAt: "2026-09-14T01:30:00Z",
  rows: [
    { rowNumber: 2, propertyName: "Office A", floorName: "2F", roomName: "Room 1", unitName: "AC 1", modelCode: "SPL-100", serial: null, installedOn: null, warrantyEnd: null, result: "ready", messageKey: null },
    { rowNumber: 3, propertyName: "Office A", floorName: null, roomName: null, unitName: "", modelCode: "", serial: "AC-9", installedOn: null, warrantyEnd: null, result: "error", messageKey: "import.unitNameRequired" },
    { rowNumber: 4, propertyName: "Office B", floorName: null, roomName: null, unitName: "AC 3", modelCode: "SPL-100", serial: null, installedOn: null, warrantyEnd: null, result: "warning", messageKey: "import.propertyCreated" },
  ],
  ...over,
});
const imported = (over: Partial<ApiUnitImport> = {}): ApiUnitImport => ({ id: "imp-1", version: 1, customerId: "a", createdPropertyIds: ["p-9"], createdSpaceIds: ["s-1", "s-2"], createdUnitIds: ["u-9"], skippedRowNumbers: [3], undoUntil: "2026-09-15T01:00:00Z", state: "imported", ...over });

async function chooseFile(text = csv, name = "units.csv") {
  fireEvent.change(dialog().getByLabelText("CSV file"), { target: { files: [fileOf(text, name)] } });
  await flush();
}

describe("the CSV import", () => {

  it("asks for a file and the required fields, maps the columns it knows, and downloads the template", async () => {
    render(<UnitsView live={liveOf()} />);
    fireEvent.click(button(/Import CSV/));
    fireEvent.click(dialog().getByRole("button", { name: /Validate/ }));
    expect(dialog().getByText("Choose a CSV file")).toBeDefined();
    fireEvent.click(dialog().getByRole("button", { name: "Download template" }));
    expect(nav.downloads).toStrictEqual(["units-template.csv"]);
    fireEvent.change(dialog().getByLabelText("CSV file"), { target: { files: [] } }); // the picker closed without a file
    await flush();
    expect(dialog().queryByText(/· UTF-8 ·/)).toBeNull();
    await chooseFile("site,unit\nOffice A,AC 1\n");
    expect(dialog().getByText(/units\.csv · 1 row · 2 columns · UTF-8 · 1 KB/)).toBeDefined();
    expect(dialog().getByLabelText<HTMLSelectElement>("Column for Property name").value).toBe("site");
    fireEvent.click(dialog().getByRole("button", { name: /Validate/ }));
    expect(dialog().getByText("Map the required fields: Model (model register)")).toBeDefined();
    fireEvent.change(dialog().getByLabelText("Column for Model (model register)"), { target: { value: "unit" } });
    fireEvent.change(dialog().getByLabelText("Column for Unit name"), { target: { value: "" } });
    fireEvent.click(dialog().getByRole("button", { name: /Validate/ }));
    expect(dialog().getByText("Map the required fields: Unit name")).toBeDefined();
    expect(actions.importPreview).not.toHaveBeenCalled();
    fireEvent.click(dialog().getByRole("button", { name: "Cancel" }));
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("refuses a file that is too large, and one without a customer", async () => {
    render(<UnitsView live={liveOf({ activeCustomers: [] })} />);
    fireEvent.click(button(/Import CSV/));
    await chooseFile(`${csv}${"x".repeat(900_001)}`, "big.csv");
    expect(dialog().getByText(/too large: split the file/)).toBeDefined();
    fireEvent.click(dialog().getByRole("button", { name: /Validate/ }));
    expect(dialog().getByText("Choose a customer")).toBeDefined();
    expect(actions.importPreview).not.toHaveBeenCalled();
  });

  it("previews the rows, downloads the error report, imports the valid ones and undoes the import with a reason", async () => {
    actions.importPreview.mockResolvedValue({ ok: true, value: previewOf() });
    actions.importCommit.mockResolvedValue({ ok: true, value: imported() });
    actions.importUndo.mockResolvedValue({ ok: true, value: imported({ state: "undone" }) });
    render(<UnitsView live={liveOf()} />);
    fireEvent.click(button(/Import CSV/));
    fireEvent.change(dialog().getByLabelText(/^Customer/), { target: { value: "b" } });
    await chooseFile();
    fireEvent.click(dialog().getByRole("button", { name: /Validate/ }));
    await flush();
    expect(actions.importPreview).toHaveBeenCalledWith("b", "units.csv", csv, { property: "property", floor: "floor", room: "room", unit_name: "unit_name", model_code: "model_code" });
    expect(nav.toasts).toStrictEqual(["2 of 3 rows can be imported"]);
    expect(dialog().getByText("Office A › 2F › Room 1")).toBeDefined();
    expect(dialog().getByText("— · AC-9")).toBeDefined();
    fireEvent.click(dialog().getByRole("button", { name: "Download error report" }));
    expect(nav.downloads).toStrictEqual(["units-report.csv"]);
    fireEvent.click(dialog().getByRole("button", { name: /Back/ }));
    expect(dialog().getByRole("button", { name: /Validate/ })).toBeDefined();
    fireEvent.click(dialog().getByRole("button", { name: /Validate/ }));
    await flush();
    fireEvent.click(dialog().getByRole("button", { name: "Import 2 valid rows" }));
    await flush();
    expect(actions.importCommit).toHaveBeenCalledWith("prev-1", "b");
    expect(dialog().getByText("1 unit created for Customer b · 2 locations and 1 property added · 1 row skipped.")).toBeDefined();
    fireEvent.click(dialog().getByRole("button", { name: "Undo import" }));
    expect(actions.importUndo).not.toHaveBeenCalled(); // a reason first
    fireEvent.change(dialog().getByLabelText(/^Undo reason/), { target: { value: "Wrong customer" } });
    fireEvent.click(dialog().getByRole("button", { name: "Undo import" }));
    await flush();
    expect(actions.importUndo).toHaveBeenCalledWith("imp-1", 1, "Wrong customer");
    expect(dialog().getByText(/The imported units, rooms and properties are archived again/)).toBeDefined();
    expect(nav.toasts).toStrictEqual(["2 of 3 rows can be imported", "2 of 3 rows can be imported", "1 unit imported", "Import undone"]); // validated twice
    fireEvent.click(dialog().getAllByRole("button", { name: "Close" }).at(-1) ?? document.body);
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("offers no import of a preview without valid rows, nor an undo once the 24 hours have passed", async () => {
    actions.importPreview.mockResolvedValue({ ok: true, value: previewOf({ readyCount: 0, warningCount: 0, errorCount: 0, rows: [] }) });
    actions.importCommit.mockResolvedValue({ ok: true, value: imported({ undoUntil: "2026-09-14T00:00:00Z", skippedRowNumbers: [], createdUnitIds: ["u-1", "u-2"], createdSpaceIds: ["s-1"], createdPropertyIds: [] }) });
    render(<UnitsView live={liveOf()} />);
    fireEvent.click(button(/Import CSV/));
    await chooseFile();
    fireEvent.click(dialog().getByRole("button", { name: /Validate/ }));
    await flush();
    expect(dialog().getByRole<HTMLButtonElement>("button", { name: "Import 0 valid rows" }).disabled).toBe(true);
    expect(dialog().getByRole<HTMLButtonElement>("button", { name: "Download error report" }).disabled).toBe(true);
    actions.importPreview.mockResolvedValue({ ok: true, value: previewOf({ readyCount: 1, warningCount: 0, errorCount: 0, rows: [] }) });
    fireEvent.click(dialog().getByRole("button", { name: /Back/ }));
    fireEvent.click(dialog().getByRole("button", { name: /Validate/ }));
    await flush();
    fireEvent.click(dialog().getByRole("button", { name: "Import 1 valid row" }));
    await flush();
    expect(dialog().getByText("2 units created for Customer a · 1 location and 0 properties added · 0 rows skipped.")).toBeDefined();
    expect(dialog().getByText("none")).toBeDefined();
    expect(dialog().queryByRole("button", { name: "Undo import" })).toBeNull();
  });
});
