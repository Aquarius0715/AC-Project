// The reads of the technician's alert evidence (FR-T07, DD-T07, SCR-T07) through the DAL: the unit's alerts with the
// condition each was raised by (alerts.list, Alert.rule), the unit for its name and the latest reading of the rule's
// metric (units.get), the URL's job (jobs.get) and whether the user holds alert.resolve. ?alertId= picks the alert.
// Before the work window the page says when it opens (IR76). Texts in the user's display language; every time is
// formatted here (IR284).
import "server-only";
import { coreDisplay, coreNow, coreOp, corePermissions, CoreError } from "@ac/web/lib/dal";
import { i18nOf, showTime } from "@ac/web/lib/i18n";
import type { ApiUnitDetail } from "@ac/web/lib/units";
import { alertChoices, alertView, selectAlert, type ApiTechAlert } from "@ac/web/lib/techAlerts";
import type { TechJobRead } from "@ac/web/lib/techJob";

type Page<T> = { items: T[] };
const optional = <T,>(p: Promise<T>, fallback: T): Promise<T> => p.catch((e) => {
  if (e instanceof CoreError && e.error.code !== "UNAVAILABLE" && e.error.code !== "TIMEOUT") return fallback;
  throw e;
});
const UNAVAILABLE: Record<string, string> = {
  "errors.assignment_ended": "Your work window on this unit has ended.",
  "errors.assignment_required": "You need an active assignment on this unit to see its alerts.",
  "error.notFound": "This AC is not in your assignments.",
};

export async function loadAlerts(unitId: string, sp: { jobId?: string; alertId?: string }) {
  const [now, display] = await Promise.all([coreNow(), coreDisplay()]);
  const i = i18nOf(display);
  const { t } = i;
  const nowMs = now.getTime();
  const unitHref = `/technician/units/${unitId}${sp.jobId ? `?jobId=${sp.jobId}` : ""}`;
  const alerts = await coreOp<Page<ApiTechAlert>>("alerts.list", { limit: 50, filters: { unitId } }).then((r) => r.items).catch((e) => {
    if (e instanceof CoreError && e.status < 500) return e.error;
    throw e; // error.tsx
  });
  if (!Array.isArray(alerts)) {
    if (alerts.messageKey === "errors.assignment_not_started") { // IR76: the start time from the URL's job
      const job = sp.jobId ? await optional(coreOp<TechJobRead<{ assignment: { scheduledStart: string } | null }>>("jobs.get", { jobId: sp.jobId }), null) : null;
      const start = job?.projection === "detail" ? job.assignment?.scheduledStart ?? null : null;
      return {
        kind: "unavailable" as const, title: t("Not started yet"), unitHref, back: t("← Unit"),
        text: start ? t("You can see this AC from {time}, when your work window starts.", { time: showTime(start, display) }) : t("Available from the work start time of your assigned job."),
      };
    }
    return { kind: "unavailable" as const, title: t("Alerts aren’t available"), unitHref, back: t("← Unit"), text: UNAVAILABLE[alerts.messageKey] ? t(UNAVAILABLE[alerts.messageKey]) : alerts.code };
  }
  const [unit, job, permissions] = await Promise.all([
    optional(coreOp<ApiUnitDetail>("units.get", { id: unitId }), null),
    sp.jobId ? optional(coreOp<TechJobRead<{ id: string; type: string }>>("jobs.get", { jobId: sp.jobId }), null) : Promise.resolve(null),
    corePermissions(),
  ]);
  const selected = selectAlert(alerts, sp.alertId);
  return {
    kind: "live" as const, unitId, unitHref, unitName: unit?.displayName ?? unitId.slice(0, 8),
    choices: alertChoices(alerts, nowMs, i), count: alerts.length,
    alert: selected ? alertView(selected, { alerts, unit, job: job?.projection === "detail" ? { id: job.id, type: job.type } : null, canResolve: permissions.has("alert.resolve") }, nowMs, i) : null,
  };
}

export type AlertsLive = Extract<Awaited<ReturnType<typeof loadAlerts>>, { kind: "live" }>;
