// /admin/restrictions (FR-A09, SCR-A09): in API mode a Server Component reads restrictions.list for the URL scope
// (contractId, invoiceId, status) with per-state counts, and for restrictionId restrictions.get with the cause invoices
// (invoices.list), the latest command per unit (commands.get) and unit names. restriction.write holders also get the
// schedule form data: eligible RTO contracts, their overdue unpaid invoices, unit capabilities, units already under an
// active restriction and the active clients per customer (IR05). Override-only callers (IR03) see the release
// projection without billing fields. Writes are Server Actions (actions.ts). Texts in the display language; the times
// of the first render are formatted here, in the display time zone (IR282, IR301). The Phase 1A demo keeps the fixtures.
import { connection } from "next/server";
import { apiMode, coreDisplay, coreNow, coreOp, corePermissions } from "@ac/web/lib/dal";
import type { ApiContract, ApiCustomer, ApiInvoice } from "@ac/web/lib/billing";
import type { ApiCapability } from "@ac/web/lib/devices";
import { i18nOf, showTime } from "@ac/web/lib/i18n";
import {
  activePeriod, executeBlocker, intentText, releaseBlocker, restrictionRows, states, stateWord, unitRows, type ApiCommand, type ApiRestriction, type RestrictionState,
} from "@ac/web/lib/restrictions";
import { RestrictionsDemo } from "./_components/restrictions-demo";
import { RestrictionsView, type RestrictionsLive } from "./_components/restrictions-view";

type Page<T> = { items: T[] };
type Unit = { id: string; displayName: string; modelId: string; archived: boolean };
const active: RestrictionState[] = ["scheduled", "requested", "applied", "release_requested"];

export default async function AdminRestrictionsPage({ searchParams }: PageProps<"/admin/restrictions">) {
  await connection();
  if (!apiMode()) return <RestrictionsDemo />;
  const sp = await searchParams;
  const one = (k: string) => (typeof sp[k] === "string" && sp[k] ? (sp[k] as string) : undefined);
  const [now, perms, display] = await Promise.all([coreNow(), corePermissions(), coreDisplay()]);
  const i = i18nOf(display), { t } = i;
  const full = perms.has("restriction.read") || perms.has("restriction.write");
  const canWrite = perms.has("restriction.write");
  if (!full && !perms.has("restriction.override")) return <RestrictionsView live={null} />;
  const status = states.find((s) => s === one("status"));
  const scope = { contractId: full ? one("contractId") : undefined, invoiceId: full ? one("invoiceId") : undefined, status };
  const filters = { ...(scope.contractId && { contractId: scope.contractId }), ...(scope.invoiceId && { invoiceId: scope.invoiceId }), ...(status && { status }) };
  const [scoped, units] = await Promise.all([
    coreOp<Page<ApiRestriction>>("restrictions.list", { limit: 100, filters }),
    perms.has("asset.read") ? coreOp<Page<Unit>>("units.list", { limit: 100 }) : Promise.resolve({ items: [] as Unit[] }), // override-only callers see unit IDs
  ]);
  const names = new Map(units.items.map((u) => [u.id, u.displayName]));
  const [contracts, customers] = full
    ? await Promise.all([coreOp<Page<ApiContract & { rulesVersion: string | null }>>("contracts.list", { limit: 100 }), coreOp<Page<ApiCustomer>>("customers.list", { limit: 100 })])
    : [{ items: [] }, { items: [] }];
  const customerName = new Map(customers.items.map((c) => [c.id, c.name]));
  const contractLabel = (id?: string) => {
    const k = contracts.items.find((c) => c.id === id);
    return k ? `${customerName.get(k.customerId) ?? t("customer")} · ${t("{plan} contract v{version}", { plan: k.planType, version: k.version })}` : full ? t("contract") : t("release view");
  };
  const rows = restrictionRows(scoped.items, contractLabel, i);
  const counts = Object.fromEntries(states.map((s) => [s, rows.filter((r) => r.state === s).length])) as Record<RestrictionState, number>;
  const live: RestrictionsLive = { now: now.toISOString(), full, canWrite, scope, rows, counts, contracts: contracts.items.map((k) => ({ id: k.id, label: contractLabel(k.id) })) };

  const id = rows.find((r) => r.id === one("restrictionId"))?.id ?? rows[0]?.id;
  if (id) {
    const r = await coreOp<ApiRestriction>("restrictions.get", { id });
    const commandIds = r.perUnit.flatMap((u) => [u.applyCommandIds.at(-1), u.releaseCommandIds.at(-1)]).filter((x): x is string => !!x);
    const [commands, invoices] = await Promise.all([
      Promise.all(commandIds.map((c) => coreOp<ApiCommand>("commands.get", { id: c }).catch(() => null))),
      full && r.contractId ? coreOp<Page<ApiInvoice>>("invoices.list", { limit: 100, filters: { contractId: r.contractId } }) : Promise.resolve({ items: [] as ApiInvoice[] }),
    ]);
    const invoice = new Map(invoices.items.map((i) => [i.id, i]));
    const causes = (r.causeInvoiceIds ?? []).map((i) => ({ id: i, number: invoice.get(i)?.number ?? i.slice(0, 8), paid: invoice.get(i)?.status === "paid", amountMinor: invoice.get(i)?.amountMinor ?? null, currency: invoice.get(i)?.currency ?? "" }));
    const allPaid = causes.length > 0 && causes.every((c) => c.paid);
    const notices = r.noticeNotificationIds?.length ?? 0;
    live.selected = {
      r, contract: contractLabel(r.contractId), causes, allPaid,
      units: unitRows(r, names, new Map(commands.filter((c): c is ApiCommand => !!c).map((c) => [c.id, c])), i),
      texts: {
        notice: t(notices === 1 ? "{time} · 1 client notice" : "{time} · {n} client notices", { time: showTime(r.noticeAt, display), n: notices }),
        executeAfter: showTime(r.executeAfter, display), period: activePeriod(r, now, i), intent: intentText(r, i),
        execute: executeBlocker(r, now, i), release: releaseBlocker(r, allPaid, now, i),
      },
    };
  }

  if (canWrite) { // the schedule form: eligible contracts, their overdue unpaid invoices, units and recipients
    const [overdue, caps, all, clients] = await Promise.all([
      coreOp<Page<ApiInvoice>>("invoices.list", { limit: 100, filters: { overdueOnly: true } }),
      coreOp<Page<ApiCapability>>("capabilities.list", { limit: 100 }),
      Object.keys(filters).length ? coreOp<Page<ApiRestriction>>("restrictions.list", { limit: 100 }) : Promise.resolve(scoped),
      coreOp<Page<{ customerId: string; status: string }>>("clientUsers.list", { limit: 100, filters: { status: "active" } }),
    ]);
    const busy = new Map<string, string>();
    for (const x of all.items) if (active.includes(x.state)) for (const u of x.unitIds) busy.set(u, t("under restriction {id} ({state})", { id: x.id.slice(0, 8), state: stateWord(x.state, t) }));
    for (const x of all.items) for (const c of x.recoveryCases) if (c.state !== "resolved") busy.set(c.unitId, t("unresolved recovery case"));
    const cap = new Map(caps.items.map((k) => [k.id, k]));
    const term = (k: ApiContract) => Date.parse(k.startAt) <= now.getTime() && now.getTime() < Date.parse(k.endAt);
    live.schedule = contracts.items.filter((k) => k.planType === "rto" && k.restrictionEligible && k.rulesVersion && term(k)).map((k) => ({
      id: k.id, version: k.version, label: contractLabel(k.id), rulesVersion: k.rulesVersion!, recipients: clients.items.filter((c) => c.customerId === k.customerId).length,
      invoices: overdue.items.filter((i) => i.contractId === k.id && i.status === "unpaid").map((i) => ({ id: i.id, number: i.number })),
      units: k.unitIds.map((u) => {
        const unit = units.items.find((x) => x.id === u);
        const c = unit ? cap.get(unit.modelId) : undefined;
        return { id: u, name: names.get(u) ?? u.slice(0, 8), blocked: !unit || unit.archived ? t("archived") : busy.get(u) ?? (!c?.control ? t("no power control") : null), temperature: c?.temperature ?? null };
      }),
    }));
  }
  return <RestrictionsView live={live} />;
}
