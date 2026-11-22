import { beforeEach, describe, expect, it, vi } from "vitest";
import { isValidElement, type ComponentProps } from "react";
import { ZodError } from "zod";
import type { ApiAutomation } from "@ac/web/lib/clientAutomations";

// The customer Automations page (FR-C04, FR-C05): its Server Actions send what the screen chose at the version it
// showed, check the two arguments that are not typed (a proposal card's selection and a browser position) before they
// reach the Core API, and re-read the page after a write but not after a test or a position report; the page reads the
// rules, ACs, consent and proposals, and the editor's capabilities only while editing.
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
  return { coreOp: vi.fn(), coreAll: vi.fn(), coreNow: vi.fn(), coreDisplay: vi.fn(), apiMode: vi.fn(), CoreError };
});
const cache = vi.hoisted(() => ({ refresh: vi.fn() }));
vi.mock("server-only", () => ({}));
vi.mock("next/cache", () => cache);
vi.mock("next/server", () => ({ connection: () => Promise.resolve() }));
vi.mock("@ac/web/lib/dal", () => dal);
vi.mock("../app/customer/automations/_components/automations-demo", () => ({ AutomationsDemo: () => null }));
vi.mock("../app/customer/automations/_components/automations-view", () => ({ AutomationsView: () => null }));

import { approveStartProposal, deleteAutomation, reportPresence, saveAutomation, testEvent, updateConsent } from "../app/customer/automations/actions";
import { parseStartProposals } from "../app/customer/automations/start-proposals";
import CustomerAutomationsPage from "../app/customer/automations/page";
import { AutomationsDemo } from "../app/customer/automations/_components/automations-demo";
import { AutomationsView } from "../app/customer/automations/_components/automations-view";
import AutomationsLoading from "../app/customer/automations/loading";
import AutomationsErrorState from "../app/customer/automations/error";
import { RouteError, RouteLoading } from "@ac/web/components/RouteStates";

const RULE = "11111111-1111-4111-8111-111111111111";
const UNIT = "22222222-2222-4222-8222-222222222222";
const PROPOSAL = "33333333-3333-4333-8333-333333333333";
const refused = (code: string, messageKey: string) => new dal.CoreError({ code, messageKey, fieldErrors: {}, retryAfterSeconds: null });
const VALIDATION = { ok: false, code: "VALIDATION", messageKey: "error.validation" };

beforeEach(() => {
  vi.resetAllMocks();
  dal.coreOp.mockResolvedValue({ id: RULE, version: 2 });
});

describe("automations actions", () => {
  it("a save goes at the rule's version (none for a new rule), a delete and a consent change at theirs, and each re-reads", async () => {
    const input = { name: "Pre-cool", unitIds: [UNIT], timezone: "Asia/Kuala_Lumpur", enabled: true, priority: 0, kind: "event" as const, condition: { type: "location" as const, event: "arrival" as const }, action: { kind: "set_power" as const, power: true } };
    await expect(saveAutomation(input, null)).resolves.toStrictEqual({ ok: true, value: { id: RULE, version: 2 } });
    expect(dal.coreOp.mock.lastCall).toStrictEqual(["automations.save", input, { write: true }]);
    await saveAutomation({ ...input, id: RULE }, 4);
    expect(dal.coreOp.mock.lastCall).toStrictEqual(["automations.save", { ...input, id: RULE }, { write: true, expectedVersion: 4 }]);
    await deleteAutomation(RULE, 5);
    expect(dal.coreOp.mock.lastCall).toStrictEqual(["automations.delete", { id: RULE }, { write: true, expectedVersion: 5 }]);
    dal.coreOp.mockRejectedValueOnce(refused("CONFLICT", "error.versionConflict"));
    await expect(updateConsent(false, 7)).resolves.toMatchObject({ ok: false, code: "CONFLICT", messageKey: "error.versionConflict" });
    expect(dal.coreOp.mock.lastCall).toStrictEqual(["consents.update", { purpose: "location_automation", granted: false }, { write: true, expectedVersion: 7 }]);
    expect(cache.refresh).toHaveBeenCalledTimes(4);
  });

  it("approves a proposal at its version with the proposal id as the idempotency key, and refuses a malformed selection unsent", async () => {
    const selection = { id: RULE, proposalId: PROPOSAL, weekday: 3, version: 2 };
    await expect(approveStartProposal(selection)).resolves.toStrictEqual({ ok: true, value: null });
    expect(dal.coreOp.mock.calls).toStrictEqual([["automations.approveStartProposal", { id: RULE, proposalId: PROPOSAL, weekday: 3 }, { write: true, expectedVersion: 2, idempotencyKey: PROPOSAL }]]);
    for (const bad of [null, { ...selection, id: "rule-1" }, { ...selection, weekday: 0 }, { ...selection, weekday: 8 }, { ...selection, weekday: 2.5 }, { ...selection, version: 0 }]) {
      await expect(approveStartProposal(bad)).resolves.toStrictEqual({ ...VALIDATION, fieldErrors: {} });
    }
    expect(dal.coreOp).toHaveBeenCalledOnce();
    expect(cache.refresh).toHaveBeenCalledOnce();
  });

  it("tests an event at the business clock's minute without storing anything or re-reading", async () => {
    dal.coreNow.mockResolvedValue(new Date("2026-09-15T10:42:37.512Z"));
    const decision = { unitId: UNIT, decision: "selected", ruleId: RULE, reason: null };
    dal.coreOp.mockResolvedValueOnce({ results: [{ unitId: "other", decision: "suppressed", ruleId: null, reason: "x" }, decision] });
    const at = "2026-09-15T10:42:00.000Z";
    await expect(testEvent(UNIT, [{ metric: "location", value: "arrival", unit: "event", quality: "valid" }])).resolves.toStrictEqual({ ok: true, value: { at, decision } });
    expect(dal.coreOp.mock.lastCall).toMatchObject(["automations.simulate", { occurredAt: at, unitIds: [UNIT], phase: "condition", facts: [{ metric: "location", value: "arrival", unit: "event", quality: "valid", unitId: UNIT, observedAt: at }] }]);
    dal.coreOp.mockResolvedValueOnce({ results: [] });
    await expect(testEvent(UNIT, [])).resolves.toStrictEqual({ ok: true, value: { at, decision: null } });
    expect(cache.refresh).not.toHaveBeenCalled();
  });

  it("reports a position of the right shape without re-reading, and refuses another shape unsent", async () => {
    const fix = { latitude: 3.1, longitude: 101.7, accuracyMeters: 12 };
    dal.coreOp.mockResolvedValueOnce({ transitions: [] });
    await expect(reportPresence(fix)).resolves.toStrictEqual({ ok: true, value: { transitions: [] } });
    expect(dal.coreOp.mock.lastCall).toStrictEqual(["presence.report", fix, { write: true }]);
    dal.coreOp.mockResolvedValueOnce({ unexpected: true });
    await expect(reportPresence(fix)).resolves.toStrictEqual({ ok: true, value: null });
    await expect(reportPresence({ ...fix, latitude: "3.1" })).resolves.toStrictEqual({ ...VALIDATION, fieldErrors: { latitude: "error.invalid" } });
    expect(dal.coreOp).toHaveBeenCalledTimes(2);
    expect(cache.refresh).not.toHaveBeenCalled();
  });
});

const proposal = { id: PROPOSAL, automationId: RULE, unitId: UNIT, version: 2, weekday: 3, from: "18:00", to: "17:30", endLocal: "22:00", occurrences: 4, supportDates: ["2026-09-02", "2026-09-09", "2026-09-16"] };

describe("parseStartProposals", () => {
  it("keeps well-formed proposals and refuses a bad time, weekday, count or evidence", () => {
    expect(parseStartProposals([proposal])).toStrictEqual([proposal]);
    for (const bad of [{ from: "24:00" }, { to: "7:30" }, { weekday: 0 }, { occurrences: 5 }, { supportDates: ["2026-09-02", "2026-09-09"] }, { supportDates: ["2026-09-02", "x", "2026-09-16"] }, { unitId: "u-1" }]) {
      expect(() => parseStartProposals([{ ...proposal, ...bad }])).toThrow(ZodError);
    }
  });
});

const base = { createdAt: "2026-09-01T00:00:00Z", updatedAt: "2026-09-01T00:00:00Z", ownerMembershipId: "m-1", timezone: "Asia/Kuala_Lumpur", priority: 0, disabledReason: null, onlyIf: null, lastRun: null } as const;
const schedule = (id: string, enabled: boolean): ApiAutomation => ({
  ...base, id, enabled, version: 2, name: `Rule ${id}`, unitIds: [UNIT], kind: "schedule", weekdays: [3], startLocal: "18:00", endLocal: "22:00", endsNextDay: false,
  startAction: { kind: "set_power", power: true }, endAction: { kind: "set_power", power: false },
});
const arrival: ApiAutomation = { ...base, id: "r-loc", version: 1, name: "Arrive", unitIds: [UNIT], enabled: false, disabledReason: "consent_revoked", kind: "event", condition: { type: "location", event: "arrival" }, action: { kind: "set_power", power: true } };
const unit = (id: string, name: string, archived = false) => ({ id, archived, displayName: name, propertyId: "p-1", spaceId: null });
const reads = (over: Partial<Record<string, unknown>> = {}) => {
  const ops = new Map<string, unknown>([
    ["preferences.get", { timezone: "" }], ["consents.get", { id: "c-1", version: 6, granted: false, grantedAt: null, revokedAt: "2026-09-10T00:00:00Z" }],
    ["automations.startProposals", [proposal]], ["automations.nextRuns", [{ automationId: RULE, phase: "schedule_end", at: "2026-09-16T14:00:00Z" }, { automationId: RULE, phase: "schedule_start", at: "2026-09-16T10:00:00Z" }]],
    ["units.get", { capabilities: { control: true } }], ...Object.entries(over),
  ]);
  dal.coreOp.mockImplementation((op: string, input: { id?: string }) => {
    const value = ops.get(op);
    if (Error.isError(value)) return Promise.reject(value);
    if (op === "units.get" && input.id === "u-2") return Promise.reject(new Error("unreachable"));
    return Promise.resolve(value);
  });
  const lists = new Map<string, unknown[]>([
    ["automations.list", [schedule(RULE, true), schedule("r-off", false), arrival]], ["units.list", [unit(UNIT, "Bedroom AC"), unit("u-2", "Study AC"), unit("u-old", "Old AC", true)]],
    ["properties.list", [{ id: "p-1", name: "Home" }]], ["spaces.list", []],
  ]);
  dal.coreAll.mockImplementation((op: string) => Promise.resolve(lists.get(op)));
  dal.coreDisplay.mockResolvedValue({ locale: "en", timeZone: "Asia/Kuala_Lumpur" });
};
const page = (params: Record<string, string> = {}) => CustomerAutomationsPage({ params: Promise.resolve({}), searchParams: Promise.resolve(params) });
const liveOf = (element: unknown) => {
  if (!isValidElement<ComponentProps<typeof AutomationsView>>(element) || element.type !== AutomationsView) throw new TypeError("not AutomationsView");
  return element.props.live;
};
const opsRead = () => dal.coreOp.mock.calls.map(([op]: unknown[]) => op);

describe("automations page", () => {
  it("renders the demo outside API mode without reading the Core API", async () => {
    dal.apiMode.mockReturnValue(false);
    const element = await page();
    expect(isValidElement(element) && element.type).toBe(AutomationsDemo);
    expect(dal.coreOp).not.toHaveBeenCalled();
  });

  it("lists the rules with the ACs not archived, the next start of each schedule that is on, the consent and the proposals", async () => {
    dal.apiMode.mockReturnValue(true);
    reads();
    const live = liveOf(await page({ preset: "off", unitIds: UNIT }));
    expect(live.editing).toBeNull();
    expect(live.acs).toStrictEqual([{ id: UNIT, name: "Bedroom AC", path: "Home", property: "Home" }, { id: "u-2", name: "Study AC", path: "Home", property: "Home" }]);
    expect(live.nextRuns).toStrictEqual({ [RULE]: "2026-09-16T10:00:00Z" });
    expect(dal.coreOp.mock.calls.filter(([op]: unknown[]) => op === "automations.nextRuns")).toStrictEqual([["automations.nextRuns", { automationId: RULE, count: 8 }]]);
    expect(live.proposals).toStrictEqual([proposal]);
    expect(live.timezone).toBe("Asia/Kuala_Lumpur"); // no preference: the default zone
    expect(live.consent).toMatchObject({ version: 6, granted: false });
    expect(live.cards.map((c) => c.id)).toStrictEqual([RULE, "r-off", "r-loc"]);
    expect(live.preset).toStrictEqual([]);
    expect(live.caps).toStrictEqual({});
    expect(live.places).toStrictEqual({ "p-1": "Home" });
    expect(opsRead()).not.toContain("units.get");
  });

  it("opens a new after-hours rule for the listed ACs only, with each AC's capabilities (none when unreadable) and no proposals", async () => {
    dal.apiMode.mockReturnValue(true);
    reads({ "preferences.get": { timezone: "Asia/Jakarta" }, "consents.get": refused("NOT_FOUND", "error.notFound") });
    const live = liveOf(await page({ automationId: "new", preset: "off", unitIds: `${UNIT},,u-old,u-other` }));
    expect(live.editing).toBe("new");
    expect(live.preset).toStrictEqual([UNIT]);
    expect(live.caps).toStrictEqual({ [UNIT]: { control: true }, "u-2": null });
    expect(live.proposals).toStrictEqual([]);
    expect(opsRead()).not.toContain("automations.startProposals");
    expect(live.timezone).toBe("Asia/Jakarta");
    expect(live.consent).toMatchObject({ version: null, granted: false, tone: "muted" });
  });

  it("an existing rule's editor takes no preset, and a consent read refused otherwise than not found fails the page", async () => {
    dal.apiMode.mockReturnValue(true);
    reads();
    expect(liveOf(await page({ automationId: RULE, preset: "off", unitIds: UNIT })).preset).toStrictEqual([]);
    reads({ "consents.get": refused("UNAVAILABLE", "error.unavailable") });
    await expect(page()).rejects.toThrow("UNAVAILABLE");
  });

  it("names the screen in its loading and error states", () => {
    const error = new Error("boom");
    const retry = vi.fn();
    const loading = AutomationsLoading();
    expect(loading.type).toBe(RouteLoading);
    expect(loading.props).toStrictEqual({ what: "automations" });
    const failed = AutomationsErrorState({ error, retry });
    expect(failed.type).toBe(RouteError);
    expect(failed.props).toStrictEqual({ what: "Automations", error, retry });
  });
});
