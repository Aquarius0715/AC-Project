// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { campaignRows, iotModelRows, type ApiCampaign, type ApiIoTModel, type DeviceRow } from "@ac/web/lib/devices";
import type { ActionFailure } from "@ac/web/lib/actionMessage";

// HQ firmware campaigns (DD-A20): the list and the campaignId detail (waves, progress, per-device results), pause /
// resume / abort with a reason / retry a device / delete an ended one, and scheduling a campaign at least 24 hours
// ahead in the display time zone. The Phase 1A demo of the device registry moves locally.
const actions = vi.hoisted(() => ({ controlCampaign: vi.fn<(...args: unknown[]) => Promise<unknown>>(), deleteCampaign: vi.fn<(...args: unknown[]) => Promise<unknown>>(), scheduleCampaign: vi.fn<(...args: unknown[]) => Promise<unknown>>() }));
const nav = vi.hoisted(() => ({ patch: vi.fn<(p: Record<string, string | null>) => void>(), toasts: [] as string[] }));
vi.mock("../app/admin/devices/actions", () => actions);
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
vi.mock("@ac/web/components/TimeSelect", async (importOriginal) => {
  type P = Readonly<{ value: string; onChange: (v: string) => void }>;
  return {
    ...(await importOriginal<Record<string, unknown>>()),
    TimeSelect: ({ value, onChange }: P) => <input aria-label="time" value={value} onChange={(e) => { onChange(e.target.value); }} />,
    DateTimeSelect: ({ value, onChange }: P) => <input aria-label="date and time" value={value} onChange={(e) => { onChange(e.target.value); }} />,
  };
});

import { FirmwareTab } from "../app/admin/devices/_components/firmware-tab";
import { DevicesDemo } from "../app/admin/devices/_components/devices-demo";
import type { DevicesLive } from "../app/admin/devices/_components/devices-view";

const iot = (id: string, candidates: string[]): ApiIoTModel => ({ id, version: 1, updatedAt: "2026-09-01T00:00:00Z", manufacturer: "IoT-Co", model: id, sensors: [], firmwareCandidates: candidates });
const iots = [iot("i-1", ["1.1.0", "1.2.0"]), iot("i-2", [])];
type Results = ApiCampaign["results"];
const campaign = (id: string, state: ApiCampaign["state"], results: Results = [], over: Partial<ApiCampaign> = {}): ApiCampaign => ({
  id, version: 4, modelId: "i-1", fromVersions: ["1.0.0"], targetVersion: "1.2.0", deviceIds: results.map((r) => r.deviceId), waves: [{ label: "Wave 1 · 50 %", percent: 50 }, { label: "Wave 2 · 100 %", percent: 100 }],
  window: { startLocal: "02:00", endLocal: "05:00" }, autoPauseFailurePercent: 5, startAt: "2026-09-15T18:00:00Z", state, checksum: "sha256:9f3c00a1", createdAt: "2026-09-14T00:00:00Z",
  createdByMembershipId: "m-1", progress: { succeeded: 1, installing: 0, pending: 0, failed: 1, skipped: 1 }, results, reason: null, ...over,
});
const results: Results = [
  { deviceId: "d-1", waveIndex: 0, status: "succeeded", reasonKey: null, at: "2026-09-15T18:10:00Z" },
  { deviceId: "d-2", waveIndex: 1, status: "failed", reasonKey: "errors.timeout", at: "2026-09-15T18:20:00Z" },
  { deviceId: "d-gone-12345678", waveIndex: 1, status: "skipped", reasonKey: "tamper_open", at: null },
];
const devices: DeviceRow[] = [
  { id: "d-1", version: 1, serial: "AC-1", unitId: "u-1", iotModelId: "i-1", targetUnitId: null, unit: "Lobby AC", conn: "online", fw: "1.0.0", tamper: false, power: "on" },
  { id: "d-2", version: 1, serial: "AC-2", unitId: "u-2", iotModelId: "i-1", targetUnitId: null, unit: "Office AC", conn: "online", fw: "1.0.0", tamper: true, power: "on" },
  { id: "d-3", version: 1, serial: "AC-3", unitId: null, iotModelId: "i-1", targetUnitId: null, unit: "Unbound", conn: "online", fw: "1.0.0", tamper: false, power: "on" },
];
/** The firmware tab's data; `cs` null when the list was not read. */
function liveOf(cs: ApiCampaign[] | null, openId: string | null, over: Partial<DevicesLive> = {}): DevicesLive {
  const rows = campaignRows(cs ?? [], iots, undefined, new Map([["m-1", "HQ Ops"]]));
  return {
    tab: "firmware", now: "2026-09-14T01:00:00Z", canAudit: true, canWrite: true, deviceTotal: 3, models: [], iotModels: iotModelRows(iots, [], []), devices, units: [],
    ...(cs && { campaigns: rows }), ...(openId && { campaign: rows.find((c) => c.id === openId) }), ...over,
  };
}
const button = (name: string | RegExp) => screen.getByRole<HTMLButtonElement>("button", { name });
const dialog = () => within(screen.getByRole("dialog"));
const flush = () => act(() => new Promise<void>((r) => { setTimeout(r, 0); }));
const ok = { ok: true, value: null };

beforeEach(() => {
  nav.patch.mockReset(); nav.toasts = [];
  actions.controlCampaign.mockReset(); actions.deleteCampaign.mockReset(); actions.scheduleCampaign.mockReset();
  actions.controlCampaign.mockResolvedValue(ok); actions.deleteCampaign.mockResolvedValue(ok);
});
afterEach(cleanup);

describe("a firmware campaign", () => {
  it("shows its facts, waves and device results, and retries a failed or skipped device while it runs", async () => {
    render(<FirmwareTab live={liveOf([campaign("c-1", "running", results), campaign("c-2", "scheduled")], "c-1")} />);
    expect(screen.getByText(/^1\.0\.0 → 1\.2\.0 · 3 devices · starts/)).toBeDefined();
    expect(screen.getByText(/^HQ Ops · /)).toBeDefined(); // who scheduled it
    expect(screen.getByText("Wave 1 · 50 %", { selector: "b" })).toBeDefined();
    expect(screen.getByText("1 / 1 succeeded")).toBeDefined();
    expect(screen.getByText("0 / 2 succeeded")).toBeDefined();
    expect(screen.getByText("Done")).toBeDefined();
    expect(screen.getByText("Running", { selector: "span" })).toBeDefined();
    expect(screen.getByText("AC-1 · Lobby AC")).toBeDefined();
    expect(screen.getByText("d-gone-1")).toBeDefined(); // a device no longer listed: its ID
    expect(screen.getAllByRole("button", { name: "Retry" })).toHaveLength(2);
    fireEvent.click(screen.getAllByRole("button", { name: "Retry" })[0] ?? document.body);
    await flush();
    expect(actions.controlCampaign).toHaveBeenLastCalledWith("c-1", 4, "retry_device", "d-2", undefined);
    fireEvent.click(button("Pause"));
    await flush();
    expect(actions.controlCampaign).toHaveBeenLastCalledWith("c-1", 4, "pause", undefined, undefined);
    fireEvent.click(button(/IoT-Co i-1.*scheduled/));
    expect(nav.patch).toHaveBeenLastCalledWith({ campaignId: "c-2" });
    expect(nav.toasts).toStrictEqual(["Device queued again", "Campaign paused"]);
  });

  it("resumes a paused campaign and aborts it with a reason", async () => {
    render(<FirmwareTab live={liveOf([campaign("c-1", "paused", results, { reason: "Failure rate above 5 %" })], "c-1")} />);
    expect(screen.getByText("Failure rate above 5 %")).toBeDefined();
    expect(screen.getByText("Paused", { selector: "span" })).toBeDefined();
    fireEvent.click(button("Resume"));
    await flush();
    expect(actions.controlCampaign).toHaveBeenLastCalledWith("c-1", 4, "resume", undefined, undefined);
    fireEvent.click(button("Abort"));
    expect(dialog().getByRole<HTMLButtonElement>("button", { name: "Abort" }).disabled).toBe(true);
    fireEvent.change(dialog().getByLabelText(/^Reason/), { target: { value: " Bad build " } });
    fireEvent.click(dialog().getByRole("button", { name: "Abort" }));
    await flush();
    expect(actions.controlCampaign).toHaveBeenLastCalledWith("c-1", 4, "abort", undefined, "Bad build");
    expect(screen.queryByRole("dialog")).toBeNull();
    fireEvent.click(button("Abort"));
    fireEvent.click(dialog().getByRole("button", { name: "Cancel" }));
    expect(nav.toasts).toStrictEqual(["Campaign resumed", "Campaign aborted"]);
  });

  it("deletes an ended campaign with a reason, and offers no retry or control once it has ended", async () => {
    const { rerender } = render(<FirmwareTab live={liveOf([campaign("c-1", "aborted", results)], "c-1")} />);
    expect(screen.queryByRole("button", { name: "Retry" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Pause" })).toBeNull();
    expect(screen.getByText("Aborted", { selector: "span" })).toBeDefined();
    fireEvent.click(button("Delete campaign…"));
    fireEvent.change(dialog().getByLabelText(/^Reason/), { target: { value: "Cleanup" } });
    fireEvent.click(dialog().getByRole("button", { name: "Delete" }));
    await flush();
    expect(actions.deleteCampaign).toHaveBeenCalledWith("c-1", 4, "Cleanup");
    expect(nav.patch).toHaveBeenLastCalledWith({ campaignId: null });
    rerender(<FirmwareTab live={liveOf([campaign("c-1", "completed", [{ deviceId: "d-1", waveIndex: 0, status: "pending", reasonKey: null, at: null }], { fromVersions: [], createdByMembershipId: null })], "c-1")} />);
    expect(screen.getByText("IoT-Co i-1 · any → 1.2.0")).toBeDefined();
    expect(screen.getAllByText("Done")).toHaveLength(2); // a completed campaign's waves are done
    fireEvent.click(button("Delete campaign…"));
    fireEvent.click(dialog().getByRole("button", { name: "Cancel" }));
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("waits before the start, and offers no controls to a read-only role", () => {
    render(<FirmwareTab live={liveOf([campaign("c-1", "scheduled", results)], "c-1", { canWrite: false })} />);
    expect(screen.getAllByText("Waiting")).toHaveLength(2);
    for (const name of ["Pause", "Abort", "Retry", /New campaign/]) expect(screen.queryByRole("button", { name })).toBeNull();
  });

  it("says when there is no campaign", () => {
    render(<FirmwareTab live={liveOf(null, null)} />);
    expect(screen.getByText("Schedule a firmware campaign for a model.")).toBeDefined();
    expect(screen.getByText("Choose or schedule a campaign.")).toBeDefined();
  });
});

describe("scheduling a campaign", () => {
  it("checks the form, offers the model's bound devices, and schedules it at the start read in the display time zone", async () => {
    actions.scheduleCampaign.mockResolvedValue({ ok: true, value: "c-new" });
    render(<FirmwareTab live={liveOf([], null)} />);
    fireEvent.click(button(/New campaign/));
    fireEvent.change(dialog().getByLabelText(/^Waves/), { target: { value: "50, 20" } });
    fireEvent.change(dialog().getByLabelText(/^Auto-pause/), { target: { value: "60" } });
    fireEvent.change(dialog().getAllByLabelText("time")[0] ?? document.body, { target: { value: "2am" } });
    fireEvent.change(dialog().getByLabelText("date and time"), { target: { value: "2026-09-14T12:00" } });
    fireEvent.click(dialog().getByRole("button", { name: "Schedule" }));
    for (const text of ["Choose a target version", "Choose at least one device", "Ascending percentages ending at 100, e.g. 10, 50, 100", "1–50 %", "HH:MM – HH:MM", "At least 24 hours ahead"]) {
      expect(dialog().getByText(text)).toBeDefined();
    }
    expect(dialog().queryByRole("checkbox", { name: /AC-3/ })).toBeNull(); // not bound to a unit
    expect(dialog().getByText(/tamper \(skipped\)/)).toBeDefined();
    fireEvent.change(dialog().getByLabelText(/^Target version/), { target: { value: "1.2.0" } });
    fireEvent.click(dialog().getByRole("checkbox", { name: /AC-1/ }));
    fireEvent.click(dialog().getByRole("checkbox", { name: /AC-2/ }));
    fireEvent.click(dialog().getByRole("checkbox", { name: /AC-2/ }));
    fireEvent.change(dialog().getByLabelText(/^Waves/), { target: { value: "20, 100" } });
    fireEvent.change(dialog().getByLabelText(/^Auto-pause/), { target: { value: "10" } });
    fireEvent.change(dialog().getAllByLabelText("time")[0] ?? document.body, { target: { value: "01:00" } });
    fireEvent.change(dialog().getAllByLabelText("time")[1] ?? document.body, { target: { value: "04:00" } });
    fireEvent.change(dialog().getByLabelText("date and time"), { target: { value: "2026-09-16T02:00" } });
    fireEvent.click(dialog().getByRole("button", { name: "Schedule" }));
    await flush();
    expect(actions.scheduleCampaign).toHaveBeenCalledWith({ modelId: "i-1", targetVersion: "1.2.0", deviceIds: ["d-1"], waves: "20, 100", startLocal: "01:00", endLocal: "04:00", autoPause: "10", startAt: "2026-09-16T02:00" }, "2026-09-15T18:00:00.000Z");
    expect(nav.patch).toHaveBeenLastCalledWith({ campaignId: "c-new" });
    expect(nav.toasts).toStrictEqual(["Campaign scheduled"]);
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("starts the device choice again for another model, and says when it has no bound device", () => {
    render(<FirmwareTab live={liveOf([], null)} />);
    fireEvent.click(button(/New campaign/));
    fireEvent.click(dialog().getByRole("checkbox", { name: /AC-1/ }));
    fireEvent.change(dialog().getByLabelText(/^Model/), { target: { value: "i-2" } });
    expect(dialog().getByText("No device of this IoT model is bound to a unit.")).toBeDefined();
    expect([...dialog().getByLabelText<HTMLSelectElement>(/^Target version/).options].map((o) => o.value)).toStrictEqual([""]);
    fireEvent.click(dialog().getByRole("button", { name: "Cancel" }));
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("asks for a model when the register has none", () => {
    render(<FirmwareTab live={liveOf([], null, { iotModels: [] })} />);
    fireEvent.click(button(/New campaign/));
    fireEvent.click(dialog().getByRole("button", { name: "Schedule" }));
    expect(dialog().getByText("Choose a model")).toBeDefined();
  });
});

describe("the device registry demo", () => {
  it("checks the model form, moves between devices, and records the demo dialogs", () => {
    render(<DevicesDemo />);
    fireEvent.click(button("Save new version"));
    expect(screen.getByText("A reason is required")).toBeDefined();
    fireEvent.change(screen.getByLabelText(/^Min °C/), { target: { value: "35" } });
    expect(screen.getByText("min must be ≤ max")).toBeDefined();
    fireEvent.change(screen.getByLabelText(/^Max °C/), { target: { value: "36" } });
    fireEvent.click(screen.getByRole("checkbox", { name: "Fan high" }));
    fireEvent.click(screen.getByRole("checkbox", { name: "Low" }));
    expect(screen.getByText("Ventilation levels must include Low.")).toBeDefined();
    fireEvent.change(screen.getByLabelText(/^Change reason/), { target: { value: "Bulletin" } });
    fireEvent.click(button("Save new version"));
    fireEvent.click(button("Version history"));
    expect(dialog().getByText("v1 — created")).toBeDefined();
    fireEvent.click(dialog().getAllByRole("button", { name: "Close" }).at(-1) ?? document.body);
    fireEvent.click(button("Version history"));
    fireEvent.keyDown(screen.getByRole("dialog"), { key: "Escape" });
    expect(screen.queryByRole("dialog")).toBeNull();
    fireEvent.click(button(/New$/));
    fireEvent.change(dialog().getByLabelText(/^Manufacturer/), { target: { value: "Acme" } });
    fireEvent.change(dialog().getByLabelText(/^Model/), { target: { value: "X" } });
    fireEvent.click(dialog().getByRole("button", { name: "Create" }));
    fireEvent.click(screen.getByRole("tab", { name: /IoT devices/ }));
    fireEvent.change(screen.getByPlaceholderText("Search serial, device or unit…"), { target: { value: "tamper" } });
    fireEvent.click(button(/AC-DEMO-0002/));
    expect(screen.getByText("Tamper + device events")).toBeDefined();
    fireEvent.click(button("Calibrate sensor"));
    fireEvent.click(dialog().getByRole("button", { name: "Record" }));
    fireEvent.click(button("Calibrate sensor"));
    fireEvent.click(dialog().getByRole("button", { name: "Cancel" }));
    fireEvent.click(button("Calibrate sensor"));
    fireEvent.keyDown(screen.getByRole("dialog"), { key: "Escape" });
    fireEvent.click(button("Rebind"));
    fireEvent.click(dialog().getByRole("button", { name: "Rebind" }));
    fireEvent.click(button("Rebind"));
    fireEvent.click(dialog().getByRole("button", { name: "Cancel" }));
    fireEvent.click(button("Rebind"));
    fireEvent.keyDown(screen.getByRole("dialog"), { key: "Escape" });
    expect(screen.queryByRole("dialog")).toBeNull();
    fireEvent.click(button(/Register device/));
    fireEvent.click(dialog().getByRole("button", { name: "Register" }));
    expect(dialog().getByText("Serial is required")).toBeDefined();
    fireEvent.change(dialog().getByLabelText(/^Serial/), { target: { value: "AC-9" } });
    fireEvent.click(dialog().getByRole("button", { name: "Register" }));
    fireEvent.click(screen.getByRole("tab", { name: /Firmware campaigns/ }));
    expect(nav.toasts).toStrictEqual(["Saved — new capability version created (v4)", "Model created", "Calibration recorded", "Device rebound", "Device registered"]);
  });
});
