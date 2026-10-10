// The technician's unit QR scan (FR-T13, DD-T13, Figma Technician 01-6) from the Core API: units.resolveQr answers
// the unit and the next open job of the user on it (only assigned units; anything else reads as absent), units.get
// and jobs.get describe them. Pure code shared by the scan dialog; Vitest covers it. Texts in the display language
// (`i` / `t`, IR288); the job's window is a span in the user's display time zone (IR44), while "today" is the Kuala
// Lumpur day the work happens on, as on the overview (IR281).
import { EN, showSpan, translator, type I18n, type T } from "@ac/web/lib/i18n";
import { statusWord, typeLabel } from "@ac/web/lib/partnerJobDetail";
import type { Slot } from "@ac/web/lib/partnerOverview";

const en = translator("en");

export type ApiQrResolution = { unitId: string; jobId: string | null };
export type ApiQrUnit = { id: string; displayName: string; location: { pathLabels: string[] }; capabilities: { manufacturer: string; model: string } };
export type ApiQrJob = { id: string; type: string; status: string; scheduledSlot: Slot | null; alertIds?: string[] };

const KL = 8 * 3600_000;
const klDate = (ms: number) => new Date(ms + KL).toISOString().slice(0, 10);

/** The scanned label as the API takes it: the printed text trimmed; empty is not a scan. */
export const scanCode = (raw: string) => raw.trim();

export type QrMatch = { title: string; sub: string; job: { label: string; text: string; tone: "ok" | "primary" } | null; unitHref: string; jobHref: string | null };
/** The matched card: the unit with its location and model, and the user's job on it (today's, or the next one). Before
 * the work window units.get is FORBIDDEN (IR94): the card then names the unit from the user's unit list and says so. */
export function qrMatch(r: ApiQrResolution, unit: ApiQrUnit | null, job: ApiQrJob | null, now: number, opts: { name?: string; locked?: boolean } = {}, i: I18n = EN): QrMatch {
  const { t, display } = i;
  const title = unit?.displayName ?? opts.name ?? t("Assigned unit");
  const sub = unit ? [unit.location.pathLabels.join(" › "), `${unit.capabilities.manufacturer} ${unit.capabilities.model}`].filter(Boolean).join(" · ")
    : opts.locked ? t("Unit details open when your work window starts (IR94).") : "";
  let card: QrMatch["job"] = null;
  if (r.jobId) {
    const s = job?.scheduledSlot ?? null;
    const today = !!s && klDate(Date.parse(s.startAt)) <= klDate(now) && klDate(now) <= klDate(Date.parse(s.endAt));
    const alerts = job?.alertIds?.length ? t(job.alertIds.length === 1 ? "1 linked alert" : "{n} linked alerts", { n: job.alertIds.length }) : "";
    card = {
      label: t(today ? "Your job today: {id}" : "Your next job: {id}", { id: r.jobId.slice(0, 8) }),
      text: [s ? showSpan(s.startAt, s.endAt, display) : "", job ? typeLabel(job.type, t) : "", job ? statusWord(job.status, t) : "", alerts].filter(Boolean).join(" · "), tone: today ? "ok" : "primary",
    };
  }
  return { title, sub, job: card, unitHref: `/technician/units/${r.unitId}${r.jobId ? `?jobId=${r.jobId}` : ""}`, jobHref: r.jobId ? `/technician/jobs/${r.jobId}` : null };
}

/** Readable refusals of a scan: anything outside the user's assignments is absent without unit data (DD-T13). `absent`
 * marks the NOT_FOUND answer, which the dialog shows as a warning rather than an error. */
export function qrRefusal(f: { code: string; messageKey: string }, t: T = en): { title: string; text: string; absent: boolean } {
  if (f.code === "NOT_FOUND") return { title: t("Page unavailable"), text: t("This label is not on a unit in your assignments, or it is unknown. Check the label, or enter the unit ID manually."), absent: true };
  if (f.code === "VALIDATION") return { title: t("Not a label"), text: t("Enter the label code, a device serial or the unit ID."), absent: false };
  return { title: t("Scan failed"), text: t("{code} — try again.", { code: f.code }), absent: false };
}
