"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import {
  Badge, Banner, Btn, Card, Check, Choice, ConnBadge, cx, DataTable, EmptyState, Field, Input, Kpi, LinkBtn, Modal, Page, PowerBadge, Search, Select, SummaryList, Tabs,
  Textarea, TextLink, Toggle,
} from "@ac/web/components/ui";
import { useAction } from "@ac/web/lib/useAction";
import { useUrlPatch } from "@ac/web/lib/useUrlPatch";
import {
  changedFields, connections, contractMatch, customerDraft, customerErrors, customerMatch, flatten, nameError, powerStates, profileLabel, propertyDraft, propertyErrors,
  propertyInput, reasonError, relocates, unitDraft, unitErrors, unitInput,
  type ApiUnitDetail, type Connection, type ContractFilter, type CustomerDraft, type CustomerRow, type PolicyLine, type PowerState, type Profile, type PropertyDraft,
  type RuleLine, type Scope, type Selection, type TreeProperty, type TreeSpace, type UnitDraft, type UnitRow, type registerKpis,
} from "@ac/web/lib/assets";
import {
  archiveLocation, archiveUnit, createCustomer, deleteUnit, saveProperty, saveSpace, saveUnit, setDefaultRule, setUnitPolicies, updateCustomer,
} from "../actions";

export type UnitLive = {
  u: ApiUnitDetail; path: string; model: string; device: { id: string; serial: string; connection: Connection } | null; deviceKnown: boolean;
  attached: { id: string; kind: "default_alert" | "alert"; name: string; type: string; condition: string; madeBy: string; on: string }[];
  blockers: { label: string; href: string | null }[] | null; archivedNote: string | null; seen: string | null;
};
export type CustomerLive = {
  row: CustomerRow; tab: "overview" | "policies"; tree: TreeProperty[]; selection: Selection | null; places: { propertyId: string; spaceId: string; label: string }[];
  models: { id: string; label: string }[]; perProperty: string; alertsSub: string | null; filter: { powerState?: PowerState; connections: Connection[]; search: string };
  total: number; units: UnitRow[]; lastEdit: string | null; policies: PolicyLine[] | null; rules: { policyId: string; items: RuleLine[] } | null;
  canRules: boolean; canAttach: boolean; unit?: UnitLive; unitMissing?: boolean;
};
export type UnitsLive = {
  now: string; todayKL: string; canWrite: boolean; standingKnown: boolean; rows: CustomerRow[]; kpis: ReturnType<typeof registerKpis>; search: string; missing: boolean;
  customer?: CustomerLive;
};
type Nav = ReturnType<typeof useUrlPatch>;
type Run = ReturnType<typeof useAction>[1];

const scopes: Scope[] = ["indoor", "outdoor", "electrical"];
const kindLabel: Record<string, string> = { home: "Home", office: "Office", floor: "Floor", area: "Area", room: "Room", space: "Space" };
const powerLabel: Record<PowerState, string> = { on: "Running", off: "Stopped", unknown: "Unknown" };
const plural = (n: number, w: string) => `${n} ${w}${n === 1 ? "" : "s"}`;
const policyCount = (n: number) => `${n} ${n === 1 ? "policy" : "policies"}`;
const clearUnit = { unitId: null };

/** Customers & units (FR-A02) in API mode: the register, then one customer's location tree, units and unit edit. The
 * customer, location, unit and unit filters live in the URL; every write is a Server Action and re-renders the page. */
export function UnitsView({ live }: { live: UnitsLive | null }) {
  if (!live) return <Page><EmptyState title="Customers & units need asset.read">Ask an identity administrator for Customers & units access (Access & roles).</EmptyState></Page>;
  return live.customer ? <CustomerScreen live={live} c={live.customer} /> : <CustomerList live={live} />;
}

// ---- the register ----

function CustomerList({ live }: { live: UnitsLive }) {
  const nav = useUrlPatch();
  const [pending, run] = useAction();
  const [q, setQ] = useState(live.search);
  const [status, setStatus] = useState<"active" | "all">("active");
  const [contract, setContract] = useState<ContractFilter>("all");
  const [open, setOpen] = useState(false);
  const rows = live.rows.filter((r) => (status === "all" || r.status === "active") && contractMatch(contract, r) && customerMatch(r, q));
  const k = live.kpis;
  return (
    <Page>
      {live.missing && <Banner tone="warn">This customer no longer exists or is outside your scope — showing the register.</Banner>}
      <div className="grid-fluid" style={{ ["--min" as string]: "180px" }}>
        <Kpi label="Customers" value={k.customers} sub={k.inactive ? `+${k.inactive} inactive (not counted)` : "all active"} />
        <Kpi label="Properties" value={k.properties} sub={k.propertyNames} />
        <Kpi label="Units" value={k.units} sub={k.power} />
        <Kpi label="Needs attention" value={k.attention} tone={k.attention ? "warn" : undefined} sub={live.standingKnown ? "overdue billing / restriction / open alert" : "open alerts (billing needs billing.read)"} />
      </div>
      <Card title="Customers" sub="Select a customer to manage its properties, spaces and units" action={live.canWrite && <Btn size="sm" variant="primary" onClick={() => setOpen(true)}>+ New customer</Btn>}>
        <div className="mb-3 flex flex-wrap items-center gap-3">
          <div className="min-w-[220px] flex-1 sm:max-w-sm"><Search placeholder="Search customer name, ID or property…" value={q} onChange={setQ} /></div>
          <Select aria-label="Status" className="w-auto" value={status} onChange={(e) => setStatus(e.target.value as "active" | "all")}><option value="active">Status: Active</option><option value="all">Status: All</option></Select>
          {live.standingKnown && (
            <Select aria-label="Contract" className="w-auto" value={contract} onChange={(e) => setContract(e.target.value as ContractFilter)}>
              <option value="all">Contract: All</option><option value="overdue">Overdue</option><option value="restricted">Restriction</option><option value="good">Good standing</option><option value="none">No contract</option>
            </Select>
          )}
        </div>
        {live.rows.length === 0 ? <EmptyState title="No customers yet">{live.canWrite ? "Create the first customer, then add its properties and units." : "Customers appear here once HQ registers them."}</EmptyState> : (
          <DataTable rows={rows} rowKey={(r) => r.id} onRowClick={(r) => nav({ customerId: r.id, tab: null, locationId: null, unitId: null, powerState: null, connections: null, search: null })} cols={[
            { key: "c", label: "Customer", render: (r) => <><b>{r.name}</b><div className="text-xs text-muted">{r.billingName} · {r.id.slice(0, 8)}</div></> },
            { key: "s", label: "Status", render: (r) => <Badge tone={r.status === "active" ? "ok" : "unknown"}>{r.status}</Badge> },
            { key: "p", label: "Properties", render: (r) => r.properties, hideBelow: "sm" },
            { key: "u", label: "Units", render: (r) => r.units },
            { key: "o", label: "Operation", render: (r) => r.operation, hideBelow: "md" },
            { key: "a", label: "Alerts", render: (r) => (r.status === "inactive" ? "—" : r.alerts), hideBelow: "md" },
            ...(live.standingKnown ? [{ key: "k", label: "Contract", render: (r: CustomerRow) => <div className="flex flex-wrap gap-1">{r.marks.map((m) => <Badge key={m.label} tone={m.tone}>{m.label}</Badge>)}</div>, hideBelow: "md" as const }] : []),
            { key: "x", label: "", render: () => "›" },
          ]} />
        )}
        <p className="mt-2 text-[11px] text-muted">{rows.length} of {live.rows.length} · sorted by name · Inactive customers appear only with Status: All and are not counted (IR40). Archived properties, spaces and units never appear in lists, summaries or KPI denominators (IR39).</p>
      </Card>
      {open && <CustomerModal pending={pending} onClose={() => setOpen(false)} onSave={(d) => run(() => createCustomer({ billingName: d.billingName, name: d.name, serviceProfile: d.profile, status: d.status }), `Customer “${d.name.trim()}” created — add a property next`, (id) => { setOpen(false); nav({ customerId: id, tab: null, search: null }); })} />}
    </Page>
  );
}

function CustomerModal({ row, pending, onClose, onSave }: { row?: CustomerRow; pending: boolean; onClose: () => void; onSave: (d: CustomerDraft) => void }) {
  const [d, setD] = useState<CustomerDraft>(customerDraft(row));
  const [tried, setTried] = useState(false);
  const e = customerErrors(d);
  const err = (k: string) => (tried ? e[k] : undefined);
  const save = () => { setTried(true); if (!Object.keys(e).length) onSave(d); };
  return (
    <Modal open onClose={onClose} title={row ? `Edit ${row.name}` : "New customer"} footer={<><Btn onClick={onClose}>Cancel</Btn><Btn variant="primary" disabled={pending} onClick={save}>{row ? "Save customer" : "Create customer"}</Btn></>}>
      <div className="flex flex-col gap-3">
        <Field label="Billing name · required" hint="1–120 characters · organization kind customer · tenant from the session (IR74). A second customer with the same billing name is CONFLICT." error={err("billingName")}><Input value={d.billingName} onChange={(x) => setD({ ...d, billingName: x.target.value })} /></Field>
        <Field label="Customer display name · required" hint="1–120 characters" error={err("name")}><Input value={d.name} onChange={(x) => setD({ ...d, name: x.target.value })} /></Field>
        <Field label="Service profile · required"><Choice value={d.profile} onChange={(v: Profile) => setD({ ...d, profile: v })} options={(Object.keys(profileLabel) as Profile[]).map((p) => ({ id: p, label: p }))} /></Field>
        <Field label="Status" hint={row ? "Deactivating is CONFLICT while the customer still has units, contracts or open jobs (D05)." : "Inactive customers cannot get new units."}>
          <Select value={d.status} onChange={(x) => setD({ ...d, status: x.target.value as "active" | "inactive" })}><option value="active">active</option><option value="inactive">inactive</option></Select>
        </Field>
        {!row && <p className="text-[11px] text-muted">Next step: add a property (home or office) for this customer.</p>}
      </div>
    </Modal>
  );
}

// ---- one customer ----

type ModalState =
  | null | { kind: "customer" } | { kind: "property"; p?: TreeProperty } | { kind: "space"; propertyId: string; parentSpaceId: string | null; spaceKind: TreeSpace["kind"] }
  | { kind: "rename"; p?: TreeProperty; s?: TreeSpace } | { kind: "archive"; p?: TreeProperty; s?: TreeSpace } | { kind: "unit"; propertyId: string; spaceId: string };

function CustomerScreen({ live, c }: { live: UnitsLive; c: CustomerLive }) {
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
        <button type="button" className="font-semibold text-primary" onClick={() => nav({ customerId: null, tab: null, locationId: null, propertyId: null, unitId: null, powerState: null, connections: null, search: null })}>‹ Customers</button>
        <span className="text-muted">/ {r.name}{c.unit ? ` / ${c.unit.u.displayName}` : ""}</span>
        {r.marks.map((x) => <Badge key={x.label} tone={x.tone}>{x.label}</Badge>)}
        <div className="ml-auto flex gap-2">
          {canWrite && <Btn size="sm" onClick={() => setM({ kind: "customer" })}>Edit customer</Btn>}
          {canWrite && r.status === "active" && c.tree.length > 0 && <Btn size="sm" variant="primary" onClick={newUnitAt}>+ New unit</Btn>}
        </div>
      </div>
      <Card>
        <div className="flex flex-wrap items-center gap-3">
          <div aria-hidden className="flex h-11 w-11 items-center justify-center rounded-full bg-primary-soft font-bold text-primary">{r.name.split(/\s+/).map((w) => w[0]).join("").slice(0, 2).toUpperCase()}</div>
          <div className="min-w-0 flex-1">
            <div className="flex flex-wrap items-center gap-2"><b className="text-base">{r.name}</b><Badge tone={r.status === "active" ? "ok" : "unknown"}>{r.status === "active" ? "Active" : "Inactive"}</Badge></div>
            <div className="text-xs text-muted">{r.billingName} · customer since {r.since} · {profileLabel[r.profile]}</div>
          </div>
          {s && <div className="text-xs text-muted">{s.contracts ? `Plan: ${s.plans.map((p) => profileLabel[p]).join(", ")} · ${plural(s.contractUnits, "unit")} under contract` : "No current contract"}</div>}
        </div>
      </Card>
      <div className="grid-fluid" style={{ ["--min" as string]: "180px" }}>
        <Kpi label="Units" value={unitsTotal} sub={c.perProperty} />
        <Kpi label="Operation" value={r.operation} sub={`Running ${r.running} / known ${r.known}`} />
        <Kpi label="Open alerts" value={r.alerts} tone={r.alerts ? "warn" : undefined} sub={c.alertsSub ?? "severity needs alert.read"} href="/admin/alerts" link="Alerts ›" />
        {s ? <Kpi label="Overdue billing" value={s.overdueAmount ?? "None"} tone={s.overdue ? "crit" : undefined}
          sub={s.restriction ? `Restriction ${s.restriction.state.replace("_", " ")}` : s.overdue ? `${plural(s.overdue, "invoice")} overdue` : "no overdue invoice"}
          href={s.restriction ? `/admin/restrictions?restrictionId=${s.restriction.id}` : s.overdue ? `/admin/billing?customerId=${r.id}&overdueOnly=true` : undefined} link={s.restriction ? "View restriction →" : s.overdue ? "View invoices →" : undefined} />
          : <Kpi label="Overdue billing" value="—" sub="needs billing.read" />}
      </div>
      <Tabs value={c.tab} onChange={(t) => nav({ tab: t === "overview" ? null : t, unitId: null })} tabs={[{ id: "overview" as const, label: "Units & locations", count: unitsTotal }, { id: "policies" as const, label: "Alert policies", count: c.policies?.length ?? "—" }]} />
      {c.tab === "policies" ? <PoliciesTab c={c} run={run} pending={pending} /> : (
        <div className="split-rev">
          <Card title="Locations" action={canWrite && <Btn size="sm" onClick={() => setM({ kind: "property" })}>+ Add property</Btn>} className="self-start">
            {c.tree.length === 0 ? <EmptyState title="No properties yet">{canWrite ? "Add the customer's home or office first." : "HQ has not registered a property yet."}</EmptyState> : (
              <div className="flex flex-col gap-0.5">
                {c.tree.map((p) => (
                  <div key={p.id}>
                    <Node depth={0} selected={selId === p.id} onClick={() => nav({ locationId: p.id, ...clearUnit })} icon="⌂" name={p.name} meta={`${kindLabel[p.kind]} · ${plural(p.units, "unit")}`}
                      menu={canWrite && <LocationMenu onRename={() => setM({ kind: "rename", p })} onDelete={() => setM({ kind: "archive", p })} />} />
                    {p.spaces.map((x) => <SpaceNode key={x.id} s={x} depth={1} selId={selId} nav={nav} canWrite={canWrite} setM={setM} />)}
                    {p.unassigned > 0 && <Node depth={1} selected={selId === `unassigned:${p.id}`} onClick={() => nav({ locationId: `unassigned:${p.id}`, ...clearUnit })} icon="·" name="Unassigned" meta={plural(p.unassigned, "unit")} />}
                    {canWrite && <AddLink depth={1} onClick={() => setM({ kind: "space", propertyId: p.id, parentSpaceId: null, spaceKind: p.kind === "office" ? "area" : "floor" })}>{p.kind === "office" ? "+ Add floor / area" : "+ Add floor"}</AddLink>}
                  </div>
                ))}
              </div>
            )}
            <p className="mt-3 text-[11px] text-muted">⋯ on any location: Rename · Delete. Delete is blocked while it still contains locations or units.</p>
          </Card>
          <div className="flex min-w-0 flex-col gap-4">
            {c.unitMissing && <Banner tone="warn" action={<Btn size="sm" onClick={() => nav(clearUnit)}>Close</Btn>}>This unit no longer exists or belongs to another customer.</Banner>}
            {c.unit ? <UnitEdit key={c.unit.u.id} live={live} c={c} x={c.unit} nav={nav} run={run} pending={pending} /> : sel && <>
              <LocationCard c={c} sel={sel} canWrite={canWrite} active={r.status === "active"} setM={setM} />
              <UnitsCard key={selId} c={c} sel={sel} nav={nav} canWrite={canWrite && r.status === "active"} onNew={newUnitAt} />
            </>}
          </div>
        </div>
      )}
      {m?.kind === "customer" && <CustomerModal row={r} pending={pending} onClose={close} onSave={(d) => run(() => updateCustomer(r.id, r.version, { id: r.orgId, version: r.orgVersion ?? 1, name: r.billingName, status: r.orgStatus }, { billingName: d.billingName, name: d.name, serviceProfile: d.profile, status: d.status }), "Customer saved", close)} />}
      {m?.kind === "property" && <PropertyModal p={m.p} pending={pending} onClose={close} onSave={(d) => run(() => saveProperty(propertyInput(d, r.orgId, m.p?.id), m.p?.version), m.p ? "Property saved" : "Property added", (id) => { close(); nav({ locationId: id, ...clearUnit }); })} />}
      {m?.kind === "space" && <SpaceModal c={c} init={m} pending={pending} onClose={close} onSave={(x) => run(() => saveSpace(x), `${kindLabel[x.kind]} “${x.name}” added`, (id) => { close(); nav({ locationId: id, ...clearUnit }); })} />}
      {m?.kind === "rename" && <RenameModal name={m.p?.name ?? m.s?.name ?? ""} where={m.p ? `${kindLabel[m.p.kind]} · property` : `${kindLabel[m.s!.kind]}`} pending={pending} onClose={close}
        onSave={(name) => run(() => (m.p ? saveProperty(propertyInput({ ...propertyDraft(m.p), name }, r.orgId, m.p.id), m.p.version)
          : saveSpace({ id: m.s!.id, propertyId: m.s!.propertyId, parentSpaceId: m.s!.parentSpaceId, kind: m.s!.kind, name: name.trim() }, m.s!.version)), "Name saved", close)} />}
      {m?.kind === "archive" && <ArchiveLocationModal p={m.p} s={m.s} pending={pending} onClose={close}
        onSave={(reason) => run(() => archiveLocation(m.p ? "property" : "space", m.p?.id ?? m.s!.id, m.p?.version ?? m.s!.version, reason), "Location deleted — its history stays in the audit log", () => { close(); nav({ locationId: null, ...clearUnit }); })} />}
      {m?.kind === "unit" && <NewUnitModal live={live} c={c} init={m} pending={pending} onClose={close}
        onSave={(d) => run(() => saveUnit(unitInput(d, r.orgId)), `Unit “${d.displayName.trim()}” registered`, (id) => { close(); nav({ locationId: d.spaceId || d.propertyId, unitId: id }); })} />}
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
  const container = s.kind === "floor" || s.kind === "area";
  return (
    <>
      <Node depth={depth} selected={selId === s.id} onClick={() => nav({ locationId: s.id, ...clearUnit })} icon={s.children.length ? "▾" : "·"} name={s.name}
        meta={`${kindLabel[s.kind]}${s.units ? ` · ${s.units}` : ""}`} menu={canWrite && <LocationMenu onRename={() => setM({ kind: "rename", s })} onDelete={() => setM({ kind: "archive", s })} />} />
      {s.children.map((x) => <SpaceNode key={x.id} s={x} depth={depth + 1} selId={selId} nav={nav} canWrite={canWrite} setM={setM} />)}
      {canWrite && container && <AddLink depth={depth + 1} onClick={() => setM({ kind: "space", propertyId: s.propertyId, parentSpaceId: s.id, spaceKind: "room" })}>+ Add room</AddLink>}
    </>
  );
}
const AddLink = ({ depth, onClick, children }: { depth: number; onClick: () => void; children: React.ReactNode }) => (
  <button type="button" onClick={onClick} className="block py-1 text-left text-[12px] font-semibold text-primary" style={{ paddingLeft: depth * 14 + 8 }}>{children}</button>
);
function LocationMenu({ onRename, onDelete }: { onRename: () => void; onDelete: () => void }) {
  return (
    <select aria-label="Location actions" className="h-7 w-8 shrink-0 cursor-pointer appearance-none rounded-md bg-transparent text-center text-[15px] font-bold leading-none text-muted hover:bg-surface2" value=""
      onChange={(e) => (e.target.value === "rename" ? onRename() : e.target.value === "delete" && onDelete())}>
      <option value="">⋯</option><option value="rename">Rename</option><option value="delete">Delete…</option>
    </select>
  );
}

function LocationCard({ c, sel, canWrite, active, setM }: { c: CustomerLive; sel: Selection; canWrite: boolean; active: boolean; setM: (m: ModalState) => void }) {
  if (sel.kind === "unassigned") {
    return <Card title="Unassigned" sub={`${sel.property.name} · units without a space (IR62)`}><p className="text-[13px] text-muted">These units belong to the property but no floor or room. Open a unit and pick its space under Location.</p></Card>;
  }
  if (sel.kind === "property") {
    const p = sel.property;
    return (
      <Card title={p.name} sub={`${kindLabel[p.kind]} · property · ${[p.address, plural(p.floors, "floor"), plural(p.rooms, "room"), plural(p.units, "unit")].filter(Boolean).join(" · ")}`}
        action={canWrite && <div className="flex flex-wrap gap-2"><Btn size="sm" onClick={() => setM({ kind: "rename", p })}>Rename</Btn><Btn size="sm" onClick={() => setM({ kind: "property", p })}>Edit details</Btn>
          <Btn size="sm" onClick={() => setM({ kind: "space", propertyId: p.id, parentSpaceId: null, spaceKind: p.kind === "office" ? "area" : "floor" })}>{p.kind === "office" ? "+ Add floor / area" : "+ Add floor"}</Btn><Btn size="sm" variant="danger" onClick={() => setM({ kind: "archive", p })}>Delete</Btn></div>}>
        <SummaryList cols={2} items={[["Type", kindLabel[p.kind]], ["Address", p.address ?? "—"], ["Access instructions", p.accessInstructions ?? "—"], ["Time zone", "Asia/Kuala_Lumpur"], ["Last edited", c.lastEdit ?? "—"]]} />
      </Card>
    );
  }
  const s = sel.space;
  const holds = s.kind === "room" || s.kind === "space";
  return (
    <Card title={s.name} sub={`${kindLabel[s.kind]} · ${sel.path} · ${plural(s.units, "unit")}`}
      action={canWrite && <div className="flex flex-wrap gap-2"><Btn size="sm" onClick={() => setM({ kind: "rename", s })}>Rename</Btn>
        {!holds && <Btn size="sm" onClick={() => setM({ kind: "space", propertyId: s.propertyId, parentSpaceId: s.id, spaceKind: "room" })}>+ Add room</Btn>}
        <Btn size="sm" variant="danger" onClick={() => setM({ kind: "archive", s })}>Delete</Btn>
        {holds && active && <Btn size="sm" variant="primary" onClick={() => setM({ kind: "unit", propertyId: s.propertyId, spaceId: s.id })}>+ New unit in {s.name}</Btn>}</div>}>
      <SummaryList cols={2} items={[["Kind", kindLabel[s.kind]], ["Child locations", s.children.length || "none"], ["Units here", s.direct], ["Last edited", c.lastEdit ?? "—"]]} />
    </Card>
  );
}

function UnitsCard({ c, sel, nav, canWrite, onNew }: { c: CustomerLive; sel: Selection; nav: Nav; canWrite: boolean; onNew: () => void }) {
  const [q, setQ] = useState(c.filter.search);
  const rows = c.units.filter((u) => !q.trim() || u.name.toLowerCase().includes(q.trim().toLowerCase()) || u.id.toLowerCase().includes(q.trim().toLowerCase()));
  const filtered = !!c.filter.powerState || c.filter.connections.length > 0;
  const title = sel.kind === "property" ? `All units in ${sel.property.name}` : sel.kind === "space" ? `Units in ${sel.space.name}` : `Unassigned units in ${sel.property.name}`;
  return (
    <Card title={`${title} · ${filtered || q.trim() ? `${rows.length} of ${c.total}` : c.total}`} action={canWrite && <Btn size="sm" variant="primary" onClick={onNew}>+ New unit</Btn>}>
      {filtered && <div className="mb-3"><Banner action={<Btn size="sm" onClick={() => nav({ powerState: null, connections: null })}>Clear filters</Btn>}>
        Filtered by {[c.filter.powerState && `Power: ${powerLabel[c.filter.powerState]}`, c.filter.connections.length && `Connection: ${c.filter.connections.join(", ")}`].filter(Boolean).join(" · ")} — clear the filter to see all {plural(c.total, "unit")}.
      </Banner></div>}
      <div className="mb-3 flex flex-wrap items-center gap-3">
        <div className="min-w-[200px] flex-1 sm:max-w-xs"><Search placeholder="Search unit name or ID…" value={q} onChange={setQ} /></div>
        <Select aria-label="Power" className="w-auto" value={c.filter.powerState ?? ""} onChange={(e) => nav({ powerState: e.target.value || null })}>
          <option value="">Power: All</option>{powerStates.map((p) => <option key={p} value={p}>Power: {powerLabel[p]}</option>)}
        </Select>
        <Select aria-label="Connection" className="w-auto" value={c.filter.connections.length === 1 ? c.filter.connections[0] : ""} onChange={(e) => nav({ connections: e.target.value || null })}>
          <option value="">Connection: All</option>{connections.map((x) => <option key={x} value={x}>Connection: {x}</option>)}
        </Select>
      </div>
      {c.total === 0 ? <EmptyState title="No units here">{canWrite ? "Register a unit for this location with + New unit." : "No unit is registered at this location."}</EmptyState> : (
        <DataTable rows={rows} rowKey={(u) => u.id} onRowClick={(u) => nav({ unitId: u.id })} cols={[
          { key: "n", label: "Unit", render: (u) => <><b>{u.name}</b><div className="text-xs text-muted">{u.id.slice(0, 8)} · {u.model}</div></> },
          { key: "l", label: "Location", render: (u) => u.location, hideBelow: "sm" },
          { key: "p", label: "Power", render: (u) => <PowerBadge s={u.power} /> },
          { key: "c", label: "Connection", render: (u) => <ConnBadge s={u.conn} />, hideBelow: "sm" },
          { key: "a", label: "Alerts", render: (u) => (u.alerts ? <Badge tone="warn" icon="⚠">{u.alerts}</Badge> : "—"), hideBelow: "md" },
          { key: "y", label: "Policies", render: (u) => u.policies, hideBelow: "md" },
          { key: "x", label: "", render: () => "›" },
        ]} />
      )}
      <p className="mt-2 text-[11px] text-muted">Pick a room in the tree to list only its units; a row opens the unit edit. Power is the derived state (SR27): unknown never counts as running or stopped.</p>
    </Card>
  );
}

// ---- unit edit ----

function UnitEdit({ live, c, x, nav, run, pending }: { live: UnitsLive; c: CustomerLive; x: UnitLive; nav: Nav; run: Run; pending: boolean }) {
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
  const errors = unitErrors(d, live.todayKL);
  const err = (k: string) => (tried ? errors[k] : undefined);
  const changes = changedFields(u, d);
  const set = (patch: Partial<UnitDraft>) => setD((v) => ({ ...v, ...patch }));
  const place = `${d.propertyId}|${d.spaceId}`;
  const back = x.path.split(" › ").at(-1) ?? "location";
  const save = () => {
    setTried(true);
    if (Object.keys(errors).length) return;
    if (relocates(u, d)) return setModal("relocate");
    run(() => saveUnit(unitInput(d, c.row.orgId, u.id), u.version), "Unit saved");
  };
  const attachedIds = x.attached.filter((p) => p.kind === "alert").map((p) => p.id);
  return (
    <>
      <Card title={<span className="flex flex-wrap items-center gap-2">{u.displayName}<PowerBadge s={u.effectivePowerState === "on" ? "running" : u.effectivePowerState === "off" ? "stopped" : "unknown"} /><ConnBadge s={u.connection} />{x.seen && <span className="text-[11px] font-normal text-muted">seen {x.seen}</span>}{u.activeAlertCount > 0 && <Badge tone="warn" icon="⚠">{plural(u.activeAlertCount, "open alert")}</Badge>}</span>}
        sub={<>{u.id.slice(0, 8)} · version {u.version} · <button type="button" className="font-semibold text-primary" onClick={() => nav({ unitId: null, locationId: u.archived ? null : u.spaceId ?? `unassigned:${u.propertyId}` })}>‹ back to {back}</button></>}
        action={live.canWrite && !u.archived && <Btn size="sm" variant="danger" onClick={() => setModal("delete")}>Delete unit…</Btn>}>
        {u.archived && <div className="mb-3"><Banner tone="warn">Read only — {x.archivedNote}. Archived units are excluded from lists, summaries and KPI denominators (IR39).</Banner></div>}
        <div className="grid gap-x-4 gap-y-3 sm:grid-cols-2">
          <Field label="Name" error={err("displayName")}><Input value={d.displayName} disabled={ro} onChange={(e) => set({ displayName: e.target.value })} /></Field>
          <Field label="Location" hint="Changing it relocates the unit — a reason is asked on save; the old location stays in the history." error={err("propertyId")}>
            <Select value={place} disabled={ro} onChange={(e) => { const [propertyId, spaceId] = e.target.value.split("|"); set({ propertyId, spaceId }); }}>
              {!c.places.some((p) => `${p.propertyId}|${p.spaceId}` === place) && <option value={place}>{x.path} (current)</option>}
              {c.places.map((p) => <option key={`${p.propertyId}|${p.spaceId}`} value={`${p.propertyId}|${p.spaceId}`}>{p.label}</option>)}
            </Select>
          </Field>
          <Field label="Model" hint={c.models.length ? "From capabilities.list · type split (fixed)" : "Models need device.read — the current model stays"} error={err("modelId")}>
            <Select value={d.modelId} disabled={ro || !c.models.length} onChange={(e) => set({ modelId: e.target.value })}>
              {!c.models.some((k) => k.id === d.modelId) && <option value={d.modelId}>{x.model}</option>}
              {c.models.map((k) => <option key={k.id} value={k.id}>{k.label}</option>)}
            </Select>
          </Field>
          <Field label="Installed at" hint="Leave empty for “Not registered” (IR44)" error={err("installedAt")}><Input type="date" max={live.todayKL} value={d.installedAt} disabled={ro} onChange={(e) => set({ installedAt: e.target.value })} /></Field>
          <Field label="IoT device" hint={x.device ? "Rebinding is done in Devices and keeps the command history on the unit." : undefined}>
            <div className="flex min-h-9 items-center gap-2 text-[13px]">{x.device ? <><b>{x.device.serial}</b><ConnBadge s={x.device.connection} /><TextLink href={`/admin/devices?tab=devices&deviceId=${x.device.id}`}>Devices ›</TextLink></> : x.deviceKnown ? <span className="text-muted">No device bound</span> : <span className="text-muted">Needs device.read</span>}</div>
          </Field>
          <Field label="Service scope" hint="Target inspection groups" error={err("serviceScope")}>
            <div className="flex flex-wrap gap-3">{scopes.map((sc) => <Check key={sc} label={sc} disabled={ro} checked={d.serviceScope.includes(sc)} onChange={(on) => set({ serviceScope: on ? [...d.serviceScope, sc] : d.serviceScope.filter((v) => v !== sc) })} />)}</div>
          </Field>
        </div>
        {!ro && (
          <div className="mt-4 flex flex-wrap items-center justify-end gap-2 border-t border-line pt-3">
            <span className="mr-auto text-[12px] text-muted">{changes.length ? `${plural(changes.length, "unsaved change")} · ${changes.join(", ")}` : "No unsaved changes"}</span>
            <Btn disabled={!changes.length || pending} onClick={() => { setD(unitDraft(u)); setTried(false); }}>Discard</Btn>
            <Btn variant="primary" disabled={!changes.length || pending} onClick={save}>Save changes</Btn>
          </div>
        )}
      </Card>
      <Card title="Alert policies on this unit" sub={`Only ${c.row.name}’s policies can be attached. The unit carries the policies, not the other way round (IR108).`}
        action={c.canAttach && !u.archived && <Btn size="sm" onClick={() => setModal("attach")}>+ Attach policy</Btn>}>
        {c.policies === null ? <p className="text-[13px] text-muted">Policies need alert.policy.read.</p> : (
          <div className="flex flex-col">
            {x.attached.map((p) => (
              <div key={p.id} className="flex flex-wrap items-center gap-2 border-t border-line py-2.5 text-[13px] first:border-0">
                <div className="min-w-0 flex-1"><div className="flex flex-wrap items-center gap-2"><b>{p.name}</b><Badge tone={p.kind === "default_alert" ? "primary" : "muted"}>{p.type}</Badge>{p.madeBy && <span className="text-[11px] text-muted">{p.madeBy}</span>}</div><div className="text-xs text-muted">{p.condition}{p.on ? ` · ${p.on}` : ""}</div></div>
                <TextLink href={`/admin/alerts?tab=policies&policyId=${p.id}`}>{p.kind === "default_alert" ? "View" : "Edit"}</TextLink>
                {p.kind === "alert" && c.canAttach && !u.archived && <Btn size="sm" variant="ghost" disabled={pending} onClick={() => run(() => setUnitPolicies(u.id, u.version, attachedIds.filter((id) => id !== p.id)), `“${p.name}” detached`)}>Detach</Btn>}
              </div>
            ))}
          </div>
        )}
      </Card>
      <div className="flex flex-wrap gap-4 text-[13px]">
        {x.device && <TextLink href={`/admin/audit?tab=devices&deviceId=${x.device.id}`}>Device events ›</TextLink>}
        <TextLink href="/admin/audit">Audit log ›</TextLink>
      </div>
      {modal === "relocate" && <RelocateModal u={u} from={x.path} to={c.places.find((p) => `${p.propertyId}|${p.spaceId}` === place)?.label ?? ""} pending={pending} onClose={() => setModal(null)}
        onSave={(reason) => run(() => saveUnit(unitInput(d, c.row.orgId, u.id, reason), u.version), "Location changed — the old location stays in the history", () => setModal(null))} />}
      {modal === "delete" && <DeleteUnitModal x={x} pending={pending} onClose={() => setModal(null)}
        onArchive={(reason) => run(() => archiveUnit(u.id, u.version, reason), `“${u.displayName}” taken out of use`, () => { setModal(null); nav({ unitId: null }); })}
        onDelete={(reason) => run(() => deleteUnit(u.id, u.version, reason), `“${u.displayName}” deleted`, () => { setModal(null); nav({ unitId: null }); })} />}
      {modal === "attach" && <AttachModal c={c} unit={u.displayName} attached={attachedIds} pending={pending} onClose={() => setModal(null)}
        onSave={(ids) => run(() => setUnitPolicies(u.id, u.version, [...attachedIds, ...ids]), `${policyCount(ids.length)} attached`, () => setModal(null))} />}
    </>
  );
}

// ---- alert policies tab ----

function PoliciesTab({ c, run, pending }: { c: CustomerLive; run: Run; pending: boolean }) {
  const router = useRouter();
  if (!c.policies) return <EmptyState title="Alert policies need alert.policy.read">Ask an identity administrator for Alert policies access.</EmptyState>;
  return (
    <>
      <Card title={`Alert policies of ${c.row.name}`} sub="Policies belong to the customer. Each unit picks which of them it carries (unit edit › Alert policies). Air-quality limits are alert policies too."
        action={<LinkBtn size="sm" variant="primary" href="/admin/alerts?tab=policies">+ New policy for {c.row.name}</LinkBtn>}>
        <DataTable rows={c.policies} rowKey={(p) => p.id} onRowClick={(p) => router.push(`/admin/alerts?tab=policies&policyId=${p.id}`)} cols={[
          { key: "n", label: "Policy", render: (p) => <><b>{p.name}</b>{!p.enabled && <Badge className="ml-2">Off</Badge>}</> },
          { key: "t", label: "Type", render: (p) => <Badge tone={p.kind === "default_alert" ? "primary" : "muted"}>{p.type}</Badge> },
          { key: "c", label: "Condition", render: (p) => p.condition, hideBelow: "sm" },
          { key: "m", label: "Made by", render: (p) => p.madeBy, hideBelow: "md" },
          { key: "u", label: "Units", render: (p) => (p.kind === "default_alert" ? `All ${p.units}` : plural(p.units, "unit")) },
        ]} />
        <p className="mt-2 text-[11px] text-muted">A row opens the policy editor (Alert policies › Policies). The default policy is on every unit; its rules can be switched per customer but it cannot be detached.</p>
      </Card>
      {c.rules && (
        <Card title={`Default policy rules for ${c.row.name}`} sub="Limits are edited once for every customer (Alert policies › Default policy). Here you switch each rule on or off for this customer — the customer's owner can do the same in the customer app.">
          <div className="flex flex-col">
            {c.rules.items.map((r) => (
              <div key={r.ruleKey} className="flex flex-wrap items-center gap-3 border-t border-line py-2.5 text-[13px] first:border-0">
                <div className="min-w-0 flex-1"><b>{r.name}</b><div className="text-xs text-muted">{r.condition}{r.note ? ` · ${r.note}` : ""}</div></div>
                <Toggle on={r.enabled} label={r.enabled ? "On" : "Off"} disabled={!c.canRules || pending}
                  onChange={(on) => run(() => setDefaultRule(c.rules!.policyId, r.ruleKey, c.row.id, on, r.version), `${r.name} switched ${on ? "on" : "off"} for ${c.row.name}`)} />
              </div>
            ))}
          </div>
          {!c.canRules && <p className="mt-2 text-[11px] text-muted">Switching rules needs alert.policy.write.</p>}
        </Card>
      )}
    </>
  );
}

// ---- modals ----

function PropertyModal({ p, pending, onClose, onSave }: { p?: TreeProperty; pending: boolean; onClose: () => void; onSave: (d: PropertyDraft) => void }) {
  const [d, setD] = useState<PropertyDraft>(propertyDraft(p));
  const [tried, setTried] = useState(false);
  const e = propertyErrors(d);
  const err = (k: string) => (tried ? e[k] : undefined);
  return (
    <Modal open onClose={onClose} title={p ? `Edit ${p.name}` : "Add property"} footer={<><Btn onClick={onClose}>Cancel</Btn><Btn variant="primary" disabled={pending} onClick={() => { setTried(true); if (!Object.keys(e).length) onSave(d); }}>{p ? "Save property" : "Add property"}</Btn></>}>
      <div className="flex flex-col gap-3">
        <Field label="Kind · required"><Choice value={d.kind} onChange={(v: "home" | "office") => setD({ ...d, kind: v })} options={[{ id: "home", label: "Home" }, { id: "office", label: "Office" }]} /></Field>
        <Field label="Name · required" hint="1–120 characters, unique among the customer's properties" error={err("name")}><Input value={d.name} onChange={(x) => setD({ ...d, name: x.target.value })} /></Field>
        <Field label="Address" hint="0–500 characters · fictional values only; shown as the site address (IR25)" error={err("address")}><Input value={d.address} onChange={(x) => setD({ ...d, address: x.target.value })} /></Field>
        <Field label="Access instructions" hint="0–1000 characters · visible to a contractor only after acceptance, within the valid period" error={err("accessInstructions")}><Textarea rows={3} value={d.accessInstructions} onChange={(x) => setD({ ...d, accessInstructions: x.target.value })} /></Field>
      </div>
    </Modal>
  );
}

function SpaceModal({ c, init, pending, onClose, onSave }: { c: CustomerLive; init: { propertyId: string; parentSpaceId: string | null; spaceKind: TreeSpace["kind"] }; pending: boolean; onClose: () => void; onSave: (x: { propertyId: string; parentSpaceId: string | null; kind: TreeSpace["kind"]; name: string }) => void }) {
  const [propertyId, setPropertyId] = useState(init.propertyId);
  const [parent, setParent] = useState(init.parentSpaceId ?? "");
  const [kind, setKind] = useState<TreeSpace["kind"]>(init.spaceKind);
  const [name, setName] = useState("");
  const [tried, setTried] = useState(false);
  const p = c.tree.find((x) => x.id === propertyId);
  const parents = p ? flatten(p.spaces) : [];
  const e = nameError(name);
  return (
    <Modal open onClose={onClose} title="Add location" footer={<><Btn onClick={onClose}>Cancel</Btn><Btn variant="primary" disabled={pending} onClick={() => { setTried(true); if (!e) onSave({ propertyId, parentSpaceId: parent || null, kind, name: name.trim() }); }}>Add {kindLabel[kind].toLowerCase()}</Btn></>}>
      <div className="flex flex-col gap-3">
        <Field label="Property"><Select value={propertyId} onChange={(x) => { setPropertyId(x.target.value); setParent(""); }}>{c.tree.map((t) => <option key={t.id} value={t.id}>{t.name}</option>)}</Select></Field>
        <Field label="Kind · required" hint="floor · area · room · space (under the parent you pick)"><Select value={kind} onChange={(x) => setKind(x.target.value as TreeSpace["kind"])}>{(["floor", "area", "room", "space"] as const).map((k) => <option key={k} value={k}>{k}</option>)}</Select></Field>
        <Field label="Parent" hint="Same property only; floors and areas usually sit directly under the property">
          <Select value={parent} onChange={(x) => setParent(x.target.value)}><option value="">{p?.name ?? "Property"} (top level)</option>{parents.map((s) => <option key={s.id} value={s.id}>{s.name} ({kindLabel[s.kind]})</option>)}</Select>
        </Field>
        <Field label="Name · required" hint="1–120 characters, unique among its siblings" error={tried ? e : undefined}><Input value={name} onChange={(x) => setName(x.target.value)} /></Field>
        <p className="text-[11px] text-muted">A parent in another property, or a parent change that creates a cycle, is rejected with VALIDATION.</p>
      </div>
    </Modal>
  );
}

function RenameModal({ name, where, pending, onClose, onSave }: { name: string; where: string; pending: boolean; onClose: () => void; onSave: (name: string) => void }) {
  const [v, setV] = useState(name);
  const [tried, setTried] = useState(false);
  const e = nameError(v);
  return (
    <Modal open onClose={onClose} title={`Rename ${name}`} footer={<><Btn onClick={onClose}>Cancel</Btn><Btn variant="primary" disabled={pending || v.trim() === name} onClick={() => { setTried(true); if (!e) onSave(v.trim()); }}>Save name</Btn></>}>
      <div className="flex flex-col gap-3">
        <p className="text-[13px] text-muted">Only the name changes; units, child locations and history stay attached. The customer sees the new name immediately.</p>
        <SummaryList items={[["Location", `${name} · ${where}`]]} />
        <Field label="New name" hint="1–120 characters, unique among its siblings. Saved with a version check — an older version is CONFLICT." error={tried ? e : undefined}><Input value={v} onChange={(x) => setV(x.target.value)} /></Field>
      </div>
    </Modal>
  );
}

function ArchiveLocationModal({ p, s, pending, onClose, onSave }: { p?: TreeProperty; s?: TreeSpace; pending: boolean; onClose: () => void; onSave: (reason: string) => void }) {
  const [reason, setReason] = useState("");
  const [tried, setTried] = useState(false);
  const name = p?.name ?? s?.name ?? "";
  const children = p ? p.spaces.length : s?.children.length ?? 0;
  const units = p ? p.units : s?.units ?? 0;
  const blocked = children > 0 || units > 0;
  const e = reasonError(reason);
  return (
    <Modal open onClose={onClose} title={`Delete ${name}?`} footer={<><Btn onClick={onClose}>Cancel</Btn><Btn variant="danger" disabled={pending || blocked} onClick={() => { setTried(true); if (!e) onSave(reason.trim()); }}>Delete {p ? "property" : kindLabel[s!.kind].toLowerCase()}</Btn></>}>
      <div className="flex flex-col gap-3">
        <p className="text-[13px] text-muted">Deleting takes the location out of the customer’s tree (archived). Its history stays in the audit log.</p>
        {blocked && <Banner tone="crit">CONFLICT — {name} still contains {[children && plural(children, "location"), units && plural(units, "unit")].filter(Boolean).join(" and ")}. Move them (unit › Location) or delete them first.</Banner>}
        <Field label="Reason · required" hint="1–1000 characters" error={tried ? e : undefined}><Textarea rows={2} value={reason} disabled={blocked} onChange={(x) => setReason(x.target.value)} /></Field>
      </div>
    </Modal>
  );
}

function NewUnitModal({ live, c, init, pending, onClose, onSave }: { live: UnitsLive; c: CustomerLive; init: { propertyId: string; spaceId: string }; pending: boolean; onClose: () => void; onSave: (d: UnitDraft) => void }) {
  const [d, setD] = useState<UnitDraft>({ ...unitDraft(undefined, init.propertyId, init.spaceId), modelId: c.models[0]?.id ?? "" });
  const [tried, setTried] = useState(false);
  const e = unitErrors(d, live.todayKL);
  const err = (k: string) => (tried ? e[k] : undefined);
  const set = (patch: Partial<UnitDraft>) => setD((v) => ({ ...v, ...patch }));
  const p = c.tree.find((x) => x.id === d.propertyId);
  return (
    <Modal open wide onClose={onClose} title="New unit" footer={<><Btn onClick={onClose}>Cancel</Btn><Btn variant="primary" disabled={pending} onClick={() => { setTried(true); if (!Object.keys(e).length) onSave(d); }}>Register unit</Btn></>}>
      <div className="grid gap-x-4 gap-y-3 sm:grid-cols-2">
        <p className="text-[12px] text-muted sm:col-span-2">customerOrgId, spaceId and modelId must agree; the tenant comes from the session (IR74).</p>
        <Field label="Customer · fixed"><Input value={c.row.name} disabled /></Field>
        <Field label="Location · required" error={err("propertyId")}><Select value={d.propertyId} onChange={(x) => set({ propertyId: x.target.value, spaceId: "" })}>{c.tree.map((t) => <option key={t.id} value={t.id}>{t.name}</option>)}</Select></Field>
        <Field label="Room / space · optional" hint="Same property only. Empty = not assigned (IR62).">
          <Select value={d.spaceId} onChange={(x) => set({ spaceId: x.target.value })}><option value="">— not assigned —</option>{(p ? flatten(p.spaces) : []).map((s) => <option key={s.id} value={s.id}>{s.name} ({kindLabel[s.kind]})</option>)}</Select>
        </Field>
        <Field label="Model · required" hint={c.models.length ? "From capabilities.list" : "Models need device.read"} error={err("modelId")}>
          <Select value={d.modelId} onChange={(x) => set({ modelId: x.target.value })}>{!c.models.length && <option value="">no model available</option>}{c.models.map((k) => <option key={k.id} value={k.id}>{k.label}</option>)}</Select>
        </Field>
        <Field label="Display name · required" hint="1–120 characters, unique in its room" error={err("displayName")}><Input value={d.displayName} onChange={(x) => set({ displayName: x.target.value })} /></Field>
        <Field label="Installed at · optional" hint="Leave empty for “Not registered” (IR44)" error={err("installedAt")}><Input type="date" max={live.todayKL} value={d.installedAt} onChange={(x) => set({ installedAt: x.target.value })} /></Field>
        <Field label="Service scope · required · type: split (fixed)" error={err("serviceScope")} className="sm:col-span-2">
          <div className="flex flex-wrap gap-3">{scopes.map((sc) => <Check key={sc} label={sc} checked={d.serviceScope.includes(sc)} onChange={(on) => set({ serviceScope: on ? [...d.serviceScope, sc] : d.serviceScope.filter((v) => v !== sc) })} />)}</div>
        </Field>
      </div>
    </Modal>
  );
}

function RelocateModal({ u, from, to, pending, onClose, onSave }: { u: ApiUnitDetail; from: string; to: string; pending: boolean; onClose: () => void; onSave: (reason: string) => void }) {
  const [reason, setReason] = useState("");
  const [tried, setTried] = useState(false);
  const e = reasonError(reason);
  return (
    <Modal open onClose={onClose} title="Change location" footer={<><Btn onClick={onClose}>Cancel</Btn><Btn variant="primary" disabled={pending} onClick={() => { setTried(true); if (!e) onSave(reason.trim()); }}>Save location</Btn></>}>
      <div className="flex flex-col gap-3">
        <SummaryList items={[["Unit", `${u.displayName} · version ${u.version}`], ["Current", from], ["New", to]]} />
        <Field label="Change reason · required" hint="1–1000 characters. Saved with the before/after values in the audit log." error={tried ? e : undefined}><Textarea rows={2} value={reason} onChange={(x) => setReason(x.target.value)} /></Field>
        <p className="text-[11px] text-muted">Keeps the unit ID; version {u.version} → {u.version + 1}. Alert policies stay attached to the unit.</p>
      </div>
    </Modal>
  );
}

function DeleteUnitModal({ x, pending, onClose, onArchive, onDelete }: { x: UnitLive; pending: boolean; onClose: () => void; onArchive: (reason: string) => void; onDelete: (reason: string) => void }) {
  const [reason, setReason] = useState("");
  const [tried, setTried] = useState(false);
  const e = reasonError(reason);
  const blocked = !!x.blockers?.length;
  const go = (fn: (r: string) => void) => { setTried(true); if (!e) fn(reason.trim()); };
  return (
    <Modal open onClose={onClose} title="Delete unit" footer={<><Btn onClick={onClose}>Close</Btn><Btn variant="danger" disabled={pending || blocked} onClick={() => go(onArchive)}>Delete unit</Btn></>}>
      <div className="flex flex-col gap-3">
        <p className="text-[13px] text-muted">Removes the unit from lists, totals and candidates. The ID and history are kept (archived); active automations are disabled with reason unit_archived (IR39).</p>
        <SummaryList items={[["Unit", `${x.u.displayName} · version ${x.u.version}`], ["Location", x.path]]} />
        {blocked && (
          <Banner tone="crit"><b>CONFLICT · this unit is still in use</b>
            <ul className="mt-1 list-disc pl-4">{x.blockers!.map((b) => <li key={b.label}>{b.href ? <TextLink href={b.href}>{b.label}</TextLink> : b.label}</li>)}</ul>
            <div className="mt-1 text-[12px]">End or move these first. Ended history alone does not block deletion.</div>
          </Banner>
        )}
        {x.blockers === null && <Banner>The blockers (contracts, jobs, device binding) are not visible with your permissions — the Core API checks them on delete.</Banner>}
        <Field label="Reason · required" hint="1–1000 characters" error={tried ? e : undefined}><Textarea rows={2} value={reason} onChange={(v) => setReason(v.target.value)} /></Field>
        <details className="text-[12px] text-muted">
          <summary className="cursor-pointer font-semibold">Registered by mistake?</summary>
          <p className="mt-1">A unit that never had contracts, jobs or IoT can be removed completely (units.delete); otherwise this is CONFLICT and the unit has to be deleted as above.</p>
          <Btn size="sm" variant="ghost" className="mt-1" disabled={pending} onClick={() => go(onDelete)}>Remove permanently</Btn>
        </details>
      </div>
    </Modal>
  );
}

function AttachModal({ c, unit, attached, pending, onClose, onSave }: { c: CustomerLive; unit: string; attached: string[]; pending: boolean; onClose: () => void; onSave: (ids: string[]) => void }) {
  const [pick, setPick] = useState<string[]>([]);
  const options = (c.policies ?? []).filter((p) => p.kind === "alert");
  return (
    <Modal open onClose={onClose} title={`Attach policies to ${unit}`} footer={<><Btn onClick={onClose}>Cancel</Btn><Btn variant="primary" disabled={pending || !pick.length} onClick={() => onSave(pick)}>Attach {pick.length ? policyCount(pick.length) : "policy"}</Btn></>}>
      <div className="flex flex-col gap-2">
        <p className="text-[13px] text-muted">Showing {c.row.name}’s policies only. The default policy is always on and not listed.</p>
        {options.length === 0 && <EmptyState title="No customer policies yet">Create one in Alert policies first.</EmptyState>}
        {options.map((p) => {
          const already = attached.includes(p.id);
          return (
            <div key={p.id} className={cx("rounded-xl border border-line px-3 py-2", already && "bg-surface2/60")}>
              <Check disabled={already} checked={already || pick.includes(p.id)} onChange={(on) => setPick((v) => (on ? [...v, p.id] : v.filter((x) => x !== p.id)))}
                label={<span><b>{p.name}</b> <Badge>{p.type}</Badge>{!p.enabled && <Badge className="ml-1">Off</Badge>}<span className="block text-xs text-muted">{p.condition}{already ? " · Already attached" : ""}</span></span>} />
            </div>
          );
        })}
        <p className="text-[11px] text-muted">Disabled policies can be attached; they start alerting once enabled. <TextLink href="/admin/alerts?tab=policies">+ New policy for {c.row.name}</TextLink></p>
      </div>
    </Modal>
  );
}
