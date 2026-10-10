// /admin/units (FR-A02, SCR-A02): in API mode a Server Component reads the customer register (customers.list for both
// statuses, organizations.list, properties.list, units.list) with the contract standing where the session may read it
// (contracts.list, overdue invoices.list, restrictions.list). For customerId it adds the customer's spaces, models
// (capabilities.list), alert policies (policies.list for the customer) and open alerts; for unitId units.get with the
// D05 archive blockers (contracts, jobs, device bindings, restrictions, pending commands), and the customer's client users
// (clientUsers.list). tab=warranty (no customer) reads units.coverage with the claimable jobs (jobs.get, reports.get).
// URL keys: customerId, tab, locationId (property, space or "unassigned:<propertyId>"), propertyId, unitId, powerState,
// connections, search. Writes are Server Actions (actions.ts). The Phase 1A demo keeps the fixtures. Texts and times of
// the first render are formatted here in the user's language and display time zone (IR293).
import { connection } from "next/server";
import { apiMode, coreAll, coreDisplay, coreNow, coreOp, corePermissions, CoreError } from "@ac/web/lib/dal";
import { klStamp } from "@ac/web/lib/energy";
import { i18nOf, relativeTime, showDate, type I18n } from "@ac/web/lib/i18n";
import { businessDay } from "@ac/web/lib/clientBilling";
import { statusWord, typeLabel } from "@ac/web/lib/partnerJobDetail";
import { metricLabel } from "@ac/web/lib/adminAlerts";
import type { ApiCapability } from "@ac/web/lib/devices";
import {
  claimCandidate, clientUserRows, coverageKpis, coverageRows, connections, customerRows, defaultRules, filterUnits, locationTree, modelLabel, placeOptions, policyLines, powerStates, registerKpis, selection, standing, unitRows, unitsAt,
  conditionText, type ApiContractLite, type ApiCustomerPolicy, type ApiCustomerRow, type ApiInvoiceLite, type ApiOrgRow, type ApiPropertyRow, type ApiRestrictionLite,
  type ApiClientUser, type ApiCoverage, type ApiSpaceRow, type ApiUnitDetail, type ApiUnitRow, type ClaimCandidate, type Connection,
} from "@ac/web/lib/assets";
import { UnitsDemo } from "./_components/units-demo";
import { UnitsView, type CustomerLive, type UnitLive, type UnitsLive } from "./_components/units-view";

type Member = { id: string; displayName: string };
type Job = { id: string; type: string; status: string };
type Device = { id: string; serial: string; unitId: string | null; connection: Connection };
type Alert = { severity: "critical" | "warning" | "normal" };
type Audit = { occurredAt: string; actorId: string; reason: string | null };
type JobForClaim = { id: string; version: number; type: string; unitId: string; completedAt: string | null; costs: { kind: "estimate" | "actual"; amountMinor: number; currency: string }[]; reportRefs: { reportId: string; reportVersion: number }[] };
type Report = { parts: { name: string; quantity: number; catalogCode: string | null }[]; acceptedAt: string | null };
const done = ["completed", "cancelled"];
const restrictionState: Record<string, string> = { requested: "requested", applied: "applied", release_requested: "release requested" };
const day = 86_400_000; // audit periods: the last 365 days up to tomorrow (at most 366 days, IR143)

export default async function AdminUnitsPage({ searchParams }: PageProps<"/admin/units">) {
  await connection();
  if (!apiMode()) return <UnitsDemo />;
  const sp = await searchParams;
  const one = (k: string) => (typeof sp[k] === "string" && sp[k] ? (sp[k] as string) : undefined);
  const [now, perms, display] = await Promise.all([coreNow(), corePermissions(), coreDisplay()]);
  const i = i18nOf(display);
  const { t } = i;
  const has = (...p: string[]) => p.some((x) => perms.has(x));
  if (!has("asset.read")) return <UnitsView live={null} />;
  const canContracts = has("contract.read", "billing.read", "restriction.read");
  const canInvoices = has("billing.read", "restriction.read");
  const [active, inactive, orgs, properties, units, contracts, overdue, restrictions, members] = await Promise.all([
    coreAll<ApiCustomerRow>("customers.list", { filters: { status: "active" } }),
    coreAll<ApiCustomerRow>("customers.list", { filters: { status: "inactive" } }),
    coreAll<ApiOrgRow>("organizations.list", { filters: { kind: "customer" } }),
    coreAll<ApiPropertyRow>("properties.list"),
    coreAll<ApiUnitRow>("units.list"),
    canContracts ? coreAll<ApiContractLite>("contracts.list") : Promise.resolve(null),
    canInvoices ? coreAll<ApiInvoiceLite>("invoices.list", { filters: { overdueOnly: true } }) : Promise.resolve(null),
    has("restriction.read") ? coreAll<ApiRestrictionLite>("restrictions.list") : Promise.resolve(null),
    has("identity.read") ? coreAll<Member>("members.list") : Promise.resolve([] as Member[]),
  ]);
  const standingOf = (c: ApiCustomerRow) => (contracts && overdue ? standing(c.id, contracts, overdue, restrictions, now) : null);
  const rows = customerRows([...active, ...inactive], orgs, properties, units, standingOf, i);
  const live: UnitsLive = {
    now: now.toISOString(), todayKL: klStamp(now.toISOString()).slice(0, 10), canWrite: has("asset.write"), standingKnown: !!(contracts && overdue),
    rows, kpis: registerKpis(rows, units, t), search: one("search") ?? "", missing: !!one("customerId") && !rows.some((r) => r.id === one("customerId")),
    top: one("tab") === "warranty" ? "warranty" : "customers", activeCustomers: rows.filter((r) => r.status === "active").map((r) => ({ id: r.id, name: r.name })), coverageAttention: null,
  };
  const cust = rows.find((r) => r.id === one("customerId"));
  if (!cust) {
    const coverage = has("asset.read", "contract.read") ? await coreAll<ApiCoverage>("units.coverage") : null;
    live.coverageAttention = coverage ? coverage.filter((c) => c.status === "expiring" || c.status === "no_coverage").length : null;
    if (live.top === "warranty") live.warranty = coverage && await warrantyLive(coverage, { has, units, properties, rows, contracts, i });
    return <UnitsView live={live} />;
  }

  const orgUnits = units.filter((u) => u.customerOrgId === cust.orgId);
  const props = properties.filter((p) => p.customerOrgId === cust.orgId);
  const canPolicies = has("alert.policy.read");
  const unitId = one("unitId");
  const [spaces, caps, policies, openAlerts, detail] = await Promise.all([
    Promise.all(props.map((p) => coreAll<ApiSpaceRow>("spaces.list", { filters: { propertyId: p.id } }))).then((x) => x.flat()),
    has("dashboard.read", "device.read") ? coreAll<ApiCapability>("capabilities.list") : Promise.resolve([] as ApiCapability[]),
    canPolicies ? coreAll<ApiCustomerPolicy>("policies.list", { filters: { customerId: cust.id } }) : Promise.resolve(null),
    has("alert.read", "alert.policy.read")
      ? Promise.all((["open", "acknowledged"] as const).map((status) => coreAll<Alert>("alerts.list", { filters: { customerId: cust.id, status } }))).then((x) => x.flat())
      : Promise.resolve(null),
    unitId ? coreOp<ApiUnitDetail>("units.get", { id: unitId }).catch((e) => (e instanceof CoreError && e.status === 404 ? null : Promise.reject(e))) : Promise.resolve(null),
  ]);
  const clientUsers = await coreAll<ApiClientUser>("clientUsers.list", { filters: { customerId: cust.id } });
  const d = detail && detail.customerOrgId === cust.orgId ? detail : null;
  const models = new Map(caps.map((k) => [k.id, k.model]));
  const memberName = new Map(members.map((m) => [m.id, m.displayName]));
  const tree = locationTree(props, spaces, orgUnits, cust.orgId);
  // an opened unit selects its own location unless the URL names one (Figma 02-4: the tree shows where the unit is)
  const sel = selection(tree, one("locationId") ?? (d && !d.archived ? d.spaceId ?? `unassigned:${d.propertyId}` : one("propertyId")), spaces);
  const powerState = powerStates.find((p) => p === one("powerState"));
  const conns = (one("connections") ?? "").split(",").filter((c): c is Connection => (connections as string[]).includes(c));
  const at = sel ? unitsAt(sel, orgUnits) : [];
  const sev = (s: Alert["severity"]) => openAlerts?.filter((a) => a.severity === s).length ?? 0;
  const lastEdit = sel && sel.kind !== "unassigned" && has("audit.read")
    ? await coreOp<{ items: Audit[] }>("audit.list", {
      limit: 1, filters: { targetKind: sel.kind, targetId: sel.kind === "property" ? sel.property.id : sel.space.id, from: new Date(now.getTime() - 365 * day).toISOString(), to: new Date(now.getTime() + day).toISOString() },
    }).then((p) => p.items[0] ?? null)
    : null;
  const customer: CustomerLive = {
    row: cust, tab: one("tab") === "policies" ? "policies" : one("tab") === "users" ? "users" : "overview", tree, selection: sel, places: placeOptions(tree, spaces, t),
    users: clientUserRows(clientUsers, (m) => memberName.get(m) ?? t("{name} owner", { name: cust.name }), undefined, i),
    models: caps.map((k) => ({ id: k.id, label: `${k.manufacturer} ${k.model}` })).sort((a, b) => a.label.localeCompare(b.label)),
    perProperty: tree.map((p) => `${p.name} ${p.units}`).join(" · ") || t("no properties yet"),
    alertsSub: openAlerts ? ([["{n} critical", sev("critical")], ["{n} warning", sev("warning")], ["{n} info", sev("normal")]] as const).filter(([, n]) => n > 0).map(([l, n]) => t(l, { n })).join(" · ") || t("none open") : null,
    filter: { powerState, connections: conns, search: one("search") ?? "" }, total: at.length, units: unitRows(filterUnits(at, { powerState, connections: conns, search: "" }), spaces, models, t),
    lastEdit: lastEdit ? `${showDate(lastEdit.occurredAt, i.display)} · ${memberName.get(lastEdit.actorId) ?? t("HQ")}` : null,
    policies: policies ? policyLines(policies, orgUnits.map((u) => u.id), (m) => memberName.get(m) ?? cust.name, i) : null,
    rules: (() => { const d = policies?.find((p) => p.kind === "default_alert"); return d ? { policyId: d.id, items: defaultRules(d, i) } : null; })(),
    canRules: has("alert.policy.write"), canAttach: has("asset.write", "alert.policy.write") && canPolicies,
  };

  if (unitId) {
    if (!d) customer.unitMissing = true;
    else customer.unit = await unitLive(d, { now, has, contracts, restrictions, policies, models, memberName, customerName: cust.name, i });
  }
  live.customer = customer;
  return <UnitsView live={live} />;
}

type Ctx = {
  now: Date; has: (...p: string[]) => boolean; contracts: ApiContractLite[] | null; restrictions: ApiRestrictionLite[] | null; policies: ApiCustomerPolicy[] | null;
  models: Map<string, string>; memberName: Map<string, string>; customerName: string; i: I18n;
};
/** The unit edit card: the bound device, the attached policies (default first) and what blocks taking the unit out of use (D05). */
async function unitLive(d: ApiUnitDetail, x: Ctx): Promise<UnitLive> {
  const [jobs, devices, archived] = await Promise.all([
    x.has("job.read") && !d.archived ? coreAll<Job>("jobs.list", { filters: { unitId: d.id } }) : Promise.resolve(null),
    x.has("device.read") ? coreAll<Device>("devices.list", { filters: { unitId: d.id } }) : Promise.resolve(null),
    d.archived && x.has("audit.read")
      ? coreOp<{ items: Audit[] }>("audit.list", { limit: 1, filters: { targetKind: "unit", targetId: d.id, action: "units.archive", from: new Date(x.now.getTime() - 365 * day).toISOString(), to: new Date(x.now.getTime() + day).toISOString() } }).then((p) => p.items[0] ?? null)
      : Promise.resolve(null),
  ]);
  const device = devices?.find((v) => v.unitId === d.id) ?? null;
  const { t, display } = x.i;
  const nowMs = x.now.getTime();
  const blockers = d.archived ? [] : [
    ...(x.contracts ?? []).filter((k) => k.unitIds.includes(d.id) && nowMs < Date.parse(k.endAt)).map((k) => ({ label: t("Unexpired contract ({plan}, until {date})", { plan: k.planType.toUpperCase(), date: businessDay(k.endAt, display.locale) }), href: `/admin/billing/contracts?contractId=${k.id}` })),
    ...(jobs ?? []).filter((j) => !done.includes(j.status)).map((j) => ({ label: t("Active job ({type}, {status})", { type: typeLabel(j.type, t), status: statusWord(j.status, t) }), href: `/admin/jobs?jobId=${j.id}` })),
    ...(device ? [{ label: t("Active device binding ({serial})", { serial: device.serial }), href: `/admin/devices?tab=devices&deviceId=${device.id}` }] : []),
    ...(x.restrictions ?? []).filter((r) => r.unitIds.includes(d.id) && r.state in restrictionState).map((r) => ({ label: t("Active restriction ({state})", { state: t(restrictionState[r.state]) }), href: `/admin/restrictions?restrictionId=${r.id}` })),
    ...(d.pendingCommandIds.length ? [{ label: t(d.pendingCommandIds.length === 1 ? "1 pending command" : "{n} pending commands", { n: d.pendingCommandIds.length }), href: null }] : []),
  ];
  const byId = new Map((x.policies ?? []).map((p) => [p.id, p]));
  const def = x.policies?.find((p) => p.kind === "default_alert");
  const attached = [
    ...(def ? [{ id: def.id, kind: "default_alert" as const, name: def.name, type: t("Default"), condition: t((def.rules?.length ?? 0) === 1 ? "1 rule · ventilation and fault causes" : "{n} rules · ventilation and fault causes", { n: def.rules?.length ?? 0 }), madeBy: t("Always attached"), on: t((def.rules?.length ?? 0) === 1 ? "{on} of 1 rule on" : "{on} of {n} rules on", { on: defaultRules(def, x.i).filter((r) => r.enabled).length, n: def.rules?.length ?? 0 }) }] : []),
    ...d.alertPolicyIds.map((id) => {
      const p = byId.get(id);
      const metric = p?.metric ?? "";
      return { id, kind: "alert" as const, name: p?.name ?? id.slice(0, 8), type: metricLabel[metric] ? t(metricLabel[metric]) : metric, condition: p ? conditionText(p, x.i) : t("policy outside this view"), madeBy: p ? t("Made by {name}", { name: x.memberName.get(p.ownerMembershipId) ?? x.customerName }) : "", on: p && !p.enabled ? t("Off") : "" };
    }),
  ];
  return {
    u: d, path: d.location.pathLabels.join(" › "), model: `${modelLabel(d, x.models)} · split`, device: device ? { id: device.id, serial: device.serial, connection: device.connection } : null,
    deviceKnown: devices !== null, attached, blockers: x.has("job.read", "device.read") || x.contracts ? blockers : null,
    archivedNote: d.archived ? (archived ? [t("Taken out of use {date}", { date: showDate(archived.occurredAt, display) }), ...(archived.reason ? [t("reason: {reason}", { reason: archived.reason })] : []), x.memberName.get(archived.actorId) ?? t("HQ")].join(" · ") : t("Taken out of use")) : null,
    seen: d.lastSeenAt ? relativeTime(d.lastSeenAt, nowMs, x.i) : null,
  };
}

/** Warranty & coverage (FR-A19): one row per unit with its warranty end and covering contracts, and the jobs whose
 * replaced parts can still be claimed from the manufacturer (job.read for the parts, job.write to file the claim). */
async function warrantyLive(coverage: ApiCoverage[], x: { has: (...p: string[]) => boolean; units: ApiUnitRow[]; properties: ApiPropertyRow[]; rows: { id: string; name: string }[]; contracts: ApiContractLite[] | null; i: I18n }) {
  const { t, display } = x.i;
  const caps = x.has("dashboard.read", "device.read") ? await coreAll<ApiCapability>("capabilities.list") : [];
  const unit = new Map(x.units.map((u) => [u.id, u]));
  const prop = new Map(x.properties.map((p) => [p.id, p.name]));
  const customer = new Map(x.rows.map((r) => [r.id, r.name]));
  const model = new Map(caps.map((k) => [k.id, `${k.manufacturer} ${k.model}`]));
  const contract = new Map((x.contracts ?? []).map((k) => [k.id, t("{plan} until {date}", { plan: k.planType.toUpperCase(), date: businessDay(k.endAt, display.locale) })]));
  const rows = coverageRows(coverage, {
    unit: (id) => { const u = unit.get(id); return u && { name: u.displayName, place: prop.get(u.propertyId) ?? "" }; },
    customer: (id) => customer.get(id) ?? id.slice(0, 8), model: (id) => model.get(id) ?? id.slice(0, 8), contract: (id) => contract.get(id) ?? t("contract {id}", { id: id.slice(0, 8) }),
  }, await coreNow(), x.i);
  let claims: ClaimCandidate[] | null = null;
  if (x.has("job.read")) {
    const ids = [...new Set(coverage.flatMap((c) => c.claimableJobIds))];
    claims = (await Promise.all(ids.map(async (jobId) => {
      const j = await coreOp<JobForClaim>("jobs.get", { jobId }).catch(() => null);
      if (!j) return null;
      const ref = j.reportRefs.at(-1);
      const r = ref ? await coreOp<Report>("reports.get", { jobId, reportId: ref.reportId, reportVersion: ref.reportVersion }).catch(() => null) : null;
      return claimCandidate(j, r?.parts ?? [], unit.get(j.unitId)?.displayName ?? j.unitId.slice(0, 8), x.i);
    }))).filter((c): c is ClaimCandidate => !!c);
  }
  return { rows, kpis: coverageKpis(rows, t), customers: x.rows, claims, canClaim: x.has("job.write") };
}

