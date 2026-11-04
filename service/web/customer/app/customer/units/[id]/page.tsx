// /customer/units/[id] (FR-C03): in API mode a Server Component reads the unit, its room, the customer's alert
// policies and the unit's command history (commands.list, IR216) through the DAL; remote control and policy changes are
// Server Actions (actions.ts). The texts follow the user's display language (IR259). The Phase 1A demo keeps the
// fixture unit.
import { connection } from "next/server";
import { notFound } from "next/navigation";
import { apiMode, coreAll, coreDisplay, coreNow, coreOp, corePrincipal, CoreError } from "@ac/web/lib/dal";
import { i18nOf } from "@ac/web/lib/i18n";
import { conditionText, defaultRuleRows, type ApiDefaultPolicy } from "@ac/web/lib/customerPolicies";
import type { Operator } from "@ac/web/lib/adminAlerts";
import { unitRowFromApi, type ApiUnit } from "@ac/web/lib/client";
import type { ApiCommand, ApiUnitDetail } from "@ac/web/lib/units";
import type { ApiRestriction } from "@ac/web/lib/restrictions";
import { UnitView, type PolicyOption } from "./_components/unit-view";
import { orNull } from "@ac/web/lib/promises";

interface ApiPolicy { id: string; name: string; kind: string; metric?: string; operator?: string; threshold?: number; durationSeconds?: number; severity?: string }

/** The detail of the unit's active restriction, found as Contracts & payments finds it — the contract with an active
restriction, one of its invoices, restrictions.forInvoice (the customer has no restriction list) — or null (IR583). */
async function restrictionHref(unitId: string): Promise<string | null> {
  const contracts = await coreAll<{ id: string; activeRestrictionIds?: string[] }>("contracts.list", { filters: { unitId } });
  for (const k of contracts.filter((c) => c.activeRestrictionIds?.length)) {
    const invoices = await coreAll<{ id: string; status: string }>("invoices.list", { filters: { contractId: k.id } });
    for (const i of [...invoices].toSorted((a, b) => Number(a.status === "paid") - Number(b.status === "paid")).slice(0, 3)) {
      const page = await coreOp<{ items: ApiRestriction[] }>("restrictions.forInvoice", { invoiceId: i.id, query: { limit: 10 } });
      const r = page.items.find((x) => k.activeRestrictionIds!.includes(x.id) && x.unitIds.includes(unitId));
      if (r) return `/customer/payments/${i.id}?tab=restriction&restrictionId=${r.id}`;
    }
  }
  return null;
}

export default async function CustomerUnitPage({ params }: Readonly<PageProps<"/customer/units/[id]">>) {
  const { id } = await params;
  await connection();
  if (!apiMode()) return <UnitView id={id} />;
  let detail: ApiUnitDetail;
  try {
    detail = await coreOp<ApiUnitDetail>("units.get", { id });
  } catch (error) {
    // outside the customer's scope (D01) or not a unit id at all → the not-found page
    if (error instanceof CoreError && (error.error.code === "NOT_FOUND" || error.error.fieldErrors.id)) notFound();
    throw error;
  }
  const [room, policies, history, display, restriction, now, me] = await Promise.all([
    coreOp<{ items: ApiUnit[] }>("units.list", { limit: 100, filters: detail.spaceId ? { spaceId: detail.spaceId } : { unitIds: [id] } }),
    coreOp<{ items: ApiPolicy[] }>("policies.list", { limit: 100 }),
    coreOp<{ items: ApiCommand[] }>("commands.list", { unitId: id, query: { limit: 20 } }),
    coreDisplay(),
    detail.effectiveControlPolicy.state === "restricted" ? orNull(restrictionHref(id)) : Promise.resolve(null), // a link only, never a failure
    coreNow(), // the scenario's now: a time of another day keeps its date (IR613)
    corePrincipal(),
  ]);
  const i = i18nOf(display), { t } = i;
  const severity: Record<string, string> = { critical: "Critical", warning: "Warning", normal: "Info" };
  const options: PolicyOption[] = policies.items
    .filter((p) => p.kind === "alert")
    // in words, as the Alert policies tab says it (IR623): "Room temperature ≥ 30 °C for 10 min → Warning"
    .map((p) => ({ id: p.id, name: p.name, rule: `${conditionText({ metric: p.metric ?? "", operator: (p.operator ?? "gte") as Operator, threshold: p.threshold ?? 0, durationSeconds: p.durationSeconds ?? 0 }, t)} → ${(p.severity && severity[p.severity] ? t(severity[p.severity]) : p.severity) ?? "—"}` }));
  // the default policy's rules on for this customer, as the owner switched them (IR623)
  const defaultPolicy = policies.items.find((p) => p.kind === "default_alert") as unknown as ApiDefaultPolicy | undefined;
  const defaults = defaultPolicy ? { on: defaultRuleRows(defaultPolicy, me.customerId, me.membershipId, i).on, n: defaultPolicy.rules.length } : null;
  return <UnitView id={id} detail={detail} room={room.items.map((item) => unitRowFromApi(item))} policies={options} defaults={defaults} history={history.items} restrictionHref={restriction} nowMs={now.getTime()} />;
}
