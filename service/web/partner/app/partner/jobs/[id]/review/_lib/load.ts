// The reads of the quality review (FR-P05) through the DAL: the delegated job (jobs.get detail projection), its
// current submitted report (reports.get on the last reportRef), the company's technicians for the author and
// contributor names (members.list), the linked alerts (alerts.get), the unit while the access window is open
// (units.get) and the ready photos (attachments.getContent as data URLs, at most 6 of ≤ 2 MB). Texts in the user's
// display language; every time is formatted here, on the server, in their display time zone (IR274).
import "server-only";
import { coreAll, coreDisplay, coreOp, corePrincipal, CoreError } from "@ac/web/lib/dal";
import { i18nOf, intlTag, showTime } from "@ac/web/lib/i18n";
import type { ApiUnitDetail } from "@ac/web/lib/units";
import {
  alertRows, availabilityText, dueRow, evidenceCheck, inspectionRows, partLines, readingRows, timeOnSiteText, versionRows,
  type ApiAlertLite, type ApiReviewJob, type ApiWorkReport,
} from "@ac/web/lib/partnerReview";
import type { ReviewLive } from "../_components/review-view";

type Member = { userId: string; displayName: string | null };
const quiet = <T,>(p: Promise<T>) => p.catch((e) => { // outside the caller's scope or window the row reads as absent
  if (e instanceof CoreError && (e.error.code === "FORBIDDEN" || e.error.code === "NOT_FOUND")) return null;
  throw e;
});

export async function loadReview(jobId: string): Promise<ReviewLive> {
  const [job, me, members, display] = await Promise.all([coreOp<ApiReviewJob>("jobs.get", { jobId }), corePrincipal(), coreAll<Member>("members.list"), coreDisplay()]);
  const i = i18nOf(display);
  const { t } = i;
  const names = new Map(members.filter((m) => m.displayName).map((m) => [m.userId, m.displayName!]));
  names.set(me.userId, t("you")); // the signed-in reviewer is not in the technician list (members.list)
  const id = job.id ?? job.jobId ?? jobId;
  const unit = job.projection === "detail" && job.unitId ? await quiet(coreOp<ApiUnitDetail>("units.get", { id: job.unitId })) : null;
  const base = {
    job: { id, version: job.version ?? 0, status: job.status, type: job.type, unit: unit ? `${unit.displayName} · ${unit.location.pathLabels.join(" › ")}` : job.unitId ? t("unit {id}", { id: job.unitId.slice(0, 8) }) : null },
    reviewer: me.userId, report: null,
  };
  const ref = job.projection === "detail" ? job.reportRefs?.[job.reportRefs.length - 1] : undefined;
  if (!ref) return base;
  const [r, alerts] = await Promise.all([
    coreOp<ApiWorkReport>("reports.get", { jobId: id, reportId: ref.reportId, reportVersion: ref.reportVersion }),
    Promise.all((job.alertIds ?? []).map((a) => quiet(coreOp<ApiAlertLite>("alerts.get", { id: a })))),
  ]);
  const photos = (await Promise.all(r.attachmentRefs.filter((a) => a.status === "ready" && a.size <= 2_000_000).slice(0, 6).map(async (a) => {
    const b = await quiet(coreOp<{ mime: string; bytes: string }>("attachments.getContent", { jobId: id, reportId: r.id, reportVersion: r.version, attachmentId: a.id }));
    return b ? { id: a.id, name: a.name, url: `data:${b.mime};base64,${b.bytes}` } : null;
  }))).filter((p): p is { id: string; name: string; url: string } => !!p);
  const author = names.get(r.authorId) ?? t("technician {id}", { id: r.authorId.slice(0, 8) });
  const check = evidenceCheck(r, names, t);
  const last = [...r.reviewHistory].sort((a, b) => Date.parse(b.occurredAt) - Date.parse(a.occurredAt))[0] ?? null;
  // a follow-up is a calendar date the technician picked: written as it is, in the user's language
  const followDay = (d: string) => new Date(`${d.slice(0, 10)}T00:00:00Z`).toLocaleDateString(intlTag(display.locale), { timeZone: "UTC", dateStyle: "medium" }).replace(/[\u00a0\u2009\u202f]/g, " ");
  const next = r.nextAction?.kind === "follow_up" ? t("follow-up {date} — {note}", { date: followDay(r.nextAction.date), note: r.nextAction.note }) : t("no follow-up needed");
  return {
    ...base,
    report: {
      id: r.id, version: r.version, author, submitted: r.submittedAt ? showTime(r.submittedAt, display) : null, items: inspectionRows(r.items, t), readings: readingRows(r.measurements, t),
      parts: partLines(r.parts, r.refrigerant, t), signOff: r.signOff ? `${r.signOff.signerName} · ${showTime(r.signOff.signedAt, display)}` : null, workText: r.workText, next, photos,
      timeOnSite: timeOnSiteText(job.timeOnSite, i), evidence: check.rows, acceptable: check.acceptable, missing: check.missing,
      allowed: r.reviewAvailability.allowed, blocked: availabilityText(r.reviewAvailability, job.status, t),
      versions: [...versionRows(job, r, author, names, i), ...alertRows(alerts.filter((a): a is ApiAlertLite => !!a), i), dueRow(job, i)],
      last: last ? { decision: last.decision, version: last.reportVersion, reason: last.reason } : null,
    },
  };
}
