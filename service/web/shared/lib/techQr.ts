// The technician's unit QR scan (FR-T13, DD-T13, Figma Technician 01-6) from the Core API: units.resolveQr answers
// the unit and the next open job of the user on it (only assigned units; anything else reads as absent), units.get
// and jobs.get describe them. Pure code shared by the scan dialog; Vitest covers it.
import { typeLabel } from "@ac/web/lib/partnerJobDetail";
import { windowText } from "@ac/web/lib/techOverview";
import type { Slot } from "@ac/web/lib/partnerOverview";

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
export function qrMatch(r: ApiQrResolution, unit: ApiQrUnit | null, job: ApiQrJob | null, now: number, opts: { name?: string; locked?: boolean } = {}): QrMatch {
  const title = unit?.displayName ?? opts.name ?? "Assigned unit";
  const sub = unit ? [unit.location.pathLabels.join(" › "), `${unit.capabilities.manufacturer} ${unit.capabilities.model}`].filter(Boolean).join(" · ")
    : opts.locked ? "Unit details open when your work window starts (IR94)." : "";
  let card: QrMatch["job"] = null;
  if (r.jobId) {
    const s = job?.scheduledSlot ?? null;
    const today = !!s && klDate(Date.parse(s.startAt)) <= klDate(now) && klDate(now) <= klDate(Date.parse(s.endAt));
    const when = s ? windowText(s, now) : "";
    const alerts = job?.alertIds?.length ? ` · ${job.alertIds.length} linked alert${job.alertIds.length === 1 ? "" : "s"}` : "";
    card = { label: `${today ? "Your job today" : "Your next job"}: ${r.jobId.slice(0, 8)}`, text: [when, job ? typeLabel(job.type) : "", job?.status.replace(/_/g, " ") ?? ""].filter(Boolean).join(" · ") + alerts, tone: today ? "ok" : "primary" };
  }
  return { title, sub, job: card, unitHref: `/technician/units/${r.unitId}${r.jobId ? `?jobId=${r.jobId}` : ""}`, jobHref: r.jobId ? `/technician/jobs/${r.jobId}` : null };
}

/** Readable refusals of a scan: anything outside the user's assignments is absent without unit data (DD-T13). */
export function qrRefusal(f: { code: string; messageKey: string }): { title: string; text: string } {
  if (f.code === "NOT_FOUND") return { title: "Page unavailable", text: "This label is not on a unit in your assignments, or it is unknown. Check the label, or enter the unit ID manually." };
  if (f.code === "VALIDATION") return { title: "Not a label", text: "Enter the label code, a device serial or the unit ID." };
  return { title: "Scan failed", text: `${f.code} — try again.` };
}
