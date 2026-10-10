// Technician diagnostic control (FR-T10, DD-T10, SCR-T10, Figma Technician 02-18…02-28, DATA_SOURCE=api): the
// current state tiles, the command history rows of the job (commands.list, IR216), the test-run state banner
// (DiagnosticRun states, IR139), the capability, restriction and authorization texts and a refused write in words.
// Pure code shared by the Server Component and the client view; Vitest covers it. Texts in the display language (`i`
// / `t`, IR286); instants in the user's display time zone (IR44). Command codes (“set_mode = cool”) stay codes.
import { EN, showClock, showSpan, translator, type I18n, type T } from "@ac/web/lib/i18n";
import type { UnitAction } from "@ac/web/lib/contracts.gen";
import { FAN_LABEL, MODE_LABEL, type ApiUnitDetail, type CommandAction } from "@ac/web/lib/units";
import { airNumber } from "@ac/web/lib/air";

const en = translator("en");

/** Command of service-contracts.ts (fields shown here). */
export type ApiCommandRow = {
  id: string; action: CommandAction; status: "requested" | "sent" | "acknowledged" | "failed" | "expired" | "cancelled";
  requestedAt: string; acknowledgedAt: string | null; failureCode: string | null; reason: string | null; diagnosticRunId: string | null; source: string;
};
/** DiagnosticRun of service-contracts.ts. */
export type ApiRun = {
  id: string; version: number; jobId: string; unitId: string; startAction: UnitAction; endAction: UnitAction; durationMinutes: number; reason: string;
  state: "awaiting_start" | "running" | "end_requested" | "completed" | "start_failed" | "end_failed" | "end_blocked";
  startCommandId: string; endCommandId: string | null; startedAt: string | null; endAt: string | null; failureCode: string | null; createdAt: string;
};

const clock = (iso: string | null, i: I18n) => (iso ? showClock(iso, i.display) : "—");

/** "set_mode = cool", "set_temperature = 24 °C" (Figma Technician 02 control) — the command as the device gets it. */
export function actionCode(a: CommandAction): string {
  switch (a.kind) {
    case "set_power": return `set_power = ${a.power ? "ON" : "OFF"}`;
    case "set_temperature": return `set_temperature = ${a.celsius} °C`;
    case "set_mode": return `set_mode = ${a.mode}`;
    case "set_fan": return `set_fan = ${a.fanLevel}`;
    case "ventilate": return `ventilate = ${a.level}`;
    case "apply_restriction": return `apply_restriction = ${a.policy.kind === "power_off" ? "power_off" : `min ${a.policy.minimumCoolingSetpoint} °C`}`;
    case "remove_restriction": return "remove_restriction";
  }
}
/** A test run's start or end action: “Power ON” in words, the other settings as their code. */
export const actionWord = (a: UnitAction, t: T = en) => (a.kind === "set_power" ? t(a.power ? "Power ON" : "Power OFF") : actionCode(a));

/** One history row: what was sent, its device outcome and the badge. Test-run commands name their run. */
export type HistoryRow = { id: string; at: string; title: string; sub: string; badge: { text: string; tone: "ok" | "warn" | "crit" | "muted" | "primary" }; run: boolean };
export function historyRows(cs: ApiCommandRow[], runs: ApiRun[], i: I18n = EN): HistoryRow[] {
  const { t } = i;
  return cs.map((c) => {
    const run = runs.find((r) => r.startCommandId === c.id || r.endCommandId === c.id);
    const isStart = !!run && run.startCommandId === c.id;
    const title = run ? (isStart ? t("test run start ({action}, {n} min)", { action: actionWord(run.startAction, t), n: run.durationMinutes }) : t("test run end ({action})", { action: actionWord(run.endAction, t) })) : actionCode(c.action);
    const outcome = c.status === "acknowledged" ? t("acknowledged {time}", { time: clock(c.acknowledgedAt, i) })
      : c.status === "failed" ? (c.failureCode ? t("rejected by the device ({code})", { code: c.failureCode }) : t("rejected by the device"))
      : c.status === "expired" ? t("no device response (expired)") : c.status === "cancelled" ? t("cancelled") : t("waiting for the device");
    const ends = isStart && run?.state === "running" && run.endAt ? ` · ${t("ends {time}", { time: clock(run.endAt, i) })}` : "";
    const badge = isStart && run?.state === "running" ? { text: t("Running"), tone: "primary" as const }
      : c.status === "acknowledged" ? { text: t("Acked"), tone: "ok" as const } : c.status === "failed" ? { text: t("Rejected"), tone: "crit" as const }
      : c.status === "expired" ? { text: t("No response"), tone: "crit" as const } : c.status === "cancelled" ? { text: t("Cancelled"), tone: "muted" as const } : { text: t("Waiting"), tone: "warn" as const };
    return { id: c.id, at: clock(c.requestedAt, i), title, sub: `${c.id.slice(0, 8)} · ${outcome}${ends}`, badge, run: !!run };
  });
}

/** The test-run banner by DiagnosticRun state (DD-T10: stopped only after the end acknowledgement). */
export function runBanner(r: ApiRun | null, i: I18n = EN): { tone: "primary" | "warn" | "ok" | "crit"; title: string; text: string } | null {
  if (!r) return null;
  const { t } = i;
  const start = actionWord(r.startAction, t), end = actionWord(r.endAction, t);
  switch (r.state) {
    case "awaiting_start": return { tone: "primary", title: t("Starting — waiting for the device"), text: t("The start command ({action}) was sent; the run starts when the device acknowledges it.", { action: start }) };
    case "running": return {
      tone: "warn", title: t("Running — ends {end} (start acknowledged {start})", { end: clock(r.endAt, i), start: clock(r.startedAt, i) }),
      text: t("DiagnosticRun running · endAt = startedAt + {n} min. The end command ({action}) is sent at {time}.", { n: r.durationMinutes, action: end, time: clock(r.endAt, i) }),
    };
    case "end_requested": return {
      tone: "warn", title: t("End requested {time} — awaiting device response", { time: clock(r.endAt, i) }),
      text: t("end_requested · one end command ({action}) sent. Not shown as Stopped until the device confirms; a browser timer is not proof.", { action: actionCode(r.endAction) }),
    };
    case "completed": return { tone: "ok", title: t("End acknowledged — test run completed"), text: t("The device confirmed {action}.", { action: end }) };
    case "start_failed": return { tone: "crit", title: t("Start not acknowledged — the test run did not start"), text: r.failureCode ? t("start_failed ({code}) · no end command is needed.", { code: r.failureCode }) : t("start_failed · no end command is needed.") };
    case "end_failed": return { tone: "crit", title: t("No end response — the end was not confirmed"), text: r.failureCode ? t("end_failed ({code}) · the unit may still be running; check it on site.", { code: r.failureCode }) : t("end_failed · the unit may still be running; check it on site.") };
    default: return { tone: "crit", title: t("End blocked — authorization ended before the end time"), text: t("end_blocked · zero end commands were sent; the unit may still be running.") };
  }
}
export const runActive = (r: ApiRun | null) => !!r && (r.state === "awaiting_start" || r.state === "running" || r.state === "end_requested");

const QUALITY: Record<string, string> = { stale: "stale", suspect: "suspect", missing: "missing" };
const word = (v: string, labels: Record<string, string>, t: T) => (labels[v] ? t(labels[v]) : v.charAt(0).toUpperCase() + v.slice(1));
/** The current state tiles (Figma: Power, Mode, Setpoint, Fan, Room). */
export function stateTiles(d: { observedState: { power: boolean | null; celsius: number | null; mode: string | null; fanLevel: string | null; observedAt: string | null };
  latestMeasurements: { metric: string; value: number | null; quality: string }[] }, t: T = en): { label: string; value: string; warn?: boolean }[] {
  const o = d.observedState;
  const room = d.latestMeasurements.find((m) => m.metric === "temperature");
  return [
    { label: t("Power"), value: o.power === null ? t("Unknown") : t(o.power ? "On" : "Off") },
    { label: t("Mode"), value: o.mode ? word(o.mode, MODE_LABEL, t) : "—" },
    { label: t("Setpoint"), value: o.celsius === null ? "—" : `${o.celsius} °C` },
    { label: t("Fan"), value: o.fanLevel ? word(o.fanLevel, FAN_LABEL, t) : "—" },
    { label: t("Room"), value: room && room.value !== null ? `${airNumber("temperature", room.value)} °C${room.quality === "valid" ? "" : ` (${t(QUALITY[room.quality] ?? room.quality)})`}` : "—", warn: !!room && room.value !== null && room.value >= 30 },
  ];
}
export const observedText = (iso: string | null, i: I18n = EN) => (iso ? i.t("observed {time}", { time: clock(iso, i) }) : i.t("no observation yet"));
const CONNECTION: Record<string, string> = { online: "Online", offline: "Offline", unknown: "Unknown", connecting: "Connecting", error: "Error" };
/** The unit's connection after the capability text (“Connection: Online.”). */
export const connectionText = (c: string, t: T = en) => t("Connection: {state}.", { state: CONNECTION[c] ? t(CONNECTION[c]) : c });
/** The work window as one span in the display time zone. */
export const windowText = (from: string, to: string, i: I18n = EN) => showSpan(from, to, i.display);

/** What the capability version allows (Figma: “Capability v3 allows: set_mode (cool/dry/fan), …”). */
export function capabilityText(d: Pick<ApiUnitDetail, "capabilities" | "capabilityVersion">, t: T = en): string {
  const c = d.capabilities;
  const list = [
    c.modeControl && c.modes.length ? `set_mode (${c.modes.join("/")})` : null, c.temperature ? `set_temperature ${c.temperature.min}–${c.temperature.max} °C` : null,
    c.fanControl && c.fanLevels.length ? `set_fan (${c.fanLevels.join("/")})` : null, t("test run 1–15 min"),
  ].filter(Boolean).join(", ");
  return t("Capability v{version} allows: {list}.", { version: d.capabilityVersion, list });
}
/** The active restriction in words: none, a minimum setpoint or power off only. */
export function restrictionText(d: Pick<ApiUnitDetail, "effectiveControlPolicy">, t: T = en): string | null {
  const p = d.effectiveControlPolicy.state === "restricted" ? d.effectiveControlPolicy.policy : null;
  return !p ? null : p.kind === "temperature_limit" ? t("min {celsius} °C", { celsius: p.minimumCoolingSetpoint }) : t("power off only");
}
const BLOCKED: Record<string, string> = { "control.reconciliation_required": "the unit must be reconciled after a restriction" };
/** Why control is blocked now, in words (controlAvailability). */
export const blockedText = (d: Pick<ApiUnitDetail, "controlAvailability">, t: T = en) =>
  d.controlAvailability.state !== "blocked" ? null : BLOCKED[d.controlAvailability.reasonKey] ? t(BLOCKED[d.controlAvailability.reasonKey]) : d.controlAvailability.reasonKey.replace(/^(errors?|control)\./, "").replace(/_/g, " ");

const REFUSED: Record<string, string> = {
  "errors.restriction_active": "FORBIDDEN — the active restriction does not allow this action (for example a setpoint below its minimum).",
  "errors.unit_busy": "CONFLICT — the unit is busy: a firmware update or an unfinished command must finish first.",
  "errors.reconciliation_required": "CONFLICT — the unit must be reconciled after a restriction before it can be controlled.",
  "errors.assignment_not_started": "FORBIDDEN — your work window has not started.",
  "errors.assignment_ended": "FORBIDDEN — your work window has ended.",
};
const CODES: Record<string, string> = {
  OFFLINE: "OFFLINE — the device is not reachable (connection or power signal), so nothing was sent.",
  FORBIDDEN: "FORBIDDEN — outside your assignment or work window, or without diagnostic permission.",
  CONFLICT: "CONFLICT — the unit or job changed; the page now shows the latest version.",
  UNAVAILABLE: "UNAVAILABLE — not sent; your entries are kept. Try again.",
};
/** The DomainError of a refused diagnostic write, in the words of Figma Technician 02 (IR46 / IR47 / D04); null for a
 * VALIDATION that only marks fields. */
export function refusal(f: { code: string; messageKey: string }, t: T = en): string | null {
  if (REFUSED[f.messageKey]) return t(REFUSED[f.messageKey]);
  return CODES[f.code] ? t(CODES[f.code]) : f.code === "VALIDATION" ? null : `${f.code} — ${f.messageKey}`;
}
const FIELD: Record<string, string> = { "error.length": "1–1000 characters", "error.required": "Required", "error.range": "1–15 minutes", "error.unsupportedAction": "Not supported by this AC", "error.invalid": "Not valid" };
/** A field error of the Core API in words. */
export const fieldText = (key: string, t: T = en) => (FIELD[key] ? t(FIELD[key]) : key.replace(/^errors?\./, "").replace(/_/g, " "));
