// /admin/restrictions/[id] (FR-A10, SCR-A10): in API mode a Server Component reads restrictions.get (the release
// projection for override-only callers, IR03), the unit names and, only for audit.read holders, the restriction's audit
// timeline (audit.list over the last 12 months). Grace, exception, cancellation and override, plus reconcile / release
// retry where IR96 allows them, are Server Actions (../actions.ts). Texts in the display language; the times of the
// first render are formatted here, in the display time zone (IR282, IR301). The Phase 1A demo keeps the fixture
// restriction.
import { connection } from "next/server";
import { notFound } from "next/navigation";
import { EmptyState, Page as Screen } from "@ac/web/components/ui";
import { apiMode, coreDisplay, coreNow, coreOp, corePermissions, CoreError } from "@ac/web/lib/dal";
import { auditItem, type ApiAudit } from "@ac/web/lib/audit";
import { i18nOf } from "@ac/web/lib/i18n";
import { activePeriod, intentText, unitRows, type ApiRestriction } from "@ac/web/lib/restrictions";
import { ExceptionDemo } from "./_components/exception-demo";
import { ExceptionView } from "./_components/exception-view";

type Page<T> = { items: T[] };

export default async function AdminRestrictionPage({ params }: PageProps<"/admin/restrictions/[id]">) {
  await connection();
  const { id } = await params;
  if (!apiMode()) return <ExceptionDemo id={id} />;
  const [now, perms, display] = await Promise.all([coreNow(), corePermissions(), coreDisplay()]);
  const i = i18nOf(display), { t } = i;
  // the same permission state as the list (D01): without restriction.read / .write / .override nothing is read
  if (!perms.has("restriction.read") && !perms.has("restriction.write") && !perms.has("restriction.override")) {
    return <Screen><EmptyState title={t("No access")}>{t("Restrictions need restriction.read, restriction.write or restriction.override.")}</EmptyState></Screen>;
  }
  const [r, units] = await Promise.all([
    coreOp<ApiRestriction>("restrictions.get", { id }).catch((e) => {
      if (e instanceof CoreError && e.error.code === "NOT_FOUND") notFound();
      throw e;
    }),
    perms.has("asset.read") ? coreOp<Page<{ id: string; displayName: string }>>("units.list", { limit: 100 }) : Promise.resolve({ items: [] as { id: string; displayName: string }[] }),
  ]);
  const canAudit = perms.has("audit.read");
  const from = new Date(now.getTime() - 365 * 24 * 3600 * 1000).toISOString();
  const to = new Date(now.getTime() + 24 * 3600 * 1000).toISOString();
  const audit = canAudit ? await coreOp<Page<ApiAudit>>("audit.list", { limit: 100, filters: { targetKind: "restriction", targetId: id, from, to }, sort: { field: "occurredAt", direction: "asc" } }) : null;
  return (
    <ExceptionView live={{
      now: now.toISOString(), r, units: unitRows(r, new Map(units.items.map((u) => [u.id, u.displayName])), new Map(), i),
      canWrite: perms.has("restriction.write"), canOverride: perms.has("restriction.override"), canAudit,
      period: activePeriod(r, now, i), intent: intentText(r, i),
      audit: audit?.items.map((a) => auditItem(a, i)) ?? [],
    }} />
  );
}
