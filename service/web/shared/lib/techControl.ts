// Technician diagnostic control (FR-T10, DATA_SOURCE=api): the current state tiles, the command history rows of the
// job (commands.list, IR216), the test-run state banner (DiagnosticRun states, IR139) and the authorization card.
// Pure code shared by the Server Component and the client view.
import { klStamp } from "@ac/web/lib/energy";
import type { UnitAction } from "@ac/web/lib/units";
import { airNumber } from "@ac/web/lib/air";

/** Command of service-contracts.ts (fields shown here). */
export type ApiCommandRow = {
  id: string; action: UnitAction | { kind: "ventilate"; level: string } | { kind: string }; status: "requested" | "sent" | "acknowledged" | "failed" | "expired" | "cancelled";
  requestedAt: string; acknowledgedAt: string | null; failureCode: string | null; reason: string | null; diagnosticRunId: string | null; source: string;
};
/** DiagnosticRun of service-contracts.ts. */
export type ApiRun = {
  id: string; version: number; jobId: string; unitId: string; startAction: UnitAction; endAction: UnitAction; durationMinutes: number; reason: string;
  state: "awaiting_start" | "running" | "end_requested" | "completed" | "start_failed" | "end_failed" | "end_blocked";
  startCommandId: string; endCommandId: string | null; startedAt: string | null; endAt: string | null; failureCode: string | null; createdAt: string;
};

const hm = (iso: string | null) => (iso ? klStamp(iso).slice(11) : "—");
const hms = (iso: string | null) => (iso ? new Date(iso).toLocaleTimeString("en-GB", { timeZone: "Asia/Kuala_Lumpur", hour12: false }) : "—");

/** "set_mode = cool", "set_temperature = 24 °C" (Figma Technician 02 control). */
export function actionCode(a: ApiCommandRow["action"]): string {
  const x = a as Record<string, unknown>;
  switch (x.kind) {
    case "set_power": return `set_power = ${x.power ? "ON" : "OFF"}`;
    case "set_temperature": return `set_temperature = ${x.celsius} °C`;
    case "set_mode": return `set_mode = ${x.mode}`;
    case "set_fan": return `set_fan = ${x.fanLevel}`;
    case "ventilate": return `ventilate = ${x.level}`;
    default: return String(x.kind ?? "command");
  }
}
export const actionWord = (a: UnitAction) => (a.kind === "set_power" ? `Power ${a.power ? "ON" : "OFF"}` : actionCode(a));

/** One history row: what was sent, its device outcome and the badge. Test-run commands name their run. */
export type HistoryRow = { id: string; at: string; title: string; sub: string; badge: { text: string; tone: "ok" | "warn" | "crit" | "muted" | "primary" } };
export function historyRows(cs: ApiCommandRow[], runs: ApiRun[]): HistoryRow[] {
  return cs.map((c) => {
    const run = runs.find((r) => r.startCommandId === c.id || r.endCommandId === c.id);
    const isStart = !!run && run.startCommandId === c.id;
    const title = run ? (isStart ? `test run start (${actionWord(run.startAction)}, ${run.durationMinutes} min)` : `test run end (${actionWord(run.endAction)})`) : actionCode(c.action);
    const outcome = c.status === "acknowledged" ? `acknowledged ${hms(c.acknowledgedAt)}`
      : c.status === "failed" ? `rejected by the device${c.failureCode ? ` (${c.failureCode})` : ""}`
      : c.status === "expired" ? "no device response (expired)" : c.status === "cancelled" ? "cancelled" : "waiting for the device";
    const ends = isStart && run?.state === "running" && run.endAt ? ` · ends ${hm(run.endAt)}` : "";
    const badge = isStart && run?.state === "running" ? { text: "Running", tone: "primary" as const }
      : c.status === "acknowledged" ? { text: "Acked", tone: "ok" as const } : c.status === "failed" ? { text: "Rejected", tone: "crit" as const }
      : c.status === "expired" ? { text: "No response", tone: "crit" as const } : c.status === "cancelled" ? { text: "Cancelled", tone: "muted" as const } : { text: "Waiting", tone: "warn" as const };
    return { id: c.id, at: hm(c.requestedAt), title, sub: `${c.id.slice(0, 8)} · ${outcome}${ends}`, badge };
  });
}

/** The test-run banner by DiagnosticRun state (DD-T10: stopped only after the end acknowledgement). */
export function runBanner(r: ApiRun | null): { tone: "primary" | "warn" | "ok" | "crit"; title: string; text: string } | null {
  if (!r) return null;
  switch (r.state) {
    case "awaiting_start": return { tone: "primary", title: "Starting — waiting for the device", text: `The start command (${actionWord(r.startAction)}) was sent; the run starts when the device acknowledges it.` };
    case "running": return { tone: "warn", title: `Running — ends ${hm(r.endAt)} (start acknowledged ${hm(r.startedAt)})`, text: `DiagnosticRun running · endAt = startedAt + ${r.durationMinutes} min. The end command (${actionWord(r.endAction)}) is sent at ${hm(r.endAt)}.` };
    case "end_requested": return { tone: "warn", title: `End requested ${hm(r.endAt)} — awaiting device response`, text: `end_requested · one end command (${actionCode(r.endAction)}) sent. Not shown as Stopped until the device confirms; a browser timer is not proof.` };
    case "completed": return { tone: "ok", title: "End acknowledged — test run completed", text: `The device confirmed ${actionWord(r.endAction)}.` };
    case "start_failed": return { tone: "crit", title: "Start not acknowledged — the test run did not start", text: `start_failed${r.failureCode ? ` (${r.failureCode})` : ""} · no end command is needed.` };
    case "end_failed": return { tone: "crit", title: "No end response — the end was not confirmed", text: `end_failed${r.failureCode ? ` (${r.failureCode})` : ""} · the unit may still be running; check it on site.` };
    default: return { tone: "crit", title: "End blocked — authorization ended before the end time", text: "end_blocked · zero end commands were sent; the unit may still be running." };
  }
}
export const runActive = (r: ApiRun | null) => !!r && (r.state === "awaiting_start" || r.state === "running" || r.state === "end_requested");

/** The current state tiles (Figma: Power, Mode, Setpoint, Fan, Room). */
export function stateTiles(d: { observedState: { power: boolean | null; celsius: number | null; mode: string | null; fanLevel: string | null; observedAt: string | null };
  latestMeasurements: { metric: string; value: number | null; quality: string }[] }): { label: string; value: string; warn?: boolean }[] {
  const o = d.observedState;
  const room = d.latestMeasurements.find((m) => m.metric === "temperature");
  return [
    { label: "Power", value: o.power === null ? "Unknown" : o.power ? "On" : "Off" },
    { label: "Mode", value: o.mode ? o.mode.charAt(0).toUpperCase() + o.mode.slice(1) : "—" },
    { label: "Setpoint", value: o.celsius === null ? "—" : `${o.celsius} °C` },
    { label: "Fan", value: o.fanLevel ? o.fanLevel.charAt(0).toUpperCase() + o.fanLevel.slice(1) : "—" },
    { label: "Room", value: room && room.value !== null ? `${airNumber("temperature", room.value)} °C${room.quality === "valid" ? "" : ` (${room.quality})`}` : "—", warn: !!room && room.value !== null && room.value >= 30 },
  ];
}
export const observedText = (iso: string | null) => (iso ? `observed ${hms(iso)}` : "no observation yet");
export const windowText = (from: string, to: string) => `${klStamp(from).slice(5).replace("-", "/")} – ${klStamp(to).slice(5).replace("-", "/")}`;
