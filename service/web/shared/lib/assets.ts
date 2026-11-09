// HQ customers & units (FR-A02, DATA_SOURCE=API): customers with their contract standing, the property / space tree,
// unit rows, the unit form and the customer's alert policies projected for /admin/units. Pure code shared by the
// Server Component and the client view; Vitest covers it. Texts in the display language (`i` / `t`, IR293); instants
// in the user's display time zone, while dates that are Kuala Lumpur days (installation, warranty end, contract end)
// stay so (IR44).
import type { Currency } from "@ac/web/lib/contracts.gen";
import { EN, intlTag, showDate, showTime, translator, type I18n, type Locale, type T, textOf } from "@ac/web/lib/i18n";
import { businessDay } from "@ac/web/lib/clientBilling";
import { klStamp, one } from "@ac/web/lib/energy";
import { moneyByCurrency } from "@ac/web/lib/money";
import { metricLabel, metricUnit, opSymbol, type Operator, type Severity } from "@ac/web/lib/adminAlerts";
import { csvCell } from "@ac/web/lib/csv";
import type { IconName } from "@ac/web/components/Icon";
import { byText, isAfter, isBefore } from "@ac/web/lib/order";
import { codePoints, isEmail, wrapped } from "@ac/web/lib/text";
import { entryOf, lookup } from "@ac/web/lib/lookup";

const TEXT_1_120_CHARACTERS = "1–120 characters";
const DEFAULT_ALERT = "default_alert";

export const profiles = ["rto", "general", "energy", "environment"] as const;
export type Profile = (typeof profiles)[number];
export type Connection = "online" | "offline" | "unknown" | "connecting" | "error";
export type PowerState = "on" | "off" | "unknown";
export type Scope = "indoor" | "outdoor" | "electrical";

export interface ApiOrgRow { id: string; version: number; name: string; kind: "customer" | "contractor" | "operator"; status: "active" | "inactive" }
export interface ApiCustomerRow { id: string; version: number; name: string; status: "active" | "inactive"; organizationId: string; serviceProfile: Profile; createdAt: string }
export interface ApiPropertyRow {
  id: string; version: number; customerOrgId: string; kind: "home" | "office"; name: string; address: string | null; accessInstructions: string | null; archived: boolean; updatedAt: string;
}
export interface ApiSpaceRow { id: string; version: number; propertyId: string; parentSpaceId: string | null; kind: "area" | "floor" | "room" | "space"; name: string; archived: boolean }
/** UnitSummary of service-contracts.ts (fields shown on this screen). */
export interface ApiUnitRow {
  id: string; version: number; customerOrgId: string; propertyId: string; spaceId: string | null; displayName: string; modelId: string; type: "split"; installedAt: string | null;
  serviceScope: Scope[]; alertPolicyIds: string[]; archived: boolean; capabilityVersion: number; connection: Connection; lastSeenAt: string | null; warrantyEndsAt: string | null;
  effectivePowerState: PowerState; activeAlertCount: number;
}
/** UnitDetail of service-contracts.ts (fields shown on this screen). */
export type ApiUnitDetail = ApiUnitRow & { location: { pathLabels: string[]; address: string | null; accessInstructions: string | null }; pendingCommandIds: string[] };
export interface ApiContractLite { id: string; customerId: string; unitIds: string[]; planType: Profile; startAt: string; endAt: string; activeRestrictionIds: string[] }
export interface ApiInvoiceLite { id: string; contractId: string; amountMinor: number; currency: Currency }
export interface ApiRestrictionLite { id: string; contractId?: string; state: string; unitIds: string[] }

const byName = <Item extends { name: string }>(a: Item, b: Item) => a.name.localeCompare(b.name);
const en = translator("en");
/** “Apr 2026”: the month an account was created, in the user's language (a Kuala Lumpur month). */
const monthYear = (iso: string, locale: Locale) => new Date(iso).toLocaleDateString(intlTag(locale), { month: "short", year: "numeric", timeZone: "Asia/Kuala_Lumpur" }).replaceAll(/[\u{A0}\u{2009}\u{202F}]/gu, " ");

// ---- customers ----

/** The contract side of a customer: current contracts, overdue invoices and the most pressing active restriction. Without
restriction.read (`restrictions` null) the contracts' activeRestrictionIds still tell that one is active, its state
unknown ("active", IR319). */
export interface Standing { plans: Profile[]; contracts: number; contractUnits: number; overdue: number; overdueAmount: string | null; restriction: { id: string; state: string; contractId: string } | null }
const restrictionRank: Record<string, number> = { applied: 0, requested: 1, release_requested: 2, scheduled: 3 };
/** Without restriction.read a contract's activeRestrictionIds still tell that one is active (state unknown, IR319). */
function unknownRestriction(mine: ApiContractLite[], restrictions: ApiRestrictionLite[] | null): Standing["restriction"] {
  if (restrictions !== null) return null;
  const k = mine.find((x) => x.activeRestrictionIds.length > 0);
  const id = k?.activeRestrictionIds[0];
  return k && id ? { id, state: "active", contractId: k.id } : null;
}
export function standing(customerId: string, contracts: ApiContractLite[], overdue: ApiInvoiceLite[], restrictions: ApiRestrictionLite[] | null, now: Date): Standing {
  const t = now.getTime();
  const mine = contracts.filter((k) => k.customerId === customerId);
  const ids = new Set(mine.map((k) => k.id));
  const current = mine.filter((k) => Date.parse(k.startAt) <= t && t < Date.parse(k.endAt));
  const due = overdue.filter((i) => ids.has(i.contractId));
  const r = (restrictions ?? []).filter((x): x is ApiRestrictionLite & { contractId: string } => !!x.contractId && ids.has(x.contractId) && Object.hasOwn(restrictionRank, x.state)).toSorted((a, b) => (lookup(restrictionRank, a.state) ?? 9) - (lookup(restrictionRank, b.state) ?? 9))[0];
  return {
    plans: [...new Set(current.map((k) => k.planType))], contracts: current.length, contractUnits: new Set(current.flatMap((k) => k.unitIds)).size, overdue: due.length,
    overdueAmount: due.length > 0 ? moneyByCurrency(due) : null, // per currency, never added across them (IR707)
    restriction: r ? { id: r.id, state: r.state, contractId: r.contractId } : unknownRestriction(mine, restrictions),
  };
}
export interface Mark { label: string; tone: "crit" | "warn" | "ok" | "muted"; icon?: IconName }
const restrictionLabel: Record<string, string> = { applied: "Restriction applied", requested: "Restriction requested", release_requested: "Release requested", scheduled: "Restriction scheduled", active: "Restriction active" };
/** Contract column badges (Figma 02-1): Overdue / restriction state / Good standing / No contract / Inactive. */
export function standingMarks(status: "active" | "inactive", s: Standing | null, t: T = en): Mark[] {
  if (status === "inactive") return [{ label: t("Inactive"), tone: "muted" }];
  if (!s) return [];
  const out: Mark[] = [];
  if (s.overdue) out.push({ label: t("Overdue"), tone: "crit", icon: "alert" });
  if (s.restriction) out.push({ label: textOf(restrictionLabel, s.restriction.state, t) ?? s.restriction.state, tone: "warn" });
  if (out.length === 0) out.push(s.contracts ? { label: t("Good standing"), tone: "ok" } : { label: t("No contract"), tone: "muted" });
  return out;
}

export interface CustomerRow {
  id: string; version: number; name: string; billingName: string; status: "active" | "inactive"; orgId: string; orgVersion: number | null; orgStatus: "active" | "inactive"; profile: Profile; since: string;
  properties: number; propertyNames: string[]; units: number; operation: string; running: number; known: number; alerts: number; standing: Standing | null; marks: Mark[];
}
/** Operation rate = on ÷ (on + off) of the known power states; unknown stays out of the denominator (DD-A01). */
const operation = (us: ApiUnitRow[]) => {
  const on = us.filter((u) => u.effectivePowerState === "on").length;
  const known = us.filter((u) => u.effectivePowerState !== "unknown").length;
  return known ? `${one((on * 100) / known)}%` : "—";
};
export function customerRows(cs: ApiCustomerRow[], orgs: ApiOrgRow[], ps: ApiPropertyRow[], us: ApiUnitRow[], standingOf: (c: ApiCustomerRow) => Standing | null, i: I18n = EN): CustomerRow[] {
  const org = new Map(orgs.map((o) => [o.id, o]));
  return [...cs].toSorted(byName).map((c) => {
    const units = us.filter((u) => u.customerOrgId === c.organizationId);
    const props = ps.filter((p) => p.customerOrgId === c.organizationId).toSorted(byName);
    const s = standingOf(c);
    return {
      id: c.id, version: c.version, name: c.name, billingName: org.get(c.organizationId)?.name ?? "", status: c.status, orgId: c.organizationId, orgVersion: org.get(c.organizationId)?.version ?? null,
      orgStatus: org.get(c.organizationId)?.status ?? "active",
      profile: c.serviceProfile, since: monthYear(c.createdAt, i.display.locale), properties: props.length, propertyNames: props.map((p) => p.name), units: units.length,
      operation: operation(units), running: units.filter((u) => u.effectivePowerState === "on").length, known: units.filter((u) => u.effectivePowerState !== "unknown").length,
      alerts: units.reduce((n, u) => n + u.activeAlertCount, 0), standing: s, marks: standingMarks(c.status, s, i.t),
    };
  });
}
export const contractFilters = ["all", "overdue", "restricted", "good", "none"] as const;
export type ContractFilter = (typeof contractFilters)[number];
export function contractMatch(f: ContractFilter, r: CustomerRow): boolean {
  if (f === "all") return true;
  const s = r.standing;
  if (!s || r.status === "inactive") return false;
  if (f === "overdue") return s.overdue > 0;
  if (f === "restricted") return !!s.restriction;
  if (f === "good") return s.contracts > 0 && !s.overdue && !s.restriction;
  return s.contracts === 0;
}
/** Search over customer name, billing name, ID and property names (Figma 02-1). */
export const customerMatch = (r: CustomerRow, q: string) => !q.trim() || [r.name, r.billingName, r.id, ...r.propertyNames].some((x) => x.toLowerCase().includes(q.trim().toLowerCase()));
/** KPI tiles of the register: inactive customers and their assets stay out of the counts (IR40); archived assets never appear (IR39). */
export function registerKpis(rows: CustomerRow[], us: ApiUnitRow[], t: T = en) {
  const active = rows.filter((r) => r.status === "active");
  const orgs = new Set(active.map((r) => r.orgId));
  const units = us.filter((u) => orgs.has(u.customerOrgId));
  const n = (s: PowerState) => units.filter((u) => u.effectivePowerState === s).length;
  const names = active.flatMap((r) => r.propertyNames);
  return {
    customers: active.length, inactive: rows.length - active.length, properties: names.length,
    propertyNames: names.length > 3 ? `${names.slice(0, 3).join(" · ")} +${String(names.length - 3)}` : names.join(" · ") || t("none yet"),
    units: units.length, power: t("Running {on} · Stopped {off} · unknown {unknown}", { on: n("on"), off: n("off"), unknown: n("unknown") }),
    attention: active.filter((r) => r.alerts > 0 || (r.standing?.overdue ?? 0) > 0 || !!r.standing?.restriction).length,
  };
}

/** The customer form (Figma 02 “New customer”: organizations.save, then customers.save). */
export interface CustomerDraft { billingName: string; name: string; profile: Profile; status: "active" | "inactive" }
export const customerDraft = (r?: CustomerRow): CustomerDraft => ({ billingName: r?.billingName ?? "", name: r?.name ?? "", profile: r?.profile ?? "general", status: r?.status ?? "active" });
export const profileLabel: Record<Profile, string> = { rto: "Rent-to-own (RTO)", general: "General care", energy: "Energy", environment: "Environment" };
export function customerErrors(d: CustomerDraft, t: T = en): Record<string, string> {
  const e: Record<string, string> = {};
  if (!hasLength(d.billingName, 120)) e.billingName = t(TEXT_1_120_CHARACTERS);
  if (!hasLength(d.name, 120)) e.name = t(TEXT_1_120_CHARACTERS);
  return e;
}
const hasLength = (s: string, max: number) => codePoints(s.trim()) >= 1 && codePoints(s.trim()) <= max;

// ---- locations ----

export interface TreeSpace { id: string; version: number; kind: ApiSpaceRow["kind"]; name: string; propertyId: string; parentSpaceId: string | null; direct: number; units: number; children: TreeSpace[] }
export interface TreeProperty {
  id: string; version: number; kind: "home" | "office"; name: string; address: string | null; accessInstructions: string | null; updatedAt: string;
  units: number; unassigned: number; floors: number; rooms: number; spaces: TreeSpace[];
}
export const flatten = (xs: TreeSpace[]): TreeSpace[] => xs.flatMap((x) => [x, ...flatten(x.children)]);
/** The customer's properties with their space hierarchy; unit counts include descendants (Figma 02-2). */
export function locationTree(ps: ApiPropertyRow[], ss: ApiSpaceRow[], us: ApiUnitRow[], orgId: string): TreeProperty[] {
  const live = ss.filter((s) => !s.archived);
  const build = (propertyId: string, parent: string | null): TreeSpace[] => {
    return live.filter((s) => s.propertyId === propertyId && s.parentSpaceId === parent).toSorted(byName).map((s) => {
      const children = build(propertyId, s.id);
      const direct = us.filter((u) => u.spaceId === s.id).length;
      return { id: s.id, version: s.version, kind: s.kind, name: s.name, parentSpaceId: parent, units: direct + children.reduce((n, c) => n + c.units, 0), propertyId, direct, children };
    });
  };
  return ps.filter((p) => p.customerOrgId === orgId && !p.archived).toSorted(byName).map((p) => {
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
/** The tree's id of a selection: a property, a space, or a property's unassigned units ("" for none). */
export function selectionId(sel: Selection | null): string {
  if (!sel) return "";
  if (sel.kind === "property") return sel.property.id;
  return sel.kind === "space" ? sel.space.id : `unassigned:${sel.property.id}`;
}
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
  let names: string[] = [];
  let currentRow = ss.find((s) => s.id === spaceId);
  while (currentRow) {
    names = [currentRow.name, ...names];
    const parent = currentRow.parentSpaceId;
    currentRow = parent ? ss.find((s) => s.id === parent) : undefined;
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
export function placeOptions(tree: TreeProperty[], ss: ApiSpaceRow[], t: T = en) {
  return tree.flatMap((p) => {
    return [
      { propertyId: p.id, spaceId: "", label: t("{name} (no space)", { name: p.name }) },
      ...flatten(p.spaces).map((s) => ({ propertyId: p.id, spaceId: s.id, label: `${p.name} › ${spacePath(s.id, ss)}` })),
    ];
  });
}

export interface PropertyDraft { kind: "home" | "office"; name: string; address: string; accessInstructions: string }
export const propertyDraft = (p?: TreeProperty): PropertyDraft => ({ kind: p?.kind ?? "home", name: p?.name ?? "", address: p?.address ?? "", accessInstructions: p?.accessInstructions ?? "" });
export function propertyErrors(d: PropertyDraft, t: T = en): Record<string, string> {
  const e: Record<string, string> = {};
  if (!hasLength(d.name, 120)) e.name = t(TEXT_1_120_CHARACTERS);
  if (codePoints(d.address.trim()) > 500) e.address = t("At most 500 characters");
  if (codePoints(d.accessInstructions.trim()) > 1000) e.accessInstructions = t("At most 1000 characters");
  return e;
}
export const propertyInput = (d: PropertyDraft, orgId: string, id?: string) => {
  return {
    ...(id && { id }), customerOrgId: orgId, kind: d.kind, name: d.name.trim(), address: d.address.trim() || null, accessInstructions: d.accessInstructions.trim() || null,
  };
};
export const nameError = (name: string, t: T = translator("en")) => (hasLength(name, 120) ? undefined : t(TEXT_1_120_CHARACTERS));
export const reasonError = (reason: string, t: T = translator("en")) => (hasLength(reason, 1000) ? undefined : t("A reason is required (1–1000 characters)"));

// ---- units ----

export interface UnitRow { id: string; version: number; name: string; model: string; location: string; power: "running" | "stopped" | "unknown"; conn: Connection; alerts: number; policies: string }
const power = { on: "running", off: "stopped", unknown: "unknown" } as const;
/** The model as the table shows it ("ventilation-demo v3": capability model and the unit's capability version). */
export const modelLabel = (u: Pick<ApiUnitRow, "modelId" | "capabilityVersion">, models: Map<string, string>) => `${models.get(u.modelId) ?? u.modelId.slice(0, 8)} v${String(u.capabilityVersion)}`;
export function unitRows(us: ApiUnitRow[], ss: ApiSpaceRow[], models: Map<string, string>, t: T = en): UnitRow[] {
  return [...us].toSorted((a, b) => a.displayName.localeCompare(b.displayName)).map((u) => {
    return {
      id: u.id, version: u.version, name: u.displayName, model: modelLabel(u, models), location: spacePath(u.spaceId, ss) || t("Unassigned"),
      power: power[u.effectivePowerState], conn: u.connection, alerts: u.activeAlertCount, policies: u.alertPolicyIds.length > 0 ? t("Default + {n}", { n: u.alertPolicyIds.length }) : t("Default"),
    };
  });
}
export const powerStates: PowerState[] = ["on", "off", "unknown"];
export const connections: Connection[] = ["online", "offline", "unknown", "connecting", "error"];
/** units.list filters of the URL (powerState, connections, search) applied to the selection's units. */
export function filterUnits(us: ApiUnitRow[], f: { powerState?: PowerState; connections: Connection[]; search: string }): ApiUnitRow[] {
  const q = f.search.trim().toLowerCase();
  return us.filter((u) => {
    return [
      !f.powerState || u.effectivePowerState === f.powerState,
      f.connections.length === 0 || f.connections.includes(u.connection),
      !q || u.displayName.toLowerCase().includes(q) || u.id.toLowerCase().includes(q),
    ].every(Boolean);
  });
}

/** The unit form (DD-A02 fields, warranty end IR209). installedAt and warrantyEnd are Kuala Lumpur dates. */
export interface UnitDraft { displayName: string; propertyId: string; spaceId: string; modelId: string; installedAt: string; warrantyEnd: string; serviceScope: Scope[] }
const klDate = (iso: string | null | undefined) => (iso ? klStamp(iso).slice(0, 10) : "");
export const unitDraft = (u?: Pick<ApiUnitRow, "displayName" | "propertyId" | "spaceId" | "modelId" | "installedAt" | "warrantyEndsAt" | "serviceScope">, propertyId = "", spaceId = ""): UnitDraft => {
  return {
    displayName: u?.displayName ?? "", propertyId: u?.propertyId ?? propertyId, spaceId: u?.spaceId ?? (u ? "" : spaceId), modelId: u?.modelId ?? "",
    installedAt: klDate(u?.installedAt), warrantyEnd: klDate(u?.warrantyEndsAt), serviceScope: u?.serviceScope ?? ["indoor", "outdoor"],
  };
};
export function unitErrors(d: UnitDraft, todayKL: string, t: T = en): Record<string, string> {
  const e: Record<string, string> = {};
  if (!hasLength(d.displayName, 120)) e.displayName = t(TEXT_1_120_CHARACTERS);
  if (!d.propertyId) e.propertyId = t("Choose a location");
  if (!d.modelId) e.modelId = t("Choose a model");
  if (d.installedAt && isAfter(d.installedAt, todayKL)) e.installedAt = t("An installation date cannot be in the future");
  if (d.warrantyEnd && d.installedAt && isBefore(d.warrantyEnd, d.installedAt)) e.warrantyEnd = t("A warranty cannot end before the installation");
  if (d.serviceScope.length === 0) e.serviceScope = t("Choose at least one inspection group");
  return e;
}
export const unitInput = (d: UnitDraft, orgId: string, id?: string, changeReason?: string) => {
  return {
    ...(id && { id }), customerOrgId: orgId, propertyId: d.propertyId, spaceId: d.spaceId || null, displayName: d.displayName.trim(), modelId: d.modelId, type: "split" as const,
    installedAt: d.installedAt ? `${d.installedAt}T00:00:00+08:00` : null, warrantyEndsAt: d.warrantyEnd ? `${d.warrantyEnd}T00:00:00+08:00` : null,
    serviceScope: d.serviceScope, ...(changeReason && { changeReason }),
  };
};
/** A relocation (another property or space) needs a change reason (DD-A02 step 3). */
export const relocates = (u: Pick<ApiUnitRow, "propertyId" | "spaceId">, d: UnitDraft) => u.propertyId !== d.propertyId || (u.spaceId ?? "") !== d.spaceId;
/** The fields of the draft that differ from the saved unit ("1 unsaved change · Name"). */
export function changedFields(u: ApiUnitRow, d: UnitDraft, t: T = en): string[] {
  const saved = unitDraft(u);
  const out: string[] = [];
  if (saved.displayName !== d.displayName.trim()) out.push(t("Name"));
  if (relocates(u, d)) out.push(t("Location"));
  if (saved.modelId !== d.modelId) out.push(t("Model"));
  if (saved.installedAt !== d.installedAt) out.push(t("Installed at"));
  if (saved.warrantyEnd !== d.warrantyEnd) out.push(t("Warranty end"));
  if ([...saved.serviceScope].toSorted(byText).join(',') !== [...d.serviceScope].toSorted(byText).join(',')) out.push(t("Service scope"));
  return out;
}

// ---- alert policies of the customer (IR108: the unit carries the policies) ----

/** Policy of service-contracts.ts (alert and default_alert kinds as policies.list returns them for one customer). */
export interface ApiCustomerPolicy {
  id: string; version: number; kind: "alert" | "default_alert" | "automation"; name: string; unitIds: string[]; enabled: boolean; ownerMembershipId: string;
  customerId?: string | null; metric?: string; operator?: Operator; threshold?: number; durationSeconds?: number; severity?: Severity;
  activeWindow?: { weekdays: number[]; startLocal: string; endLocal: string } | null;
  rules?: { ruleKey: string; name: string; metric: string; operator: Operator; threshold: number; durationSeconds: number; category: string }[];
  ruleSettings?: { ruleKey: string; customerId: string; enabled: boolean; version: number; reason: string | null; updatedAt: string }[];
}
const sevLabel: Record<Severity, string> = { critical: "Critical", warning: "Warning", normal: "Info" };
function duration(s: number, t: T): string {
  if (s % 3600 === 0 && s >= 3600) return t("{n} h", { n: s / 3600 });
  if (s % 60 === 0 && s >= 120) return t("{n} min", { n: s / 60 });
  return t("{n} s", { n: s });
}
/** The short weekday name of ISO day 1–7 (Monday first) in the user's language: 1 Jan 2024 was a Monday. */
const dayName = (x: number, locale: Locale) => new Date(Date.UTC(2024, 0, x)).toLocaleDateString(intlTag(locale), { weekday: "short", timeZone: "UTC" });
const days = (w: number[], i: I18n) => {
  if (w.length === 7) return "";
  const sorted = [...w].toSorted((a, b) => a - b);
  const d = sorted.join(",");
  if (d === "1,2,3,4,5") return `${i.t("weekdays")} `;
  if (d === "6,7") return `${i.t("weekends")} `;
  return `${sorted.map((x) => dayName(x, i.display.locale)).join(", ")} `;
};
/** "Temperature ≥ 30 °C for 60 s, weekdays 08:00–19:00 → Warning", in the display language; the window's hours are the policy's local hours. */
export function conditionText(p: { metric?: string; operator?: Operator; threshold?: number; durationSeconds?: number; activeWindow?: NonNullable<ApiCustomerPolicy["activeWindow"]> | null; severity?: Severity }, i: I18n = EN): string {
  const { t } = i;
  const m = p.metric ?? "";
  const w = p.activeWindow ? `, ${days(p.activeWindow.weekdays, i)}${p.activeWindow.startLocal}–${p.activeWindow.endLocal}` : "";
  const cond = m === "heartbeat_gap" ? `${t("No heartbeat for {n} min", { n: p.threshold ?? 0 })}${w}` // the gap itself is the duration (minutes)
    : `${textOf(metricLabel, m, t) ?? m} ${entryOf(opSymbol, p.operator ?? "gte")} ${String(p.threshold ?? 0)} ${lookup(metricUnit, m) ?? ""} ${t("for {duration}", { duration: duration(p.durationSeconds ?? 0, t) })}${w}`;
  return p.severity ? `${cond} → ${t(sevLabel[p.severity])}` : cond;
}
export interface PolicyLine { id: string; version: number; kind: "alert" | "default_alert"; name: string; type: string; condition: string; madeBy: string; units: number; enabled: boolean }
export interface RuleLine { ruleKey: string; name: string; condition: string; enabled: boolean; version: number; note: string | null }
/** The customer's policies (default first) with the units of this customer that carry each one; the default policy's
system name in the display language (IR499). */
export function policyLines(ps: ApiCustomerPolicy[], customerUnits: string[], owner: (membershipId: string) => string, i: I18n = EN): PolicyLine[] {
  const { t } = i;
  const mine = new Set(customerUnits);
  return ps.filter((p): p is ApiCustomerPolicy & { kind: "alert" | "default_alert" } => p.kind === "alert" || p.kind === DEFAULT_ALERT)
    // the default policy first, then by name
    .toSorted((a, b) => Number(b.kind === DEFAULT_ALERT) - Number(a.kind === DEFAULT_ALERT) || a.name.localeCompare(b.name))
    .map((p) => {
      return p.kind === DEFAULT_ALERT
        ? { id: p.id, version: p.version, kind: p.kind, name: p.name === "Default policy" ? t("Default policy") : p.name, type: t("Default"), condition: t((p.rules?.length ?? 0) === 1 ? "1 rule · {on} on for this customer" : "{n} rules · {on} on for this customer", { n: p.rules?.length ?? 0, on: defaultRules(p, i).filter((r) => r.enabled).length }), madeBy: t("HQ (template)"), units: mine.size, enabled: p.enabled }
        : { id: p.id, version: p.version, kind: p.kind, name: p.name, type: textOf(metricLabel, p.metric ?? "", t) ?? (p.metric ?? ""), condition: conditionText(p, i), madeBy: owner(p.ownerMembershipId), units: p.unitIds.filter((u) => mine.has(u)).length, enabled: p.enabled };
    });
}
/** The default policy's rules with this customer's on/off settings (no setting = on). */
export function defaultRules(p: ApiCustomerPolicy, i: I18n = EN): RuleLine[] {
  const { t } = i;
  return (p.rules ?? []).map((r) => {
    const s = p.ruleSettings?.find((x) => x.ruleKey === r.ruleKey);
    const sinceText = s?.enabled ? "On since {date}" : "Off since {date}";
    const since = s ? t(sinceText, { date: showDate(s.updatedAt, i.display) }) : null;
    const reason = s?.reason ? ` · ${s.reason}` : "";
    return { ruleKey: r.ruleKey, name: r.name, condition: conditionText(r, i), enabled: s?.enabled ?? true, version: s?.version ?? 0, note: since ? `${since}${reason}` : null };
  });
}

// ---- client users of the customer (FR-A17, IR144) ----

type Channel = "inApp" | "email" | "whatsapp";
/** ClientUser of service-contracts.ts. */
export interface ApiClientUser {
  id: string; version: number; customerId: string; membershipId: string | null; email: string; displayName: string | null; clientRole: "owner" | "member";
  status: "invited" | "active" | "disabled"; lastSignInAt: string | null; allowedChannels: Channel[]; invitedAt: string; invitedByMembershipId: string;
  /** HQ ended the access in Access & roles: only HQ gives it back (IR508) */
  revokedAt?: string | null;
}
export interface ClientUserRow {
  id: string; version: number; email: string; name: string | null; role: "owner" | "member"; status: ApiClientUser["status"]; sub: string; lastSignIn: string; channels: string;
  lastOwner: boolean; you: boolean;
  membershipId: string | null; // the membership Sign out everywhere ends the sessions of (IR385); null only for an invite from before IR461, which has no account yet
  revoked: boolean; // HQ ended the access: the owner cannot change, enable or re-add the user (IR508)
}
const channelLabel: Record<Channel, string> = { inApp: "In-app", email: "Email", whatsapp: "WhatsApp" };
/** When a user last signed in: in the user's display settings when known, else Kuala Lumpur time. */
function signInText(at: string | null, i?: I18n): string {
  if (!at) return "—";
  return i ? showTime(at, i.display) : klStamp(at);
}
/** Rows by name; the last active owner is marked because it cannot be demoted, disabled or removed (CONFLICT). The
customer's Users page (IR267) and HQ's register (IR293) pass their display: the invite day and the last sign-in in
the user's time zone. */
export function clientUserRows(us: ApiClientUser[], who: (membershipId: string) => string, self?: string, i?: I18n): ClientUserRow[] {
  const owners = us.filter((u) => u.clientRole === "owner" && u.status === "active").length;
  const t = (i ?? EN).t;
  return [...us].toSorted((a, b) => (a.displayName ?? a.email).localeCompare(b.displayName ?? b.email)).map((u) => {
    return {
      id: u.id, version: u.version, email: u.email, name: u.displayName, role: u.clientRole, status: u.status,
      sub: u.displayName ? u.email : t("invited {date} by {who}", { date: i ? showDate(u.invitedAt, i.display) : klDate(u.invitedAt), who: who(u.invitedByMembershipId) }),
      lastSignIn: signInText(u.lastSignInAt, i),
      channels: u.status === "active" && u.allowedChannels.length > 0 ? u.allowedChannels.map((c) => t(entryOf(channelLabel, c))).join(" · ") : "—",
      lastOwner: u.clientRole === "owner" && u.status === "active" && owners === 1, you: !!self && u.membershipId === self, membershipId: u.membershipId, revoked: !!u.revokedAt,
    };
  });
}
/** A plain address up to 254 characters, not yet a user of this customer (case-insensitive). */
export function inviteError(email: string, us: { email: string }[], t: T = translator("en")): string | undefined {
  const e = email.trim();
  if (e.length > 254 || !isEmail(e)) return t("A valid email address (up to 254 characters)");
  if (us.some((u) => u.email.toLowerCase() === e.toLowerCase())) return t("Already a user of this customer");
  return undefined;
}

// ---- warranty & coverage (FR-A19) ----

export const coverageStatuses = ["under_warranty", "contract", "expiring", "no_coverage"] as const;
type CoverageStatus = (typeof coverageStatuses)[number];
/** UnitCoverage of service-contracts.ts. */
export interface ApiCoverage { unitId: string; customerId: string; modelId: string; warrantyEndsAt: string | null; contractIds: string[]; status: CoverageStatus; claimableJobIds: string[] }
export interface CoverageRow {
  unitId: string; unit: string; sub: string; customerId: string; customer: string; model: string; ends: string; endsDate: string; endsAt: string | null; days: number | null;
  contractIds: string[]; contracts: string; status: CoverageStatus; statusText: string;
}
const dayMs = 86_400_000;
/** A unit's cover in words: contract, warranty, ending soon, or none. */
function coverageText(c: ApiCoverage, warrantyDays: number | null, t: T): string {
  switch (c.status) {
    case "contract": { return t("Covered by contract"); }
    case "under_warranty": { return t("Under warranty"); }
    case "expiring": { return t("Ends in {n} d · no contract", { n: warrantyDays ?? 0 }); }
    default: { return t(c.warrantyEndsAt ? "Warranty ended · no contract" : "No warranty · no contract"); }
  }
}
/** The soonest warranty end first, units without one last; then by unit name. */
function byWarrantyEnd(a: { endsAt: string | null; unit: string }, b: { endsAt: string | null; unit: string }): number {
  if (a.endsAt === b.endsAt) return a.unit.localeCompare(b.unit);
  if (a.endsAt === null) return 1;
  return b.endsAt === null ? -1 : a.endsAt.localeCompare(b.endsAt);
}
/** One row per unit with its warranty end (a Kuala Lumpur day, in the user's language; `endsDate` YYYY-MM-DD for the
CSV) and the covering contracts. */
export function coverageRows(cs: ApiCoverage[], x: { unit: (id: string) => { name: string; place: string } | undefined; customer: (id: string) => string; model: (id: string) => string; contract: (id: string) => string }, now: Date, i: I18n = EN): CoverageRow[] {
  const { t } = i;
  return cs.map((c) => {
    const u = x.unit(c.unitId);
    const warrantyDays = c.warrantyEndsAt ? Math.ceil((Date.parse(c.warrantyEndsAt) - now.getTime()) / dayMs) : null;
    const statusText = coverageText(c, warrantyDays, t);
    return {
      unitId: c.unitId,
      unit: u?.name ?? c.unitId.slice(0, 8),
      sub: [c.unitId.slice(0, 8), u?.place].filter(Boolean).join(" · "),
      customerId: c.customerId,
      customer: x.customer(c.customerId),
      model: x.model(c.modelId),
      ends: c.warrantyEndsAt ? businessDay(c.warrantyEndsAt, i.display.locale) : "—",
      endsDate: klDate(c.warrantyEndsAt),
      endsAt: c.warrantyEndsAt,
      contractIds: c.contractIds,
      contracts: c.contractIds.map((id) => x.contract(id)).join(", ") || "—",
      status: c.status,
      days: warrantyDays,
      statusText,
    };
  }).toSorted(byWarrantyEnd);
}
/** KPI tiles (Figma 02 warranty): under warranty (any unit whose warranty still runs), ending within 90 / 30 days, no coverage, contracts. */
export function coverageKpis(rows: CoverageRow[], t: T = en) {
  const running = rows.filter((r): r is CoverageRow & { days: number } => r.days !== null && r.days > 0);
  const contracts = [...new Set(rows.flatMap((r) => (r.status === "contract" ? r.contracts.split(", ") : [])))];
  return {
    total: rows.length, underWarranty: running.length, within90: running.filter((r) => r.days <= 90).length, within30: running.filter((r) => r.days <= 30).length,
    noCoverage: rows.filter((r) => r.status === "no_coverage").length, contract: rows.filter((r) => r.status === "contract").length,
    contractNames: contracts.length > 2 ? `${contracts.slice(0, 2).join(", ")} …` : contracts.join(", ") || t("none"),
  };
}
export const withinDays = ["30", "90", "180", "365"] as const;
export interface CoverageFilter { customerId: string; coverage: "" | CoverageStatus; within: "" | (typeof withinDays)[number] }
export const coverageMatch = (r: CoverageRow, f: CoverageFilter) => {
  return [
    !f.customerId || r.customerId === f.customerId,
    !f.coverage || r.status === f.coverage,
    !f.within || (r.days !== null && r.days > 0 && r.days <= Number(f.within)),
  ].every(Boolean);
};
const toCsv = (header: string[], rows: string[][]) => [header, ...rows].map((r) => r.map((item) => csvCell(item)).join(",")).join("\n") + "\n";
export const coverageCsv = (rows: CoverageRow[]) => {
  return toCsv(["unit", "unit_id", "customer", "model", "warranty_end", "maintenance_contract", "status"],
    rows.map((r) => [r.unit, r.unitId, r.customer, r.model, r.endsDate, r.contracts === "—" ? "" : r.contracts, r.statusText]));
};

/** A job completed under warranty whose accepted report lists replaced parts and has no filed claim (IR209). */
export interface ClaimCandidate { jobId: string; version: number; unit: string; type: string; completed: string; parts: string; partLabel: string; amountMinor: number | null; currency: Currency }
interface Part { name: string; quantity: number; catalogCode: string | null }
interface Cost { kind: "estimate" | "actual"; amountMinor: number; currency: Currency }
export function claimCandidate(j: { id: string; version: number; type: string; completedAt: string | null; costs: Cost[] }, parts: Part[], unit: string, i: I18n = EN): ClaimCandidate {
  const label = parts.map((p) => `${p.name}${wrapped(p.catalogCode, " ")} ×${String(p.quantity)}`).join(", ");
  const actual = j.costs.filter((c) => c.kind === "actual");
  return {
    jobId: j.id,
    version: j.version,
    type: j.type,
    completed: j.completedAt ? showDate(j.completedAt, i.display) : "—",
    parts: label || i.t("parts listed in the report"),
    partLabel: label.slice(0, 120),
    amountMinor: actual.length > 0 ? actual.reduce((n, c) => n + c.amountMinor, 0) : null,
    currency: actual[0]?.currency ?? "MYR",
    unit,
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
  const at = { cell: "", isQuoted: false }; // the cell being read, and whether it is inside quotes
  const endCell = () => { records.at(-1)?.push(at.cell); at.cell = ""; };
  // inside quotes: a doubled quote is one quote, a single one ends the quoting, anything else is text
  const quoted = (ch: string, next: string | undefined): number => {
    if (ch !== '"') at.cell += ch;
    else if (next === '"') { at.cell += '"'; return 2; }
    else at.isQuoted = false;
    return 1;
  };
  // outside quotes: a quote starts quoting, a comma ends the cell, CR, LF or CRLF ends the record
  const plain = (ch: string, next: string | undefined): number => {
    switch (ch) {
      case '"': { at.isQuoted = true; return 1; }
      case ",": { endCell(); return 1; }
      case "\n":
      case "\r": { endCell(); records.push([]); return ch === "\r" && next === "\n" ? 2 : 1; }
      default: { at.cell += ch; return 1; }
    }
  };
  const t = text.replace(/^\u{FEFF}/u, "");
  for (let i = 0; i < t.length;) i += (at.isQuoted ? quoted : plain)(t.charAt(i), t[i + 1]);
  endCell();
  const full = records.filter((r) => r.some((c) => c.trim() !== ""));
  return { header: (full[0] ?? []).map((h) => h.trim()), rows: Math.max(0, full.length - 1) };
}
const norm = (s: string) => s.toLowerCase().replaceAll(/[^a-z0-9]/g, "");
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
export interface ApiImportRow {
  rowNumber: number; propertyName: string; floorName: string | null; roomName: string | null; unitName: string; modelCode: string; serial: string | null;
  installedOn: string | null; warrantyEnd: string | null; result: "ready" | "warning" | "error"; messageKey: string | null;
}
export interface ApiImportPreview { previewId: string; customerId: string; fileName: string; rows: ApiImportRow[]; readyCount: number; warningCount: number; errorCount: number; expiresAt: string }
export interface ApiUnitImport { id: string; version: number; customerId: string; createdPropertyIds: string[]; createdSpaceIds: string[]; createdUnitIds: string[]; skippedRowNumbers: number[]; undoUntil: string; state: "imported" | "undone" }
export const importPlace = (r: ApiImportRow) => [r.propertyName, r.floorName, r.roomName].filter(Boolean).join(" › ");
/** The row result in words (the message keys of units.importPreview). */
export function importMessage(r: ApiImportRow, t: T = en): string {
  const place = r.roomName ?? r.floorName ?? "";
  switch (r.messageKey) {
    case null: { return t("Ready");
    }
    case "warning.importCreatesProperty": { return t("Property “{name}” does not exist — it will be created (office; add its address afterwards).", { name: r.propertyName });
    }
    case "warning.importCreatesSpace": { return t("“{place}” does not exist — it will be created.", { place });
    }
    case "error.unknownModel": { return t("Model {model} is not in the model register — add it under Devices & models first.", { model: r.modelCode });
    }
    case "error.serialBound": { return t("Serial {serial} is already bound to another unit.", { serial: r.serial ?? "" });
    }
    case "error.unknownSerial": { return t("Serial {serial} is not a registered device.", { serial: r.serial ?? "" });
    }
    case "error.duplicateSerial": { return t("Serial {serial} appears more than once in this file.", { serial: r.serial ?? "" });
    }
    case "error.tamperUnresolved": { return t("Device {serial} has an unresolved tamper.", { serial: r.serial ?? "" });
    }
    case "error.duplicateSiblingName": { return t("A unit named “{name}” already exists there.", { name: r.unitName });
    }
    case "error.future": { return t("The installation date is in the future.");
    }
    case "error.importInvalidDate": { return t("A date is not a valid YYYY-MM-DD date.");
    }
    case "error.importMissingField": { return t("Property, unit name and model are required.");
    }
    case "error.length": { return t("The unit name is longer than 120 characters.");
    }
    default: { return r.messageKey;
    }
  }
}
/** The rows that are not ready, as a CSV with machine column names; the message in the user's language. */
export const errorReportCsv = (rows: ApiImportRow[], t: T = en) => {
  return toCsv(["row", "property", "floor", "room", "unit_name", "model_code", "serial", "result", "message"],
    rows.filter((r) => r.result !== "ready").map((r) => [String(r.rowNumber), r.propertyName, r.floorName ?? "", r.roomName ?? "", r.unitName, r.modelCode, r.serial ?? "", r.result, importMessage(r, t)]));
};

