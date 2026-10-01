"use client";

import { useState } from "react";
import { Badge, Banner, Btn, Card, Check, Choice, Field, Input, ListRow, Modal, Page, Search, Select, Textarea, cx, useToast } from "@/components/ui";

type U = { id: string; role: string; org: string; email: string; role2: "admin" | "contractor" | "technician" };
const users: U[] = [
  { id: "hq-operator", role: "Admin · HQ", org: "HQ (tenant)", email: "r.operator@hq.example", role2: "admin" },
  { id: "hq-restriction-manager", role: "Admin · HQ", org: "HQ (tenant)", email: "r.lim@hq.example", role2: "admin" },
  { id: "aziz.contractor-a", role: "Contractor · contractor-a", org: "contractor-a", email: "aziz@contractor-a.example", role2: "contractor" },
  { id: "tech-internal-a", role: "Technician · internal · HQ field team", org: "HQ (tenant)", email: "ali@hq.example", role2: "technician" },
  { id: "tech-external-b", role: "Technician · external · contractor-a", org: "contractor-a", email: "b@contractor-a.example", role2: "technician" },
  { id: "tech-external-old", role: "Technician · external · contractor-a", org: "contractor-a", email: "old@contractor-a.example", role2: "technician" },
];
const perms: Record<string, string[]> = {
  "MONITORING & REPORTS": ["dashboard.read", "audit.read", "energy.manage", "mrv.manage", "offset.manage"],
  "UNITS & DEVICES": ["asset.manage", "control.execute", "control.diagnose", "device.maintain", "device.manage"],
  "ALERTS, AUTOMATION & JOBS": ["alert.resolve", "alert.policy.manage", "automation.policy.manage", "job.manage"],
  PARTNERS: ["partner.accept", "partner.assign", "partner.review"],
  "BILLING & RESTRICTIONS": ["contract.manage", "billing.manage", "restriction.manage", "restriction.override"],
  IDENTITY: ["identity.manage"],
};
const preset: Record<U["role2"], string[]> = {
  admin: ["dashboard.read", "audit.read", "energy.manage", "asset.manage", "control.execute", "alert.resolve", "alert.policy.manage", "job.manage", "contract.manage", "billing.manage", "restriction.manage", "identity.manage", "device.manage", "automation.policy.manage", "mrv.manage"],
  contractor: ["partner.accept", "partner.assign", "partner.review"],
  technician: ["control.diagnose", "device.maintain"],
};

export default function Access() {
  const toast = useToast();
  const [q, setQ] = useState("");
  const [sel, setSel] = useState<U | "new">(users[1]);
  const [role, setRole] = useState<U["role2"]>("admin");
  const [checked, setChecked] = useState<string[]>(preset.admin);
  const [reason, setReason] = useState("");
  const [end, setEnd] = useState("");
  const [msg, setMsg] = useState<{ t: "crit" | "ok" | "warn"; m: string } | null>(null);
  const [revoke, setRevoke] = useState(false);
  const list = users.filter((u) => !q || (u.id + u.role + u.email).toLowerCase().includes(q.toLowerCase()));
  const pick = (u: U | "new") => { setSel(u); setMsg(null); const r = u === "new" ? "contractor" : u.role2; setRole(r); setChecked(preset[r]); setReason(""); setEnd(u !== "new" && u.id.startsWith("tech-external") ? "" : ""); };
  const choose = (r: U["role2"]) => { setRole(r); setChecked(preset[r]); };
  const total = Object.values(perms).flat().length;
  const save = () => {
    if (!reason.trim()) return setMsg({ t: "crit", m: "Change reason is required (1–1000 characters)." });
    if (role === "technician" && sel !== "new" && sel.role.includes("external") && !end) return setMsg({ t: "crit", m: "External technicians need a “Valid until” end date (VALIDATION)." });
    if (checked.includes("restriction.override") && sel !== "new" && sel.id === "hq-operator") return setMsg({ t: "crit", m: "FORBIDDEN — you cannot grant yourself restriction.override." });
    setMsg({ t: "ok", m: "Saved — scopeVersion incremented and recorded in the audit log." }); toast("Access saved");
  };
  const cur = sel === "new" ? null : sel;
  return (
    <Page>
      <div className="split-rev">
        <Card title="Users" action={<Btn size="sm" variant="primary" onClick={() => pick("new")}>+ New user</Btn>} sub={`${users.length} · members.list · identity.manage`} className="self-start">
          <div className="mb-2"><Search placeholder="Search name, email, organization…" value={q} onChange={setQ} /></div>
          <div className="flex flex-col gap-1.5">{list.map((u) => <ListRow key={u.id} selected={sel !== "new" && sel.id === u.id} onClick={() => pick(u)}><div className="min-w-0"><b className="text-[13px]">{u.id}</b><div className="truncate text-[11px] text-muted">{u.role}</div></div></ListRow>)}</div>
          <p className="mt-3 text-[11px] text-muted">Client accounts are not listed here — they are managed per customer in Customers & units › Users.</p>
        </Card>
        <Card title={cur ? cur.id : "New user"} sub={cur ? `${cur.email} · scopeVersion 1 · last changed 2026-09-02 by hq-operator` : "Identity & role"}>
          <div className="flex flex-col gap-5">
            <section>
              <h3 className="mb-2 text-[13px] font-bold">Identity & role</h3>
              <p className="mb-2 text-xs text-muted">Choosing a role only pre-checks its typical permissions; every permission below stays editable.</p>
              <div className="grid-fluid" style={{ ["--min"as string]: "220px" }}>
                <Field label="Organization"><Select defaultValue={cur?.org}><option>HQ (tenant)</option><option>contractor-a</option></Select></Field>
                <Field label="Person (email)"><Input defaultValue={cur?.email ?? ""} placeholder="name@example.com" /></Field>
              </div>
              <div className="mt-3"><div className="mb-1 text-xs font-semibold">Role</div><Choice value={role} onChange={choose} options={[{ id: "admin", label: "Admin" }, { id: "contractor", label: "Contractor" }, { id: "technician", label: "Technician" }]} /><p className="mt-1 text-[11px] text-muted">Client role is not offered here (Customers & units › Users).</p></div>
            </section>
            <section><h3 className="mb-1 text-[13px] font-bold">Scope</h3><p className="text-xs text-muted">Admins act on every customer of the tenant. <Badge tone="primary">All customers (HQ tenant)</Badge></p></section>
            <section>
              <h3 className="mb-2 text-[13px] font-bold">Valid period</h3>
              <div className="grid-fluid" style={{ ["--min"as string]: "200px" }}><Field label="Valid from"><Input defaultValue="2026-01-05 09:00" /></Field><Field label="Valid until" hint="Optional for HQ and contractors. Revoking = setting “Valid until” to now."><Input type="date" value={end} onChange={(e) => setEnd(e.target.value)} /></Field></div>
            </section>
            <section>
              <h3 className="mb-2 text-[13px] font-bold">Permissions · {checked.length} of {total}</h3>
              <div className="grid-fluid" style={{ ["--min"as string]: "200px" }}>{Object.entries(perms).map(([g, ps]) => <div key={g}><div className="mb-1 text-[10px] font-bold tracking-wide text-muted">{g}</div><div className="flex flex-col gap-1">{ps.map((p) => <Check key={p} label={<span className="font-mono text-xs">{p}</span>} checked={checked.includes(p)} onChange={(v) => setChecked((c) => (v ? [...c, p] : c.filter((x) => x !== p)))} />)}</div></div>)}</div>
            </section>
            <Field label="Change reason · required" hint="1–1000 characters. Stored in the audit log."><Textarea value={reason} onChange={(e) => setReason(e.target.value)} /></Field>
            {msg && <Banner tone={msg.t}>{msg.m}</Banner>}
            <div className="flex flex-wrap justify-end gap-2">{cur && <Btn variant="danger" onClick={() => setRevoke(true)}>Revoke access…</Btn>}<Btn variant="primary" onClick={save}>Save changes</Btn></div>
          </div>
        </Card>
      </div>
      <Modal open={revoke} onClose={() => setRevoke(false)} title={`Revoke access — ${cur?.id}`} footer={<><Btn onClick={() => setRevoke(false)}>Cancel</Btn><Btn variant="danger" onClick={() => { setRevoke(false); toast("Access revoked — user is now Inactive", "warn"); }}>Revoke</Btn></>}><p className="text-[13px]">Revoking sets “Valid until” to now. The user stays listed as Inactive.</p></Modal>
    </Page>
  );
}
