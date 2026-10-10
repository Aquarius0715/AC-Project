// The evidence an alert's resolution may cite (IR327, DD-A05 item 6, DD-T07, Figma Admin 259:2): the candidates of
// alerts.evidence — the alert's own evidence at detection, the unit's valid remeasurements after detection (measured or
// recorded on site; of the rule's metric when the alert has a rule) and the recovery events of the unit's device linked
// to the alert. An alert without a policy resolves only with at least one; a policy alert may be resolved by hand
// without. Pure code shared by the HQ Resolve dialog and the technician's resolution; Vitest covers it. Texts in the
// display language; instants in the user's display time zone (IR44).
import { EN, relativeTime, type I18n, type T } from "@ac/web/lib/i18n";
import { metricLabel } from "@ac/web/lib/adminAlerts";
import { airNumber } from "@ac/web/lib/air";

/** EvidenceCandidate of service-contracts.ts. */
export type ApiEvidenceCandidate = {
  id: string; kind: "detection" | "remeasurement" | "device_event"; observedAt: string; metric: string | null; value: number | null; unit: string | null;
  origin: "measured" | "estimated" | "inspection" | null; quality: string | null; eventType: string | null;
};
/** One row of the picker: what the record is and when, then where it comes from. */
export type EvidenceRow = { id: string; kind: ApiEvidenceCandidate["kind"]; title: string; sub: string };

const ORIGIN: Record<string, string> = { measured: "measured", inspection: "recorded on site", estimated: "estimated" };
const EVENT: Record<string, string> = { restored: "The device's signal is back" };
/** The metric's name without the unit some labels carry (“CO₂ (ppm)” → “CO₂”): the value names its unit. */
const metricName = (metric: string, t: T) => (metricLabel[metric] ? t(metricLabel[metric]).replace(/\s*\([^)]*\)$/, "") : metric.replace(/_/g, " "));

/** The candidates as the picker lists them: the newest first, as alerts.evidence returns them. */
export function evidenceRows(items: ApiEvidenceCandidate[], nowMs: number, i: I18n = EN): EvidenceRow[] {
  const { t } = i;
  return items.map((c) => {
    const time = relativeTime(c.observedAt, nowMs, i);
    if (c.kind === "remeasurement") {
      const metric = c.metric ?? "";
      const value = c.value === null ? "—" : `${airNumber(metric, c.value)} ${c.unit ?? ""}`.trim();
      return { id: c.id, kind: c.kind, title: t("Remeasurement · {metric} {value} · {time}", { metric: metricName(metric, t), value, time }), sub: t("{origin} · valid", { origin: t(ORIGIN[c.origin ?? ""] ?? c.origin ?? "") }) };
    }
    if (c.kind === "device_event") return { id: c.id, kind: c.kind, title: t("Device event · {time}", { time }), sub: EVENT[c.eventType ?? ""] ? t(EVENT[c.eventType ?? ""]) : c.eventType ?? "" };
    return { id: c.id, kind: c.kind, title: t("Evidence attached to this alert"), sub: `${c.id.slice(0, 8)} · ${t("observed {time}", { time })}` };
  });
}

/** The picker's heading: required for an alert without a policy (IR66), optional for a policy alert. */
export function evidenceHeading(policyless: boolean, t: T): { title: string; sub: string } {
  return policyless
    ? { title: t("Resolution evidence · required"), sub: t("At least one · measurements after detection") }
    : { title: t("Resolution evidence · optional"), sub: t("A policy alert may also be resolved by hand without evidence") };
}

/** At most this many evidence IDs go with one resolution (the Core API's bound). */
export const MAX_EVIDENCE = 20;
/** Why the picked evidence cannot be sent yet, or null. */
export function evidenceProblem(policyless: boolean, picked: string[], t: T): string | null {
  if (policyless && picked.length === 0) return t("Pick at least one evidence record — this alert has no policy");
  if (picked.length > MAX_EVIDENCE) return t("Pick at most {n} evidence records", { n: MAX_EVIDENCE });
  return null;
}

const REFUSED: Record<string, string> = {
  "errors.evidence_required": "Pick at least one evidence record — this alert has no policy",
  "errors.evidence_unknown": "An evidence record is no longer a candidate of this alert — pick again from the list",
};
/** The resolution's refusal of its evidence (fieldErrors.resolutionEvidenceIds) in words, or null for another refusal. */
export function evidenceRefusal(fieldErrors: Record<string, string> | undefined, t: T): string | null {
  const key = fieldErrors?.resolutionEvidenceIds;
  if (!key) return null;
  return REFUSED[key] ? t(REFUSED[key]) : t("Pick each evidence record once, at most {n}", { n: MAX_EVIDENCE });
}

/** The picked IDs after ticking one on or off, in the order they were picked. */
export const togglePick = (picked: string[], id: string, on: boolean): string[] => (on ? (picked.includes(id) ? picked : [...picked, id]) : picked.filter((x) => x !== id));
