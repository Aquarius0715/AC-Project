"use client";

import { useState } from "react";
import { setSearchParam, useSearchParam } from "@/lib/urlState";
import { ImportCsvModal, WarrantyTab } from "@/components/Features";
import { ClientRole, ClientUser, clientUserActions, emailError, useClientUsers } from "@/lib/clientUsers";
import { Badge, Banner, Btn, Card, DataTable, Field, Input, Kpi, ListRow, Modal, Page, PowerBadge, ConnBadge, Search, Select, SummaryList, Tabs, useToast } from "@/components/ui";

type Cust = { id: string; name: string; status: "active" | "inactive"; props: number; units: number; op: string; alerts: number | string; contract: string };
const customers: Cust[] = [
  { id: "customer-a", name: "Tan household", status: "active", props: 1, units: 3, op: "50.0%", alerts: 1, contract: "contract-rto-a" },
  { id: "customer-b", name: "Lim Trading Sdn Bhd", status: "active", props: 1, units: 2, op: "50.0%", alerts: 0, contract: "contract-rto-b" },
  { id: "customer-z", name: "inactive since 2025-12-31", status: "inactive", props: 0, units: 0, op: "—", alerts: "—", contract: "—" },
];
const locations = [{ id: "home-a", name: "Home A", kids: ["1F", "2F"] }, { id: "bedroom", name: "Bedroom", kids: [] }];
const unitRows = [
  { id: "unit-online-rto", name: "Bedroom AC", loc: "Home A › 1F › Bedroom", model: "ventilation-demo v3", power: "running" as const, conn: "online" as const },
  { id: "unit-bedroom-2", name: "Bedroom AC #2", loc: "Home A › 1F › Bedroom", model: "cap-split-std v1", power: "stopped" as const, conn: "online" as const },
  { id: "unit-non-rto", name: "Living room AC", loc: "Home A › 1F › Living room", model: "cap-split-std v1", power: "running" as const, conn: "online" as const },
];

export default function AdminUnits() {
  const toast = useToast();
  const [q, setQ] = useState("");
  // URL state is the source of truth (SCR-A02: customerId + tab=overview|users|policies|warranty, Figma 02-15 / 02-16 / 02-19)
  const urlTab = useSearchParam("tab");
  const urlCustomer = useSearchParam("customerId");
  const cust = customers.find((x) => x.id === urlCustomer) ?? null;
  const top: "customers" | "warranty" = !cust && urlTab === "warranty" ? "warranty" : "customers";
  const tab: "units" | "users" | "policies" = urlTab === "users" || urlTab === "policies" ? urlTab : "units";
  const setTop = (t: "customers" | "warranty") => setSearchParam("tab", t === "warranty" ? "warranty" : "overview");
  const setTab = (t: "units" | "users" | "policies") => setSearchParam("tab", t === "units" ? "overview" : t);
  const setCust = (c: Cust | null) => {
    setSearchParam("customerId", c ? c.id : null);
    setSearchParam("tab", "overview");
  };
  const [imp, setImp] = useState(false);
  const [status, setStatus] = useState<"active" | "all">("active");
  const [loc, setLoc] = useState("home-a");
  const [unit, setUnit] = useState<(typeof unitRows)[number] | null>(null);
  const [modal, setModal] = useState<null | "customer" | "location" | "rename" | "delete" | "unit" | "move" | "attach" | "deleteUnit">(null);
  const [name, setName] = useState("");
  const [reason, setReason] = useState("");
  const [tried, setTried] = useState(false);
  const rows = customers.filter((c) => (status === "all" || c.status === "active") && (!q || (c.id + c.name).toLowerCase().includes(q.toLowerCase())));
  const close = () => { setModal(null); setTried(false); setName(""); setReason(""); };
  const nameErr = tried && !name.trim() ? "Name is required (1–120 characters)" : undefined;

  if (!cust) {
    return (
      <Page>
        <div className="flex flex-wrap items-center justify-between gap-2"><Tabs value={top} onChange={setTop} tabs={[{ id: "customers", label: "Customers & units", count: 3 }, { id: "warranty", label: "Warranty & coverage", count: 5 }]} /><Btn size="sm" onClick={() => setImp(true)}>↑ Import CSV</Btn></div>
        <ImportCsvModal open={imp} onClose={() => setImp(false)} />
        {top === "warranty" ? <WarrantyTab /> : <>
        <div className="grid-fluid" style={{ ["--min"as string]: "180px" }}><Kpi label="Customers" value={2} sub="+1 inactive (not counted)" /><Kpi label="Properties" value={2} sub="Home A · Home B" /><Kpi label="Units" value={5} sub="Running 2 · Stopped 2 · unknown 1" /><Kpi label="Needs attention" value={1} tone="warn" sub="overdue billing / open alert" /></div>
        <Card title="Customers" sub="Select a customer to manage its properties, spaces, and units" action={<Btn size="sm" variant="primary" onClick={() => setModal("customer")}>+ New customer</Btn>}>
          <div className="mb-3 flex flex-wrap items-center gap-3"><div className="min-w-[220px] flex-1 sm:max-w-sm"><Search placeholder="Search customer name, ID, or property…" value={q} onChange={setQ} /></div><Select aria-label="Status" className="w-auto" value={status} onChange={(e) => setStatus(e.target.value as "all")}><option value="active">Status: Active</option><option value="all">Status: All</option></Select></div>
          <DataTable rows={rows} rowKey={(r) => r.id} onRowClick={(c) => setCust(c)} cols={[{ key: "c", label: "Customer", render: (c) => <><b>{c.id}</b><div className="text-xs text-muted">{c.name}</div></> }, { key: "s", label: "Status", render: (c) => <Badge tone={c.status === "active" ? "ok" : "unknown"}>{c.status}</Badge> }, { key: "p", label: "Properties", render: (c) => c.props, hideBelow: "sm" }, { key: "u", label: "Units", render: (c) => c.units }, { key: "o", label: "Operation", render: (c) => c.op, hideBelow: "md" }, { key: "a", label: "Alerts", render: (c) => c.alerts, hideBelow: "md" }, { key: "k", label: "Contract", render: (c) => c.contract, hideBelow: "md" }, { key: "x", label: "", render: () => "›" }]} />
          <p className="mt-2 text-[11px] text-muted">{rows.length} of 3 · sorted by name · Inactive customers appear only with Status: All (IR40). Archived properties/spaces/units never appear in lists, summaries, or KPI denominators (IR39).</p>
        </Card>
        </>}
        <Modal open={modal === "customer"} onClose={close} title="New customer" footer={<><Btn onClick={close}>Cancel</Btn><Btn variant="primary" onClick={() => { setTried(true); if (!name.trim()) return; toast(`Customer “${name}” created`); close(); }}>Create</Btn></>}><Field label="Customer name" error={nameErr}><Input value={name} onChange={(e) => setName(e.target.value)} /></Field></Modal>
      </Page>
    );
  }
  return (
    <Page>
      <div className="flex flex-wrap items-center gap-2 text-[13px]"><button className="font-semibold text-primary" onClick={() => { setCust(null); setUnit(null); }}>← Customers & units</button><span className="text-muted">› {cust.id} · {cust.name}</span></div>
      <Tabs value={tab} onChange={setTab} tabs={[{ id: "units", label: "Units & locations" }, { id: "users", label: "Users" }, { id: "policies", label: "Alert policies" }]} />
      {tab === "units" && (
        <div className="split-rev">
          <Card title="Locations" action={<Btn size="sm" onClick={() => setModal("location")}>+ Add</Btn>} className="self-start">
            <div className="flex flex-col gap-1.5">{locations.map((l) => <ListRow key={l.id} selected={loc === l.id} onClick={() => { setLoc(l.id); setUnit(null); }}><b className="flex-1 text-[13px]">⌂ {l.name}</b><button aria-label="Rename" className="text-muted" onClick={(e) => { e.stopPropagation(); setName(l.name); setModal("rename"); }}>✎</button></ListRow>)}</div>
            <Btn size="sm" variant="danger" className="mt-3" onClick={() => setModal("delete")}>Delete location…</Btn>
          </Card>
          <div className="flex min-w-0 flex-col gap-4">
            <Card title={`${loc === "home-a" ? "Home A" : "Bedroom"} — units`} action={<Btn size="sm" variant="primary" onClick={() => setModal("unit")}>+ New unit</Btn>}>
              <DataTable rows={loc === "bedroom" ? unitRows.slice(0, 2) : unitRows} rowKey={(r) => r.id} selectedKey={unit?.id} onRowClick={setUnit} cols={[{ key: "n", label: "Unit", render: (r) => <><b>{r.name}</b><div className="text-xs text-muted">{r.id}</div></> }, { key: "l", label: "Location", render: (r) => r.loc, hideBelow: "sm" }, { key: "m", label: "Model", render: (r) => r.model, hideBelow: "md" }, { key: "p", label: "Power", render: (r) => <PowerBadge s={r.power} /> }, { key: "c", label: "Conn.", render: (r) => <ConnBadge s={r.conn} />, hideBelow: "sm" }]} />
            </Card>
            {unit && (
              <Card title={`Unit edit — ${unit.name}`} sub={unit.id} action={<Btn size="sm" variant="ghost" onClick={() => setUnit(null)}>✕</Btn>}>
                <SummaryList cols={2} items={[["Name", unit.name], ["Location", unit.loc], ["Model", unit.model], ["Status", "Active"]]} />
                <div className="mt-3 flex flex-wrap gap-2"><Btn size="sm" onClick={() => setModal("move")}>Change location…</Btn><Btn size="sm" onClick={() => setModal("attach")}>+ Attach policy</Btn><Btn size="sm" variant="danger" onClick={() => setModal("deleteUnit")}>Delete unit…</Btn></div>
              </Card>
            )}
          </div>
        </div>
      )}
      {tab === "users" && <ClientUsersTab />}
      {tab === "policies" && <Card title="Customer › Alert policies" sub="customer-a policies only">{[["Bedroom too hot", "2 units"], ["Stuffy office", "2 units"], ["Night humidity", "0 units · Off"]].map(([a, b]) => <div key={a} className="flex justify-between border-t border-line py-2 text-[13px] first:border-0"><b>{a}</b><span className="text-muted">{b}</span></div>)}</Card>}

      <Modal open={modal === "location" || modal === "rename"} onClose={close} title={modal === "rename" ? "Rename location" : "Add location"} footer={<><Btn onClick={close}>Cancel</Btn><Btn variant="primary" onClick={() => { setTried(true); if (!name.trim()) return; toast("Location saved"); close(); }}>Save</Btn></>}><Field label="Name" error={nameErr}><Input value={name} onChange={(e) => setName(e.target.value)} /></Field></Modal>
      <Modal open={modal === "delete"} onClose={close} title="Delete location" footer={<Btn onClick={close}>Close</Btn>}><Banner tone="crit">CONFLICT — this location still has units. Move or delete them first.</Banner></Modal>
      <Modal open={modal === "deleteUnit"} onClose={close} title="Delete unit" footer={<Btn onClick={close}>Close</Btn>}><Banner tone="crit">CONFLICT — unit is in use (contract scope / open jobs). It cannot be deleted.</Banner></Modal>
      <Modal open={modal === "unit"} onClose={close} title="New unit in Bedroom" footer={<><Btn onClick={close}>Cancel</Btn><Btn variant="primary" onClick={() => { setTried(true); if (!name.trim()) return; toast("Unit created"); close(); }}>Create</Btn></>}><Field label="Unit name" error={nameErr}><Input value={name} onChange={(e) => setName(e.target.value)} /></Field><Field label="Model"><Select><option>ventilation-demo v3</option><option>cap-split-std v1</option></Select></Field></Modal>
      <Modal open={modal === "move"} onClose={close} title="Change location" footer={<><Btn onClick={close}>Cancel</Btn><Btn variant="primary" onClick={() => { setTried(true); if (!reason.trim()) return; toast("Location changed"); close(); }}>Save</Btn></>}><Field label="New location"><Select><option>Home A › 1F › Kitchen</option><option>Home A › 2F › Study</option></Select></Field><Field label="Reason" error={tried && !reason.trim() ? "A reason is required" : undefined}><Input value={reason} onChange={(e) => setReason(e.target.value)} /></Field></Modal>
      <Modal open={modal === "attach"} onClose={close} title="Attach policies to Bedroom AC" footer={<><Btn onClick={close}>Cancel</Btn><Btn variant="primary" onClick={() => { toast("Policy attached"); close(); }}>Attach</Btn></>}><Field label="customer-a policies only"><Select><option>Bedroom too hot</option><option>Stuffy office</option><option>Night humidity</option></Select></Field></Modal>
    </Page>
  );
}

/** FR-A17 — HQ manages every client user; owners can also invite members from the customer app (FR-C19). */
function ClientUsersTab() {
  const toast = useToast();
  const users = useClientUsers();
  const [open, setOpen] = useState(false);
  const [email, setEmail] = useState("");
  const [role, setRole] = useState<ClientRole>("member");
  const [tried, setTried] = useState(false);
  const err = tried ? emailError(email, users) : undefined;
  const close = () => { setOpen(false); setTried(false); setEmail(""); setRole("member"); };
  const run = (e: string | undefined, ok: string) => toast(e ?? ok, e ? "warn" : undefined);
  const act = (u: ClientUser, a: string) => {
    if (a === "resend") run(clientUserActions.resend(u.id), `Invitation preview re-created for ${u.email}`);
    if (a === "role") run(clientUserActions.setRole(u.id, u.role === "owner" ? "member" : "owner"), `Role changed to ${u.role === "owner" ? "Member" : "Owner"}`);
    if (a === "reset") toast("Password reset preview created (generic message)");
    if (a === "disable") run(clientUserActions.setStatus(u.id, u.status === "disabled" ? "active" : "disabled"), u.status === "disabled" ? "Sign-in enabled" : "Sign-in disabled");
    if (a === "remove") run(clientUserActions.remove(u.id), `${u.email} removed from customer-a`);
  };
  return (
    <Card title="Client users of customer-a" sub="People who sign in to the customer app. They only ever see customer-a’s properties and units." action={<Btn size="sm" variant="primary" onClick={() => setOpen(true)}>+ Invite user</Btn>}>
      <DataTable rows={users} rowKey={(u) => u.id} cols={[
        { key: "u", label: "User", render: (u) => <><b>{u.name ?? u.email}</b><div className="text-xs text-muted">{u.name ? u.email : `invited ${u.invitedAt ?? ""} by ${u.invitedBy ?? "—"}`}</div></> },
        { key: "r", label: "Role", render: (u) => <Badge tone={u.role === "owner" ? "primary" : "muted"}>{u.role === "owner" ? "Owner" : "Member"}</Badge> },
        { key: "s", label: "Status", render: (u) => <Badge tone={u.status === "active" ? "ok" : u.status === "invited" ? "warn" : "muted"}>{u.status === "active" ? "Active" : u.status === "invited" ? "Invite pending" : "Disabled"}</Badge> },
        { key: "l", label: "Last sign-in", render: (u) => u.lastSignIn ?? "—", hideBelow: "md" },
        { key: "a", label: "", render: (u) => <Select aria-label="Actions" className="w-auto" value="" onChange={(e) => act(u, e.target.value)}><option value="">⋯</option><option value="role">Change role (Owner / Member)</option>{u.status === "invited" && <option value="resend">Resend invite</option>}<option value="reset">Reset password</option><option value="disable">{u.status === "disabled" ? "Enable sign-in" : "Disable sign-in"}</option><option value="remove">Remove from customer</option></Select> },
      ]} />
      <p className="mt-2 text-[11px] text-muted">Client users have no permission editor — the owner can also invite members from the customer app (Users). HQ, contractor and technician accounts are managed in Access &amp; roles.</p>
      <Modal open={open} onClose={close} title="Invite client user" footer={<><Btn onClick={close}>Cancel</Btn><Btn variant="primary" onClick={() => { setTried(true); const e = clientUserActions.invite(email, role, "hq-operator"); if (e) return; toast(`Invitation preview created for ${email.trim()}`); close(); }}>Send invite</Btn></>}>
        <Field label="Email" error={err}><Input type="email" value={email} onChange={(e) => setEmail(e.target.value)} /></Field>
        <Field label="Role"><Select value={role} onChange={(e) => setRole(e.target.value as ClientRole)}><option value="member">Member</option><option value="owner">Owner</option></Select></Field>
      </Modal>
    </Card>
  );
}
