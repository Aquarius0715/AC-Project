"use client";

import { useState } from "react";
import { Badge, Banner, Btn, Card, Check, Choice, EmptyState, Field, Input, ListRow, Modal, Page, Search, Select, Textarea } from "@ac/web/components/ui";
import { useAction } from "@ac/web/lib/useAction";
import { useUrlPatch } from "@ac/web/lib/useUrlPatch";
import { klLocal } from "@ac/web/lib/energy";
import {
  allowedFor, allPermissions, memberDraft, memberErrors, memberInput, orgKind, permissionRows, roleDefaults, selfBlocked, validity,
  type ApiOrganization, type MemberDraft, type MemberRow, type PermissionRow, type Role, type ScopeRef,
} from "@ac/web/lib/members";
import { saveMember } from "../actions";

type Live = {
  now: string; canWrite: boolean; selfUserId: string; tenantId: string; scope: { role?: string; status?: string }; rows: MemberRow[]; selected: string;
  users: { id: string; name: string }[]; orgs: ApiOrganization[]; properties: { id: string; name: string }[]; units: { id: string; name: string }[];
};

/** Access & roles (FR-A03) in API mode: the role / status filters and the selected membership live in the URL; saving
 * and revoking are members.save through a Server Action (a revoke sets validUntil to now). */
export function AccessView({ live }: { live: Live }) {
  const nav = useUrlPatch();
  const [pending, run] = useAction();
  const [q, setQ] = useState("");
  const sel = live.selected === "new" ? null : live.rows.find((r) => r.id === live.selected) ?? null;
  const key = sel ? `${sel.id}:${sel.version}` : "new";
  const [source, setSource] = useState(key);
  const [d, setD] = useState<MemberDraft>(memberDraft(sel?.m, live.tenantId));
  const [tried, setTried] = useState(false);
  const [revoke, setRevoke] = useState<string | null>(null);
  const [scopeKind, setScopeKind] = useState<"unit" | "property" | "organization">("unit");
  const [scopeId, setScopeId] = useState("");
  if (source !== key) { // the form restarts from each saved membership version (adjust state during render)
    setSource(key);
    setD(memberDraft(sel?.m, live.tenantId));
    setTried(false);
  }
  const errors = memberErrors(d, { selfUserId: live.selfUserId, current: sel?.m });
  const set = (patch: Partial<MemberDraft>) => setD((x) => ({ ...x, ...patch }));
  const isNew = !sel;
  const self = d.userId === live.selfUserId;
  const allowed = allowedFor[d.role];
  const has = (p?: string) => !!p && d.permissions.includes(p);
  const toggle = (p: string, on: boolean) => set({ permissions: on ? [...new Set([...d.permissions, p])] : d.permissions.filter((x) => x !== p) });
  const setWrite = (r: PermissionRow, on: boolean) => set({ permissions: on ? [...new Set([...d.permissions, r.write!, ...(r.read ? [r.read] : [])])] : d.permissions.filter((x) => x !== r.write) });
  const blocked = (p: string) => !allowed.includes(p) || !live.canWrite || (self && selfBlocked.includes(p) && !(sel?.m.permissions ?? []).includes(p));
  const chooseRole = (r: Role) => {
    const scopes: ScopeRef[] = r === "admin" ? [{ kind: "tenant", id: live.tenantId }] : r === "contractor" && d.organizationId ? [{ kind: "organization", id: d.organizationId }] : [];
    set({ role: r, permissions: roleDefaults[r], organizationId: "", scopes });
  };
  const chooseOrg = (id: string) => set({ organizationId: id, scopes: d.role === "contractor" ? [{ kind: "organization", id }] : d.scopes });
  const orgs = live.orgs.filter((o) => o.kind === orgKind(d.role, d.employment));
  const customers = live.orgs.filter((o) => o.kind === "customer");
  const targets = scopeKind === "unit" ? live.units : scopeKind === "property" ? live.properties : customers.map((o) => ({ id: o.id, name: o.name }));
  const scopeName = (s: ScopeRef) => (s.kind === "tenant" ? "All customers (tenant)" : (s.kind === "unit" ? live.units : s.kind === "property" ? live.properties : live.orgs).find((x) => x.id === s.id)?.name ?? s.id.slice(0, 8));
  const save = () => {
    setTried(true);
    if (Object.keys(errors).length > 0) return;
    run(() => saveMember(memberInput(d, sel?.id), sel?.version), sel ? "Access saved — scopeVersion updated" : "Membership created", (m) => nav({ membershipId: m.id }));
  };
  const doRevoke = () => sel && revoke !== null && revoke.trim() && run(
    () => saveMember(memberInput({ ...d, permissions: sel.m.permissions, scopes: sel.m.scopes, validUntil: klLocal(live.now), reason: revoke.trim() }, sel.id), sel.version),
    "Access revoked — the membership is now inactive", () => setRevoke(null));
  const err = (k: string) => (tried ? errors[k] : undefined);
  const shown = live.rows.filter((r) => !q || `${r.name} ${r.label}`.toLowerCase().includes(q.toLowerCase()));
  return (
    <Page>
      <div className="flex flex-wrap items-center gap-3">
        <Choice value={live.scope.role ?? "all"} onChange={(v) => nav({ role: v === "all" ? null : v, membershipId: null })} options={[{ id: "all", label: "All roles" }, { id: "admin", label: "Admin" }, { id: "contractor", label: "Contractor" }, { id: "technician", label: "Technician" }]} />
        <Choice value={live.scope.status ?? "all"} onChange={(v) => nav({ status: v === "all" ? null : v, membershipId: null })} options={[{ id: "all", label: "All" }, { id: "active", label: "Active" }, { id: "inactive", label: "Inactive" }]} />
      </div>
      <div className="split-rev">
        <Card title="Memberships" sub={`${live.rows.length} · HQ, contractor and technician`} action={live.canWrite && <Btn size="sm" variant="primary" onClick={() => nav({ membershipId: "new" })}>+ New membership</Btn>} className="self-start">
          <div className="mb-2"><Search placeholder="Search name or organization…" value={q} onChange={setQ} /></div>
          {shown.length === 0 ? <EmptyState title="No memberships">No membership matches these filters.</EmptyState> : <div className="flex flex-col gap-1.5">{shown.map((u) => <ListRow key={u.id} selected={sel?.id === u.id} onClick={() => nav({ membershipId: u.id })}><div className="min-w-0 flex-1"><div className="flex items-center justify-between gap-2"><b className="truncate text-[13px]">{u.name}</b><Badge tone={u.active ? "ok" : "muted"}>{u.active ? "Active" : "Inactive"}</Badge></div><div className="truncate text-[11px] text-muted">{u.label}</div></div></ListRow>)}</div>}
          <p className="mt-3 text-[11px] text-muted">Client accounts are not listed here — they are managed per customer in Customers & units › Users.</p>
        </Card>
        <Card title={sel ? sel.name : "New membership"} sub={sel ? `${validity(sel.m)} · scopeVersion ${sel.m.scopeVersion} · version ${sel.version}` : "Another membership for an existing user (users come from the identity provider)"}>
          <div className="flex flex-col gap-5">
            {!live.canWrite && <Banner>Read only — editing needs identity.write.</Banner>}
            <section>
              <h3 className="mb-2 text-[13px] font-bold">Identity & role</h3>
              <div className="grid-fluid" style={{ ["--min" as string]: "220px" }}>
                <Field label="User" error={err("userId")} hint={!isNew ? "Fixed after creation" : undefined}><Select value={d.userId} disabled={!isNew || !live.canWrite} onChange={(e) => set({ userId: e.target.value })}><option value="">Select…</option>{live.users.map((u) => <option key={u.id} value={u.id}>{u.name}</option>)}</Select></Field>
                <Field label="Organization" error={err("organizationId")} hint={!isNew ? "Fixed after creation" : undefined}><Select value={d.organizationId} disabled={!isNew || !live.canWrite} onChange={(e) => chooseOrg(e.target.value)}><option value="">Select…</option>{orgs.map((o) => <option key={o.id} value={o.id}>{o.name}</option>)}</Select></Field>
              </div>
              <div className="mt-3"><div className="mb-1 text-xs font-semibold">Role</div>{isNew ? <Choice value={d.role} onChange={chooseRole} options={[{ id: "admin", label: "Admin" }, { id: "contractor", label: "Contractor" }, { id: "technician", label: "Technician" }]} /> : <Badge tone="primary">{d.role}</Badge>}<p className="mt-1 text-[11px] text-muted">Choosing a role only pre-checks its typical permissions. The client role is not offered here.</p></div>
              {d.role === "technician" && <div className="mt-3 max-w-xs"><Field label="Employment"><Select value={d.employment} disabled={!isNew || !live.canWrite} onChange={(e) => set({ employment: e.target.value as MemberDraft["employment"], organizationId: "" })}><option value="internal">Internal (HQ field team)</option><option value="external">External (contractor)</option></Select></Field></div>}
            </section>
            <section>
              <h3 className="mb-1 text-[13px] font-bold">Scope</h3>
              {d.role === "admin" ? <p className="text-xs text-muted">Admins act on every customer of the tenant. <Badge tone="primary">All customers (tenant)</Badge></p>
                : d.role === "contractor" ? <p className="text-xs text-muted">Contractors act for their own organization{d.organizationId ? ` (${scopeName({ kind: "organization", id: d.organizationId })})` : ""}.</p>
                : <>
                  <div className="flex flex-wrap gap-1.5">{d.scopes.map((s) => <span key={`${s.kind}:${s.id}`} className="inline-flex items-center gap-1 rounded-full bg-surface2 px-2.5 py-1 text-xs">{s.kind} · {scopeName(s)}{live.canWrite && <button aria-label="Remove scope" onClick={() => set({ scopes: d.scopes.filter((x) => x !== s) })}>✕</button>}</span>)}{d.scopes.length === 0 && <span className="text-xs text-muted">No targets — zero business targets.</span>}</div>
                  {live.canWrite && <div className="mt-2 flex flex-wrap items-end gap-2"><Field label="Add scope"><Select value={scopeKind} onChange={(e) => { setScopeKind(e.target.value as typeof scopeKind); setScopeId(""); }}><option value="unit">Unit</option><option value="property">Property</option><option value="organization">Customer organization</option></Select></Field><Field label="Target"><Select value={scopeId} onChange={(e) => setScopeId(e.target.value)}><option value="">Select…</option>{targets.filter((t) => !d.scopes.some((s) => s.id === t.id)).map((t) => <option key={t.id} value={t.id}>{t.name}</option>)}</Select></Field><Btn size="sm" disabled={!scopeId} onClick={() => { set({ scopes: [...d.scopes, { kind: scopeKind, id: scopeId }] }); setScopeId(""); }}>+ Add</Btn></div>}
                  {err("scopes") && <p className="mt-1 text-xs text-crit">{err("scopes")}</p>}
                </>}
            </section>
            <section>
              <h3 className="mb-2 text-[13px] font-bold">Valid period</h3>
              <div className="grid-fluid" style={{ ["--min" as string]: "200px" }}>
                <Field label="Valid from (Kuala Lumpur)" error={err("validFrom")}><Input type="datetime-local" value={d.validFrom} disabled={!live.canWrite} onChange={(e) => set({ validFrom: e.target.value })} /></Field>
                <Field label="Valid until" error={err("validUntil")} hint={d.role === "technician" && d.employment === "external" ? "Required for external technicians" : "Optional — revoking sets it to now"}><Input type="datetime-local" value={d.validUntil} disabled={!live.canWrite} onChange={(e) => set({ validUntil: e.target.value })} /></Field>
              </div>
            </section>
            <section>
              <h3 className="mb-2 text-[13px] font-bold">Permissions · {d.permissions.length} of {allPermissions.length}</h3>
              <div className="scroll-x"><table className="w-full min-w-[560px] text-left text-[13px]"><thead className="text-[11px] uppercase text-muted"><tr><th className="py-1.5 pr-3">Resource</th><th className="pr-3">Read</th><th className="pr-3">Write</th><th>Actions</th></tr></thead><tbody>{permissionRows.map((r) => (
                <tr key={r.res} className="border-t border-line"><td className="py-2 pr-3 font-semibold">{r.res}</td>
                  <td className="pr-3">{r.read ? <Check label={<span className="font-mono text-xs">{r.read}</span>} checked={has(r.read)} disabled={blocked(r.read) || has(r.write)} onChange={(v) => toggle(r.read!, v)} /> : <span className="text-muted">—</span>}</td>
                  <td className="pr-3">{r.write ? <Check label={<span className="font-mono text-xs">{r.write}</span>} checked={has(r.write)} disabled={blocked(r.write)} onChange={(v) => setWrite(r, v)} /> : <span className="text-muted">—</span>}</td>
                  <td><div className="flex flex-wrap gap-x-3 gap-y-1">{(r.actions ?? []).map((a) => <Check key={a} label={<span className="font-mono text-xs">{a}</span>} checked={has(a)} disabled={blocked(a)} onChange={(v) => toggle(a, v)} />)}{!r.actions && <span className="text-muted">—</span>}</div></td></tr>
              ))}</tbody></table></div>
              <p className="mt-2 text-[11px] text-muted">Write includes Read (Read stays on while Write is on). Actions are independent. {self ? "identity.write and restriction.override cannot be granted to yourself." : ""}</p>
              {err("permissions") && <p className="mt-1 text-xs text-crit">{err("permissions")}</p>}
            </section>
            {live.canWrite && <>
              <Field label="Change reason · required" error={err("reason")} hint="1–1000 characters, stored in the audit log"><Textarea value={d.reason} maxLength={1000} onChange={(e) => set({ reason: e.target.value })} /></Field>
              <div className="flex flex-wrap justify-end gap-2">{sel?.active && <Btn variant="danger" disabled={pending} onClick={() => setRevoke("")}>Revoke access…</Btn>}<Btn variant="primary" disabled={pending} onClick={save}>{sel ? "Save changes" : "Create membership"}</Btn></div>
            </>}
          </div>
        </Card>
      </div>
      <Modal open={revoke !== null} onClose={() => setRevoke(null)} title={`Revoke access — ${sel?.name ?? ""}`} footer={<><Btn onClick={() => setRevoke(null)}>Cancel</Btn><Btn variant="danger" disabled={pending || !revoke?.trim()} onClick={doRevoke}>Revoke</Btn></>}>
        <p className="text-[13px]">Revoking sets “Valid until” to now. The membership stays listed as inactive; nothing is deleted.</p>
        <Field label="Reason"><Textarea value={revoke ?? ""} maxLength={1000} onChange={(e) => setRevoke(e.target.value)} /></Field>
      </Modal>
    </Page>
  );
}
