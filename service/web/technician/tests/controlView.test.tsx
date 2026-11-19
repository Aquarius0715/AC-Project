// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import type { ActionFailure } from "@ac/web/lib/actionMessage";
import type { ActionGroup } from "@ac/web/lib/clientAutomations";
import type { ApiRun, HistoryRow } from "@ac/web/lib/techControl";

// diagnostic control in API mode (FR-T10) and its Phase 1A demo (review round 1082)
type Answer = { ok: true; value: unknown } | ({ ok: false } & ActionFailure);
const store = vi.hoisted((): { next: Answer; toasts: string[]; refresh: ReturnType<typeof vi.fn> } => ({ next: { ok: true, value: { id: "cmd-12345678abcd" } }, toasts: [], refresh: vi.fn() }));
vi.mock("next/navigation", () => ({ useRouter: () => ({ refresh: store.refresh }) }));
vi.mock("@ac/web/components/ui", async (importOriginal) => ({ ...(await importOriginal<Record<string, unknown>>()), useToast: () => (m: string) => { store.toasts.push(m); } }));
vi.mock("../app/technician/units/[id]/control/actions", () => ({
  sendDiagnostic: vi.fn(() => Promise.resolve(store.next)),
  startTestRun: vi.fn(() => Promise.resolve(store.next)),
}));
// the action runner without the transition and the refresh: the success text, then `after`, or `failed`
vi.mock("@ac/web/lib/useAction", () => ({
  useAction: () => [false, (fn: () => Promise<Answer>, ok: string | ((v: unknown) => string), after?: (v: unknown) => void, failed?: (f: ActionFailure) => void) => {
    void fn().then((r) => {
      if (r.ok) { store.toasts.push(typeof ok === "string" ? ok : ok(r.value)); after?.(r.value); } else failed?.(r);
    });
  }],
}));

import { sendDiagnostic, startTestRun } from "../app/technician/units/[id]/control/actions";
import { ControlView } from "../app/technician/units/[id]/control/_components/control-view";
import type { ControlLive } from "../app/technician/units/[id]/control/_lib/load";
import { ControlDemo } from "../app/technician/units/[id]/control/_components/control-demo";

const opt = (key: string) => ({ key, label: key, short: key });
const OPTIONS: ActionGroup[] = [
  { id: "power", group: "Power", options: [opt("power:on"), opt("power:off")] },
  { id: "mode", group: "Mode", options: [opt("mode:cool"), opt("mode:dry")] },
];
const testRun = (over: Partial<ApiRun> = {}): ApiRun => ({
  id: "run-1", version: 1, jobId: "job-12345678", unitId: "unit-1", startAction: { kind: "set_power", power: true }, endAction: { kind: "set_power", power: false },
  durationMinutes: 5, reason: "Check cooling", state: "running", startCommandId: "cmd-1", endCommandId: null, startedAt: null, endAt: null, failureCode: null, createdAt: "2026-11-18T02:00:00Z", ...over,
});
const row = (over: Partial<HistoryRow> = {}): HistoryRow => ({ id: "h-1", at: "10:01", title: "set_mode = cool", sub: "acknowledged 10:01", badge: { text: "Acknowledged", tone: "ok" }, run: false, ...over });
type Unit = ControlLive["unit"];
type Job = NonNullable<ControlLive["job"]>;
const live = (over: Partial<ControlLive> = {}, unit: Partial<Unit> = {}, job: Partial<Job> = {}): ControlLive => ({
  kind: "live", canDiagnose: true, unitHref: "/technician/units/unit-1?jobId=job-12345678",
  unit: {
    id: "unit-1", name: "Living AC", version: 7, connection: "online", tiles: [{ label: "Power", value: "On", warn: false }, { label: "Room", value: "31 °C", warn: true }],
    observed: "observed 10:00", pending: 0, options: OPTIONS, capabilityText: "Capability allows power and mode.", restriction: null, blocked: null, current: "On · Cool", ...unit,
  },
  job: { id: "job-12345678", version: 3, status: "in_progress", window: "10:00–12:00", inWindow: true, ...job },
  run: null, banner: null, active: false, history: [], ...over,
});
const button = (name: string | RegExp, root: HTMLElement = document.body) => within(root).getByRole<HTMLButtonElement>("button", { name });
const area = (placeholder: string) => screen.getByPlaceholderText<HTMLTextAreaElement>(placeholder);
const CMD_REASON = "Why this command is needed for the diagnosis";
const RUN_REASON = "What the test run should show";
const refusedWith = (code: string, fieldErrors: Record<string, string>): Answer => ({ ok: false, messageKey: `errors.${code.toLowerCase()}`, code, fieldErrors });

beforeEach(() => { store.next = { ok: true, value: { id: "cmd-12345678abcd" } }; store.toasts = []; store.refresh.mockReset(); vi.mocked(sendDiagnostic).mockClear(); vi.mocked(startTestRun).mockClear(); });
afterEach(() => { cleanup(); vi.useRealTimers(); });

describe("ControlView without a job or the permission", () => {
  it("says the permission is missing before anything else", () => {
    render(<ControlView live={live({ canDiagnose: false, job: null })} />);
    expect(screen.getByText("No diagnostic permission")).toBeDefined();
    expect(screen.queryByText("Choose the job first")).toBeNull();
    expect(screen.getByRole("link", { name: "Back" }).getAttribute("href")).toBe("/technician/units/unit-1?jobId=job-12345678");
  });
  it("asks for the job first, with a link back to the unit", () => {
    render(<ControlView live={live({ job: null })} />);
    expect(screen.getByText("Choose the job first")).toBeDefined();
    expect(screen.getByRole("link", { name: /Unit and its jobs/ }).getAttribute("href")).toBe("/technician/units/unit-1?jobId=job-12345678");
    expect(screen.queryByRole("button", { name: "Review & send" })).toBeNull();
  });
});

describe("ControlView diagnostic command", () => {
  it("starts on the first mode, refuses an empty reason with focus on it, and sends the chosen action after the confirmation", async () => {
    render(<ControlView live={live()} />);
    const action = screen.getByRole<HTMLSelectElement>("combobox", { name: "Action" });
    expect(action.value).toBe("mode:cool");
    fireEvent.click(button("Review & send"));
    expect(screen.getByText("Reason is required (1–1000 characters)")).toBeDefined();
    await waitFor(() => { expect(document.activeElement).toBe(area(CMD_REASON)); });
    expect(screen.queryByRole("dialog")).toBeNull();
    fireEvent.change(action, { target: { value: "mode:dry" } });
    fireEvent.change(area(CMD_REASON), { target: { value: "  Airflow complaint  " } });
    fireEvent.click(button("Review & send"));
    const dialog = screen.getByRole("dialog", { name: "Send this diagnostic command?" });
    expect(within(dialog).getByText("set_mode = dry")).toBeDefined();
    expect(within(dialog).getByText("Airflow complaint")).toBeDefined();
    expect(within(dialog).getByText("job-1234 · window 10:00–12:00")).toBeDefined();
    fireEvent.click(button("Send command", dialog));
    await waitFor(() => { expect(store.toasts).toStrictEqual(["cmd-1234 sent — waiting for the device"]); });
    expect(vi.mocked(sendDiagnostic)).toHaveBeenCalledWith({ unitId: "unit-1", jobId: "job-12345678", action: { kind: "set_mode", mode: "dry" }, expectedUnitVersion: 7, reason: "  Airflow complaint  " });
    expect(screen.queryByRole("dialog")).toBeNull();
  });
  it("refuses a reason over 1000 characters, and Cancel closes the confirmation without sending", () => {
    render(<ControlView live={live()} />);
    fireEvent.change(area(CMD_REASON), { target: { value: "x".repeat(1001) } });
    fireEvent.click(button("Review & send"));
    expect(screen.getByText("Reason is required (1–1000 characters)")).toBeDefined();
    fireEvent.change(area(CMD_REASON), { target: { value: "x".repeat(1000) } });
    fireEvent.click(button("Review & send"));
    expect(screen.queryByText("Reason is required (1–1000 characters)")).toBeNull();
    fireEvent.click(button("Cancel", screen.getByRole("dialog")));
    expect(screen.queryByRole("dialog")).toBeNull();
    fireEvent.click(button("Review & send"));
    fireEvent.keyDown(screen.getByRole("dialog"), { key: "Escape" });
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(vi.mocked(sendDiagnostic)).not.toHaveBeenCalled();
  });
  it("shows the API's refusal as a banner and its field errors in words; the next success clears them", async () => {
    store.next = refusedWith("VALIDATION", { action: "error.unsupportedAction" });
    render(<ControlView live={live()} />);
    fireEvent.change(area(CMD_REASON), { target: { value: "Check" } });
    fireEvent.click(button("Review & send"));
    fireEvent.click(button("Send command", screen.getByRole("dialog")));
    expect(await screen.findByText("Not supported by this AC")).toBeDefined();
    store.next = { ok: true, value: { id: "cmd-abcdefgh" } };
    fireEvent.click(button("Review & send"));
    fireEvent.click(button("Send command", screen.getByRole("dialog")));
    await waitFor(() => { expect(screen.queryByText("Not supported by this AC")).toBeNull(); });
  });
  it("cannot be reviewed while the unit is blocked, a run is open or the unit offers no actions", () => {
    const view = render(<ControlView live={live({}, { blocked: "firmware update" })} />);
    expect(button("Review & send").disabled).toBe(true);
    expect(screen.getByText("Control is blocked right now (firmware update) — commands and test runs wait until it clears.")).toBeDefined();
    view.unmount();
    const busy = render(<ControlView live={live({ active: true })} />);
    expect(button("Review & send").disabled).toBe(true);
    busy.unmount();
    render(<ControlView live={live({}, { options: [] })} />);
    expect(button("Review & send").disabled).toBe(true);
    expect(button("Start test run").disabled).toBe(true);
    expect(screen.getByRole<HTMLSelectElement>("combobox", { name: "Action" }).value).toBe("");
  });
  it("falls back to the first offered action, and to power on with nothing offered", () => {
    const fan: ActionGroup[] = [{ id: "fan", group: "Fan", options: [opt("fan:high")] }];
    render(<ControlView live={live({}, { options: fan })} />);
    expect(screen.getByRole<HTMLSelectElement>("combobox", { name: "Action" }).value).toBe("fan:high");
    expect(screen.getByRole<HTMLSelectElement>("combobox", { name: "Start action" }).value).toBe("fan:high");
  });
});

describe("ControlView test run", () => {
  it("checks the duration and the reason, then starts the run with both versions", async () => {
    render(<ControlView live={live()} />);
    const minutes = screen.getByRole<HTMLInputElement>("textbox", { name: "Duration (min)" });
    for (const bad of ["0", "16", "2.5", ""]) {
      fireEvent.change(minutes, { target: { value: bad } });
      fireEvent.click(button("Start test run"));
      expect(screen.getByText("A test run lasts 1–15 minutes")).toBeDefined();
    }
    await waitFor(() => { expect(document.activeElement).toBe(minutes); });
    fireEvent.change(minutes, { target: { value: "15" } });
    fireEvent.click(button("Start test run"));
    expect(screen.queryByText("A test run lasts 1–15 minutes")).toBeNull();
    await waitFor(() => { expect(document.activeElement).toBe(area(RUN_REASON)); });
    fireEvent.change(area(RUN_REASON), { target: { value: "Recovery after cleaning" } });
    fireEvent.click(button("Start test run"));
    const dialog = screen.getByRole("dialog", { name: "Start this test run?" });
    expect(within(dialog).getByText("Power ON for 15 min, then Power OFF")).toBeDefined();
    expect(within(dialog).getByText("On · Cool")).toBeDefined();
    fireEvent.click(button("Start test run", dialog));
    await waitFor(() => { expect(store.toasts).toStrictEqual(["Test run requested — waiting for the start acknowledgement"]); });
    expect(vi.mocked(startTestRun)).toHaveBeenCalledWith({
      jobId: "job-12345678", unitId: "unit-1", startAction: { kind: "set_power", power: true }, endAction: { kind: "set_power", power: false }, durationMinutes: 15,
      reason: "Recovery after cleaning", expectedUnitVersion: 7, expectedJobVersion: 3,
    });
    expect(vi.mocked(sendDiagnostic)).not.toHaveBeenCalled();
  });
  it("puts the API's reason error under the test run's own reason, not the command's", async () => {
    store.next = refusedWith("VALIDATION", { reason: "error.length", durationMinutes: "error.range" });
    render(<ControlView live={live()} />);
    fireEvent.change(screen.getByRole("combobox", { name: "End action" }), { target: { value: "mode:dry" } });
    fireEvent.change(screen.getByRole("combobox", { name: "Start action" }), { target: { value: "mode:cool" } });
    fireEvent.change(area(RUN_REASON), { target: { value: "Run" } });
    fireEvent.click(button("Start test run"));
    expect(within(screen.getByRole("dialog")).getByText("set_mode = cool for 5 min, then set_mode = dry")).toBeDefined();
    fireEvent.click(button("Start test run", screen.getByRole("dialog")));
    expect(await screen.findByText("1–1000 characters")).toBeDefined();
    expect(screen.getByText("1–15 minutes")).toBeDefined();
    // the run's reason field is marked, the command card's is not
    expect(area(RUN_REASON).getAttribute("aria-invalid")).toBe("true");
    expect(area(CMD_REASON).getAttribute("aria-invalid")).toBeNull();
    expect(screen.getByDisplayValue("5").getAttribute("aria-invalid")).toBe("true");
  });
  it("names the run button by the run's state", () => {
    const running = render(<ControlView live={live({ run: testRun(), active: true })} />);
    expect(button("Start test run (running)").disabled).toBe(true);
    running.unmount();
    const done = render(<ControlView live={live({ run: testRun({ state: "completed" }) })} />);
    expect(button("Start new test run").disabled).toBe(false);
    done.unmount();
    render(<ControlView live={live({ run: testRun({ state: "start_failed" }) })} />);
    expect(button("Start test run").disabled).toBe(false);
  });
  it("sends the end of a run with no answer again as a command carrying the run's reason", async () => {
    render(<ControlView live={live({ run: testRun({ state: "end_failed" }), banner: { tone: "crit", title: "End not acknowledged", text: "Send it again" } })} />);
    expect(screen.getByText("End not acknowledged")).toBeDefined();
    fireEvent.click(button("Retry end command"));
    await waitFor(() => { expect(store.toasts).toStrictEqual(["cmd-1234 sent — waiting for the device"]); });
    expect(vi.mocked(sendDiagnostic)).toHaveBeenCalledWith({ unitId: "unit-1", jobId: "job-12345678", action: { kind: "set_power", power: false }, reason: "End of test run sent again: Check cooling", expectedUnitVersion: 7 });
  });
  it("keeps the retried end's reason within 1000 characters, and shows a refused retry", async () => {
    store.next = refusedWith("CONFLICT", {});
    render(<ControlView live={live({ run: testRun({ state: "end_failed", reason: "r".repeat(1000) }) })} />);
    fireEvent.click(button("Retry end command"));
    await waitFor(() => { expect(vi.mocked(sendDiagnostic)).toHaveBeenCalledOnce(); });
    const sent = vi.mocked(sendDiagnostic).mock.calls[0]?.[0];
    expect(sent?.reason).toHaveLength(1000);
    expect(store.toasts).toStrictEqual([]);
  });
  it("offers no retry outside the work window, nor for a blocked end", () => {
    const out = render(<ControlView live={live({ run: testRun({ state: "end_failed" }) }, {}, { inWindow: false })} />);
    expect(button("Retry end command").disabled).toBe(true);
    expect(screen.getByText("Retry not available — no authorization")).toBeDefined();
    expect(screen.getByText("Outside your work window 10:00–12:00 — diagnostic commands can’t be sent now.")).toBeDefined();
    expect(screen.getByText("10:00–12:00", { selector: "span" }).className).toContain("text-crit");
    out.unmount();
    render(<ControlView live={live({ run: testRun({ state: "end_blocked" }) })} />);
    expect(screen.queryByRole("button", { name: "Retry end command" })).toBeNull();
    expect(screen.getByText("Retry not available — no authorization")).toBeDefined();
  });
});

describe("ControlView state, history and refresh", () => {
  it("shows the restriction, the tiles, the job's history and the card's last command in its tone", () => {
    const history = [row({ id: "h-run", run: true, title: "test run start", badge: { text: "Running", tone: "primary" } }), row({ id: "h-2", title: "set_mode = dry", sub: "rejected", badge: { text: "Rejected", tone: "crit" } }), row()];
    render(<ControlView live={live({ history }, { restriction: "min 24 °C", pending: 2 })} />);
    expect(screen.getByText("Restriction active — min 24 °C applies to diagnostics too.")).toBeDefined();
    expect(screen.getByText("31 °C").className).toContain("text-crit");
    expect(screen.getByText("On").className).not.toContain("text-crit");
    expect(screen.getByText("set_mode = dry — rejected").className).toContain("bg-crit-soft");
    expect(screen.getByText("10:01 · test run start")).toBeDefined();
    expect(screen.getByText("rejected", { selector: "div" }).className).toContain("text-crit");
    expect(screen.getByText("2")).toBeDefined();
    expect(screen.getByText("min 24 °C")).toBeDefined();
  });
  it("gives a last command of another tone the neutral look, and says when there is no history", () => {
    const view = render(<ControlView live={live({ history: [row({ sub: "waiting", badge: { text: "Sent", tone: "warn" } })] })} />);
    expect(screen.getByText("set_mode = cool — waiting").className).toContain("bg-surface2");
    view.unmount();
    render(<ControlView live={live()} />);
    expect(screen.getByText("No commands for this job yet.")).toBeDefined();
    expect(screen.getByText("none")).toBeDefined();
  });
  it("reads the page again every 4 s only while a command or run is open, and stops when it closes", () => {
    vi.useFakeTimers();
    const view = render(<ControlView live={live({ active: true })} />);
    act(() => { vi.advanceTimersByTime(8000); });
    expect(store.refresh).toHaveBeenCalledTimes(2);
    view.rerender(<ControlView live={live({ active: false })} />);
    act(() => { vi.advanceTimersByTime(8000); });
    expect(store.refresh).toHaveBeenCalledTimes(2);
  });
});

describe("ControlDemo", () => {
  it("checks the reason and the setpoint, then adds the sent command to the history", () => {
    render(<ControlDemo id="unit-12" />);
    const [commandReason, runReason] = screen.getAllByRole<HTMLTextAreaElement>("textbox", { name: "Reason (required)" });
    if (!commandReason || !runReason) throw new Error("two reasons expected");
    fireEvent.change(commandReason, { target: { value: " " } });
    fireEvent.change(screen.getByRole("spinbutton", { name: "Set temp (°C)" }), { target: { value: "31" } });
    fireEvent.click(button("Review & send"));
    expect(screen.getByText("Reason is required.")).toBeDefined();
    expect(screen.getByText("VALIDATION — temperature must be 16–30 °C.")).toBeDefined();
    expect(screen.queryByRole("dialog")).toBeNull();
    fireEvent.change(commandReason, { target: { value: "Airflow" } });
    fireEvent.change(screen.getByRole("spinbutton", { name: "Set temp (°C)" }), { target: { value: "20" } });
    fireEvent.change(screen.getByRole("combobox", { name: "Action" }), { target: { value: "dry" } });
    fireEvent.click(button("Review & send"));
    expect(screen.queryByText("Reason is required.")).toBeNull();
    const dialog = screen.getByRole("dialog");
    expect(within(dialog).getByText("set_mode = dry")).toBeDefined();
    fireEvent.click(button("Send command", dialog));
    expect(screen.getByText("now · set_mode = dry")).toBeDefined();
    expect(store.toasts).toStrictEqual(["Command acknowledged by device"]);
    expect(screen.getByText("cmd-0416 · rejected — below model minimum 16 °C (VALIDATION)").className).toContain("text-crit");
  });
  it("refuses a setpoint below the restriction's minimum on the limited unit only", () => {
    render(<ControlDemo id="unit-limited" />);
    expect(screen.getByText("Cooling restriction active — minimum 24 °C applies to diagnostics too.")).toBeDefined();
    expect(screen.getByText("min 24 °C")).toBeDefined();
    fireEvent.change(screen.getByRole("spinbutton", { name: "Set temp (°C)" }), { target: { value: "20" } });
    fireEvent.click(button("Review & send"));
    expect(screen.getByText("FORBIDDEN — 20 °C is below the restriction minimum 24 °C.")).toBeDefined();
    fireEvent.change(screen.getByRole("spinbutton", { name: "Set temp (°C)" }), { target: { value: "15" } });
    fireEvent.click(button("Review & send"));
    expect(screen.queryByText(/FORBIDDEN/)).toBeNull();
    expect(screen.getByText("VALIDATION — temperature must be 16–30 °C.")).toBeDefined();
  });
  it("ends the running test run, then checks and starts a new one; Cancel closes the confirmation", () => {
    render(<ControlDemo id="unit-12" />);
    expect(button("Start test run").disabled).toBe(true);
    fireEvent.click(button("Simulate end"));
    expect(screen.getByText("End acknowledged — test run completed, unit Stopped.")).toBeDefined();
    expect(store.toasts).toStrictEqual(["End acknowledged — Stopped"]);
    const duration = screen.getByRole("spinbutton", { name: "Duration (min)" });
    const [, runReason] = screen.getAllByRole<HTMLTextAreaElement>("textbox", { name: "Reason (required)" });
    if (!runReason) throw new Error("the run's reason expected");
    fireEvent.change(duration, { target: { value: "16" } });
    fireEvent.change(runReason, { target: { value: "" } });
    fireEvent.click(button("Start test run"));
    expect(screen.getByText("VALIDATION — test run is 1–15 minutes only.")).toBeDefined();
    expect(screen.getByText("Reason is required.")).toBeDefined();
    fireEvent.change(duration, { target: { value: "0" } });
    fireEvent.click(button("Start test run"));
    expect(screen.getByText("VALIDATION — test run is 1–15 minutes only.")).toBeDefined();
    fireEvent.change(duration, { target: { value: "10" } });
    fireEvent.change(runReason, { target: { value: "Recheck" } });
    fireEvent.click(button("Start test run"));
    expect(within(screen.getByRole("dialog")).getByText("test run 10 min")).toBeDefined();
    expect(within(screen.getByRole("dialog")).getByText("Recheck")).toBeDefined();
    fireEvent.click(button("Cancel", screen.getByRole("dialog")));
    expect(screen.queryByRole("dialog")).toBeNull();
    fireEvent.click(button("Start test run"));
    fireEvent.keyDown(screen.getByRole("dialog"), { key: "Escape" });
    expect(screen.queryByRole("dialog")).toBeNull();
    fireEvent.click(button("Start test run"));
    fireEvent.click(button("Send command", screen.getByRole("dialog")));
    expect(screen.getByText("now · test run start (Power ON, 10 min)")).toBeDefined();
    expect(screen.getByRole("button", { name: "Simulate end" })).toBeDefined();
  });
});
