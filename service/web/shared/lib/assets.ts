// HQ customers & units (FR-A02, DATA_SOURCE=api): customers with their contract standing, the property / space tree,
// unit rows, the unit form and the customer's alert policies projected for /admin/units. Pure code shared by the
// Server Component and the client view.
import { amount, klStamp, one } from "@ac/web/lib/energy";
import { metricLabel, metricUnit, opSymbol, type Operator, type Severity } from "@ac/web/lib/adminAlerts";

export type Profile = "rto" | "general" | "energy" | "environment";
export type Connection = "online" | "offline" | "unknown" | "connecting" | "error";
export type PowerState = "on" | "off" | "unknown";
export type Scope = "indoor" | "outdoor" | "electrical";

export type ApiOrgRow = { id: string; version: number; name: string; kind: "customer" | "contractor" | "operator"; status: "active" | "inactive" };
export type ApiCustomerRow = { id: string; version: number; name: string; status: "active" | "inactive"; organizationId: string; serviceProfile: Profile; createdAt: string };
export type ApiPropertyRow = {
  id: string; version: number; customerOrgId: string; kind: "home" | "office"; name: string; address: string | null; accessInstructions: string | null; archived: boolean; updatedAt: string;
};
export type ApiSpaceRow = { id: string; version: number; propertyId: string; parentSpaceId: string | null; kind: "area" | "floor" | "room" | "space"; name: string; archived: boolean };
/** UnitSummary of service-contracts.ts (fields shown on this screen). */
export type ApiUnitRow = {
  id: string; version: number; customerOrgId: string; propertyId: string; spaceId: string | null; displayName: string; modelId: string; type: "split"; installedAt: string | null;
  serviceScope: Scope[]; alertPolicyIds: string[]; archived: boolean; capabilityVersion: number; connection: Connection; lastSeenAt: string | null;
  effectivePowerState: PowerState; activeAlertCount: number;
};
/** UnitDetail of service-contracts.ts (fields shown on this screen). */
export type ApiUnitDetail = ApiUnitRow & { location: { pathLabels: string[]; address: string | null; accessInstructions: string | null }; pendingCommandIds: string[] };
export type ApiContractLite = { id: string; customerId: string; unitIds: string[]; planType: Profile; startAt: string; endAt: string };
export type ApiInvoiceLite = { id: string; contractId: string; amountMinor: number; currency: string };
export type ApiRestrictionLite = { id: string; contractId?: string; state: string; unitIds: string[] };

const byName = <T extends { name: string }>(a: T, b: T) => a.name.localeCompare(b.name);

// ---- customers ----

/** The contract side of a customer: current contracts, overdue invoices and the most pressing active restriction. */
export type Standing = { plans: Profile[]; contracts: number; contractUnits: number; overdue: number; overdueAmount: string | null; restriction: { id: string; state: string; contractId: string } | null };
const restrictionRank: Record<string, number> = { applied: 0, requested: 1, release_requested: 2, scheduled: 3 };
export function standing(customerId: string, contracts: ApiContractLite[], overdue: ApiInvoiceLite[], restrictions: ApiRestrictionLite[] | null, now: Date): Standing {
  const t = now.getTime();
  const mine = contracts.filter((k) => k.customerId === customerId);
  const ids = new Set(mine.map((k) => k.id));
  const current = mine.filter((k) => Date.parse(k.startAt) <= t && t < Date.parse(k.endAt));
  const due = overdue.filter((i) => ids.has(i.contractId));
  const r = (restrictions ?? []).filter((x) => x.contractId && ids.has(x.contractId) && x.state in restrictionRank).sort((a, b) => restrictionRank[a.state] - restrictionRank[b.state])[0];
  return {
    plans: [...new Set(current.map((k) => k.planType))], contracts: current.length, contractUnits: new Set(current.flatMap((k) => k.unitIds)).size, overdue: due.length,
    overdueAmount: due.length ? amount(due.reduce((n, i) => n + i.amountMinor, 0), due[0].currency) : null,
    restriction: r ? { id: r.id, state: r.state, contractId: r.contractId! } : null,
  };
}
export type Mark = { label: string; tone: "crit" | "warn" | "ok" | "muted" };
const restrictionLabel: Record<string, string> = { applied: "Restriction applied", requested: "Restriction requested", release_requested: "Release requested", scheduled: "Restriction scheduled" };
/** Contract column badges (Figma 02-1): Overdue / restriction state / Good standing / No contract / Inactive. */
export function standingMarks(status: "active" | "inactive", s: Standing | null): Mark[] {
  if (status === "inactive") return [{ label: "Inactive", tone: "muted" }];
  if (!s) return [];
  const out: Mark[] = [];
  if (s.overdue) out.push({ label: "‼ Overdue", tone: "crit" });
  if (s.restriction) out.push({ label: restrictionLabel[s.restriction.state], tone: "warn" });
  if (!out.length) out.push(s.contracts ? { label: "Good standing", tone: "ok" } : { label: "No contract", tone: "muted" });
  return out;
}

export type CustomerRow = {
  id: string; version: number; name: string; billingName: string; status: "active" | "inactive"; orgId: string; orgVersion: number | null; orgStatus: "active" | "inactive"; profile: Profile; since: string;
  properties: number; propertyNames: string[]; units: number; operation: string; running: number; known: number; alerts: number; standing: Standing | null; marks: Mark[];
};
/** Operation rate = on ÷ (on + off) of the known power states; unknown stays out of the denominator (DD-A01). */
export const operation = (us: ApiUnitRow[]) => {
  const on = us.filter((u) => u.effectivePowerState === "on").length;
  const known = us.filter((u) => u.effectivePowerState !== "unknown").length;
  return known ? `${one((on * 100) / known)}%` : "—";
};
export function customerRows(cs: ApiCustomerRow[], orgs: ApiOrgRow[], ps: ApiPropertyRow[], us: ApiUnitRow[], standingOf: (c: ApiCustomerRow) => Standing | null): CustomerRow[] {
  const org = new Map(orgs.map((o) => [o.id, o]));
  return [...cs].sort(byName).map((c) => {
    const units = us.filter((u) => u.customerOrgId === c.organizationId);
    const props = ps.filter((p) => p.customerOrgId === c.organizationId).sort(byName);
    const s = standingOf(c);
    return {
      id: c.id, version: c.version, name: c.name, billingName: org.get(c.organizationId)?.name ?? "", status: c.status, orgId: c.organizationId, orgVersion: org.get(c.organizationId)?.version ?? null,
      orgStatus: org.get(c.organizationId)?.status ?? "active",
      profile: c.serviceProfile, since: klStamp(c.createdAt).slice(0, 7), properties: props.length, propertyNames: props.map((p) => p.name), units: units.length,
      operation: operation(units), running: units.filter((u) => u.effectivePowerState === "on").length, known: units.filter((u) => u.effectivePowerState !== "unknown").length,
      alerts: units.reduce((n, u) => n + u.activeAlertCount, 0), standing: s, marks: standingMarks(c.status, s),
    };
  });
}
export type ContractFilter = "all" | "overdue" | "restricted" | "good" | "none";
export function contractMatch(f: ContractFilter, r: CustomerRow): boolean {
  const s = r.standing;
  if (f === "all") return true;
  if (!s || r.status === "inactive") return false;
  if (f === "overdue") return s.overdue > 0;
  if (f === "restricted") return !!s.restriction;
  if (f === "good") return s.contracts > 0 && !s.overdue && !s.restriction;
  return s.contracts === 0;
}
/** Search over customer name, billing name, ID and property names (Figma 02-1). */
export const customerMatch = (r: CustomerRow, q: string) => !q.trim() || [r.name, r.billingName, r.id, ...r.propertyNames].some((x) => x.toLowerCase().includes(q.trim().toLowerCase()));
/** KPI tiles of the register: inactive customers and their assets stay out of the counts (IR40); archived assets never appear (IR39). */
export function registerKpis(rows: CustomerRow[], us: ApiUnitRow[]) {
  const active = rows.filter((r) => r.status === "active");
  const orgs = new Set(active.map((r) => r.orgId));
  const units = us.filter((u) => orgs.has(u.customerOrgId));
  const n = (s: PowerState) => units.filter((u) => u.effectivePowerState === s).length;
  const names = active.flatMap((r) => r.propertyNames);
  return {
    customers: active.length, inactive: rows.length - active.length, properties: names.length,
    propertyNames: names.length > 3 ? `${names.slice(0, 3).join(" · ")} +${names.length - 3}` : names.join(" · ") || "none yet",
    units: units.length, power: `Running ${n("on")} · Stopped ${n("off")} · unknown ${n("unknown")}`,
    attention: active.filter((r) => r.alerts > 0 || (r.standing?.overdue ?? 0) > 0 || !!r.standing?.restriction).length,
  };
}

/** The customer form (Figma 02 “New customer”: organizations.save, then customers.save). */
export type CustomerDraft = { billingName: string; name: string; profile: Profile; status: "active" | "inactive" };
export const customerDraft = (r?: CustomerRow): CustomerDraft => ({ billingName: r?.billingName ?? "", name: r?.name ?? "", profile: r?.profile ?? "general", status: r?.status ?? "active" });
export const profileLabel: Record<Profile, string> = { rto: "Rent-to-own (RTO)", general: "General care", energy: "Energy", environment: "Environment" };
export function customerErrors(d: CustomerDraft): Record<string, string> {
  const e: Record<string, string> = {};
  if (!between(d.billingName, 1, 120)) e.billingName = "1–120 characters";
  if (!between(d.name, 1, 120)) e.name = "1–120 characters";
  return e;
}
const between = (s: string, min: number, max: number) => s.trim().length >= min && s.trim().length <= max;

// ---- locations ----

export type TreeSpace = { id: string; version: number; kind: ApiSpaceRow["kind"]; name: string; propertyId: string; parentSpaceId: string | null; direct: number; units: number; children: TreeSpace[] };
export type TreeProperty = {
  id: string; version: number; kind: "home" | "office"; name: string; address: string | null; accessInstructions: string | null; updatedAt: string;
  units: number; unassigned: number; floors: number; rooms: number; spaces: TreeSpace[];
};
export const flatten = (xs: TreeSpace[]): TreeSpace[] => xs.flatMap((x) => [x, ...flatten(x.children)]);
/** The customer's properties with their space hierarchy; unit counts include descendants (Figma 02-2). */
export function locationTree(ps: ApiPropertyRow[], ss: ApiSpaceRow[], us: ApiUnitRow[], orgId: string): TreeProperty[] {
  const live = ss.filter((s) => !s.archived);
  const build = (propertyId: string, parent: string | null): TreeSpace[] =>
    live.filter((s) => s.propertyId === propertyId && s.parentSpaceId === parent).sort(byName).map((s) => {
      const children = build(propertyId, s.id);
      const direct = us.filter((u) => u.spaceId === s.id).length;
      return { id: s.id, version: s.version, kind: s.kind, name: s.name, propertyId, parentSpaceId: parent, direct, units: direct + children.reduce((n, c) => n + c.units, 0), children };
    });
  return ps.filter((p) => p.customerOrgId === orgId && !p.archived).sort(byName).map((p) => {
    const spaces = build(p.id, null);
    const all = flatten(spaces);
    return {
      id: p.id, version: p.version, kind: p.kind, name: p.name, address: p.address, accessInstructions: p.accessInstructions, updatedAt: p.updatedAt,
      units: us.filter((u) => u.propertyId === p.id).length, unassigned: us.filter((u) => u.propertyId === p.id && u.spaceId === null).length,
      floors: all.filter((s) => s.kind === "floor").length, rooms: all.filter((s) => s.kind === "room").length, spaces,
    };
  });
}
/** The selected location of the URL (locationId = property, space or "unassigned:<propertyId>"); the first property by default. */
export type Selection =
  | { kind: "property"; property: TreeProperty }
  | { kind: "space"; property: TreeProperty; space: TreeSpace; path: string }
  | { kind: "unassigned"; property: TreeProperty };
export function selection(tree: TreeProperty[], locationId: string | undefined, ss: ApiSpaceRow[]): Selection | null {
  if (locationId?.startsWith("unassigned:")) {
    const p = tree.find((x) => x.id === locationId.slice(11));
    if (p) return { kind: "unassigned", property: p };
  }
  for (const p of tree) {
    if (p.id === locationId) return { kind: "property", property: p };
    const s = flatten(p.spaces).find((x) => x.id === locationId);
    if (s) return { kind: "space", property: p, space: s, path: [p.name, spacePath(s.id, ss)].filter(Boolean).join(" › ") };
  }
  return tree[0] ? { kind: "property", property: tree[0] } : null;
}
/** The space path under the property ("1F › Bedroom"); "" for none. */
export function spacePath(spaceId: string | null, ss: ApiSpaceRow[]): string {
  const names: string[] = [];
  let cur = ss.find((s) => s.id === spaceId);
  while (cur) {
    names.unshift(cur.name);
    const parent = cur.parentSpaceId;
    cur = parent ? ss.find((s) => s.id === parent) : undefined;
  }
  return names.join(" › ");
}
/** Units at the selection: a property lists all of its units, a space includes its descendants (includeDescendants). */
export function unitsAt(sel: Selection, us: ApiUnitRow[]): ApiUnitRow[] {
  if (sel.kind === "unassigned") return us.filter((u) => u.propertyId === sel.property.id && u.spaceId === null);
  if (sel.kind === "property") return us.filter((u) => u.propertyId === sel.property.id);
  const ids = new Set(flatten([sel.space]).map((s) => s.id));
  return us.filter((u) => u.spaceId !== null && ids.has(u.spaceId));
}
/** Every place a unit of the customer can sit: a property (no space, IR62) or one of its spaces. */
export function placeOptions(tree: TreeProperty[], ss: ApiSpaceRow[]) {
  return tree.flatMap((p) => [
    { propertyId: p.id, spaceId: "", label: `${p.name} (no space)` },
    ...flatten(p.spaces).map((s) => ({ propertyId: p.id, spaceId: s.id, label: `${p.name} › ${spacePath(s.id, ss)}` })),
  ]);
}

export type PropertyDraft = { kind: "home" | "office"; name: string; address: string; accessInstructions: string };
export const propertyDraft = (p?: TreeProperty): PropertyDraft => ({ kind: p?.kind ?? "home", name: p?.name ?? "", address: p?.address ?? "", accessInstructions: p?.accessInstructions ?? "" });
export function propertyErrors(d: PropertyDraft): Record<string, string> {
  const e: Record<string, string> = {};
  if (!between(d.name, 1, 120)) e.name = "1–120 characters";
  if (d.address.trim().length > 500) e.address = "At most 500 characters";
  if (d.accessInstructions.trim().length > 1000) e.accessInstructions = "At most 1000 characters";
  return e;
}
export const propertyInput = (d: PropertyDraft, orgId: string, id?: string) => ({
  ...(id ? { id } : {}), customerOrgId: orgId, kind: d.kind, name: d.name.trim(), address: d.address.trim() || null, accessInstructions: d.accessInstructions.trim() || null,
});
export const nameError = (name: string) => (between(name, 1, 120) ? undefined : "1–120 characters");
export const reasonError = (reason: string) => (between(reason, 1, 1000) ? undefined : "A reason is required (1–1000 characters)");

// ---- units ----

export type UnitRow = { id: string; version: number; name: string; model: string; location: string; power: "running" | "stopped" | "unknown"; conn: Connection; alerts: number; policies: string };
const power = { on: "running", off: "stopped", unknown: "unknown" } as const;
/** The model as the table shows it ("ventilation-demo v3": capability model and the unit's capability version). */
export const modelLabel = (u: Pick<ApiUnitRow, "modelId" | "capabilityVersion">, models: Map<string, string>) => `${models.get(u.modelId) ?? u.modelId.slice(0, 8)} v${u.capabilityVersion}`;
export function unitRows(us: ApiUnitRow[], ss: ApiSpaceRow[], models: Map<string, string>): UnitRow[] {
  return [...us].sort((a, b) => a.displayName.localeCompare(b.displayName)).map((u) => ({
    id: u.id, version: u.version, name: u.displayName, model: modelLabel(u, models), location: spacePath(u.spaceId, ss) || "Unassigned",
    power: power[u.effectivePowerState], conn: u.connection, alerts: u.activeAlertCount, policies: u.alertPolicyIds.length ? `Default + ${u.alertPolicyIds.length}` : "Default",
  }));
}
export const powerStates: PowerState[] = ["on", "off", "unknown"];
export const connections: Connection[] = ["online", "offline", "unknown", "connecting", "error"];
/** units.list filters of the URL (powerState, connections, search) applied to the selection's units. */
export function filterUnits(us: ApiUnitRow[], f: { powerState?: PowerState; connections: Connection[]; search: string }): ApiUnitRow[] {
  const q = f.search.trim().toLowerCase();
  return us.filter((u) => (!f.powerState || u.effectivePowerState === f.powerState) && (!f.connections.length || f.connections.includes(u.connection))
    && (!q || u.displayName.toLowerCase().includes(q) || u.id.toLowerCase().includes(q)));
}

/** The unit form (DD-A02 fields). installedAt is the Kuala Lumpur date. */
export type UnitDraft = { displayName: string; propertyId: string; spaceId: string; modelId: string; installedAt: string; serviceScope: Scope[] };
export const unitDraft = (u?: Pick<ApiUnitRow, "displayName" | "propertyId" | "spaceId" | "modelId" | "installedAt" | "serviceScope">, propertyId = "", spaceId = ""): UnitDraft => ({
  displayName: u?.displayName ?? "", propertyId: u?.propertyId ?? propertyId, spaceId: u?.spaceId ?? (u ? "" : spaceId), modelId: u?.modelId ?? "",
  installedAt: u?.installedAt ? klStamp(u.installedAt).slice(0, 10) : "", serviceScope: u?.serviceScope ?? ["indoor", "outdoor"],
});
export function unitErrors(d: UnitDraft, todayKL: string): Record<string, string> {
  const e: Record<string, string> = {};
  if (!between(d.displayName, 1, 120)) e.displayName = "1–120 characters";
  if (!d.propertyId) e.propertyId = "Choose a location";
  if (!d.modelId) e.modelId = "Choose a model";
  if (d.installedAt && d.installedAt > todayKL) e.installedAt = "An installation date cannot be in the future (IR44)";
  if (d.serviceScope.length === 0) e.serviceScope = "Choose at least one inspection group";
  return e;
}
export const unitInput = (d: UnitDraft, orgId: string, id?: string, changeReason?: string) => ({
  ...(id ? { id } : {}), customerOrgId: orgId, propertyId: d.propertyId, spaceId: d.spaceId || null, displayName: d.displayName.trim(), modelId: d.modelId, type: "split" as const,
  installedAt: d.installedAt ? `${d.installedAt}T00:00:00+08:00` : null, serviceScope: d.serviceScope, ...(changeReason ? { changeReason } : {}),
});
/** A relocation (another property or space) needs a change reason (DD-A02 step 3). */
export const relocates = (u: Pick<ApiUnitRow, "propertyId" | "spaceId">, d: UnitDraft) => u.propertyId !== d.propertyId || (u.spaceId ?? "") !== d.spaceId;
/** The fields of the draft that differ from the saved unit ("1 unsaved change · Name"). */
export function changedFields(u: ApiUnitRow, d: UnitDraft): string[] {
  const saved = unitDraft(u);
  const out: string[] = [];
  if (saved.displayName !== d.displayName.trim()) out.push("Name");
  if (relocates(u, d)) out.push("Location");
  if (saved.modelId !== d.modelId) out.push("Model");
  if (saved.installedAt !== d.installedAt) out.push("Installed at");
  if ([...saved.serviceScope].sort().join() !== [...d.serviceScope].sort().join()) out.push("Service scope");
  return out;
}

// ---- alert policies of the customer (IR108: the unit carries the policies) ----

/** Policy of service-contracts.ts (alert and default_alert kinds as policies.list returns them for one customer). */
export type ApiCustomerPolicy = {
  id: string; version: number; kind: "alert" | "default_alert" | "automation"; name: string; unitIds: string[]; enabled: boolean; ownerMembershipId: string;
  customerId?: string | null; metric?: string; operator?: Operator; threshold?: number; durationSeconds?: number; severity?: Severity;
  activeWindow?: { weekdays: number[]; startLocal: string; endLocal: string } | null;
  rules?: { ruleKey: string; name: string; metric: string; operator: Operator; threshold: number; durationSeconds: number; category: string }[];
  ruleSettings?: { ruleKey: string; customerId: string; enabled: boolean; version: number; reason: string | null; updatedAt: string }[];
};
const sevLabel: Record<Severity, string> = { critical: "Critical", warning: "Warning", normal: "Info" };
const duration = (s: number) => (s % 3600 === 0 && s >= 3600 ? `${s / 3600} h` : s % 60 === 0 && s >= 120 ? `${s / 60} min` : `${s} s`);
const days = (w: number[]) => {
  const d = [...w].sort((a, b) => a - b).join();
  return w.length === 7 ? "" : d === "1,2,3,4,5" ? "weekdays " : d === "6,7" ? "weekends " : `${[...w].sort((a, b) => a - b).map((x) => ["Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"][x - 1]).join(", ")} `;
};
/** "Temperature ≥ 30 °C for 60 s, weekdays 08:00–19:00 → Warning". */
export function conditionText(p: { metric?: string; operator?: Operator; threshold?: number; durationSeconds?: number; activeWindow?: ApiCustomerPolicy["activeWindow"]; severity?: Severity }): string {
  const m = p.metric ?? "";
  const w = p.activeWindow ? `, ${days(p.activeWindow.weekdays)}${p.activeWindow.startLocal}–${p.activeWindow.endLocal}` : "";
  const cond = m === "heartbeat_gap" ? `No heartbeat for ${p.threshold ?? 0} min${w}` // the gap itself is the duration (minutes)
    : `${metricLabel[m] ?? m} ${opSymbol[p.operator ?? "gte"]} ${p.threshold ?? 0} ${metricUnit[m] ?? ""} for ${duration(p.durationSeconds ?? 0)}${w}`;
  return p.severity ? `${cond} → ${sevLabel[p.severity]}` : cond;
}
export type PolicyLine = { id: string; version: number; kind: "alert" | "default_alert"; name: string; type: string; condition: string; madeBy: string; units: number; enabled: boolean };
export type RuleLine = { ruleKey: string; name: string; condition: string; enabled: boolean; version: number; note: string | null };
/** The customer's policies (default first) with the units of this customer that carry each one. */
export function policyLines(ps: ApiCustomerPolicy[], customerUnits: string[], owner: (membershipId: string) => string): PolicyLine[] {
  const mine = new Set(customerUnits);
  return ps.filter((p): p is ApiCustomerPolicy & { kind: "alert" | "default_alert" } => p.kind === "alert" || p.kind === "default_alert")
    .sort((a, b) => (a.kind === b.kind ? a.name.localeCompare(b.name) : a.kind === "default_alert" ? -1 : 1))
    .map((p) => p.kind === "default_alert"
      ? { id: p.id, version: p.version, kind: p.kind, name: p.name, type: "Default", condition: `${p.rules?.length ?? 0} rules · ${defaultRules(p).filter((r) => r.enabled).length} on for this customer`, madeBy: "HQ (template)", units: mine.size, enabled: p.enabled }
      : { id: p.id, version: p.version, kind: p.kind, name: p.name, type: metricLabel[p.metric ?? ""] ?? p.metric ?? "", condition: conditionText(p), madeBy: owner(p.ownerMembershipId), units: p.unitIds.filter((u) => mine.has(u)).length, enabled: p.enabled });
}
/** The default policy's rules with this customer's on/off settings (no setting = on). */
export function defaultRules(p: ApiCustomerPolicy): RuleLine[] {
  return (p.rules ?? []).map((r) => {
    const s = p.ruleSettings?.find((x) => x.ruleKey === r.ruleKey);
    return { ruleKey: r.ruleKey, name: r.name, condition: conditionText(r), enabled: s?.enabled ?? true, version: s?.version ?? 0, note: s ? `${s.enabled ? "On" : "Off"} since ${klStamp(s.updatedAt).slice(0, 10)}${s.reason ? ` · ${s.reason}` : ""}` : null };
  });
}
