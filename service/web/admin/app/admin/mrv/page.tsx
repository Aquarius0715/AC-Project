// /admin/mrv (FR-A14, SCR-A14): in API mode a Server Component reads customer organizations, units, baselines and
// emission factors; on the reports tab mrv.list for the URL filters (organizationId, unitId, status, from/to) and the
// reportId detail (mrv.get of reportVersion, mrv.versions for the selector). Previews, drafts, demo reviews and factor
// versions are Server Actions (actions.ts). Texts in the display language; the report periods stay Kuala Lumpur time and
// the review times are formatted here, in the display time zone (IR282, IR297). The Phase 1A demo keeps the fixture
// reports.
import { connection } from "next/server";
import { apiMode, coreDisplay, coreOp, corePermissions } from "@ac/web/lib/dal";
import { baselineRows, klInstant, type ApiBaseline } from "@ac/web/lib/energy";
import { i18nOf } from "@ac/web/lib/i18n";
import { mrvView, reportRows, reviewItems, type ApiFactor, type ApiMRVReport } from "@ac/web/lib/mrv";
import { MRVDemo } from "./_components/mrv-demo";
import { MRVView, type MRVLive } from "./_components/mrv-view";

type Page<T> = { items: T[] };
type Unit = { id: string; displayName: string; customerOrgId: string; archived: boolean };

export default async function AdminMRVPage({ searchParams }: PageProps<"/admin/mrv">) {
  await connection();
  if (!apiMode()) return <MRVDemo />;
  const sp = await searchParams;
  const one = (k: string) => (typeof sp[k] === "string" && sp[k] ? (sp[k] as string) : undefined);
  const tab = one("tab") === "factors" ? "factors" : "reports";
  const [perms, display, orgs, units, baselines, factors] = await Promise.all([
    corePermissions(), coreDisplay(),
    coreOp<Page<{ id: string; name: string; kind: string; status: string }>>("organizations.list", { limit: 100, filters: { kind: "customer" } }),
    coreOp<Page<Unit>>("units.list", { limit: 100 }),
    coreOp<Page<ApiBaseline>>("baselines.list", { limit: 100 }),
    coreOp<Page<ApiFactor>>("factors.list", { limit: 100 }),
  ]);
  const i = i18nOf(display);
  const customers = orgs.items.map((o) => ({ id: o.id, name: o.name }));
  const unitOptions = units.items.filter((u) => !u.archived).map((u) => ({ id: u.id, label: u.displayName, organizationId: u.customerOrgId }));
  const live: MRVLive = {
    tab, canWrite: perms.has("mrv.write"), canReview: perms.has("mrv.review"), canFactors: perms.has("mrv.factors"),
    orgs: customers, units: unitOptions, baselines: baselineRows(baselines.items, i.t), factors: factors.items,
  };
  if (tab === "reports") {
    const status = one("status") === "draft" || one("status") === "demo_reviewed" ? one("status") : undefined;
    const filters = {
      ...(one("organizationId") && { organizationId: one("organizationId") }), ...(one("unitId") && { unitId: one("unitId") }), ...(status && { status }),
      ...(one("from") && { from: klInstant(one("from")!) }), ...(one("to") && { to: klInstant(one("to")!) }),
    };
    const list = await coreOp<Page<ApiMRVReport>>("mrv.list", { limit: 100, filters });
    const rows = reportRows(list.items, customers, i.t);
    const id = rows.find((r) => r.id === one("reportId"))?.id ?? rows[0]?.id;
    live.reports = { rows, scope: { organizationId: one("organizationId"), unitId: one("unitId"), status, from: one("from") ?? "", to: one("to") ?? "" } };
    if (id) {
      const wanted = Number(one("reportVersion"));
      const [versions, latest] = await Promise.all([
        coreOp<Page<ApiMRVReport>>("mrv.versions", { id, query: { limit: 100 } }),
        coreOp<ApiMRVReport>("mrv.get", { id }),
      ]);
      const shown = Number.isInteger(wanted) && wanted > 0 && wanted !== latest.version ? await coreOp<ApiMRVReport>("mrv.get", { id, reportVersion: wanted }) : latest;
      live.reports.selected = {
        report: shown, latestVersion: latest.version, versions: versions.items.map((v) => ({ version: v.version, status: v.status })),
        view: mrvView(shown, customers, unitOptions, i), review: reviewItems(shown, i),
      };
    }
  }
  return <MRVView live={live} />;
}
