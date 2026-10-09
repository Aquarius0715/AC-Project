// /customer/units/[id] (FR-C03): in API mode a Server Component reads the unit, its room, the customer's alert
// policies and the unit's command history (commands.list, IR216) through the DAL; remote control and policy changes are
// Server Actions (actions.ts). The texts follow the user's display language (IR259). The Phase 1A demo keeps the
// fixture unit.
import { connection } from "next/server";
import { notFound } from "next/navigation";
import { apiMode, coreDisplay, coreOp, CoreError } from "@ac/web/lib/dal";
import { translator } from "@ac/web/lib/i18n";
import { unitRowFromApi, type ApiUnit } from "@ac/web/lib/client";
import type { ApiCommand, ApiUnitDetail } from "@ac/web/lib/units";
import { UnitView, type PolicyOption } from "./_components/unit-view";

type ApiPolicy = { id: string; name: string; kind: string; metric?: string; operator?: string; threshold?: number; durationSeconds?: number; severity?: string };

export default async function CustomerUnitPage({ params }: PageProps<"/customer/units/[id]">) {
  const { id } = await params;
  await connection();
  if (!apiMode()) return <UnitView id={id} />;
  let detail: ApiUnitDetail;
  try {
    detail = await coreOp<ApiUnitDetail>("units.get", { id });
  } catch (e) {
    // outside the customer's scope (D01) or not a unit id at all → the not-found page
    if (e instanceof CoreError && (e.error.code === "NOT_FOUND" || e.error.fieldErrors.id)) notFound();
    throw e;
  }
  const [room, policies, history, display] = await Promise.all([
    coreOp<{ items: ApiUnit[] }>("units.list", { limit: 100, filters: detail.spaceId ? { spaceId: detail.spaceId } : { unitIds: [id] } }),
    coreOp<{ items: ApiPolicy[] }>("policies.list", { limit: 100 }),
    coreOp<{ items: ApiCommand[] }>("commands.list", { unitId: id, query: { limit: 20 } }),
    coreDisplay(),
  ]);
  const t = translator(display.locale);
  const severity: Record<string, string> = { critical: "Critical", warning: "Warning", normal: "Info" };
  const options: PolicyOption[] = policies.items
    .filter((p) => p.kind === "alert")
    .map((p) => ({ id: p.id, name: p.name, rule: `${p.metric} ${p.operator} ${p.threshold}${p.durationSeconds ? t(" for {n} s", { n: p.durationSeconds }) : ""} → ${p.severity && severity[p.severity] ? t(severity[p.severity]) : p.severity}` }));
  return <UnitView id={id} detail={detail} room={room.items.map(unitRowFromApi)} policies={options} history={history.items} />;
}
