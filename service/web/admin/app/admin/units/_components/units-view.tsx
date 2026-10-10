"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import {
  Badge, Banner, Btn, Card, Check, Choice, ConnBadge, cx, DataTable, EmptyState, Field, Input, Kpi, LinkBtn, Modal, Page, PowerBadge, Search, Select, SummaryList, Tabs,
  Textarea, TextLink, Toggle,
} from "@ac/web/components/ui";
import { useI18n, useT } from "@ac/web/components/I18n";
import { useAction } from "@ac/web/lib/useAction";
import { useUrlPatch } from "@ac/web/lib/useUrlPatch";
import { showTime, type T } from "@ac/web/lib/i18n";
import {
  autoMapping, changedFields, connections, contractMatch, coverageCsv, coverageMatch, csvShape, customerDraft, customerErrors, customerMatch, errorReportCsv, flatten,
  importFields, importMessage, importPlace, importTemplate, inviteError, nameError, powerStates, profileLabel, propertyDraft, propertyErrors, propertyInput, reasonError,
  relocates, unitDraft, unitErrors, unitInput,
  type ApiImportPreview, type ApiUnitDetail, type ApiUnitImport, type ClaimCandidate, type ClientUserRow, type Connection, type ContractFilter, type CoverageFilter,
  type CoverageRow, type coverageKpis, type CustomerDraft, type CustomerRow, type PolicyLine, type PowerState, type Profile, type PropertyDraft, type RuleLine, type Scope,
  type Selection, type TreeProperty, type TreeSpace, type UnitDraft, type UnitRow, type registerKpis,
} from "@ac/web/lib/assets";
import { amount } from "@ac/web/lib/energy";
import { typeLabel } from "@ac/web/lib/partnerJobDetail";
import {
  archiveLocation, archiveUnit, createCustomer, deleteUnit, importCommit, importPreview, importUndo, inviteClientUser, previewPasswordReset, recordWarrantyClaim,
  removeClientUser, resendInvite, saveProperty, saveSpace, saveUnit, setDefaultRule, setUnitPolicies, updateClientUser, updateCustomer,
} from "../actions";

export type UnitLive = {
  u: ApiUnitDetail; path: string; model: string; device: { id: string; serial: string; connection: Connection } | null; deviceKnown: boolean;
  attached: { id: string; kind: "default_alert" | "alert"; name: string; type: string; condition: string; madeBy: string; on: string }[];
  blockers: { label: string; href: string | null }[] | null; archivedNote: string | null; seen: string | null;
};
export type CustomerLive = {
  row: CustomerRow; tab: "overview" | "users" | "policies"; users: ClientUserRow[]; tree: TreeProperty[]; selection: Selection | null; places: { propertyId: string; spaceId: string; label: string }[];
  models: { id: string; label: string }[]; perProperty: string; alertsSub: string | null; filter: { powerState?: PowerState; connections: Connection[]; search: string };
  total: number; units: UnitRow[]; lastEdit: string | null; policies: PolicyLine[] | null; rules: { policyId: string; items: RuleLine[] } | null;
  canRules: boolean; canAttach: boolean; unit?: UnitLive; unitMissing?: boolean;
};
export type WarrantyLive = { rows: CoverageRow[]; kpis: ReturnType<typeof coverageKpis>; customers: { id: string; name: string }[]; claims: ClaimCandidate[] | null; canClaim: boolean };
export type UnitsLive = {
  now: string; todayKL: string; canWrite: boolean; standingKnown: boolean; rows: CustomerRow[]; kpis: ReturnType<typeof registerKpis>; search: string; missing: boolean;
  top: "customers" | "warranty"; activeCustomers: { id: string; name: string }[]; coverageAttention: number | null; warranty?: WarrantyLive | null; customer?: CustomerLive;
};
type Nav = ReturnType<typeof useUrlPatch>;
type Run = ReturnType<typeof useAction>[1];

const scopes: Scope[] = ["indoor", "outdoor", "electrical"];
const kindLabel: Record<string, string> = { home: "Home", office: "Office", floor: "Floor", area: "Area", room: "Room", space: "Space" };
const addKind: Record<TreeSpace["kind"], string> = { floor: "Add floor", area: "Add area", room: "Add room", space: "Add space" };
const deleteKind: Record<TreeSpace["kind"], string> = { floor: "Delete floor", area: "Delete area", room: "Delete room", space: "Delete space" };
const powerLabel: Record<PowerState, string> = { on: "Running", off: "Stopped", unknown: "Unknown" };
const connLabel: Record<Connection, string> = { online: "Online", offline: "Offline", unknown: "Unknown", connecting: "Connecting", error: "Error" };
const scopeLabel: Record<Scope, string> = { indoor: "indoor", outdoor: "outdoor", electrical: "electrical" };
/** “1 unit” / “3 units” in the display language: the singular and the plural key of one count. */
const count = (t: T, n: number, one: string, many: string) => t(n === 1 ? one : many, { n });
const units = (t: T, n: number) => count(t, n, "1 unit", "{n} units");
const clearUnit = { unitId: null };

/** Customers & units (FR-A02) in API mode: the register, then one customer's location tree, units and unit edit. The
 * customer, location, unit and unit filters live in the URL; every write is a Server Action and re-renders the page.
 * Texts in the display language; the first render's times come formatted from the page (IR293). */
export function UnitsView({ live }: { live: UnitsLive | null }) {
  const t = useT();
  if (!live) return <Page><EmptyState title={t("Customers & units need asset.read")}>{t("Ask an identity administrator for Customers & units access (Access & roles).")}</EmptyState></Page>;
  return live.customer ? <CustomerScreen live={live} c={live.customer} /> : <CustomerList live={live} />;
}

// ---- the register ----

function CustomerList({ live }: { live: UnitsLive }) {
  const t = useT();
  const nav = useUrlPatch();
  const [pending, run] = useAction();
  const [q, setQ] = useState(live.search);
  const [status, setStatus] = useState<"active" | "all">("active");
  const [contract, setContract] = useState<ContractFilter>("all");
  const [open, setOpen] = useState(false);
  const [importing, setImporting] = useState(false);
  const rows = live.rows.filter((r) => (status === "all" || r.status === "active") && contractMatch(contract, r) && customerMatch(r, q));
  const k = live.kpis;
  return (
    <Page>
      <div className="flex flex-wrap items-center justify-between gap-2">
        <Tabs value={live.top} onChange={(v) => nav({ tab: v === "warranty" ? "warranty" : null })} tabs={[
          { id: "customers" as const, label: t("Customers & units"), count: k.customers },
          { id: "warranty" as const, label: t("Warranty & coverage"), count: live.coverageAttention ?? "—" },
        ]} />
        {live.canWrite && live.top === "customers" && <Btn size="sm" onClick={() => setImporting(true)}>{t("↑ Import CSV")}</Btn>}
      </div>
      {live.top === "warranty" ? <WarrantyTab live={live} /> : <>
      {live.missing && <Banner tone="warn">{t("This customer no longer exists or is outside your scope — showing the register.")}</Banner>}
      <div className="grid-fluid" style={{ ["--min" as string]: "180px" }}>
        <Kpi label={t("Customers")} value={k.customers} sub={k.inactive ? t("+{n} inactive (not counted)", { n: k.inactive }) : t("all active")} />
        <Kpi label={t("Properties")} value={k.properties} sub={k.propertyNames} />
        <Kpi label={t("Units")} value={k.units} sub={k.power} />
        <Kpi label={t("Needs attention")} value={k.attention} tone={k.attention ? "warn" : undefined} sub={t(live.standingKnown ? "overdue billing / restriction / open alert" : "open alerts (billing needs billing.read)")} />
      </div>
      <Card title={t("Customers")} sub={t("Select a customer to manage its properties, spaces and units")} action={live.canWrite && <Btn size="sm" variant="primary" onClick={() => setOpen(true)}>{t("+ New customer")}</Btn>}>
        <div className="mb-3 flex flex-wrap items-center gap-3">
          <div className="min-w-[220px] flex-1 sm:max-w-sm"><Search placeholder={t("Search customer name, ID or property…")} value={q} onChange={setQ} /></div>
          <Select aria-label={t("Status")} className="w-auto" value={status} onChange={(e) => setStatus(e.target.value as "active" | "all")}><option value="active">{t("Status: Active")}</option><option value="all">{t("Status: All")}</option></Select>
          {live.standingKnown && (
            <Select aria-label={t("Contract")} className="w-auto" value={contract} onChange={(e) => setContract(e.target.value as ContractFilter)}>
              <option value="all">{t("Contract: All")}</option><option value="overdue">{t("Overdue")}</option><option value="restricted">{t("Restriction")}</option><option value="good">{t("Good standing")}</option><option value="none">{t("No contract")}</option>
            </Select>
          )}
        </div>
        {live.rows.length === 0 ? <EmptyState title={t("No customers yet")}>{t(live.canWrite ? "Create the first customer, then add its properties and units." : "Customers appear here once HQ registers them.")}</EmptyState> : (
          <DataTable rows={rows} rowKey={(r) => r.id} onRowClick={(r) => nav({ customerId: r.id, tab: null, locationId: null, unitId: null, powerState: null, connections: null, search: null })} cols={[
            { key: "c", label: t("Customer"), render: (r) => <><b>{r.name}</b><div className="text-xs text-muted">{r.billingName} · {r.id.slice(0, 8)}</div></> },
            { key: "s", label: t("Status"), render: (r) => <Badge tone={r.status === "active" ? "ok" : "unknown"}>{t(r.status === "active" ? "Active" : "Inactive")}</Badge> },
            { key: "p", label: t("Properties"), render: (r) => r.properties, hideBelow: "sm" },
            { key: "u", label: t("Units"), render: (r) => r.units },
            { key: "o", label: t("Operation"), render: (r) => r.operation, hideBelow: "md" },
            { key: "a", label: t("Alerts"), render: (r) => (r.status === "inactive" ? "—" : r.alerts), hideBelow: "md" },
            ...(live.standingKnown ? [{ key: "k", label: t("Contract"), render: (r: CustomerRow) => <div className="flex flex-wrap gap-1">{r.marks.map((m) => <Badge key={m.label} tone={m.tone}>{m.label}</Badge>)}</div>, hideBelow: "md" as const }] : []),
            { key: "x", label: "", render: () => "›" },
          ]} />
        )}
        <p className="mt-2 text-[11px] text-muted">{t("{n} of {total} · sorted by name · Inactive customers appear only with Status: All and are not counted (IR40). Archived properties, spaces and units never appear in lists, summaries or KPI denominators (IR39).", { n: rows.length, total: live.rows.length })}</p>
      </Card>
      </>}
      {importing && <ImportWizard live={live} onClose={() => setImporting(false)} />}
      {open && <CustomerModal pending={pending} onClose={() => setOpen(false)} onSave={(d) => run(() => createCustomer({ billingName: d.billingName, name: d.name, serviceProfile: d.profile, status: d.status }), t("Customer “{name}” created — add a property next", { name: d.name.trim() }), (id) => { setOpen(false); nav({ customerId: id, tab: null, search: null }); })} />}
    </Page>
  );
}

function CustomerModal({ row, pending, onClose, onSave }: { row?: CustomerRow; pending: boolean; onClose: () => void; onSave: (d: CustomerDraft) => void }) {
  const t = useT();
  const [d, setD] = useState<CustomerDraft>(customerDraft(row));
  const [tried, setTried] = useState(false);
  const e = customerErrors(d, t);
  const err = (k: string) => (tried ? e[k] : undefined);
  const save = () => { setTried(true); if (!Object.keys(e).length) onSave(d); };
  return (
    <Modal open onClose={onClose} title={row ? t("Edit {name}", { name: row.name }) : t("New customer")} footer={<><Btn onClick={onClose}>{t("Cancel")}</Btn><Btn variant="primary" disabled={pending} onClick={save}>{t(row ? "Save customer" : "Create customer")}</Btn></>}>
      <div className="flex flex-col gap-3">
        <Field label={t("Billing name · required")} hint={t("1–120 characters · organization kind customer · tenant from the session (IR74). A second customer with the same billing name is CONFLICT.")} error={err("billingName")}><Input value={d.billingName} onChange={(x) => setD({ ...d, billingName: x.target.value })} /></Field>
        <Field label={t("Customer display name · required")} hint={t("1–120 characters")} error={err("name")}><Input value={d.name} onChange={(x) => setD({ ...d, name: x.target.value })} /></Field>
        <Field label={t("Service profile · required")}><Choice value={d.profile} onChange={(v: Profile) => setD({ ...d, profile: v })} options={(Object.keys(profileLabel) as Profile[]).map((p) => ({ id: p, label: t(profileLabel[p]) }))} /></Field>
        <Field label={t("Status")} hint={t(row ? "Deactivating is CONFLICT while the customer still has units, contracts or open jobs (D05)." : "Inactive customers cannot get new units.")}>
          <Select value={d.status} onChange={(x) => setD({ ...d, status: x.target.value as "active" | "inactive" })}><option value="active">{t("Active")}</option><option value="inactive">{t("Inactive")}</option></Select>
        </Field>
        {!row && <p className="text-[11px] text-muted">{t("Next step: add a property (home or office) for this customer.")}</p>}
      </div>
    </Modal>
  );
}

// ---- one customer ----

type ModalState =
  | null | { kind: "customer" } | { kind: "property"; p?: TreeProperty } | { kind: "space"; propertyId: string; parentSpaceId: string | null; spaceKind: TreeSpace["kind"] }
  | { kind: "rename"; p?: TreeProperty; s?: TreeSpace } | { kind: "archive"; p?: TreeProperty; s?: TreeSpace } | { kind: "unit"; propertyId: string; spaceId: string };

function CustomerScreen({ live, c }: { live: UnitsLive; c: CustomerLive }) {
  const t = useT();
  const nav = useUrlPatch();
  const [pending, run] = useAction();
  const [m, setM] = useState<ModalState>(null);
  const close = () => setM(null);
  const r = c.row;
  const s = r.standing;
  const canWrite = live.canWrite;
  const sel = c.selection;
  const selId = !sel ? "" : sel.kind === "property" ? sel.property.id : sel.kind === "space" ? sel.space.id : `unassigned:${sel.property.id}`;
  const newUnitAt = () => setM({ kind: "unit", propertyId: sel?.property.id ?? c.tree[0]?.id ?? "", spaceId: sel?.kind === "space" ? sel.space.id : "" });
  const unitsTotal = c.tree.reduce((n, p) => n + p.units, 0);
  return (
    <Page>
      <div className="flex flex-wrap items-center gap-2 text-[13px]">
        <button type="button" className="font-semibold text-primary" onClick={() => nav({ customerId: null, tab: null, locationId: null, propertyId: null, unitId: null, powerState: null, connections: null, search: null })}>{t("‹ Customers")}</button>
        <span className="text-muted">/ {r.name}{c.unit ? ` / ${c.unit.u.displayName}` : ""}</span>
        {r.marks.map((x) => <Badge key={x.label} tone={x.tone}>{x.label}</Badge>)}
        <div className="ml-auto flex gap-2">
          {canWrite && <Btn size="sm" onClick={() => setM({ kind: "customer" })}>{t("Edit customer")}</Btn>}
          {canWrite && r.status === "active" && c.tree.length > 0 && <Btn size="sm" variant="primary" onClick={newUnitAt}>{t("+ New unit")}</Btn>}
        </div>
      </div>
      <Card>
        <div className="flex flex-wrap items-center gap-3">
          <div aria-hidden className="flex h-11 w-11 items-center justify-center rounded-full bg-primary-soft font-bold text-primary">{r.name.split(/\s+/).map((w) => w[0]).join("").slice(0, 2).toUpperCase()}</div>
          <div className="min-w-0 flex-1">
            <div className="flex flex-wrap items-center gap-2"><b className="text-base">{r.name}</b><Badge tone={r.status === "active" ? "ok" : "unknown"}>{t(r.status === "active" ? "Active" : "Inactive")}</Badge></div>
            <div className="text-xs text-muted">{r.billingName} · {t("customer since {month}", { month: r.since })} · {t(profileLabel[r.profile])}</div>
          </div>
          {s && <div className="text-xs text-muted">{s.contracts ? t("Plan: {plans} · {units} under contract", { plans: s.plans.map((p) => t(profileLabel[p])).join(", "), units: units(t, s.contractUnits) }) : t("No current contract")}</div>}
        </div>
      </Card>
      <div className="grid-fluid" style={{ ["--min" as string]: "180px" }}>
        <Kpi label={t("Units")} value={unitsTotal} sub={c.perProperty} />
        <Kpi label={t("Operation")} value={r.operation} sub={t("Running {on} / known {known}", { on: r.running, known: r.known })} />
        <Kpi label={t("Open alerts")} value={r.alerts} tone={r.alerts ? "warn" : undefined} sub={c.alertsSub ?? t("severity needs alert.read")} href="/admin/alerts" link={t("Alerts ›")} />
        {s ? <Kpi label={t("Overdue billing")} value={s.overdueAmount ?? t("None")} tone={s.overdue ? "crit" : undefined}
          sub={s.restriction ? (s.restriction.state === "active" ? t("Restriction active") : t("Restriction {state}", { state: t(s.restriction.state.replace("_", " ")) })) : s.overdue ? count(t, s.overdue, "1 invoice overdue", "{n} invoices overdue") : t("no overdue invoice")}
          href={s.restriction && s.restriction.state !== "active" ? `/admin/restrictions?restrictionId=${s.restriction.id}` : s.overdue ? `/admin/billing?customerId=${r.id}&overdueOnly=true` : undefined}
          link={s.restriction && s.restriction.state !== "active" ? t("View restriction →") : s.overdue ? t("View invoices →") : undefined} />
          : <Kpi label={t("Overdue billing")} value="—" sub={t("needs billing.read")} />}
      </div>
      <Tabs value={c.tab} onChange={(v) => nav({ tab: v === "overview" ? null : v, unitId: null })} tabs={[
        { id: "overview" as const, label: t("Units & locations"), count: unitsTotal }, { id: "users" as const, label: t("Users"), count: c.users.length },
        { id: "policies" as const, label: t("Alert policies"), count: c.policies?.length ?? "—" },
      ]} />
      {c.tab === "policies" ? <PoliciesTab c={c} run={run} pending={pending} /> : c.tab === "users" ? <UsersTab live={live} c={c} run={run} pending={pending} /> : (
        <div className="split-rev">
          <Card title={t("Locations")} action={canWrite && <Btn size="sm" onClick={() => setM({ kind: "property" })}>{t("+ Add property")}</Btn>} className="self-start">
            {c.tree.length === 0 ? <EmptyState title={t("No properties yet")}>{t(canWrite ? "Add the customer’s home or office first." : "HQ has not registered a property yet.")}</EmptyState> : (
              <div className="flex flex-col gap-0.5">
                {c.tree.map((p) => (
                  <div key={p.id}>
                    <Node depth={0} selected={selId === p.id} onClick={() => nav({ locationId: p.id, ...clearUnit })} icon="⌂" name={p.name} meta={`${t(kindLabel[p.kind])} · ${units(t, p.units)}`}
                      menu={canWrite && <LocationMenu onRename={() => setM({ kind: "rename", p })} onDelete={() => setM({ kind: "archive", p })} />} />
                    {p.spaces.map((x) => <SpaceNode key={x.id} s={x} depth={1} selId={selId} nav={nav} canWrite={canWrite} setM={setM} />)}
                    {p.unassigned > 0 && <Node depth={1} selected={selId === `unassigned:${p.id}`} onClick={() => nav({ locationId: `unassigned:${p.id}`, ...clearUnit })} icon="·" name={t("Unassigned")} meta={units(t, p.unassigned)} />}
                    {canWrite && <AddLink depth={1} onClick={() => setM({ kind: "space", propertyId: p.id, parentSpaceId: null, spaceKind: p.kind === "office" ? "area" : "floor" })}>{t(p.kind === "office" ? "+ Add floor / area" : "+ Add floor")}</AddLink>}
                  </div>
                ))}
              </div>
            )}
            <p className="mt-3 text-[11px] text-muted">{t("⋯ on any location: Rename · Delete. Delete is blocked while it still contains locations or units.")}</p>
          </Card>
          <div className="flex min-w-0 flex-col gap-4">
            {c.unitMissing && <Banner tone="warn" action={<Btn size="sm" onClick={() => nav(clearUnit)}>{t("Close")}</Btn>}>{t("This unit no longer exists or belongs to another customer.")}</Banner>}
            {c.unit ? <UnitEdit key={c.unit.u.id} live={live} c={c} x={c.unit} nav={nav} run={run} pending={pending} /> : sel && <>
              <LocationCard c={c} sel={sel} canWrite={canWrite} active={r.status === "active"} setM={setM} />
              <UnitsCard key={selId} c={c} sel={sel} nav={nav} canWrite={canWrite && r.status === "active"} onNew={newUnitAt} />
            </>}
          </div>
        </div>
      )}
      {m?.kind === "customer" && <CustomerModal row={r} pending={pending} onClose={close} onSave={(d) => run(() => updateCustomer(r.id, r.version, { id: r.orgId, version: r.orgVersion ?? 1, name: r.billingName, status: r.orgStatus }, { billingName: d.billingName, name: d.name, serviceProfile: d.profile, status: d.status }), t("Customer saved"), close)} />}
      {m?.kind === "property" && <PropertyModal p={m.p} pending={pending} onClose={close} onSave={(d) => run(() => saveProperty(propertyInput(d, r.orgId, m.p?.id), m.p?.version), t(m.p ? "Property saved" : "Property added"), (id) => { close(); nav({ locationId: id, ...clearUnit }); })} />}
      {m?.kind === "space" && <SpaceModal c={c} init={m} pending={pending} onClose={close} onSave={(x) => run(() => saveSpace(x), t("{kind} “{name}” added", { kind: t(kindLabel[x.kind]), name: x.name }), (id) => { close(); nav({ locationId: id, ...clearUnit }); })} />}
      {m?.kind === "rename" && <RenameModal name={m.p?.name ?? m.s?.name ?? ""} where={m.p ? t("{kind} · property", { kind: t(kindLabel[m.p.kind]) }) : t(kindLabel[m.s!.kind])} pending={pending} onClose={close}
        onSave={(name) => run(() => (m.p ? saveProperty(propertyInput({ ...propertyDraft(m.p), name }, r.orgId, m.p.id), m.p.version)
          : saveSpace({ id: m.s!.id, propertyId: m.s!.propertyId, parentSpaceId: m.s!.parentSpaceId, kind: m.s!.kind, name: name.trim() }, m.s!.version)), t("Name saved"), close)} />}
      {m?.kind === "archive" && <ArchiveLocationModal p={m.p} s={m.s} pending={pending} onClose={close}
        onSave={(reason) => run(() => archiveLocation(m.p ? "property" : "space", m.p?.id ?? m.s!.id, m.p?.version ?? m.s!.version, reason), t("Location deleted — its history stays in the audit log"), () => { close(); nav({ locationId: null, ...clearUnit }); })} />}
      {m?.kind === "unit" && <NewUnitModal live={live} c={c} init={m} pending={pending} onClose={close}
        onSave={(d) => run(() => saveUnit(unitInput(d, r.orgId)), t("Unit “{name}” registered", { name: d.displayName.trim() }), (id) => { close(); nav({ locationId: d.spaceId || d.propertyId, unitId: id }); })} />}
    </Page>
  );
}

function Node({ depth, selected, onClick, icon, name, meta, menu }: { depth: number; selected: boolean; onClick: () => void; icon: string; name: string; meta: string; menu?: React.ReactNode }) {
  return (
    <div className={cx("flex items-center gap-1 rounded-lg pr-1", selected ? "bg-primary-soft/60" : "hover:bg-surface2/70")} style={{ paddingLeft: depth * 14 }}>
      <button type="button" onClick={onClick} aria-pressed={selected} className="flex min-w-0 flex-1 items-center gap-2 px-2 py-1.5 text-left text-[13px]">
        <span aria-hidden className="text-muted">{icon}</span><b className="truncate">{name}</b><span className="ml-auto shrink-0 text-[11px] text-muted">{meta}</span>
      </button>
      {menu}
    </div>
  );
}
function SpaceNode({ s, depth, selId, nav, canWrite, setM }: { s: TreeSpace; depth: number; selId: string; nav: Nav; canWrite: boolean; setM: (m: ModalState) => void }) {
  const t = useT();
  const container = s.kind === "floor" || s.kind === "area";
  return (
    <>
      <Node depth={depth} selected={selId === s.id} onClick={() => nav({ locationId: s.id, ...clearUnit })} icon={s.children.length ? "▾" : "·"} name={s.name}
        meta={`${t(kindLabel[s.kind])}${s.units ? ` · ${s.units}` : ""}`} menu={canWrite && <LocationMenu onRename={() => setM({ kind: "rename", s })} onDelete={() => setM({ kind: "archive", s })} />} />
      {s.children.map((x) => <SpaceNode key={x.id} s={x} depth={depth + 1} selId={selId} nav={nav} canWrite={canWrite} setM={setM} />)}
      {canWrite && container && <AddLink depth={depth + 1} onClick={() => setM({ kind: "space", propertyId: s.propertyId, parentSpaceId: s.id, spaceKind: "room" })}>{t("+ Add room")}</AddLink>}
    </>
  );
}
const AddLink = ({ depth, onClick, children }: { depth: number; onClick: () => void; children: React.ReactNode }) => (
  <button type="button" onClick={onClick} className="block py-1 text-left text-[12px] font-semibold text-primary" style={{ paddingLeft: depth * 14 + 8 }}>{children}</button>
);
function LocationMenu({ onRename, onDelete }: { onRename: () => void; onDelete: () => void }) {
  const t = useT();
  return (
    <select aria-label={t("Location actions")} className="h-7 w-8 shrink-0 cursor-pointer appearance-none rounded-md bg-transparent text-center text-[15px] font-bold leading-none text-muted hover:bg-surface2" value=""
      onChange={(e) => (e.target.value === "rename" ? onRename() : e.target.value === "delete" && onDelete())}>
      <option value="">⋯</option><option value="rename">{t("Rename")}</option><option value="delete">{t("Delete…")}</option>
    </select>
  );
}

function LocationCard({ c, sel, canWrite, active, setM }: { c: CustomerLive; sel: Selection; canWrite: boolean; active: boolean; setM: (m: ModalState) => void }) {
  const t = useT();
  if (sel.kind === "unassigned") {
    return <Card title={t("Unassigned")} sub={t("{property} · units without a space (IR62)", { property: sel.property.name })}><p className="text-[13px] text-muted">{t("These units belong to the property but no floor or room. Open a unit and pick its space under Location.")}</p></Card>;
  }
  if (sel.kind === "property") {
    const p = sel.property;
    return (
      <Card title={p.name} sub={[t("{kind} · property", { kind: t(kindLabel[p.kind]) }), p.address, count(t, p.floors, "1 floor", "{n} floors"), count(t, p.rooms, "1 room", "{n} rooms"), units(t, p.units)].filter(Boolean).join(" · ")}
        action={canWrite && <div className="flex flex-wrap gap-2"><Btn size="sm" onClick={() => setM({ kind: "rename", p })}>{t("Rename")}</Btn><Btn size="sm" onClick={() => setM({ kind: "property", p })}>{t("Edit details")}</Btn>
          <Btn size="sm" onClick={() => setM({ kind: "space", propertyId: p.id, parentSpaceId: null, spaceKind: p.kind === "office" ? "area" : "floor" })}>{t(p.kind === "office" ? "+ Add floor / area" : "+ Add floor")}</Btn><Btn size="sm" variant="danger" onClick={() => setM({ kind: "archive", p })}>{t("Delete")}</Btn></div>}>
        <SummaryList cols={2} items={[[t("Type"), t(kindLabel[p.kind])], [t("Address"), p.address ?? "—"], [t("Access instructions"), p.accessInstructions ?? "—"], [t("Time zone"), "Asia/Kuala_Lumpur"], [t("Last edited"), c.lastEdit ?? "—"]]} />
      </Card>
    );
  }
  const s = sel.space;
  const holds = s.kind === "room" || s.kind === "space";
  return (
    <Card title={s.name} sub={`${t(kindLabel[s.kind])} · ${sel.path} · ${units(t, s.units)}`}
      action={canWrite && <div className="flex flex-wrap gap-2"><Btn size="sm" onClick={() => setM({ kind: "rename", s })}>{t("Rename")}</Btn>
        {!holds && <Btn size="sm" onClick={() => setM({ kind: "space", propertyId: s.propertyId, parentSpaceId: s.id, spaceKind: "room" })}>{t("+ Add room")}</Btn>}
        <Btn size="sm" variant="danger" onClick={() => setM({ kind: "archive", s })}>{t("Delete")}</Btn>
        {holds && active && <Btn size="sm" variant="primary" onClick={() => setM({ kind: "unit", propertyId: s.propertyId, spaceId: s.id })}>{t("+ New unit in {name}", { name: s.name })}</Btn>}</div>}>
      <SummaryList cols={2} items={[[t("Kind"), t(kindLabel[s.kind])], [t("Child locations"), s.children.length || t("none")], [t("Units here"), s.direct], [t("Last edited"), c.lastEdit ?? "—"]]} />
    </Card>
  );
}

function UnitsCard({ c, sel, nav, canWrite, onNew }: { c: CustomerLive; sel: Selection; nav: Nav; canWrite: boolean; onNew: () => void }) {
  const t = useT();
  const [q, setQ] = useState(c.filter.search);
  const rows = c.units.filter((u) => !q.trim() || u.name.toLowerCase().includes(q.trim().toLowerCase()) || u.id.toLowerCase().includes(q.trim().toLowerCase()));
  const filtered = !!c.filter.powerState || c.filter.connections.length > 0;
  const title = sel.kind === "property" ? t("All units in {name}", { name: sel.property.name }) : sel.kind === "space" ? t("Units in {name}", { name: sel.space.name }) : t("Unassigned units in {name}", { name: sel.property.name });
  const conn = (x: Connection) => t(connLabel[x]);
  return (
    <Card title={`${title} · ${filtered || q.trim() ? t("{n} of {total}", { n: rows.length, total: c.total }) : c.total}`} action={canWrite && <Btn size="sm" variant="primary" onClick={onNew}>{t("+ New unit")}</Btn>}>
      {filtered && <div className="mb-3"><Banner action={<Btn size="sm" onClick={() => nav({ powerState: null, connections: null })}>{t("Clear filters")}</Btn>}>
        {t("Filtered by {filters} — clear the filter to see all {units}.", { filters: [c.filter.powerState && t("Power: {state}", { state: t(powerLabel[c.filter.powerState]) }), c.filter.connections.length && t("Connection: {state}", { state: c.filter.connections.map(conn).join(", ") })].filter(Boolean).join(" · "), units: units(t, c.total) })}
      </Banner></div>}
      <div className="mb-3 flex flex-wrap items-center gap-3">
        <div className="min-w-[200px] flex-1 sm:max-w-xs"><Search placeholder={t("Search unit name or ID…")} value={q} onChange={setQ} /></div>
        <Select aria-label={t("Power")} className="w-auto" value={c.filter.powerState ?? ""} onChange={(e) => nav({ powerState: e.target.value || null })}>
          <option value="">{t("Power: All")}</option>{powerStates.map((p) => <option key={p} value={p}>{t("Power: {state}", { state: t(powerLabel[p]) })}</option>)}
        </Select>
        <Select aria-label={t("Connection")} className="w-auto" value={c.filter.connections.length === 1 ? c.filter.connections[0] : ""} onChange={(e) => nav({ connections: e.target.value || null })}>
          <option value="">{t("Connection: All")}</option>{connections.map((x) => <option key={x} value={x}>{t("Connection: {state}", { state: conn(x) })}</option>)}
        </Select>
      </div>
      {c.total === 0 ? <EmptyState title={t("No units here")}>{t(canWrite ? "Register a unit for this location with + New unit." : "No unit is registered at this location.")}</EmptyState> : (
        <DataTable rows={rows} rowKey={(u) => u.id} onRowClick={(u) => nav({ unitId: u.id })} cols={[
          { key: "n", label: t("Unit"), render: (u) => <><b>{u.name}</b><div className="text-xs text-muted">{u.id.slice(0, 8)} · {u.model}</div></> },
          { key: "l", label: t("Location"), render: (u) => u.location, hideBelow: "sm" },
          { key: "p", label: t("Power"), render: (u) => <PowerBadge s={u.power} /> },
          { key: "c", label: t("Connection"), render: (u) => <ConnBadge s={u.conn} />, hideBelow: "sm" },
          { key: "a", label: t("Alerts"), render: (u) => (u.alerts ? <Badge tone="warn" icon="⚠">{u.alerts}</Badge> : "—"), hideBelow: "md" },
          { key: "y", label: t("Policies"), render: (u) => u.policies, hideBelow: "md" },
          { key: "x", label: "", render: () => "›" },
        ]} />
      )}
      <p className="mt-2 text-[11px] text-muted">{t("Pick a room in the tree to list only its units; a row opens the unit edit. Power is the derived state (SR27): unknown never counts as running or stopped.")}</p>
    </Card>
  );
}

// ---- unit edit ----

function UnitEdit({ live, c, x, nav, run, pending }: { live: UnitsLive; c: CustomerLive; x: UnitLive; nav: Nav; run: Run; pending: boolean }) {
  const t = useT();
  const u = x.u;
  const key = `${u.id}:${u.version}`;
  const [source, setSource] = useState(key);
  const [d, setD] = useState<UnitDraft>(unitDraft(u));
  const [tried, setTried] = useState(false);
  const [modal, setModal] = useState<null | "relocate" | "delete" | "attach">(null);
  if (source !== key) { // the form restarts from each saved unit version (adjust state during render)
    setSource(key);
    setD(unitDraft(u));
    setTried(false);
  }
  const ro = !live.canWrite || u.archived;
  const errors = unitErrors(d, live.todayKL, t);
  const err = (k: string) => (tried ? errors[k] : undefined);
  const changes = changedFields(u, d, t);
  const set = (patch: Partial<UnitDraft>) => setD((v) => ({ ...v, ...patch }));
  const place = `${d.propertyId}|${d.spaceId}`;
  const back = x.path.split(" › ").at(-1) ?? t("location");
  const save = () => {
    setTried(true);
    if (Object.keys(errors).length) return;
    if (relocates(u, d)) return setModal("relocate");
    run(() => saveUnit(unitInput(d, c.row.orgId, u.id), u.version), t("Unit saved"));
  };
  const attachedIds = x.attached.filter((p) => p.kind === "alert").map((p) => p.id);
  return (
    <>
      <Card title={<span className="flex flex-wrap items-center gap-2">{u.displayName}<PowerBadge s={u.effectivePowerState === "on" ? "running" : u.effectivePowerState === "off" ? "stopped" : "unknown"} /><ConnBadge s={u.connection} />{x.seen && <span className="text-[11px] font-normal text-muted">{t("seen {time}", { time: x.seen })}</span>}{u.activeAlertCount > 0 && <Badge tone="warn" icon="⚠">{count(t, u.activeAlertCount, "1 open alert", "{n} open alerts")}</Badge>}</span>}
        sub={<>{u.id.slice(0, 8)} · {t("version {v}", { v: u.version })} · <button type="button" className="font-semibold text-primary" onClick={() => nav({ unitId: null, locationId: u.archived ? null : u.spaceId ?? `unassigned:${u.propertyId}` })}>{t("‹ back to {place}", { place: back })}</button></>}
        action={live.canWrite && !u.archived && <Btn size="sm" variant="danger" onClick={() => setModal("delete")}>{t("Delete unit…")}</Btn>}>
        {u.archived && <div className="mb-3"><Banner tone="warn">{t("Read only — {note}. Archived units are excluded from lists, summaries and KPI denominators (IR39).", { note: x.archivedNote ?? "" })}</Banner></div>}
        <div className="grid gap-x-4 gap-y-3 sm:grid-cols-2">
          <Field label={t("Name")} error={err("displayName")}><Input value={d.displayName} disabled={ro} onChange={(e) => set({ displayName: e.target.value })} /></Field>
          <Field label={t("Location")} hint={t("Changing it relocates the unit — a reason is asked on save; the old location stays in the history.")} error={err("propertyId")}>
            <Select value={place} disabled={ro} onChange={(e) => { const [propertyId, spaceId] = e.target.value.split("|"); set({ propertyId, spaceId }); }}>
              {!c.places.some((p) => `${p.propertyId}|${p.spaceId}` === place) && <option value={place}>{t("{place} (current)", { place: x.path })}</option>}
              {c.places.map((p) => <option key={`${p.propertyId}|${p.spaceId}`} value={`${p.propertyId}|${p.spaceId}`}>{p.label}</option>)}
            </Select>
          </Field>
          <Field label={t("Model")} hint={t(c.models.length ? "From capabilities.list · type split (fixed)" : "Models need device.read — the current model stays")} error={err("modelId")}>
            <Select value={d.modelId} disabled={ro || !c.models.length} onChange={(e) => set({ modelId: e.target.value })}>
              {!c.models.some((k) => k.id === d.modelId) && <option value={d.modelId}>{x.model}</option>}
              {c.models.map((k) => <option key={k.id} value={k.id}>{k.label}</option>)}
            </Select>
          </Field>
          <Field label={t("Installed at")} hint={t("A Kuala Lumpur day · leave empty for “Not registered” (IR44)")} error={err("installedAt")}><Input type="date" max={live.todayKL} value={d.installedAt} disabled={ro} onChange={(e) => set({ installedAt: e.target.value })} /></Field>
          <Field label={t("Warranty end")} hint={t("Manufacturer warranty; coverage also counts active maintenance contracts (FR-A19)")} error={err("warrantyEnd")}><Input type="date" min={d.installedAt || undefined} value={d.warrantyEnd} disabled={ro} onChange={(e) => set({ warrantyEnd: e.target.value })} /></Field>
          <Field label={t("IoT device")} hint={x.device ? t("Rebinding is done in Devices and keeps the command history on the unit.") : undefined}>
            <div className="flex min-h-9 items-center gap-2 text-[13px]">{x.device ? <><b>{x.device.serial}</b><ConnBadge s={x.device.connection} /><TextLink href={`/admin/devices?tab=devices&deviceId=${x.device.id}`}>{t("Devices ›")}</TextLink></> : x.deviceKnown ? <span className="text-muted">{t("No device bound")}</span> : <span className="text-muted">{t("Needs device.read")}</span>}</div>
          </Field>
          <Field label={t("Service scope")} hint={t("Target inspection groups")} error={err("serviceScope")}>
            <div className="flex flex-wrap gap-3">{scopes.map((sc) => <Check key={sc} label={t(scopeLabel[sc])} disabled={ro} checked={d.serviceScope.includes(sc)} onChange={(on) => set({ serviceScope: on ? [...d.serviceScope, sc] : d.serviceScope.filter((v) => v !== sc) })} />)}</div>
          </Field>
        </div>
        {!ro && (
          <div className="mt-4 flex flex-wrap items-center justify-end gap-2 border-t border-line pt-3">
            <span className="mr-auto text-[12px] text-muted">{changes.length ? `${count(t, changes.length, "1 unsaved change", "{n} unsaved changes")} · ${changes.join(", ")}` : t("No unsaved changes")}</span>
            <Btn disabled={!changes.length || pending} onClick={() => { setD(unitDraft(u)); setTried(false); }}>{t("Discard")}</Btn>
            <Btn variant="primary" disabled={!changes.length || pending} onClick={save}>{t("Save changes")}</Btn>
          </div>
        )}
      </Card>
      <Card title={t("Alert policies on this unit")} sub={t("Only {name}’s policies can be attached. The unit carries the policies, not the other way round (IR108).", { name: c.row.name })}
        action={c.canAttach && !u.archived && <Btn size="sm" onClick={() => setModal("attach")}>{t("+ Attach policy")}</Btn>}>
        {c.policies === null ? <p className="text-[13px] text-muted">{t("Policies need alert.policy.read.")}</p> : (
          <div className="flex flex-col">
            {x.attached.map((p) => (
              <div key={p.id} className="flex flex-wrap items-center gap-2 border-t border-line py-2.5 text-[13px] first:border-0">
                <div className="min-w-0 flex-1"><div className="flex flex-wrap items-center gap-2"><b>{p.name}</b><Badge tone={p.kind === "default_alert" ? "primary" : "muted"}>{p.type}</Badge>{p.madeBy && <span className="text-[11px] text-muted">{p.madeBy}</span>}</div><div className="text-xs text-muted">{p.condition}{p.on ? ` · ${p.on}` : ""}</div></div>
                <TextLink href={`/admin/alerts?tab=policies&policyId=${p.id}`}>{t(p.kind === "default_alert" ? "View" : "Edit")}</TextLink>
                {p.kind === "alert" && c.canAttach && !u.archived && <Btn size="sm" variant="ghost" disabled={pending} onClick={() => run(() => setUnitPolicies(u.id, u.version, attachedIds.filter((id) => id !== p.id)), t("“{name}” detached", { name: p.name }))}>{t("Detach")}</Btn>}
              </div>
            ))}
          </div>
        )}
      </Card>
      <div className="flex flex-wrap gap-4 text-[13px]">
        {x.device && <TextLink href={`/admin/audit?tab=devices&deviceId=${x.device.id}`}>{t("Device events ›")}</TextLink>}
        <TextLink href="/admin/audit">{t("Audit log ›")}</TextLink>
      </div>
      {modal === "relocate" && <RelocateModal u={u} from={x.path} to={c.places.find((p) => `${p.propertyId}|${p.spaceId}` === place)?.label ?? ""} pending={pending} onClose={() => setModal(null)}
        onSave={(reason) => run(() => saveUnit(unitInput(d, c.row.orgId, u.id, reason), u.version), t("Location changed — the old location stays in the history"), () => setModal(null))} />}
      {modal === "delete" && <DeleteUnitModal x={x} pending={pending} onClose={() => setModal(null)}
        onArchive={(reason) => run(() => archiveUnit(u.id, u.version, reason), t("“{name}” taken out of use", { name: u.displayName }), () => { setModal(null); nav({ unitId: null }); })}
        onDelete={(reason) => run(() => deleteUnit(u.id, u.version, reason), t("“{name}” deleted", { name: u.displayName }), () => { setModal(null); nav({ unitId: null }); })} />}
      {modal === "attach" && <AttachModal c={c} unit={u.displayName} attached={attachedIds} pending={pending} onClose={() => setModal(null)}
        onSave={(ids) => run(() => setUnitPolicies(u.id, u.version, [...attachedIds, ...ids]), count(t, ids.length, "1 policy attached", "{n} policies attached"), () => setModal(null))} />}
    </>
  );
}

// ---- alert policies tab ----

function PoliciesTab({ c, run, pending }: { c: CustomerLive; run: Run; pending: boolean }) {
  const t = useT();
  const router = useRouter();
  if (!c.policies) return <EmptyState title={t("Alert policies need alert.policy.read")}>{t("Ask an identity administrator for Alert policies access.")}</EmptyState>;
  return (
    <>
      <Card title={t("Alert policies of {name}", { name: c.row.name })} sub={t("Policies belong to the customer. Each unit picks which of them it carries (unit edit › Alert policies). Air-quality limits are alert policies too.")}
        action={<LinkBtn size="sm" variant="primary" href="/admin/alerts?tab=policies">{t("+ New policy for {name}", { name: c.row.name })}</LinkBtn>}>
        <DataTable rows={c.policies} rowKey={(p) => p.id} onRowClick={(p) => router.push(`/admin/alerts?tab=policies&policyId=${p.id}`)} cols={[
          { key: "n", label: t("Policy"), render: (p) => <><b>{p.name}</b>{!p.enabled && <Badge className="ml-2">{t("Off")}</Badge>}</> },
          { key: "t", label: t("Type"), render: (p) => <Badge tone={p.kind === "default_alert" ? "primary" : "muted"}>{p.type}</Badge> },
          { key: "c", label: t("Condition"), render: (p) => p.condition, hideBelow: "sm" },
          { key: "m", label: t("Made by"), render: (p) => p.madeBy, hideBelow: "md" },
          { key: "u", label: t("Units"), render: (p) => (p.kind === "default_alert" ? t("All {n}", { n: p.units }) : units(t, p.units)) },
        ]} />
        <p className="mt-2 text-[11px] text-muted">{t("A row opens the policy editor (Alert policies › Policies). The default policy is on every unit; its rules can be switched per customer but it cannot be detached.")}</p>
      </Card>
      {c.rules && (
        <Card title={t("Default policy rules for {name}", { name: c.row.name })} sub={t("Limits are edited once for every customer (Alert policies › Default policy). Here you switch each rule on or off for this customer — the customer’s owner can do the same in the customer app.")}>
          <div className="flex flex-col">
            {c.rules.items.map((r) => (
              <div key={r.ruleKey} className="flex flex-wrap items-center gap-3 border-t border-line py-2.5 text-[13px] first:border-0">
                <div className="min-w-0 flex-1"><b>{r.name}</b><div className="text-xs text-muted">{r.condition}{r.note ? ` · ${r.note}` : ""}</div></div>
                <Toggle on={r.enabled} label={t(r.enabled ? "On" : "Off")} disabled={!c.canRules || pending}
                  onChange={(on) => run(() => setDefaultRule(c.rules!.policyId, r.ruleKey, c.row.id, on, r.version), t(on ? "{rule} switched on for {name}" : "{rule} switched off for {name}", { rule: r.name, name: c.row.name }))} />
              </div>
            ))}
          </div>
          {!c.canRules && <p className="mt-2 text-[11px] text-muted">{t("Switching rules needs alert.policy.write.")}</p>}
        </Card>
      )}
    </>
  );
}

// ---- modals ----

function PropertyModal({ p, pending, onClose, onSave }: { p?: TreeProperty; pending: boolean; onClose: () => void; onSave: (d: PropertyDraft) => void }) {
  const t = useT();
  const [d, setD] = useState<PropertyDraft>(propertyDraft(p));
  const [tried, setTried] = useState(false);
  const e = propertyErrors(d, t);
  const err = (k: string) => (tried ? e[k] : undefined);
  return (
    <Modal open onClose={onClose} title={p ? t("Edit {name}", { name: p.name }) : t("Add property")} footer={<><Btn onClick={onClose}>{t("Cancel")}</Btn><Btn variant="primary" disabled={pending} onClick={() => { setTried(true); if (!Object.keys(e).length) onSave(d); }}>{t(p ? "Save property" : "Add property")}</Btn></>}>
      <div className="flex flex-col gap-3">
        <Field label={t("Kind · required")}><Choice value={d.kind} onChange={(v: "home" | "office") => setD({ ...d, kind: v })} options={[{ id: "home", label: t("Home") }, { id: "office", label: t("Office") }]} /></Field>
        <Field label={t("Name · required")} hint={t("1–120 characters, unique among the customer’s properties")} error={err("name")}><Input value={d.name} onChange={(x) => setD({ ...d, name: x.target.value })} /></Field>
        <Field label={t("Address")} hint={t("0–500 characters · fictional values only; shown as the site address (IR25)")} error={err("address")}><Input value={d.address} onChange={(x) => setD({ ...d, address: x.target.value })} /></Field>
        <Field label={t("Access instructions")} hint={t("0–1000 characters · visible to a contractor only after acceptance, within the valid period")} error={err("accessInstructions")}><Textarea rows={3} value={d.accessInstructions} onChange={(x) => setD({ ...d, accessInstructions: x.target.value })} /></Field>
      </div>
    </Modal>
  );
}

function SpaceModal({ c, init, pending, onClose, onSave }: { c: CustomerLive; init: { propertyId: string; parentSpaceId: string | null; spaceKind: TreeSpace["kind"] }; pending: boolean; onClose: () => void; onSave: (x: { propertyId: string; parentSpaceId: string | null; kind: TreeSpace["kind"]; name: string }) => void }) {
  const t = useT();
  const [propertyId, setPropertyId] = useState(init.propertyId);
  const [parent, setParent] = useState(init.parentSpaceId ?? "");
  const [kind, setKind] = useState<TreeSpace["kind"]>(init.spaceKind);
  const [name, setName] = useState("");
  const [tried, setTried] = useState(false);
  const p = c.tree.find((x) => x.id === propertyId);
  const parents = p ? flatten(p.spaces) : [];
  const e = nameError(name, t);
  return (
    <Modal open onClose={onClose} title={t("Add location")} footer={<><Btn onClick={onClose}>{t("Cancel")}</Btn><Btn variant="primary" disabled={pending} onClick={() => { setTried(true); if (!e) onSave({ propertyId, parentSpaceId: parent || null, kind, name: name.trim() }); }}>{t(addKind[kind])}</Btn></>}>
      <div className="flex flex-col gap-3">
        <Field label={t("Property")}><Select value={propertyId} onChange={(x) => { setPropertyId(x.target.value); setParent(""); }}>{c.tree.map((x) => <option key={x.id} value={x.id}>{x.name}</option>)}</Select></Field>
        <Field label={t("Kind · required")} hint={t("floor · area · room · space (under the parent you pick)")}><Select value={kind} onChange={(x) => setKind(x.target.value as TreeSpace["kind"])}>{(["floor", "area", "room", "space"] as const).map((k) => <option key={k} value={k}>{t(kindLabel[k])}</option>)}</Select></Field>
        <Field label={t("Parent")} hint={t("Same property only; floors and areas usually sit directly under the property")}>
          <Select value={parent} onChange={(x) => setParent(x.target.value)}><option value="">{t("{name} (top level)", { name: p?.name ?? t("Property") })}</option>{parents.map((x) => <option key={x.id} value={x.id}>{x.name} ({t(kindLabel[x.kind])})</option>)}</Select>
        </Field>
        <Field label={t("Name · required")} hint={t("1–120 characters, unique among its siblings")} error={tried ? e : undefined}><Input value={name} onChange={(x) => setName(x.target.value)} /></Field>
        <p className="text-[11px] text-muted">{t("A parent in another property, or a parent change that creates a cycle, is rejected with VALIDATION.")}</p>
      </div>
    </Modal>
  );
}

function RenameModal({ name, where, pending, onClose, onSave }: { name: string; where: string; pending: boolean; onClose: () => void; onSave: (name: string) => void }) {
  const t = useT();
  const [v, setV] = useState(name);
  const [tried, setTried] = useState(false);
  const e = nameError(v, t);
  return (
    <Modal open onClose={onClose} title={t("Rename {name}", { name })} footer={<><Btn onClick={onClose}>{t("Cancel")}</Btn><Btn variant="primary" disabled={pending || v.trim() === name} onClick={() => { setTried(true); if (!e) onSave(v.trim()); }}>{t("Save name")}</Btn></>}>
      <div className="flex flex-col gap-3">
        <p className="text-[13px] text-muted">{t("Only the name changes; units, child locations and history stay attached. The customer sees the new name immediately.")}</p>
        <SummaryList items={[[t("Location"), `${name} · ${where}`]]} />
        <Field label={t("New name")} hint={t("1–120 characters, unique among its siblings. Saved with a version check — an older version is CONFLICT.")} error={tried ? e : undefined}><Input value={v} onChange={(x) => setV(x.target.value)} /></Field>
      </div>
    </Modal>
  );
}

function ArchiveLocationModal({ p, s, pending, onClose, onSave }: { p?: TreeProperty; s?: TreeSpace; pending: boolean; onClose: () => void; onSave: (reason: string) => void }) {
  const t = useT();
  const [reason, setReason] = useState("");
  const [tried, setTried] = useState(false);
  const name = p?.name ?? s?.name ?? "";
  const children = p ? p.spaces.length : s?.children.length ?? 0;
  const held = p ? p.units : s?.units ?? 0;
  const blocked = children > 0 || held > 0;
  const e = reasonError(reason, t);
  return (
    <Modal open onClose={onClose} title={t("Delete {name}?", { name })} footer={<><Btn onClick={onClose}>{t("Cancel")}</Btn><Btn variant="danger" disabled={pending || blocked} onClick={() => { setTried(true); if (!e) onSave(reason.trim()); }}>{t(p ? "Delete property" : deleteKind[s!.kind])}</Btn></>}>
      <div className="flex flex-col gap-3">
        <p className="text-[13px] text-muted">{t("Deleting takes the location out of the customer’s tree (archived). Its history stays in the audit log.")}</p>
        {blocked && <Banner tone="crit">{t("CONFLICT — {name} still contains {what}. Move them (unit › Location) or delete them first.", { name, what: [children && count(t, children, "1 location", "{n} locations"), held && units(t, held)].filter(Boolean).join(t(" and ")) })}</Banner>}
        <Field label={t("Reason · required")} hint={t("1–1000 characters")} error={tried ? e : undefined}><Textarea rows={2} value={reason} disabled={blocked} onChange={(x) => setReason(x.target.value)} /></Field>
      </div>
    </Modal>
  );
}

function NewUnitModal({ live, c, init, pending, onClose, onSave }: { live: UnitsLive; c: CustomerLive; init: { propertyId: string; spaceId: string }; pending: boolean; onClose: () => void; onSave: (d: UnitDraft) => void }) {
  const t = useT();
  const [d, setD] = useState<UnitDraft>({ ...unitDraft(undefined, init.propertyId, init.spaceId), modelId: c.models[0]?.id ?? "" });
  const [tried, setTried] = useState(false);
  const e = unitErrors(d, live.todayKL, t);
  const err = (k: string) => (tried ? e[k] : undefined);
  const set = (patch: Partial<UnitDraft>) => setD((v) => ({ ...v, ...patch }));
  const p = c.tree.find((x) => x.id === d.propertyId);
  return (
    <Modal open wide onClose={onClose} title={t("New unit")} footer={<><Btn onClick={onClose}>{t("Cancel")}</Btn><Btn variant="primary" disabled={pending} onClick={() => { setTried(true); if (!Object.keys(e).length) onSave(d); }}>{t("Register unit")}</Btn></>}>
      <div className="grid gap-x-4 gap-y-3 sm:grid-cols-2">
        <p className="text-[12px] text-muted sm:col-span-2">{t("customerOrgId, spaceId and modelId must agree; the tenant comes from the session (IR74).")}</p>
        <Field label={t("Customer · fixed")}><Input value={c.row.name} disabled /></Field>
        <Field label={t("Location · required")} error={err("propertyId")}><Select value={d.propertyId} onChange={(x) => set({ propertyId: x.target.value, spaceId: "" })}>{c.tree.map((x) => <option key={x.id} value={x.id}>{x.name}</option>)}</Select></Field>
        <Field label={t("Room / space · optional")} hint={t("Same property only. Empty = not assigned (IR62).")}>
          <Select value={d.spaceId} onChange={(x) => set({ spaceId: x.target.value })}><option value="">{t("— not assigned —")}</option>{(p ? flatten(p.spaces) : []).map((x) => <option key={x.id} value={x.id}>{x.name} ({t(kindLabel[x.kind])})</option>)}</Select>
        </Field>
        <Field label={t("Model · required")} hint={t(c.models.length ? "From capabilities.list" : "Models need device.read")} error={err("modelId")}>
          <Select value={d.modelId} onChange={(x) => set({ modelId: x.target.value })}>{!c.models.length && <option value="">{t("no model available")}</option>}{c.models.map((k) => <option key={k.id} value={k.id}>{k.label}</option>)}</Select>
        </Field>
        <Field label={t("Display name · required")} hint={t("1–120 characters, unique in its room")} error={err("displayName")}><Input value={d.displayName} onChange={(x) => set({ displayName: x.target.value })} /></Field>
        <Field label={t("Installed at · optional")} hint={t("A Kuala Lumpur day · leave empty for “Not registered” (IR44)")} error={err("installedAt")}><Input type="date" max={live.todayKL} value={d.installedAt} onChange={(x) => set({ installedAt: x.target.value })} /></Field>
        <Field label={t("Warranty end · optional")} error={err("warrantyEnd")}><Input type="date" min={d.installedAt || undefined} value={d.warrantyEnd} onChange={(x) => set({ warrantyEnd: x.target.value })} /></Field>
        <Field label={t("Service scope · required · type: split (fixed)")} error={err("serviceScope")} className="sm:col-span-2">
          <div className="flex flex-wrap gap-3">{scopes.map((sc) => <Check key={sc} label={t(scopeLabel[sc])} checked={d.serviceScope.includes(sc)} onChange={(on) => set({ serviceScope: on ? [...d.serviceScope, sc] : d.serviceScope.filter((v) => v !== sc) })} />)}</div>
        </Field>
      </div>
    </Modal>
  );
}

function RelocateModal({ u, from, to, pending, onClose, onSave }: { u: ApiUnitDetail; from: string; to: string; pending: boolean; onClose: () => void; onSave: (reason: string) => void }) {
  const t = useT();
  const [reason, setReason] = useState("");
  const [tried, setTried] = useState(false);
  const e = reasonError(reason, t);
  return (
    <Modal open onClose={onClose} title={t("Change location")} footer={<><Btn onClick={onClose}>{t("Cancel")}</Btn><Btn variant="primary" disabled={pending} onClick={() => { setTried(true); if (!e) onSave(reason.trim()); }}>{t("Save location")}</Btn></>}>
      <div className="flex flex-col gap-3">
        <SummaryList items={[[t("Unit"), `${u.displayName} · ${t("version {v}", { v: u.version })}`], [t("Current"), from], [t("New"), to]]} />
        <Field label={t("Change reason · required")} hint={t("1–1000 characters. Saved with the before/after values in the audit log.")} error={tried ? e : undefined}><Textarea rows={2} value={reason} onChange={(x) => setReason(x.target.value)} /></Field>
        <p className="text-[11px] text-muted">{t("Keeps the unit ID; version {from} → {to}. Alert policies stay attached to the unit.", { from: u.version, to: u.version + 1 })}</p>
      </div>
    </Modal>
  );
}

function DeleteUnitModal({ x, pending, onClose, onArchive, onDelete }: { x: UnitLive; pending: boolean; onClose: () => void; onArchive: (reason: string) => void; onDelete: (reason: string) => void }) {
  const t = useT();
  const [reason, setReason] = useState("");
  const [tried, setTried] = useState(false);
  const e = reasonError(reason, t);
  const blocked = !!x.blockers?.length;
  const go = (fn: (r: string) => void) => { setTried(true); if (!e) fn(reason.trim()); };
  return (
    <Modal open onClose={onClose} title={t("Delete unit")} footer={<><Btn onClick={onClose}>{t("Close")}</Btn><Btn variant="danger" disabled={pending || blocked} onClick={() => go(onArchive)}>{t("Delete unit")}</Btn></>}>
      <div className="flex flex-col gap-3">
        <p className="text-[13px] text-muted">{t("Removes the unit from lists, totals and candidates. The ID and history are kept (archived); active automations are disabled with reason unit_archived (IR39).")}</p>
        <SummaryList items={[[t("Unit"), `${x.u.displayName} · ${t("version {v}", { v: x.u.version })}`], [t("Location"), x.path]]} />
        {blocked && (
          <Banner tone="crit"><b>{t("CONFLICT · this unit is still in use")}</b>
            <ul className="mt-1 list-disc pl-4">{x.blockers!.map((b) => <li key={b.label}>{b.href ? <TextLink href={b.href}>{b.label}</TextLink> : b.label}</li>)}</ul>
            <div className="mt-1 text-[12px]">{t("End or move these first. Ended history alone does not block deletion.")}</div>
          </Banner>
        )}
        {x.blockers === null && <Banner>{t("The blockers (contracts, jobs, device binding) are not visible with your permissions — the Core API checks them on delete.")}</Banner>}
        <Field label={t("Reason · required")} hint={t("1–1000 characters")} error={tried ? e : undefined}><Textarea rows={2} value={reason} onChange={(v) => setReason(v.target.value)} /></Field>
        <details className="text-[12px] text-muted">
          <summary className="cursor-pointer font-semibold">{t("Registered by mistake?")}</summary>
          <p className="mt-1">{t("A unit that never had contracts, jobs or IoT can be removed completely (units.delete); otherwise this is CONFLICT and the unit has to be deleted as above.")}</p>
          <Btn size="sm" variant="ghost" className="mt-1" disabled={pending} onClick={() => go(onDelete)}>{t("Remove permanently")}</Btn>
        </details>
      </div>
    </Modal>
  );
}

function AttachModal({ c, unit, attached, pending, onClose, onSave }: { c: CustomerLive; unit: string; attached: string[]; pending: boolean; onClose: () => void; onSave: (ids: string[]) => void }) {
  const t = useT();
  const [pick, setPick] = useState<string[]>([]);
  const options = (c.policies ?? []).filter((p) => p.kind === "alert");
  return (
    <Modal open onClose={onClose} title={t("Attach policies to {unit}", { unit })} footer={<><Btn onClick={onClose}>{t("Cancel")}</Btn><Btn variant="primary" disabled={pending || !pick.length} onClick={() => onSave(pick)}>{pick.length ? count(t, pick.length, "Attach 1 policy", "Attach {n} policies") : t("Attach policy")}</Btn></>}>
      <div className="flex flex-col gap-2">
        <p className="text-[13px] text-muted">{t("Showing {name}’s policies only. The default policy is always on and not listed.", { name: c.row.name })}</p>
        {options.length === 0 && <EmptyState title={t("No customer policies yet")}>{t("Create one in Alert policies first.")}</EmptyState>}
        {options.map((p) => {
          const already = attached.includes(p.id);
          return (
            <div key={p.id} className={cx("rounded-xl border border-line px-3 py-2", already && "bg-surface2/60")}>
              <Check disabled={already} checked={already || pick.includes(p.id)} onChange={(on) => setPick((v) => (on ? [...v, p.id] : v.filter((x) => x !== p.id)))}
                label={<span><b>{p.name}</b> <Badge>{p.type}</Badge>{!p.enabled && <Badge className="ml-1">{t("Off")}</Badge>}<span className="block text-xs text-muted">{p.condition}{already ? ` · ${t("Already attached")}` : ""}</span></span>} />
            </div>
          );
        })}
        <p className="text-[11px] text-muted">{t("Disabled policies can be attached; they start alerting once enabled.")} <TextLink href="/admin/alerts?tab=policies">{t("+ New policy for {name}", { name: c.row.name })}</TextLink></p>
      </div>
    </Modal>
  );
}

// ---- client users (FR-A17) ----

function UsersTab({ live, c, run, pending }: { live: UnitsLive; c: CustomerLive; run: Run; pending: boolean }) {
  const t = useT();
  const [invite, setInvite] = useState(false);
  const [change, setChange] = useState<null | { u: ClientUserRow; kind: "role" | "status" | "remove" }>(null);
  const canWrite = live.canWrite;
  const act = (u: ClientUserRow, a: string) => {
    if (a === "role" || a === "status" || a === "remove") setChange({ u, kind: a });
    if (a === "resend") run(() => resendInvite(u.id), t("Invitation preview re-created for {email} — nothing is sent in the demo", { email: u.email }));
    if (a === "reset") run(() => previewPasswordReset(u.email), t("Password reset preview created — the same generic message for every address"));
  };
  return (
    <Card title={t("Client users of {name}", { name: c.row.name })} sub={t("People who sign in to the customer app. They only ever see {name}’s properties and units.", { name: c.row.name })}
      action={canWrite && <Btn size="sm" variant="primary" onClick={() => setInvite(true)}>{t("+ Invite user")}</Btn>}>
      {c.users.length === 0 ? <EmptyState title={t("No client users yet")}>{t("Invite the customer’s owner first; the owner can then invite members from the customer app.")}</EmptyState> : (
        <DataTable rows={c.users} rowKey={(u) => u.id} cols={[
          { key: "u", label: t("User"), render: (u) => <><b>{u.name ?? u.email}</b><div className="text-xs text-muted">{u.sub}</div></> },
          { key: "r", label: t("Role"), render: (u) => <Badge tone={u.role === "owner" ? "primary" : "muted"}>{t(u.role === "owner" ? "Owner" : "Member")}</Badge> },
          { key: "s", label: t("Status"), render: (u) => <Badge tone={u.status === "active" ? "ok" : u.status === "invited" ? "warn" : "muted"}>{t(u.status === "active" ? "Active" : u.status === "invited" ? "Invite pending" : "Disabled")}</Badge> },
          { key: "l", label: t("Last sign-in"), render: (u) => u.lastSignIn, hideBelow: "md" },
          { key: "c", label: t("Notification channels"), render: (u) => u.channels, hideBelow: "md" },
          { key: "a", label: "", render: (u) => canWrite && (
            <Select aria-label={t("Actions for {email}", { email: u.email })} className="w-auto" value="" disabled={pending} onChange={(e) => act(u, e.target.value)}>
              <option value="">⋯</option>
              {!u.lastOwner && <option value="role">{t("Change role (Owner / Member)")}</option>}
              {u.status === "invited" && <option value="resend">{t("Resend invite")}</option>}
              <option value="reset">{t("Reset password")}</option>
              {u.status !== "invited" && !u.lastOwner && <option value="status">{t(u.status === "disabled" ? "Enable sign-in" : "Disable sign-in")}</option>}
              {!u.lastOwner && <option value="remove">{t("Remove from customer")}</option>}
            </Select>
          ) },
        ]} />
      )}
      <p className="mt-2 text-[11px] text-muted">{t("Client users have no permission editor — the owner can also invite members from the customer app (Users). HQ, contractor and technician accounts are managed in Access & roles. The last active owner cannot be demoted, disabled or removed.")}</p>
      {invite && <InviteModal c={c} pending={pending} onClose={() => setInvite(false)} onSave={(email, role) => run(() => inviteClientUser(c.row.id, email, role), t("Invitation preview created for {email}", { email: email.trim() }), () => setInvite(false))} />}
      {change && <ClientUserModal change={change} pending={pending} onClose={() => setChange(null)} onSave={(role, reason) => {
        const { u, kind } = change;
        const close = () => setChange(null);
        if (kind === "remove") return run(() => removeClientUser(u.id, u.version, reason), t("{email} removed from {name}", { email: u.email, name: c.row.name }), close);
        const status = kind === "status" ? (u.status === "disabled" ? "active" : "disabled") : undefined;
        run(() => updateClientUser(u.id, u.version, { customerId: c.row.id, email: u.email, clientRole: role, ...(status ? { status } : {}), reason }),
          t(kind === "role" ? (role === "owner" ? "Role changed to Owner" : "Role changed to Member") : status === "active" ? "Sign-in enabled" : "Sign-in disabled"), close);
      }} />}
    </Card>
  );
}

function InviteModal({ c, pending, onClose, onSave }: { c: CustomerLive; pending: boolean; onClose: () => void; onSave: (email: string, role: "owner" | "member") => void }) {
  const t = useT();
  const [email, setEmail] = useState("");
  const [role, setRole] = useState<"owner" | "member">("member");
  const [tried, setTried] = useState(false);
  const e = inviteError(email, c.users, t);
  return (
    <Modal open onClose={onClose} title={t("Invite client user")} footer={<><Btn onClick={onClose}>{t("Cancel")}</Btn><Btn variant="primary" disabled={pending} onClick={() => { setTried(true); if (!e) onSave(email, role); }}>{t("Send invite")}</Btn></>}>
      <div className="flex flex-col gap-3">
        <Field label={t("Email")} hint={t("Unique per customer; the demo creates an invite preview and sends nothing")} error={tried ? e : undefined}><Input type="email" value={email} onChange={(x) => setEmail(x.target.value)} /></Field>
        <Field label={t("Role")}><Choice value={role} onChange={(v: "owner" | "member") => setRole(v)} options={[{ id: "member", label: t("Member") }, { id: "owner", label: t("Owner") }]} /></Field>
      </div>
    </Modal>
  );
}

function ClientUserModal({ change, pending, onClose, onSave }: { change: { u: ClientUserRow; kind: "role" | "status" | "remove" }; pending: boolean; onClose: () => void; onSave: (role: "owner" | "member", reason: string) => void }) {
  const t = useT();
  const { u, kind } = change;
  const [role, setRole] = useState<"owner" | "member">(u.role === "owner" ? "member" : "owner");
  const [reason, setReason] = useState("");
  const [tried, setTried] = useState(false);
  const e = reasonError(reason, t);
  const title = t(kind === "role" ? "Change role of {email}" : kind === "remove" ? "Remove {email}?" : u.status === "disabled" ? "Enable sign-in for {email}" : "Disable sign-in for {email}", { email: u.email });
  return (
    <Modal open onClose={onClose} title={title} footer={<><Btn onClick={onClose}>{t("Cancel")}</Btn><Btn variant={kind === "remove" ? "danger" : "primary"} disabled={pending} onClick={() => { setTried(true); if (!e) onSave(kind === "role" ? role : u.role, reason.trim()); }}>{t(kind === "remove" ? "Remove user" : "Save")}</Btn></>}>
      <div className="flex flex-col gap-3">
        {kind === "role" && <Field label={t("New role")}><Choice value={role} onChange={(v: "owner" | "member") => setRole(v)} options={[{ id: "member", label: t("Member") }, { id: "owner", label: t("Owner") }]} /></Field>}
        {kind === "remove" && <p className="text-[13px] text-muted">{t("The user loses access to the customer app; their membership ends now. The history stays in the audit log.")}</p>}
        {kind === "status" && <p className="text-[13px] text-muted">{t(u.status === "disabled" ? "The user can sign in again." : "The user cannot sign in until sign-in is enabled again; the membership ends now.")}</p>}
        <Field label={t("Reason · required")} hint={t("1–1000 characters, stored in the audit log")} error={tried ? e : undefined}><Textarea rows={2} value={reason} onChange={(x) => setReason(x.target.value)} /></Field>
      </div>
    </Modal>
  );
}

// ---- warranty & coverage (FR-A19) ----

const coverageTone: Record<CoverageRow["status"], "ok" | "primary" | "warn" | "crit"> = { contract: "ok", under_warranty: "primary", expiring: "warn", no_coverage: "crit" };
const resultLabel: Record<"ready" | "warning" | "error", string> = { ready: "ready", warning: "warning", error: "error" };
function download(name: string, text: string) {
  const url = URL.createObjectURL(new Blob([text], { type: "text/csv;charset=utf-8" }));
  const a = document.createElement("a");
  a.href = url;
  a.download = name;
  a.click();
  URL.revokeObjectURL(url);
}

function WarrantyTab({ live }: { live: UnitsLive }) {
  const t = useT();
  const [pending, run] = useAction();
  const [f, setF] = useState<CoverageFilter>({ customerId: "", coverage: "", within: "" });
  const [claim, setClaim] = useState<ClaimCandidate | null>(null);
  const w = live.warranty;
  if (!w) return <EmptyState title={t("Coverage needs asset.read or contract.read")}>{t("Ask an identity administrator for Customers & units or Contracts access.")}</EmptyState>;
  const rows = w.rows.filter((r) => coverageMatch(r, f));
  const k = w.kpis;
  return (
    <>
      <div className="flex flex-wrap items-center gap-3">
        <Select aria-label={t("Customer")} className="w-auto" value={f.customerId} onChange={(e) => setF({ ...f, customerId: e.target.value })}>
          <option value="">{t("Customer: All")}</option>{w.customers.map((x) => <option key={x.id} value={x.id}>{x.name}</option>)}
        </Select>
        <Select aria-label={t("Coverage")} className="w-auto" value={f.coverage} onChange={(e) => setF({ ...f, coverage: e.target.value as CoverageFilter["coverage"] })}>
          <option value="">{t("Coverage: All")}</option><option value="contract">{t("Maintenance contract")}</option><option value="under_warranty">{t("Under warranty")}</option><option value="expiring">{t("Ending within 90 days")}</option><option value="no_coverage">{t("No coverage")}</option>
        </Select>
        <Select aria-label={t("Ends within")} className="w-auto" value={f.within} onChange={(e) => setF({ ...f, within: e.target.value as CoverageFilter["within"] })}>
          <option value="">{t("Ends within: any")}</option>{["30", "90", "180", "365"].map((d) => <option key={d} value={d}>{t("Ends within: {n} days", { n: d })}</option>)}
        </Select>
      </div>
      <div className="grid-fluid" style={{ ["--min" as string]: "180px" }}>
        <Kpi label={t("Under warranty")} value={k.underWarranty} sub={t("of {units}", { units: units(t, k.total) })} />
        <Kpi label={t("Warranty ends ≤ 90 days")} value={k.within90} tone={k.within90 ? "warn" : undefined} sub={t("{n} within 30 days", { n: k.within30 })} />
        <Kpi label={t("Out of warranty, no contract")} value={k.noCoverage} tone={k.noCoverage ? "crit" : undefined} sub={t("Offer maintenance")} />
        <Kpi label={t("Maintenance contract")} value={k.contract} sub={k.contractNames} />
      </div>
      <Card title={t("Units by coverage end")} sub={t("{n} of {total}", { n: rows.length, total: w.rows.length })} action={<Btn size="sm" disabled={!rows.length} onClick={() => download("warranty-coverage.csv", coverageCsv(rows))}>{t("Export CSV")}</Btn>}>
        <DataTable rows={rows} rowKey={(r) => r.unitId} cols={[
          { key: "u", label: t("Unit"), render: (r) => <><b>{r.unit}</b><div className="text-xs text-muted">{r.sub}</div></> },
          { key: "c", label: t("Customer"), render: (r) => r.customer, hideBelow: "sm" },
          { key: "m", label: t("Model"), render: (r) => r.model, hideBelow: "md" },
          { key: "w", label: t("Warranty end"), render: (r) => r.ends },
          { key: "k", label: t("Maintenance contract"), render: (r) => r.contracts, hideBelow: "md" },
          { key: "s", label: t("Status"), render: (r) => <Badge tone={coverageTone[r.status]}>{r.statusText}</Badge> },
          { key: "a", label: t("Action"), render: (r) => (r.contractIds.length
            ? <TextLink href={`/admin/billing/contracts?contractId=${r.contractIds[0]}`}>{t("Open contract")}</TextLink>
            : <TextLink href={`/admin/billing/contracts?customerId=${r.customerId}&unitId=${r.unitId}`}>{t("Renewal offer")}</TextLink>) },
        ]} />
        <p className="mt-2 text-[11px] text-muted">{t("The warranty end is set on the unit (unit edit › Warranty end) or by the CSV import. Coverage = an active maintenance contract, otherwise the warranty; fewer than 90 days left counts as ending (IR111).")}</p>
      </Card>
      <Card title={t("Warranty on jobs")} sub={t("Jobs completed while the unit’s warranty ran whose accepted report lists replaced parts — the parts cost is claimable from the manufacturer until a claim is filed.")}>
        {w.claims === null ? <p className="text-[13px] text-muted">{t("Claimable jobs need job.read.")}</p> : w.claims.length === 0 ? <p className="text-[13px] text-muted">{t("No claimable job right now.")}</p> : (
          <div className="flex flex-col">
            {w.claims.map((x) => (
              <div key={x.jobId} className="flex flex-wrap items-center gap-3 border-t border-line py-2.5 text-[13px] first:border-0">
                <div className="min-w-0 flex-1"><b>{x.unit}</b> <span className="text-muted">· {t("{type} job {id} · completed {date}", { type: typeLabel(x.type, t), id: x.jobId.slice(0, 8), date: x.completed })}</span><div className="text-xs text-muted">{t("Parts: {parts} · Claim {amount} · not filed", { parts: x.parts, amount: x.amountMinor ? amount(x.amountMinor, x.currency) : t("amount to enter") })}</div></div>
                <TextLink href={`/admin/jobs?jobId=${x.jobId}`}>{t("Job ›")}</TextLink>
                {w.canClaim && <Btn size="sm" disabled={pending} onClick={() => setClaim(x)}>{t("Mark claim filed")}</Btn>}
              </div>
            ))}
          </div>
        )}
      </Card>
      {claim && <ClaimModal x={claim} pending={pending} onClose={() => setClaim(null)}
        onSave={(label, minor, reason) => run(() => recordWarrantyClaim(claim.jobId, claim.version, label, minor, claim.currency, reason), t("Warranty claim filed for {unit}", { unit: claim.unit }), () => setClaim(null))} />}
    </>
  );
}

function ClaimModal({ x, pending, onClose, onSave }: { x: ClaimCandidate; pending: boolean; onClose: () => void; onSave: (label: string, amountMinor: number, reason: string) => void }) {
  const t = useT();
  const [label, setLabel] = useState(x.partLabel);
  const [value, setValue] = useState(x.amountMinor ? (x.amountMinor / 100).toFixed(2) : "");
  const [reason, setReason] = useState("");
  const [tried, setTried] = useState(false);
  const minor = Math.round(Number(value) * 100);
  const e = { label: nameError(label, t), value: /^\d+(\.\d{1,2})?$/.test(value.trim()) && minor > 0 ? undefined : t("An amount above 0 with at most 2 decimals"), reason: reasonError(reason, t) };
  return (
    <Modal open onClose={onClose} title={t("File warranty claim · {unit}", { unit: x.unit })} footer={<><Btn onClick={onClose}>{t("Cancel")}</Btn><Btn variant="primary" disabled={pending} onClick={() => { setTried(true); if (!e.label && !e.value && !e.reason) onSave(label, minor, reason.trim()); }}>{t("Mark claim filed")}</Btn></>}>
      <div className="flex flex-col gap-3">
        <SummaryList items={[[t("Job"), t("{type} · {id} · completed {date}", { type: typeLabel(x.type, t), id: x.jobId.slice(0, 8), date: x.completed })], [t("Parts in the report"), x.parts]]} />
        <Field label={t("Part · required")} hint={t("1–120 characters")} error={tried ? e.label : undefined}><Input value={label} onChange={(v) => setLabel(v.target.value)} /></Field>
        <Field label={t("Claim amount ({currency}) · required", { currency: x.currency })} error={tried ? e.value : undefined}><Input inputMode="decimal" value={value} onChange={(v) => setValue(v.target.value)} /></Field>
        <Field label={t("Reason · required")} hint={t("1–1000 characters, e.g. the manufacturer’s claim reference")} error={tried ? e.reason : undefined}><Textarea rows={2} value={reason} onChange={(v) => setReason(v.target.value)} /></Field>
      </div>
    </Modal>
  );
}

// ---- CSV import (FR-A18) ----

function ImportWizard({ live, onClose }: { live: UnitsLive; onClose: () => void }) {
  const i = useI18n(), { t } = i;
  const [pending, run] = useAction();
  const [customerId, setCustomerId] = useState(live.activeCustomers[0]?.id ?? "");
  const [file, setFile] = useState<{ name: string; text: string; size: number } | null>(null);
  const [mapping, setMapping] = useState<Record<string, string>>({});
  const [preview, setPreview] = useState<ApiImportPreview | null>(null);
  const [result, setResult] = useState<ApiUnitImport | null>(null);
  const [undoReason, setUndoReason] = useState("");
  const [tried, setTried] = useState(false);
  const shape = file ? csvShape(file.text) : null;
  const tooBig = !!file && file.size > 900_000;
  const missing = importFields.filter((x) => x.required && !mapping[x.key]);
  const customer = live.activeCustomers.find((x) => x.id === customerId)?.name ?? "";
  const pick = async (f: File | undefined) => {
    if (!f) return;
    const text = await f.text();
    setFile({ name: f.name, text, size: f.size });
    setMapping(autoMapping(csvShape(text).header));
    setPreview(null);
  };
  const validate = () => {
    setTried(true);
    if (!file || !customerId || missing.length || tooBig) return;
    run(() => importPreview(customerId, file.name, file.text, Object.fromEntries(Object.entries(mapping).filter(([, v]) => v))),
      (p) => t("{n} of {total} rows can be imported", { n: p.readyCount + p.warningCount, total: p.rows.length }), setPreview);
  };
  if (result) {
    const undoable = result.state === "imported" && Date.parse(result.undoUntil) > Date.parse(live.now); // the business clock of the last render
    return (
      <Modal open wide onClose={onClose} title={t(result.state === "undone" ? "Import undone" : "Units imported")} footer={<Btn onClick={onClose}>{t("Close")}</Btn>}>
        <div className="flex flex-col gap-3">
          <Banner tone={result.state === "undone" ? "warn" : "ok"}>{result.state === "undone"
            ? t("The imported units, rooms and properties are archived again and their devices unbound.")
            : t("{units} created for {customer} · {locations} and {properties} added · {rows} skipped.", { units: units(t, result.createdUnitIds.length), customer, locations: count(t, result.createdSpaceIds.length, "1 location", "{n} locations"), properties: count(t, result.createdPropertyIds.length, "1 property", "{n} properties"), rows: count(t, result.skippedRowNumbers.length, "1 row", "{n} rows") })}</Banner>
          {result.state === "imported" && <SummaryList items={[[t("Skipped rows"), result.skippedRowNumbers.join(", ") || t("none")], [t("Undo possible until"), t("{time} — only while no unit has telemetry or jobs", { time: showTime(result.undoUntil, i.display) })]]} />}
          {undoable && <>
            <Field label={t("Undo reason")} error={tried && reasonError(undoReason, t) ? reasonError(undoReason, t) : undefined}><Textarea rows={2} value={undoReason} onChange={(v) => setUndoReason(v.target.value)} /></Field>
            <div><Btn variant="danger" disabled={pending} onClick={() => { setTried(true); if (!reasonError(undoReason, t)) run(() => importUndo(result.id, result.version, undoReason), t("Import undone"), setResult); }}>{t("Undo import")}</Btn></div>
          </>}
        </div>
      </Modal>
    );
  }
  if (preview) {
    const importable = preview.readyCount + preview.warningCount;
    return (
      <Modal open wide onClose={onClose} title={t("Import units from CSV · 2 / 2 — preview")} footer={<>
        <Btn onClick={() => setPreview(null)}>{t("← Back")}</Btn>
        <Btn disabled={!preview.errorCount && !preview.warningCount} onClick={() => download(`${preview.fileName.replace(/\.csv$/i, "")}-report.csv`, errorReportCsv(preview.rows, t))}>{t("Download error report")}</Btn>
        <Btn variant="primary" disabled={pending || !importable} onClick={() => run(() => importCommit(preview.previewId, customerId), (x) => t("{units} imported", { units: units(t, x.createdUnitIds.length) }), setResult)}>{count(t, importable, "Import 1 valid row", "Import {n} valid rows")}</Btn>
      </>}>
        <div className="flex flex-col gap-3">
          <div className="flex flex-wrap items-center gap-2 text-[13px]"><Badge tone="ok">{t("{n} ready", { n: preview.readyCount })}</Badge><Badge tone="warn">{count(t, preview.warningCount, "1 warning", "{n} warnings")}</Badge><Badge tone="crit">{count(t, preview.errorCount, "1 error", "{n} errors")}</Badge><span className="text-muted">{customer} · {preview.fileName}</span></div>
          <DataTable rows={preview.rows} rowKey={(r) => String(r.rowNumber)} cols={[
            { key: "n", label: t("Row"), render: (r) => r.rowNumber },
            { key: "l", label: t("Location / unit"), render: (r) => <><b>{importPlace(r)}</b><div className="text-xs text-muted">{r.unitName || "—"}</div></> },
            { key: "m", label: t("Model · serial"), render: (r) => `${r.modelCode || "—"} · ${r.serial ?? "—"}`, hideBelow: "sm" },
            { key: "r", label: t("Result"), render: (r) => <span className="flex items-start gap-2"><Badge tone={r.result === "ready" ? "ok" : r.result === "warning" ? "warn" : "crit"}>{t(resultLabel[r.result])}</Badge><span className="text-xs">{importMessage(r, t)}</span></span> },
          ]} />
          <p className="text-[11px] text-muted">{t("Nothing is written until you import. Error rows are skipped; the import runs as one change set, is recorded in the audit log and can be undone for 24 h while no unit has telemetry or jobs. The preview expires after 30 minutes.")}</p>
        </div>
      </Modal>
    );
  }
  return (
    <Modal open wide onClose={onClose} title={t("Import units from CSV · 1 / 2")} footer={<><Btn onClick={onClose}>{t("Cancel")}</Btn><Btn variant="primary" disabled={pending} onClick={validate}>{t("Validate →")}</Btn></>}>
      <div className="flex flex-col gap-3">
        <Field label={t("Customer")} hint={t("Rows are created under this customer only. Properties, floors and rooms that do not exist yet are created.")} error={tried && !customerId ? t("Choose a customer") : undefined}>
          <Select value={customerId} onChange={(v) => { setCustomerId(v.target.value); setPreview(null); }}>{live.activeCustomers.map((x) => <option key={x.id} value={x.id}>{x.name}</option>)}</Select>
        </Field>
        <div className="flex flex-wrap items-center gap-3">
          <input aria-label={t("CSV file")} type="file" accept=".csv,text/csv" className="text-[13px]" onChange={(v) => pick(v.target.files?.[0])} />
          <Btn size="sm" variant="ghost" onClick={() => download("units-template.csv", importTemplate)}>{t("Download template")}</Btn>
        </div>
        {file && <p className="text-[12px] text-muted">📄 {file.name} · {count(t, shape?.rows ?? 0, "1 row", "{n} rows")} · {count(t, shape?.header.length ?? 0, "1 column", "{n} columns")} · UTF-8 · {Math.max(1, Math.round(file.size / 1024))} KB{tooBig ? ` — ${t("too large: split the file (at most about 900 KB, 1000 rows)")}` : ""}</p>}
        {tried && !file && <p className="text-[12px] text-crit">{t("Choose a CSV file")}</p>}
        {file && (
          <div className="rounded-xl border border-line">
            <div className="grid grid-cols-[1fr_1fr_auto] gap-2 bg-surface2 px-3 py-2 text-[11px] font-semibold uppercase tracking-wide text-muted"><span>{t("CSV column")}</span><span>{t("Field")}</span><span /></div>
            {importFields.map((x) => (
              <div key={x.key} className="grid grid-cols-[1fr_1fr_auto] items-center gap-2 border-t border-line px-3 py-1.5 text-[13px]">
                <Select aria-label={t("Column for {field}", { field: t(x.label) })} value={mapping[x.key] ?? ""} onChange={(v) => setMapping({ ...mapping, [x.key]: v.target.value })}>
                  <option value="">{t("— not in the file —")}</option>{shape?.header.map((h) => <option key={h} value={h}>{h}</option>)}
                </Select>
                <span>{t(x.label)}</span>
                <span className={cx("text-[11px]", x.required ? "font-semibold text-ink" : "text-muted")}>{t(x.required ? "Required" : "Optional")}</span>
              </div>
            ))}
          </div>
        )}
        {tried && missing.length > 0 && <p className="text-[12px] text-crit">{t("Map the required fields: {fields}", { fields: missing.map((x) => t(x.label)).join(", ") })}</p>}
      </div>
    </Modal>
  );
}
