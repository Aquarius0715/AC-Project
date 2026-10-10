// The customer assistant (FR-X02, D09, IR65, DATA_SOURCE=api; Figma Client 09a–09g): the fixed en/ms grammar's
// examples, voice.resolveIntent results and the command's progress as the panel's words. Pure code shared by the panel
// and its tests. The assistant speaks its chosen language — the display language unless the panel switches it (IR306).
import { one } from "@ac/web/lib/energy";
import { showClock, type Display, type Locale, type T } from "@ac/web/lib/i18n";
import { actionText, type ApiCommand, type ApiUnitDetail } from "@ac/web/lib/units";

/** ResolvedIntent of service-contracts.ts. */
export type Measurement = { value: number | null; unit: string; observedAt: string; quality: "valid" | "missing" | "stale" | "suspect" };
export type ResolvedIntent =
  | { kind: "unsupported" }
  | { kind: "help"; messageKey: "voice.help" }
  | { kind: "candidates"; candidates: { unitId: string; pathLabel: string }[] }
  | { kind: "temperature"; unitId: string; measurement: Measurement | null }
  | { kind: "change"; unitId: string; celsius: number; before: { power: boolean | null; celsius: number | null; observedAt: string | null }; expectedVersion: number };

/** The microphone consent of this device (Preferences), kept in this browser and not on the account. "off" leaves the
 * assistant text only, and turning it off while listening discards the voice request (FR-X02, AT-X02-E ③, D09). */
export const MICROPHONE = "ac-voice-microphone";

/** Sentences the fixed grammar accepts (D09): the suggestions and the simulated voice transcript (no real microphone). */
export const examples = (locale: Locale) =>
  locale === "ms"
    ? { temperature: "suhu Bedroom", change: "tetapkan Bedroom kepada 24 darjah", help: "bantuan" }
    : { temperature: "temperature Bedroom", change: "set Bedroom to 24 degrees", help: "help" };

/** The grammar's own help (voice.help): what can be asked, in the assistant's language. */
export const helpText = (t: T) =>
  t("Ask “temperature <room>”, “set <room> to <number> degrees” or “help”. A change is always confirmed before anything is sent, and only to ACs you may control.");

/** A unit's place and name, “Home A › 1F › Bedroom › Bedroom AC”. */
export const placeOf = (u: Pick<ApiUnitDetail, "displayName" | "location">) => [...u.location.pathLabels, u.displayName].join(" › ");

/** The candidates of an ambiguous room by room (Figma 09d): each matched room's path (“Home A › 1F › Bedroom”), then
 * the ACs placed in it. ACs with the same name in one room also show their ID, since nothing else tells them apart
 * (IR101). The order is the API's (by path label). */
export type CandidateGroup = { path: string; units: { unitId: string; name: string; label: string }[] };
export function candidateGroups(candidates: { unitId: string; pathLabel: string }[]): CandidateGroup[] {
  const rooms = new Map<string, { unitId: string; name: string }[]>();
  for (const c of candidates) {
    const parts = c.pathLabel.split(" > ");
    const name = parts.length > 1 ? parts.pop()! : c.pathLabel;
    const path = parts.join(" › ");
    rooms.set(path, [...(rooms.get(path) ?? []), { unitId: c.unitId, name }]);
  }
  return [...rooms].map(([path, units]) => ({
    path, units: units.map((u) => ({ ...u, label: units.filter((x) => x.name === u.name).length > 1 ? `${u.name} · ${u.unitId.slice(0, 8)}` : u.name })),
  }));
}
/** A room choice's ACs, “2 ACs: Bedroom AC, Bedroom AC #2”. */
export const groupSummary = (g: CandidateGroup, t: T) =>
  g.units.length === 1 ? t("1 AC: {name}", { name: g.units[0].label }) : t("{count} ACs: {names}", { count: g.units.length, names: g.units.map((u) => u.label).join(", ") });
/** The room the request named, as the matched rooms are called (“Bedroom”). */
export const roomOf = (groups: CandidateGroup[]) => groups[0]?.path.split(" › ").pop() ?? "";

/** The answer to a temperature question: the latest reading, or why there is none (never a zero). */
export function temperatureText(m: Measurement | null, unit: string, t: T, display: Display): string {
  if (!m || m.value === null) return t("{unit} has no temperature reading yet.", { unit });
  const at = showClock(m.observedAt, display);
  return m.quality === "valid" ? t("{unit} is {value} °C (measured {time}).", { unit, value: one(m.value), time: at })
    : t("{unit} last read {value} °C at {time} — the reading is {quality}.", { unit, value: one(m.value), time: at, quality: qualityWord(m.quality, t) });
}
const qualityWord = (q: Measurement["quality"], t: T) => ({ valid: t("valid"), missing: t("missing"), stale: t("stale"), suspect: t("suspect") })[q] ?? q;

/** The confirmation card of a change (Figma 09c): target, current setting, requested setting and the allowed range. */
export function changeCard(intent: Extract<ResolvedIntent, { kind: "change" }>, u: ApiUnitDetail | null, t: T) {
  const range = u?.capabilities.temperature;
  const current = intent.before.celsius === null ? t("not known") : t("{value} °C (confirmed by the device)", { value: intent.before.celsius });
  return {
    target: u ? placeOf(u) : intent.unitId.slice(0, 8), unit: u?.displayName ?? intent.unitId.slice(0, 8), current, requested: `${intent.celsius} °C`,
    range: range ? t("{min}–{max} °C · step {step}", { min: range.min, max: range.max, step: range.step }) : t("not supported"),
    supported: !!range && intent.celsius >= range.min && intent.celsius <= range.max,
  };
}

/** The command's progress after Confirm (Figma 09e–09g); while sending, the command and its state as rows. */
export type Progress = { tone: "info" | "ok" | "crit"; title: string; detail: string; rows?: [string, string][] };
export function progressOf(c: ApiCommand, unit: string, celsius: number, before: number | null, measured: number | null, t: T, display: Display): Progress {
  const time = showClock(c.requestedAt, display);
  if (c.status === "acknowledged") {
    const ack = c.acknowledgedAt ? t("Acknowledged by the device at {time}.", { time: showClock(c.acknowledgedAt, display) }) : t("Acknowledged by the device.");
    return {
      tone: "ok", title: t("Done — {unit} is set to {value} °C", { unit, value: celsius }),
      detail: measured === null ? ack : `${ack} ${t("Room temperature is still {value} °C (measured).", { value: one(measured) })}`,
    };
  }
  if (c.status === "failed" || c.status === "expired" || c.status === "cancelled") {
    const kept = before === null ? t("Nothing changed.") : t("Nothing changed — the confirmed setting is still {value} °C.", { value: before });
    const ttl = c.expiresAt ? Math.round((Date.parse(c.expiresAt) - Date.parse(c.requestedAt)) / 1000) : null;
    const why = c.status === "expired" ? (ttl ? t("The device did not answer within {seconds} s (expired).", { seconds: ttl }) : t("The device did not answer in time (expired)."))
      : c.status === "cancelled" ? t("The command was cancelled.") : t("The device refused it ({code}).", { code: c.failureCode ?? "—" });
    return { tone: "crit", title: t("Couldn’t change {unit}", { unit }), detail: `${why} ${kept}` };
  }
  return {
    tone: "info", title: t("Sending to {unit}…", { unit }),
    rows: [[t("Command"), `${c.id.slice(0, 8)} · ${actionText(c.action, t)}`], [t("Status"), t("Sent {time} · waiting for the device", { time })]],
    detail: before === null ? t("The confirmed setting stays as it is until the device acknowledges.") : t("The confirmed setting stays {value} °C until the device acknowledges.", { value: before }),
  };
}

/** The state chip of the header (Figma 09a–09g). */
export type PanelState = "idle" | "listening" | "confirm" | "sending" | "success" | "failure";
export const stateText = (s: PanelState, t: T) =>
  ({ idle: t("Idle"), listening: t("Listening"), confirm: t("Needs confirmation"), sending: t("Sending"), success: t("Success"), failure: t("Failure") })[s];
