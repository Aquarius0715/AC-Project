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
  serviceScope: Scope[]; alertPolicyIds: string[]; archived: boolean; capabilityVersion: number; connection: Connection; lastSeenAt: string | null; warrantyEndsAt: string | null;
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

/** The unit form (DD-A02 fields, warranty end IR209). installedAt and warrantyEnd are Kuala Lumpur dates. */
export type UnitDraft = { displayName: string; propertyId: string; spaceId: string; modelId: string; installedAt: string; warrantyEnd: string; serviceScope: Scope[] };
const klDate = (iso: string | null | undefined) => (iso ? klStamp(iso).slice(0, 10) : "");
export const unitDraft = (u?: Pick<ApiUnitRow, "displayName" | "propertyId" | "spaceId" | "modelId" | "installedAt" | "warrantyEndsAt" | "serviceScope">, propertyId = "", spaceId = ""): UnitDraft => ({
  displayName: u?.displayName ?? "", propertyId: u?.propertyId ?? propertyId, spaceId: u?.spaceId ?? (u ? "" : spaceId), modelId: u?.modelId ?? "",
  installedAt: klDate(u?.installedAt), warrantyEnd: klDate(u?.warrantyEndsAt), serviceScope: u?.serviceScope ?? ["indoor", "outdoor"],
});
export function unitErrors(d: UnitDraft, todayKL: string): Record<string, string> {
  const e: Record<string, string> = {};
  if (!between(d.displayName, 1, 120)) e.displayName = "1–120 characters";
  if (!d.propertyId) e.propertyId = "Choose a location";
  if (!d.modelId) e.modelId = "Choose a model";
  if (d.installedAt && d.installedAt > todayKL) e.installedAt = "An installation date cannot be in the future (IR44)";
  if (d.warrantyEnd && d.installedAt && d.warrantyEnd < d.installedAt) e.warrantyEnd = "A warranty cannot end before the installation";
  if (d.serviceScope.length === 0) e.serviceScope = "Choose at least one inspection group";
  return e;
}
export const unitInput = (d: UnitDraft, orgId: string, id?: string, changeReason?: string) => ({
  ...(id ? { id } : {}), customerOrgId: orgId, propertyId: d.propertyId, spaceId: d.spaceId || null, displayName: d.displayName.trim(), modelId: d.modelId, type: "split" as const,
  installedAt: d.installedAt ? `${d.installedAt}T00:00:00+08:00` : null, warrantyEndsAt: d.warrantyEnd ? `${d.warrantyEnd}T00:00:00+08:00` : null,
  serviceScope: d.serviceScope, ...(changeReason ? { changeReason } : {}),
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
  if (saved.warrantyEnd !== d.warrantyEnd) out.push("Warranty end");
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

// ---- client users of the customer (FR-A17, IR144) ----

export type Channel = "inApp" | "email" | "whatsapp";
/** ClientUser of service-contracts.ts. */
export type ApiClientUser = {
  id: string; version: number; customerId: string; membershipId: string | null; email: string; displayName: string | null; clientRole: "owner" | "member";
  status: "invited" | "active" | "disabled"; lastSignInAt: string | null; allowedChannels: Channel[]; invitedAt: string; invitedByMembershipId: string;
};
export type ClientUserRow = {
  id: string; version: number; email: string; name: string | null; role: "owner" | "member"; status: ApiClientUser["status"]; sub: string; lastSignIn: string; channels: string;
  lastOwner: boolean; you: boolean;
};
const channelLabel: Record<Channel, string> = { inApp: "In-app", email: "Email", whatsapp: "WhatsApp" };
/** Rows by name; the last active owner is marked because it cannot be demoted, disabled or removed (CONFLICT). */
export function clientUserRows(us: ApiClientUser[], who: (membershipId: string) => string, self?: string): ClientUserRow[] {
  const owners = us.filter((u) => u.clientRole === "owner" && u.status === "active").length;
  return [...us].sort((a, b) => (a.displayName ?? a.email).localeCompare(b.displayName ?? b.email)).map((u) => ({
    id: u.id, version: u.version, email: u.email, name: u.displayName, role: u.clientRole, status: u.status,
    sub: u.displayName ? u.email : `invited ${klDate(u.invitedAt)} by ${who(u.invitedByMembershipId)}`, lastSignIn: u.lastSignInAt ? klStamp(u.lastSignInAt) : "—",
    channels: u.status === "active" && u.allowedChannels.length ? u.allowedChannels.map((c) => channelLabel[c]).join(" · ") : "—",
    lastOwner: u.clientRole === "owner" && u.status === "active" && owners === 1, you: !!self && u.membershipId === self,
  }));
}
/** A plain address up to 254 characters, not yet a user of this customer (case-insensitive). */
export function inviteError(email: string, us: { email: string }[]): string | undefined {
  const e = email.trim();
  if (e.length > 254 || !/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(e)) return "A valid email address (up to 254 characters)";
  if (us.some((u) => u.email.toLowerCase() === e.toLowerCase())) return "Already a user of this customer";
  return undefined;
}

// ---- warranty & coverage (FR-A19) ----

export type CoverageStatus = "under_warranty" | "contract" | "expiring" | "no_coverage";
/** UnitCoverage of service-contracts.ts. */
export type ApiCoverage = { unitId: string; customerId: string; modelId: string; warrantyEndsAt: string | null; contractIds: string[]; status: CoverageStatus; claimableJobIds: string[] };
export type CoverageRow = {
  unitId: string; unit: string; sub: string; customerId: string; customer: string; model: string; ends: string; endsAt: string | null; days: number | null;
  contractIds: string[]; contracts: string; status: CoverageStatus; statusText: string;
};
const dayMs = 86_400_000;
export function coverageRows(cs: ApiCoverage[], x: { unit: (id: string) => { name: string; place: string } | undefined; customer: (id: string) => string; model: (id: string) => string; contract: (id: string) => string }, now: Date): CoverageRow[] {
  return cs.map((c) => {
    const u = x.unit(c.unitId);
    const days = c.warrantyEndsAt ? Math.ceil((Date.parse(c.warrantyEndsAt) - now.getTime()) / dayMs) : null;
    const statusText = c.status === "contract" ? "Covered by contract" : c.status === "under_warranty" ? "Under warranty"
      : c.status === "expiring" ? `Ends in ${days} d · no contract` : c.warrantyEndsAt ? "Warranty ended · no contract" : "No warranty · no contract";
    return {
      unitId: c.unitId, unit: u?.name ?? c.unitId.slice(0, 8), sub: [c.unitId.slice(0, 8), u?.place].filter(Boolean).join(" · "), customerId: c.customerId, customer: x.customer(c.customerId),
      model: x.model(c.modelId), ends: klDate(c.warrantyEndsAt) || "—", endsAt: c.warrantyEndsAt, days, contractIds: c.contractIds, contracts: c.contractIds.map(x.contract).join(", ") || "—",
      status: c.status, statusText,
    };
  }).sort((a, b) => (a.endsAt === b.endsAt ? a.unit.localeCompare(b.unit) : a.endsAt === null ? 1 : b.endsAt === null ? -1 : a.endsAt.localeCompare(b.endsAt)));
}
/** KPI tiles (Figma 02 warranty): under warranty (any unit whose warranty still runs), ending within 90 / 30 days, no coverage, contracts. */
export function coverageKpis(rows: CoverageRow[]) {
  const running = rows.filter((r) => r.days !== null && r.days > 0);
  const contracts = [...new Set(rows.flatMap((r) => (r.status === "contract" ? r.contracts.split(", ") : [])))];
  return {
    total: rows.length, underWarranty: running.length, within90: running.filter((r) => r.days! <= 90).length, within30: running.filter((r) => r.days! <= 30).length,
    noCoverage: rows.filter((r) => r.status === "no_coverage").length, contract: rows.filter((r) => r.status === "contract").length,
    contractNames: contracts.length > 2 ? `${contracts.slice(0, 2).join(", ")} …` : contracts.join(", ") || "none",
  };
}
export type CoverageFilter = { customerId: string; coverage: "" | CoverageStatus; within: "" | "30" | "90" | "180" | "365" };
export const coverageMatch = (r: CoverageRow, f: CoverageFilter) => (!f.customerId || r.customerId === f.customerId) && (!f.coverage || r.status === f.coverage)
  && (!f.within || (r.days !== null && r.days > 0 && r.days <= Number(f.within)));
const csvCell = (v: string) => (/[",\n]/.test(v) ? `"${v.replace(/"/g, '""')}"` : v);
export const toCsv = (header: string[], rows: string[][]) => [header, ...rows].map((r) => r.map(csvCell).join(",")).join("\n") + "\n";
export const coverageCsv = (rows: CoverageRow[]) => toCsv(["unit", "unit_id", "customer", "model", "warranty_end", "maintenance_contract", "status"],
  rows.map((r) => [r.unit, r.unitId, r.customer, r.model, r.ends === "—" ? "" : r.ends, r.contracts === "—" ? "" : r.contracts, r.statusText]));

/** A job completed under warranty whose accepted report lists replaced parts and has no filed claim (IR209). */
export type ClaimCandidate = { jobId: string; version: number; unit: string; type: string; completed: string; parts: string; partLabel: string; amountMinor: number | null; currency: string };
type Part = { name: string; quantity: number; catalogCode: string | null };
type Cost = { kind: "estimate" | "actual"; amountMinor: number; currency: string };
export function claimCandidate(j: { id: string; version: number; type: string; completedAt: string | null; costs: Cost[] }, parts: Part[], unit: string): ClaimCandidate {
  const label = parts.map((p) => `${p.name}${p.catalogCode ? ` ${p.catalogCode}` : ""} ×${p.quantity}`).join(", ");
  const actual = j.costs.filter((c) => c.kind === "actual");
  return {
    jobId: j.id, version: j.version, unit, type: j.type, completed: j.completedAt ? klStamp(j.completedAt).slice(0, 10) : "—", parts: label || "parts listed in the report",
    partLabel: label.slice(0, 120), amountMinor: actual.length ? actual.reduce((n, c) => n + c.amountMinor, 0) : null, currency: actual[0]?.currency ?? "MYR",
  };
}

// ---- CSV import (FR-A18) ----

export const importFields = [
  { key: "property", label: "Property name", required: true, aliases: ["property", "propertyname", "site"] },
  { key: "floor", label: "Floor", required: false, aliases: ["floor"] },
  { key: "room", label: "Room", required: false, aliases: ["room", "space"] },
  { key: "unit_name", label: "Unit name", required: true, aliases: ["unitname", "unit", "name"] },
  { key: "model_code", label: "Model (model register)", required: true, aliases: ["modelcode", "model"] },
  { key: "serial", label: "Device serial (binds IoT)", required: false, aliases: ["serial", "deviceserial"] },
  { key: "installed_on", label: "Install date (YYYY-MM-DD)", required: false, aliases: ["installedon", "installed", "installdate"] },
  { key: "warranty_end", label: "Warranty end (YYYY-MM-DD)", required: false, aliases: ["warrantyend", "warranty"] },
] as const;
export const importTemplate = toCsv(importFields.map((f) => f.key), [["Office A", "2F", "Meeting room 2", "Meeting room AC #2", "SPL-100", "", "2026-01-10", "2027-01-10"]]);
/** The header cells and the number of records of a CSV text (RFC 4180 quotes; a BOM is ignored). */
export function csvShape(text: string): { header: string[]; rows: number } {
  const records: string[][] = [[]];
  let cell = "", quoted = false;
  const t = text.replace(/^\ufeff/, "");
  for (let i = 0; i < t.length; i++) {
    const ch = t[i];
    if (quoted) {
      if (ch === '"' && t[i + 1] === '"') { cell += '"'; i++; } else if (ch === '"') quoted = false; else cell += ch;
    } else if (ch === '"') quoted = true;
    else if (ch === ",") { records.at(-1)!.push(cell); cell = ""; }
    else if (ch === "\n" || ch === "\r") {
      if (ch === "\r" && t[i + 1] === "\n") i++;
      records.at(-1)!.push(cell); cell = ""; records.push([]);
    } else cell += ch;
  }
  records.at(-1)!.push(cell);
  const full = records.filter((r) => r.some((c) => c.trim() !== ""));
  return { header: (full[0] ?? []).map((h) => h.trim()), rows: Math.max(0, full.length - 1) };
}
const norm = (s: string) => s.toLowerCase().replace(/[^a-z0-9]/g, "");
/** Matches CSV columns to the import fields by name ("Unit name", "unit_name" and "unit" all map to unit_name). */
export function autoMapping(header: string[]): Record<string, string> {
  const out: Record<string, string> = {};
  for (const f of importFields) {
    const hit = header.find((h) => (f.aliases as readonly string[]).includes(norm(h)) && !Object.values(out).includes(h));
    if (hit) out[f.key] = hit;
  }
  return out;
}
/** ImportPreview / UnitImport of service-contracts.ts. */
export type ApiImportRow = {
  rowNumber: number; propertyName: string; floorName: string | null; roomName: string | null; unitName: string; modelCode: string; serial: string | null;
  installedOn: string | null; warrantyEnd: string | null; result: "ready" | "warning" | "error"; messageKey: string | null;
};
export type ApiImportPreview = { previewId: string; customerId: string; fileName: string; rows: ApiImportRow[]; readyCount: number; warningCount: number; errorCount: number; expiresAt: string };
export type ApiUnitImport = { id: string; version: number; customerId: string; createdPropertyIds: string[]; createdSpaceIds: string[]; createdUnitIds: string[]; skippedRowNumbers: number[]; undoUntil: string; state: "imported" | "undone" };
export const importPlace = (r: ApiImportRow) => [r.propertyName, r.floorName, r.roomName].filter(Boolean).join(" › ");
/** The row result in words (the message keys of units.importPreview). */
export function importMessage(r: ApiImportRow): string {
  const place = r.roomName ?? r.floorName ?? "";
  switch (r.messageKey) {
    case null: return "Ready";
    case "warning.importCreatesProperty": return `Property “${r.propertyName}” does not exist — it will be created (office; add its address afterwards).`;
    case "warning.importCreatesSpace": return `“${place}” does not exist — it will be created.`;
    case "error.unknownModel": return `Model ${r.modelCode} is not in the model register (A04).`;
    case "error.serialBound": return `Serial ${r.serial} is already bound to another unit.`;
    case "error.unknownSerial": return `Serial ${r.serial} is not a registered device.`;
    case "error.duplicateSerial": return `Serial ${r.serial} appears more than once in this file.`;
    case "error.tamperUnresolved": return `Device ${r.serial} has an unresolved tamper.`;
    case "error.duplicateSiblingName": return `A unit named “${r.unitName}” already exists there.`;
    case "error.future": return "The installation date is in the future.";
    case "error.importInvalidDate": return "A date is not a valid YYYY-MM-DD date.";
    case "error.importMissingField": return "Property, unit name and model are required.";
    case "error.length": return "The unit name is longer than 120 characters.";
    default: return r.messageKey;
  }
}
export const errorReportCsv = (rows: ApiImportRow[]) => toCsv(["row", "property", "floor", "room", "unit_name", "model_code", "serial", "result", "message"],
  rows.filter((r) => r.result !== "ready").map((r) => [String(r.rowNumber), r.propertyName, r.floorName ?? "", r.roomName ?? "", r.unitName, r.modelCode, r.serial ?? "", r.result, importMessage(r)]));

