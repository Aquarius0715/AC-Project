// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { isValidElement, type ComponentProps, type ReactNode } from "react";
import type { ApiAudit, ApiAuditDeviceEvent } from "@ac/web/lib/audit";
import { historyRow, type ApiCommand } from "@ac/web/lib/units";

// The audit explorer of HQ (FR-A16, SCR-A16): the Server Component reads the log for the URL filters, the selected
// entry's correlation and the device events of one chosen device; the view keeps every filter in the URL in API mode
// and filters its seed rows in the Phase 1A demo. Read only: nothing here writes.
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
  const orWhenRefused = async <T, F>(read: Promise<T>, fallback: F): Promise<T | F> => {
    try {
      return await read;
    } catch (error) {
      if (error instanceof CoreError && error.error.code !== "UNAVAILABLE") return fallback;
      throw error;
    }
  };
  return {
    CoreError, orWhenRefused, apiMode: vi.fn(() => true), coreNow: vi.fn(() => Promise.resolve(new Date("2026-09-22T02:00:00Z"))),
    coreDisplay: vi.fn(() => Promise.resolve({ locale: "en", timeZone: "Asia/Kuala_Lumpur" })),
    coreOp: vi.fn<(op: string, input?: unknown) => Promise<unknown>>(), coreAll: vi.fn<(op: string, input?: unknown) => Promise<unknown>>(),
    corePermissions: vi.fn(() => Promise.resolve(new Set<string>())),
  };
});
const nav = vi.hoisted(() => ({ patch: vi.fn<(p: Record<string, string | null>) => void>() }));
const ops = vi.hoisted(() => {
  class OpError extends Error {
    readonly error: { code: string };
    constructor(code: string, options?: ErrorOptions) {
      super(code, options);
      this.name = "OpError";
      this.error = { code };
    }
  }
  return { OpError, callOp: vi.fn<(op: string, input?: unknown) => Promise<unknown>>() };
});
vi.mock("server-only", () => ({}));
vi.mock("next/server", () => ({ connection: () => Promise.resolve() }));
vi.mock("@ac/web/lib/dal", () => dal);
vi.mock("@ac/web/lib/useUrlPatch", () => ({ useUrlPatch: () => nav.patch }));
vi.mock("@ac/web/lib/ops", () => ops);

import AdminAuditPage from "../app/admin/audit/page";
import { AuditView, type AuditLive } from "../app/admin/audit/_components/audit-view";
import AuditLoading from "../app/admin/audit/loading";
import AuditErrorState from "../app/admin/audit/error";
import { RouteError, RouteLoading } from "@ac/web/components/RouteStates";
import { NeedsAccess } from "@ac/web/components/NeedsAccess";

const refused = (code: string) => new dal.CoreError({ code, messageKey: `error.${code.toLowerCase()}`, fieldErrors: {}, retryAfterSeconds: null });
const entry = (id: string, over: Partial<ApiAudit> = {}): ApiAudit => ({
  id, actorId: "user-op", actorName: "HQ Operator", actorRoleAtTime: "admin", action: "restriction.override", targetRef: { kind: "restriction", id: "rst-11112222" },
  previousVersion: 3, nextVersion: 4, occurredAt: "2026-09-21T01:00:00Z", correlationId: "corr-a", result: "success",
  maskedBefore: { state: "applied" }, maskedAfter: { state: "lifted" }, reason: "Paid at the counter", ...over,
});
const deviceEvent = (id: string, over: Partial<ApiAuditDeviceEvent> = {}): ApiAuditDeviceEvent => ({
  id, version: 1, deviceId: "dev-b", eventType: "tamper", evidenceSource: "tamper_signal", occurredAt: "2026-09-21T03:00:00Z", restoredAt: null, alertIds: [],
  recovery: null, responseNotes: [], sequence: 7, unitIdAtOccurrence: "unit-1", ...over,
});
/** The Core API of one test: each operation answers with its value, or `answers` decides from the input. */
type Answer = (input: unknown) => unknown;
const is = (value: unknown): Answer => () => value;
const fails = (code: string): Answer => () => Promise.reject(refused(code));
/** Whether an operation's input mentions this text (a filter or an ID). */
const mentions = (input: unknown, text: string) => JSON.stringify(input).includes(text);
function core(answers: Record<string, Answer>) {
  const table = new Map(Object.entries(answers));
  const answer = (op: string, input?: unknown) => Promise.try(() => table.get(op)?.(input));
  dal.coreOp.mockImplementation(answer);
  dal.coreAll.mockImplementation(answer);
}
const props = (sp: Record<string, string> = {}) => ({ searchParams: Promise.resolve(sp), params: Promise.resolve({}) });
async function liveOf(sp: Record<string, string> = {}): Promise<AuditLive> {
  const page: ReactNode = await AdminAuditPage(props(sp));
  if (!isValidElement<ComponentProps<typeof AuditView>>(page) || page.type !== AuditView || !page.props.live) throw new TypeError("not the live AuditView");
  return page.props.live;
}
const opCalls = (op: string) => dal.coreOp.mock.calls.filter(([name]) => name === op).map(([, input]) => input);

beforeEach(() => {
  dal.apiMode.mockReturnValue(true);
  dal.coreOp.mockReset(); dal.coreAll.mockReset(); nav.patch.mockReset(); ops.callOp.mockReset();
});
afterEach(() => { cleanup(); vi.useRealTimers(); });

describe("the audit page (Server Component)", () => {
  it("shows the demo outside API mode, and its loading and error states", async () => {
    dal.apiMode.mockReturnValue(false);
    const page: ReactNode = await AdminAuditPage(props());
    expect(isValidElement<ComponentProps<typeof AuditView>>(page) && page.type === AuditView && page.props.live).toBeUndefined();
    const loading = AuditLoading();
    expect(loading.type === RouteLoading && loading.props).toStrictEqual({ what: "the audit log" });
    const error = new Error("down"), retry = vi.fn();
    const failed = AuditErrorState({ error, retry });
    expect(failed.type === RouteError && failed.props).toStrictEqual({ what: "The audit log", error, retry });
  });

  it("reads the log for the URL filters and the newest entry's correlation, with names from members.list", async () => {
    const sel = entry("e-1"), other = entry("e-2", { action: "commands.create", targetRef: { kind: "command", id: "cmd-33334444" }, occurredAt: "2026-09-21T01:05:00Z", result: "pending" });
    core({
      "audit.list": (q) => (mentions(q, "correlationId") ? { items: [sel, other], total: 2 } : { items: [sel, entry("e-3", { correlationId: "corr-b", result: "denied" })], total: 30 }),
      "members.list": is([{ userId: "user-op", displayName: "HQ Operator" }]),
    });
    const live = await liveOf({ from: "2026-09-01", to: "2026-09-30", result: "Success", actorId: "user-op" });
    expect(opCalls("audit.list")[0]).toMatchObject({ limit: 25, filters: { actorId: "user-op", result: "success" } });
    expect(opCalls("audit.list")[1]).toMatchObject({ limit: 100, filters: { correlationId: "corr-a" } });
    expect([live.tab, live.total, live.searched, live.selected, live.entryMissing]).toStrictEqual(["log", 30, true, "e-1", false]);
    expect(live.rows.map((r) => [r.id, r.res])).toStrictEqual([["e-1", "Success"], ["e-3", "Denied"]]);
    expect(live.actors).toStrictEqual([{ id: "user-op", name: "HQ Operator" }]);
    expect(live.trace.map((x) => x.id)).toStrictEqual(["e-1", "e-2"]);
    expect([live.related?.restriction?.via, live.related?.command?.id]).toStrictEqual(["target", "cmd-33334444"]);
  });

  it("flags an entryId that is not in the results, and keeps going without members when they are refused", async () => {
    core({ "audit.list": is({ items: [entry("e-1")], total: 1 }), "members.list": fails("FORBIDDEN") });
    const live = await liveOf({ entryId: "e-gone" });
    expect([live.selected, live.entryMissing, live.trace, live.related]).toStrictEqual([null, true, [], null]);
    expect(opCalls("audit.list")).toHaveLength(1); // no correlation read without a selected entry
  });

  it("searches nothing for a reversed period", async () => {
    core({ "audit.list": is({ items: [entry("e-1")], total: 1 }), "members.list": is([]) });
    const live = await liveOf({ from: "2026-09-30", to: "2026-09-01" });
    expect([live.searched, live.total, live.rows]).toStrictEqual([false, 0, []]);
    expect(opCalls("audit.list")).toStrictEqual([]);
  });

  it("names the permission the role lacks when the log is refused (IR582)", async () => {
    core({ "audit.list": fails("FORBIDDEN"), "members.list": is([]) });
    const page: ReactNode = await AdminAuditPage(props());
    expect(isValidElement(page) && page.type === NeedsAccess && page.props).toStrictEqual({ what: "Audit", permission: "audit.read" });
  });

  it("lists the devices by serial and reads events only for the URL's device, naming a refused alert by its ID", async () => {
    core({
      "audit.list": is({ items: [], total: 0 }), "members.list": is([{ userId: "user-op", displayName: "HQ Operator" }]),
      "devices.list": is([{ id: "dev-b", serial: "SN-B", unitId: "unit-1" }, { id: "dev-a", serial: "SN-A", unitId: null }, { id: "dev-c", serial: "SN-C", unitId: "unit-99999999" }]),
      "units.list": is([{ id: "unit-1", displayName: "Lobby AC" }]),
      "devices.events": is({ items: [deviceEvent("ev-1", { alertIds: ["al-1", "al-2"] }), deviceEvent("ev-2", { eventType: "restored", evidenceSource: "heartbeat" })], total: 2 }),
      "alerts.get": (q) => (mentions(q, "al-1") ? { id: "al-1", version: 1, status: "open", type: "tamper", severity: "warning" } : fails("NOT_FOUND")(q)),
    });
    const live = await liveOf({ tab: "devices", deviceId: "dev-b", eventId: "ev-2" });
    expect(live.device?.options).toStrictEqual([{ id: "dev-a", label: "SN-A · Unbound" }, { id: "dev-b", label: "SN-B · Lobby AC" }, { id: "dev-c", label: "SN-C · unit unit-999" }]);
    expect([live.device?.deviceId, live.device?.label, live.device?.missing, live.device?.total, live.device?.selected]).toStrictEqual(["dev-b", "SN-B · Lobby AC", false, 2, "ev-2"]);
    expect(opCalls("devices.events")[0]).toMatchObject({ id: "dev-b", query: { limit: 50 } });
    expect(live.device?.events[0]?.alerts.map((a) => [a.text, a.href])).toStrictEqual([["tamper · open", "/admin/alerts?alertId=al-1"], ["alert al-2", "/admin/alerts?alertId=al-2"]]); // a refused alert keeps only its ID
  });

  it("reads no events without a device or with a reversed period, flags an unknown device, and opens the newest event by default", async () => {
    core({ "audit.list": is({ items: [], total: 0 }), "members.list": is([]), "devices.list": is([{ id: "dev-b", serial: "SN-B", unitId: null }]), "units.list": fails("FORBIDDEN"), "devices.events": is({ items: [deviceEvent("ev-9")], total: 1 }) });
    const unknown = await liveOf({ tab: "devices", deviceId: "dev-x" });
    expect(unknown.device).toMatchObject({ deviceId: null, missing: true, label: null, events: [] });
    expect(opCalls("devices.events")).toStrictEqual([]);
    const known = await liveOf({ tab: "devices", deviceId: "dev-b" });
    expect(known.device?.selected).toBe("ev-9");
    const none = await liveOf({ tab: "devices" });
    expect(none.device).toMatchObject({ deviceId: null, missing: false });
    core({ "audit.list": is({ items: [], total: 0 }), "members.list": is([]), "devices.list": is([{ id: "dev-b", serial: "SN-B", unitId: null }]), "units.list": is([]), "devices.events": is({ items: [], total: 0 }) });
    const quiet = await liveOf({ tab: "devices", deviceId: "dev-b" });
    expect(quiet.device).toMatchObject({ deviceId: "dev-b", events: [], selected: null });
    dal.coreOp.mockClear();
    const reversed = await liveOf({ tab: "devices", deviceId: "dev-b", from: "2026-09-30", to: "2026-09-01" });
    expect(reversed.device).toMatchObject({ deviceId: "dev-b", total: 0 }); // a reversed period reads no events
    expect(opCalls("devices.events")).toStrictEqual([]);
  });
});

/** A live view as the page builds it, with these changes. */
function liveView(over: Partial<AuditLive> = {}): AuditLive {
  return {
    tab: "log", total: 2, searched: true, actors: [{ id: "user-op", name: "HQ Operator" }], kinds: ["command", "restriction"], selected: "e-1", entryMissing: false,
    filters: { from: "2026-09-01", to: "2026-09-30", actorId: "", targetKind: "", targetId: "", correlationId: "", result: "All", limit: 25 },
    rows: [
      { id: "e-1", op: "restriction.override", target: "restriction · rst-1111", targetKind: "restriction", targetId: "rst-11112222", versions: "version 3 → 4", actor: "HQ Operator", actorId: "user-op", role: "admin", at: "09-21 09:00", occurred: "2026-09-21 09:00", corr: "corr-a", res: "Success", reason: "Paid", changes: [{ field: "state", before: "applied", after: "lifted", changed: true }] },
      { id: "e-2", op: "commands.create", target: "command · cmd-3333", targetKind: "command", targetId: "cmd-33334444", versions: null, actor: "HQ Operator", actorId: "user-op", role: "admin", at: "09-21 09:05", occurred: "2026-09-21 09:05", corr: "corr-a", res: "Pending", reason: null, changes: [] },
    ],
    trace: [{ id: "e-1", time: "09:00", title: "restriction.override", detail: "Success", tone: "ok" }, { id: "e-2", time: "09:05", title: "commands.create", detail: "Pending", tone: "primary" }],
    related: { restriction: { id: "rst-11112222", via: "target", note: "as recorded" }, command: { id: "cmd-33334444", via: "correlation", note: "same correlation ID" }, job: { id: "job-55556666", via: "correlation", note: "same correlation ID" }, unit: { id: "unit-77778888", via: "correlation", note: "Lobby AC" } },
    ...over,
  };
}
const button = (name: string | RegExp) => screen.getByRole("button", { name });
const hrefOf = (name: string | RegExp) => screen.getAllByRole("link", { name }).map((a) => a.getAttribute("href"));

describe("the audit view in API mode", () => {
  it("shows the selected entry with its trace, changes and related records, and filters by its actor, ID and target", () => {
    render(<AuditView live={liveView()} />);
    expect(screen.getByText("This entry")).toBeDefined();
    expect(screen.getByText("lifted").className).toContain("font-bold");
    expect(hrefOf(/Open restriction/)).toStrictEqual(["/admin/restrictions?restrictionId=rst-11112222"]);
    expect(hrefOf(/Open job/)).toStrictEqual(["/admin/jobs?jobId=job-55556666"]);
    expect(hrefOf(/Open unit/)).toStrictEqual(["/admin/units?unitId=unit-77778888"]);
    fireEvent.click(button(/Filter by actor/));
    expect(nav.patch).toHaveBeenLastCalledWith({ actorId: "user-op", limit: null }); // the open entry stays open
    fireEvent.click(button(/Filter by ID/));
    expect(nav.patch).toHaveBeenLastCalledWith({ correlationId: "corr-a", limit: null });
    fireEvent.click(button(/Filter by target/));
    expect(nav.patch).toHaveBeenLastCalledWith({ targetKind: "restriction", targetId: "rst-11112222", limit: null });
    fireEvent.click(button(/commands\.create/));
    expect(nav.patch).toHaveBeenLastCalledWith({ entryId: "e-2" });
  });

  it("puts a new search in the URL from its newest entry, and resets every filter", () => {
    render(<AuditView live={liveView({ total: 60 })} />);
    fireEvent.change(screen.getByLabelText("Actor"), { target: { value: "user-op" } });
    expect(nav.patch).toHaveBeenLastCalledWith({ actorId: "user-op", limit: null, entryId: null, eventId: null });
    fireEvent.change(screen.getByLabelText("Target"), { target: { value: "command" } });
    expect(nav.patch).toHaveBeenLastCalledWith({ targetKind: "command", targetId: "", limit: null, entryId: null, eventId: null });
    fireEvent.click(button("Load more"));
    expect(nav.patch).toHaveBeenLastCalledWith({ limit: "50" });
    fireEvent.click(button("Reset"));
    expect(nav.patch).toHaveBeenLastCalledWith({ correlationId: null, result: null, from: null, to: null, actorId: null, targetKind: null, targetId: null, limit: null, entryId: null, eventId: null });
  });

  it("waits 300 ms before searching a typed correlation ID, and searches a period only when it is valid", () => {
    vi.useFakeTimers();
    render(<AuditView live={liveView()} />);
    fireEvent.change(screen.getByLabelText("Correlation ID"), { target: { value: "corr-" } });
    fireEvent.change(screen.getByLabelText("Correlation ID"), { target: { value: "corr-z" } });
    expect(nav.patch).not.toHaveBeenCalled();
    act(() => { vi.advanceTimersByTime(300); });
    expect(nav.patch.mock.calls).toStrictEqual([[{ correlationId: "corr-z", limit: null, entryId: null }]]);
    fireEvent.change(screen.getByLabelText(/^From/), { target: { value: "2026-10-05" } }); // after To: nothing is searched
    expect(nav.patch).toHaveBeenCalledOnce();
    fireEvent.change(screen.getByLabelText(/^From/), { target: { value: "2026-09-10" } });
    expect(nav.patch).toHaveBeenLastCalledWith({ from: "2026-09-10", to: "2026-09-30", limit: null, entryId: null, eventId: null });
    fireEvent.change(screen.getByLabelText("To"), { target: { value: "2026-09-20" } });
    expect(nav.patch).toHaveBeenLastCalledWith({ from: "2026-09-10", to: "2026-09-20", limit: null, entryId: null, eventId: null });
  });

  it("clears the target ID of a filtered target, shown with its kind when it has one", () => {
    const { rerender } = render(<AuditView live={liveView({ filters: { ...liveView().filters, targetKind: "command", targetId: "cmd-33334444" } })} />);
    expect(screen.getByText("command · cmd-33334444")).toBeDefined();
    fireEvent.click(button("Clear the target ID"));
    expect(nav.patch).toHaveBeenLastCalledWith({ targetId: "", limit: null, entryId: null, eventId: null });
    rerender(<AuditView live={liveView({ filters: { ...liveView().filters, targetId: "cmd-33334444" } })} />);
    expect(screen.getByText("cmd-33334444", { selector: ".font-mono" })).toBeDefined();
  });

  it("files a result in the URL, All as no key, and switches the tab through the URL", () => {
    const { rerender } = render(<AuditView live={liveView()} />);
    fireEvent.click(button("Denied"));
    expect(nav.patch).toHaveBeenLastCalledWith({ result: "Denied", limit: null, entryId: null, eventId: null });
    rerender(<AuditView live={liveView({ filters: { ...liveView().filters, result: "Denied" } })} />);
    fireEvent.click(button("All"));
    expect(nav.patch).toHaveBeenLastCalledWith({ result: null, limit: null, entryId: null, eventId: null });
    fireEvent.click(screen.getByRole("tab", { name: /Device events/ }));
    expect(nav.patch).toHaveBeenLastCalledWith({ tab: "devices" });
  });

  it("says why the list is empty: nothing searched, an unknown correlation ID, no pending entries, or no match", () => {
    const empty = { rows: [], total: 0, selected: null, related: null, trace: [] };
    const { rerender } = render(<AuditView live={liveView({ ...empty, searched: false })} />);
    expect(screen.getByText("Nothing was searched")).toBeDefined();
    rerender(<AuditView live={liveView({ ...empty, filters: { ...liveView().filters, correlationId: "corr-x" } })} />);
    expect(screen.getByText("No entries with this correlation ID")).toBeDefined();
    rerender(<AuditView live={liveView({ ...empty, filters: { ...liveView().filters, result: "Pending" } })} />);
    expect(screen.getByText("No entries with the result Pending")).toBeDefined();
    rerender(<AuditView live={liveView({ ...empty, filters: { ...liveView().filters, result: "Denied" } })} />);
    expect(screen.getByText("No audit entries match these filters")).toBeDefined();
    rerender(<AuditView live={liveView({ entryMissing: true, selected: null })} />);
    expect(screen.getByText("That entry is not in these results")).toBeDefined();
  });

  it("loads the related command only when asked, and words a refusal", async () => {
    const command: ApiCommand & { unitId: string } = { id: "cmd-33334444", action: { kind: "set_power", power: false }, status: "acknowledged", requestedAt: "2026-09-21T01:05:00Z", failureCode: null, unitId: "unit-1" };
    ops.callOp.mockResolvedValueOnce(command);
    const { unmount } = render(<AuditView live={liveView()} />);
    expect(ops.callOp).not.toHaveBeenCalled();
    fireEvent.click(button("Load command"));
    await waitFor(() => { expect(screen.queryByRole("button", { name: "Load command" })).toBeNull(); });
    const shown = historyRow(command);
    expect(screen.getByText(`${shown.text} · ${shown.when}`)).toBeDefined();
    expect(hrefOf(/Open unit/)).toStrictEqual(["/admin/units?unitId=unit-1", "/admin/units?unitId=unit-77778888"]); // the command's unit, then the related unit
    expect(ops.callOp).toHaveBeenCalledWith("commands.get", { id: "cmd-33334444" });
    unmount();
    for (const [code, text] of [["FORBIDDEN", /You may not read this command/], ["NOT_FOUND", "This command is outside your scope or no longer exists."], ["UNAVAILABLE", "The command could not be loaded — try again."], ["", "The command could not be loaded — try again."]] as const) {
      ops.callOp.mockRejectedValueOnce(code ? new ops.OpError(code) : new TypeError("Failed to fetch")); // no answer at all reads as a failure too
      const view = render(<AuditView live={liveView()} />);
      fireEvent.click(button("Load command"));
      expect(await screen.findByText(text)).toBeDefined();
      view.unmount();
    }
  });

  it("shows a chosen device's events and puts the device and event choices in the URL", () => {
    const device: NonNullable<AuditLive["device"]> = {
      options: [{ id: "dev-b", label: "SN-B · Lobby AC" }], deviceId: "dev-b", missing: false, label: "SN-B · Lobby AC", total: 2, selected: "ev-2",
      events: [
        { id: "ev-1", title: "Tamper", type: "tamper", tone: "warn", meta: "seq 7", state: "open", badge: { text: "Not restored", tone: "crit" }, facts: [["Event type", "tamper"]], alerts: [{ id: "al-1", text: "tamper · open", href: "/admin/alerts?alertId=al-1" }], notes: [{ text: "Checked", by: "HQ · 09-21" }] },
        { id: "ev-2", title: "Restored", type: "restored", tone: "ok", meta: "seq 8", state: "restored", badge: null, facts: [["Event type", "restored"]], alerts: [], notes: [] },
      ],
    };
    const { rerender } = render(<AuditView live={liveView({ tab: "devices", device })} />);
    expect(screen.getByText("No response notes.")).toBeDefined(); // ev-2 is the URL's event
    expect(screen.getByText("SN-B · Lobby AC", { selector: "[data-slot=card-description]" })).toBeDefined(); // the event card names its device
    expect(hrefOf(/Filter log/)).toStrictEqual(["/admin/audit?targetKind=device_event&targetId=ev-2"]);
    fireEvent.click(button(/Tamper/));
    expect(nav.patch).toHaveBeenLastCalledWith({ eventId: "ev-1" });
    fireEvent.change(screen.getByLabelText("To"), { target: { value: "2026-09-25" } });
    expect(nav.patch).toHaveBeenLastCalledWith({ from: "2026-09-01", to: "2026-09-25", limit: null, entryId: null, eventId: null });
    fireEvent.change(screen.getByLabelText(/^From/), { target: { value: "2026-09-02" } });
    expect(nav.patch).toHaveBeenLastCalledWith({ from: "2026-09-02", to: "2026-09-25", limit: null, entryId: null, eventId: null });
    fireEvent.change(screen.getByLabelText("Device"), { target: { value: "" } });
    expect(nav.patch).toHaveBeenLastCalledWith({ deviceId: null, eventId: null });
    fireEvent.click(screen.getByRole("tab", { name: /Audit log/ }));
    expect(nav.patch).toHaveBeenLastCalledWith({ tab: null });
    rerender(<AuditView live={liveView({ tab: "devices", device: { ...device, selected: "ev-1" } })} />);
    expect(hrefOf(/Open alert/)).toStrictEqual(["/admin/alerts?alertId=al-1"]);
    expect(screen.getByText("“Checked”")).toBeDefined();
    rerender(<AuditView live={liveView({ tab: "devices", device: { ...device, events: [], total: 0 } })} />);
    expect(screen.getByText("This device recorded no events in the period.")).toBeDefined();
    rerender(<AuditView live={liveView({ tab: "devices", searched: false, filters: { ...liveView().filters, from: "2026-10-01" }, device: { ...device, events: [], total: 0 } })} />);
    expect(screen.getByText("The period must start before it ends and cover at most 366 days.")).toBeDefined();
    rerender(<AuditView live={liveView({ tab: "devices", device: { ...device, label: null } })} />);
    expect(screen.queryByText("SN-B · Lobby AC", { selector: "[data-slot=card-description]" })).toBeNull();
    rerender(<AuditView live={liveView({ tab: "devices", device: { ...device, deviceId: null, missing: true, events: [] } })} />);
    expect(screen.getByText("That device is not in your list")).toBeDefined();
    rerender(<AuditView live={liveView({ tab: "devices", device: { ...device, deviceId: null, missing: false, events: [] } })} />);
    expect(screen.getByText("Pick a device")).toBeDefined();
  });
});

describe("the audit view in the demo", () => {
  it("filters its seed rows locally and opens the first match", () => {
    render(<AuditView />);
    expect(screen.getByRole("tab", { name: /Audit log/ }).textContent).toBe("Audit log5");
    fireEvent.change(screen.getByLabelText("Actor"), { target: { value: "tech-ali" } });
    expect(screen.getAllByText("devices.calibrate").length).toBeGreaterThan(0);
    expect(screen.queryByText("access.grant")).toBeNull();
    fireEvent.click(button("Reset"));
    expect(screen.getAllByText("access.grant").length).toBeGreaterThan(0);
    expect(nav.patch).not.toHaveBeenCalled();
  });

  it("narrows its seed rows by result, correlation ID and target, without the URL", () => {
    render(<AuditView />);
    fireEvent.click(button("Denied"));
    expect(screen.getAllByText("access.grant").length).toBeGreaterThan(0);
    expect(screen.queryByText("devices.calibrate")).toBeNull();
    fireEvent.click(button("All"));
    fireEvent.change(screen.getByLabelText("Correlation ID"), { target: { value: "corr-51d" } });
    expect(screen.getAllByText("devices.calibrate").length).toBeGreaterThan(0);
    expect(screen.getByText("1 entries with this correlation ID")).toBeDefined();
    fireEvent.change(screen.getByLabelText("Correlation ID"), { target: { value: "" } });
    fireEvent.change(screen.getByLabelText("Target"), { target: { value: "command" } });
    expect(screen.getAllByText("commands.create").length).toBeGreaterThan(0);
    fireEvent.click(button(/Filter by target/));
    expect(screen.getByRole("button", { name: "Clear the target ID" })).toBeDefined(); // the target ID chip
    fireEvent.change(screen.getByLabelText("Target"), { target: { value: "restriction" } });
    fireEvent.click(button(/Filter by target/));
    fireEvent.change(screen.getByLabelText("Actor"), { target: { value: "tech-ali" } }); // no restriction entry by tech-ali
    expect(screen.getByText("No audit entries match these filters")).toBeDefined();
    expect(nav.patch).not.toHaveBeenCalled();
  });

  it("links only its first entry, loads its demo command without the API, and shows the demo device events", async () => {
    render(<AuditView />);
    fireEvent.click(button("Load command"));
    expect(await screen.findByText(/Set temperature 24°C/)).toBeDefined();
    expect(ops.callOp).not.toHaveBeenCalled();
    fireEvent.click(button(/access\.grant/));
    expect(screen.getByText("No related restriction")).toBeDefined();
    fireEvent.click(screen.getByRole("tab", { name: /Device events/ }));
    fireEvent.click(button(/Restored/));
    expect(screen.getByText("heartbeat — Connection restored")).toBeDefined();
    fireEvent.change(screen.getByLabelText("Device"), { target: { value: "" } }); // the demo keeps its one device
    expect(screen.getByText("heartbeat — Connection restored")).toBeDefined();
    expect(nav.patch).not.toHaveBeenCalled();
  });
});
