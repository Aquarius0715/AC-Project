import { describe, expect, it } from "vitest";
import { i18nOf, translator } from "@ac/web/lib/i18n";
import {
  actionOutcome, activePeriod, executeBlocker, intentText, policyText, progressText, releaseBlocker, restrictionRows, stateWord, unitRows,
  type ApiCommand, type ApiRestriction, type PerUnit,
} from "@ac/web/lib/restrictions";

const MS = i18nOf({ locale: "ms", timeZone: "Asia/Tokyo" });
const NOW = new Date("2026-09-15T01:00:00Z"); // 09:00 in Kuala Lumpur, 10:00 in Tokyo
const unit = (over: Partial<PerUnit>): PerUnit => ({ unitId: "u1", applyState: "applied", releaseState: "none", applyCommandIds: [], releaseCommandIds: [], observedRestriction: null, observedAt: null, pendingReason: null, ...over });
const restriction = (over: Partial<ApiRestriction>): ApiRestriction => ({
  id: "r1234567-aaaa", version: 3, createdAt: "2026-09-12T00:00:00Z", unitIds: ["u1", "u2"], rulesVersion: "demo-v1", policy: { kind: "temperature_limit", minimumCoolingSetpoint: 26 },
  state: "applied", perUnit: [unit({}), unit({ unitId: "u2", applyState: "not_applied", pendingReason: "offline" })], recoveryCases: [],
  noticeAt: "2026-09-12T00:00:00Z", executeAfter: "2026-09-13T00:00:00Z", releaseIntent: null, contractId: "k1", causeInvoiceIds: ["i1"], ...over,
});

describe("HQ restrictions (FR-A09, FR-A10)", () => {
  it("words states, policies, progress and the rows", () => {
    expect([stateWord("release_requested"), policyText({ kind: "power_off" }), policyText({ kind: "temperature_limit", minimumCoolingSetpoint: 26 })]).toEqual(["Release requested", "Power off", "Temperature limit — cooling setpoint ≥ 26 °C"]);
    expect([
      progressText(restriction({ state: "scheduled" })), progressText(restriction({})), progressText(restriction({ state: "release_requested", perUnit: [unit({ releaseState: "released" }), unit({ unitId: "u2", releaseState: "not_required" })] })),
      progressText(restriction({ state: "released" })), progressText(restriction({ state: "cancelled" })),
    ]).toEqual(["executes after 13 Sept 2026, 8:00 am MYT", "1/2 applied · 1 offline", "2/2 released", "2 released", "cancelled"]);
    expect(restrictionRows([restriction({})], () => "Demo Customer A · rto contract v1")).toEqual([{ id: "r1234567-aaaa", version: 3, state: "applied", label: "Demo Customer A · rto contract v1", policy: "Temperature limit — cooling setpoint ≥ 26 °C", units: 2, progress: "1/2 applied · 1 offline" }]);
  });

  it("words each unit's states and commands and offers only the actions it allows (SR26)", () => {
    const commands = new Map<string, ApiCommand>([["c1", { id: "c1", status: "failed", delivery: "mqtt", requestedAt: "2026-09-13T00:00:00Z", failureCode: "OFFLINE" }], ["c2", { id: "c2", status: "acknowledged", delivery: "mqtt", requestedAt: "2026-09-13T00:00:00Z", failureCode: null }]]);
    const r = restriction({ state: "requested", perUnit: [
      unit({ applyState: "not_applied", applyCommandIds: ["c1"] }), unit({ unitId: "u2", applyState: "sent_unknown", observedRestriction: { restrictionId: "r1234567-aaaa", rulesVersion: "demo-v1" }, observedAt: "2026-09-14T02:00:00Z" }),
      unit({ unitId: "u3", observedRestriction: { restrictionId: "other", rulesVersion: "v0" }, releaseCommandIds: ["c2"] }),
    ] });
    expect(unitRows(r, new Map([["u1", "Lobby AC"]]), commands).map((u) => [u.name, u.apply, u.release, u.observed, u.commands, u.retryApply, u.reconcile])).toEqual([
      ["Lobby AC", "not applied", "none", "none observed", "apply failed (OFFLINE)", true, false],
      ["u2", "sent, result unknown", "none", "this restriction · 14 Sept 2026, 10:00 am MYT", "—", false, true],
      ["u3", "applied", "none", "another restriction", "remove acknowledged", false, false],
    ]);
    const releasing = restriction({ state: "release_requested", perUnit: [unit({ releaseState: "failed" }), unit({ unitId: "u2", releaseState: "none" })] });
    expect(unitRows(releasing, new Map(), new Map()).map((u) => u.retryRelease)).toEqual([true, true]);
  });

  it("says why execute or release is not possible, and what each exception action does (IR35, IR96, IR140)", () => {
    expect([
      executeBlocker(restriction({}), NOW), executeBlocker(restriction({ state: "scheduled", executeAfter: "2026-09-16T00:00:00Z" }), NOW),
      executeBlocker(restriction({ state: "scheduled", noticeAt: "2026-09-14T12:00:00Z" }), NOW), executeBlocker(restriction({ state: "scheduled", graceUntil: "2026-09-20T00:00:00Z" }), NOW),
      executeBlocker(restriction({ state: "scheduled" }), NOW),
    ]).toEqual(["Only a scheduled restriction can be executed", "Executes after 16 Sept 2026, 8:00 am MYT", "Wait 24 hours after the notice", "Grace until 20 Sept 2026, 8:00 am MYT — execution waits", null]);
    expect([
      releaseBlocker(restriction({ state: "release_requested" }), false, NOW), releaseBlocker(restriction({ state: "scheduled" }), false, NOW), releaseBlocker(restriction({}), false, NOW),
      releaseBlocker(restriction({}), true, NOW), releaseBlocker(restriction({ exception: { until: "2026-09-20T00:00:00Z", reason: null } }), false, NOW),
    ]).toEqual(["Release already requested", "Only a requested or applied restriction can be released", "Cause invoices are unpaid and no grace period or exception is active — use the exception screen or an override", null, null]);
    expect([activePeriod(restriction({ exception: { until: "2026-09-20T00:00:00Z", reason: "x" }, graceUntil: "2026-09-18T00:00:00Z" }), NOW), activePeriod(restriction({ graceUntil: "2026-09-14T00:00:00Z" }), NOW)])
      .toEqual(["Exception until 20 Sept 2026, 8:00 am MYT", null]); // the exception wins; an expired grace is not active
    expect([intentText(restriction({})), intentText(restriction({ releaseIntent: { source: "cancel", at: "2026-09-14T02:00:00Z" } }))]).toEqual(["none", "cancellation · 14 Sept 2026, 10:00 am MYT"]);
    expect([actionOutcome("defer", "scheduled"), actionOutcome("exempt", "applied"), actionOutcome("cancel", "release_requested"), actionOutcome("override", "scheduled"), actionOutcome("override", "released")]).toEqual([
      { allowed: true, result: "Stays scheduled; execution waits until the date passes" }, { allowed: true, result: "Applied → Release requested (source: exception)" },
      { allowed: true, result: "No change — release already requested (idempotent)" }, { allowed: false, result: "Not possible for a scheduled restriction — cancel it instead" },
      { allowed: false, result: "Not possible — the restriction is released" },
    ]);
  });
});

describe("HQ restrictions in Malay with the display time zone (IR301)", () => {
  it("words the progress, units, blockers and outcomes with times in the display zone", () => {
    const t = translator("ms");
    expect([progressText(restriction({ state: "scheduled" }), MS), progressText(restriction({}), MS)]).toEqual(["dilaksanakan selepas 13 Sep 2026, 9:00 PG GMT+9", "1/2 dikenakan · 1 luar talian"]);
    expect(unitRows(restriction({}), new Map(), new Map(), MS).map((u) => [u.apply, u.release, u.pending])).toEqual([["dikenakan", "tiada", null], ["tidak dikenakan", "tiada", "luar talian"]]);
    expect([executeBlocker(restriction({ state: "scheduled", executeAfter: "2026-09-16T00:00:00Z" }), NOW, MS), releaseBlocker(restriction({ state: "cancelled" }), false, NOW, MS)])
      .toEqual(["Dilaksanakan selepas 16 Sep 2026, 9:00 PG GMT+9", "Hanya sekatan yang diminta atau dikenakan boleh dilepaskan"]);
    expect([actionOutcome("override", "requested", t).result, actionOutcome("cancel", "cancelled", t).result]).toEqual(["Diminta → Pelepasan diminta (sumber: pelepasan manual); invois kekal belum dibayar", "Tidak boleh — sekatan ini dibatalkan"]);
    expect([stateWord("scheduled", t), intentText(restriction({ releaseIntent: { source: "payment", at: "2026-09-14T02:00:00Z" } }), MS)]).toEqual(["Dijadualkan", "pembayaran · 14 Sep 2026, 11:00 PG GMT+9"]);
  });
});
