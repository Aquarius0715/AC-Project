"use client";

import { useState } from "react";
import { Badge, Banner, Btn, Card, Check, Choice, EmptyState, Field, Input, ListRow, Modal, Page, Search, Select, Textarea } from "@ac/web/components/ui";
import { useI18n } from "@ac/web/components/I18n";
import { useAction } from "@ac/web/lib/useAction";
import { useUrlPatch } from "@ac/web/lib/useUrlPatch";
import {
  allowedFor, allPermissions, memberDraft, memberErrors, memberInput, orgKind, permissionRows, resourceWord, roleDefaults, roleWord, selfBlocked, zonedLocal,
  type ApiOrganization, type MemberDraft, type MemberRow, type PermissionRow, type Role, type ScopeRef,
} from "@ac/web/lib/members";
import { saveMember } from "../actions";

type Live = {
  now: string; canWrite: boolean; selfUserId: string; tenantId: string; scope: { role?: string; status?: string }; rows: MemberRow[]; selected: string;
  users: { id: string; name: string }[]; orgs: ApiOrganization[]; properties: { id: string; name: string }[]; units: { id: string; name: string }[];
};

/** Access & roles (FR-A03) in API mode: the role / status filters and the selected membership live in the URL; saving
 * and revoking are members.save through a Server Action (a revoke sets validUntil to now). Texts in the display
 * language; the valid period is typed in the display time zone (IR302). */
export function AccessView({ live }: { live: Live }) {
  const i = useI18n(), { t } = i, zone = i.display.timeZone;
  const nav = useUrlPatch();
  const [pending, run] = useAction();
  const [q, setQ] = useState("");
  const sel = live.selected === "new" ? null : live.rows.find((r) => r.id === live.selected) ?? null;
  const key = sel ? `${sel.id}:${sel.version}` : "new";
  const [source, setSource] = useState(key);
  const [d, setD] = useState<MemberDraft>(memberDraft(sel?.m, live.tenantId, zone));
  const [tried, setTried] = useState(false);
  const [revoke, setRevoke] = useState<string | null>(null);
  const [scopeKind, setScopeKind] = useState<"unit" | "property" | "organization">("unit");
  const [scopeId, setScopeId] = useState("");
  if (source !== key) { // the form restarts from each saved membership version (adjust state during render)
    setSource(key);
    setD(memberDraft(sel?.m, live.tenantId, zone));
    setTried(false);
  }
  const errors = memberErrors(d, { selfUserId: live.selfUserId, current: sel?.m }, t);
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
  const kindWord = { tenant: t("Tenant"), unit: t("Unit"), property: t("Property"), organization: t("Customer organization") };
  const scopeName = (s: ScopeRef) => (s.kind === "tenant" ? t("All customers (tenant)") : (s.kind === "unit" ? live.units : s.kind === "property" ? live.properties : live.orgs).find((x) => x.id === s.id)?.name ?? s.id.slice(0, 8));
  const roles = [{ id: "admin" as const, label: t("Admin") }, { id: "contractor" as const, label: t("Contractor") }, { id: "technician" as const, label: t("Technician") }];
  const save = () => {
    setTried(true);
    if (Object.keys(errors).length > 0) return;
    run(() => saveMember(memberInput(d, sel?.id, zone), sel?.version), sel ? t("Access saved — scopeVersion updated") : t("Membership created"), (m) => nav({ membershipId: m.id }));
  };
  const doRevoke = () => sel && revoke !== null && revoke.trim() && run(
    () => saveMember(memberInput({ ...d, permissions: sel.m.permissions, scopes: sel.m.scopes, validUntil: zonedLocal(live.now, zone), reason: revoke.trim() }, sel.id, zone), sel.version),
    t("Access revoked — the membership is now inactive"), () => setRevoke(null));
  const err = (k: string) => (tried ? errors[k] : undefined);
  const shown = live.rows.filter((r) => !q || `${r.name} ${r.label}`.toLowerCase().includes(q.toLowerCase()));
  return (
    <Page>
      <div className="flex flex-wrap items-center gap-3">
        <Choice value={live.scope.role ?? "all"} onChange={(v) => nav({ role: v === "all" ? null : v, membershipId: null })} options={[{ id: "all", label: t("All roles") }, ...roles]} />
        <Choice value={live.scope.status ?? "all"} onChange={(v) => nav({ status: v === "all" ? null : v, membershipId: null })} options={[{ id: "all", label: t("All") }, { id: "active", label: t("Active") }, { id: "inactive", label: t("Inactive") }]} />
      </div>
      <div className="split-rev">
        <Card title={t("Memberships")} sub={t("{n} · HQ, contractor and technician", { n: live.rows.length })} action={live.canWrite && <Btn size="sm" variant="primary" onClick={() => nav({ membershipId: "new" })}>{t("+ New membership")}</Btn>} className="self-start">
          <div className="mb-2"><Search placeholder={t("Search name or organization…")} value={q} onChange={setQ} /></div>
          {shown.length === 0 ? <EmptyState title={t("No memberships")}>{t("No membership matches these filters.")}</EmptyState> : <div className="flex flex-col gap-1.5">{shown.map((u) => <ListRow key={u.id} selected={sel?.id === u.id} onClick={() => nav({ membershipId: u.id })}><div className="min-w-0 flex-1"><div className="flex items-center justify-between gap-2"><b className="truncate text-[13px]">{u.name}</b><Badge tone={u.active ? "ok" : "muted"}>{u.active ? t("Active") : t("Inactive")}</Badge></div><div className="truncate text-[11px] text-muted">{u.label}</div></div></ListRow>)}</div>}
          <p className="mt-3 text-[11px] text-muted">{t("Client accounts are not listed here — they are managed per customer in Customers & units › Users.")}</p>
        </Card>
        <Card title={sel ? sel.name : t("New membership")} sub={sel ? t("{validity} · scopeVersion {scope} · version {v}", { validity: sel.validity, scope: sel.m.scopeVersion, v: sel.version }) : t("Another membership for an existing user (users come from the identity provider)")}>
          <div className="flex flex-col gap-5">
            {!live.canWrite && <Banner>{t("Read only — editing needs identity.write.")}</Banner>}
            <section>
              <h3 className="mb-2 text-[13px] font-bold">{t("Identity & role")}</h3>
              <div className="grid-fluid" style={{ ["--min" as string]: "220px" }}>
                <Field label={t("User")} error={err("userId")} hint={!isNew ? t("Fixed after creation") : undefined}><Select value={d.userId} disabled={!isNew || !live.canWrite} onChange={(e) => set({ userId: e.target.value })}><option value="">{t("Select…")}</option>{live.users.map((u) => <option key={u.id} value={u.id}>{u.name}</option>)}</Select></Field>
                <Field label={t("Organization")} error={err("organizationId")} hint={!isNew ? t("Fixed after creation") : undefined}><Select value={d.organizationId} disabled={!isNew || !live.canWrite} onChange={(e) => chooseOrg(e.target.value)}><option value="">{t("Select…")}</option>{orgs.map((o) => <option key={o.id} value={o.id}>{o.name}</option>)}</Select></Field>
              </div>
              <div className="mt-3"><div className="mb-1 text-xs font-semibold">{t("Role")}</div>{isNew ? <Choice value={d.role} onChange={chooseRole} options={roles} /> : <Badge tone="primary">{roleWord(d.role, d.role === "technician" ? d.employment : null, t)}</Badge>}<p className="mt-1 text-[11px] text-muted">{t("Choosing a role only pre-checks its typical permissions. The client role is not offered here.")}</p></div>
              {d.role === "technician" && <div className="mt-3 max-w-xs"><Field label={t("Employment")}><Select value={d.employment} disabled={!isNew || !live.canWrite} onChange={(e) => set({ employment: e.target.value as MemberDraft["employment"], organizationId: "" })}><option value="internal">{t("Internal (HQ field team)")}</option><option value="external">{t("External (contractor)")}</option></Select></Field></div>}
            </section>
            <section>
              <h3 className="mb-1 text-[13px] font-bold">{t("Scope")}</h3>
              {d.role === "admin" ? <p className="text-xs text-muted">{t("Admins act on every customer of the tenant.")} <Badge tone="primary">{t("All customers (tenant)")}</Badge></p>
                : d.role === "contractor" ? <p className="text-xs text-muted">{d.organizationId ? t("Contractors act for their own organization ({name}).", { name: scopeName({ kind: "organization", id: d.organizationId }) }) : t("Contractors act for their own organization.")}</p>
                : <>
                  <div className="flex flex-wrap gap-1.5">{d.scopes.map((s) => <span key={`${s.kind}:${s.id}`} className="inline-flex items-center gap-1 rounded-full bg-surface2 px-2.5 py-1 text-xs">{kindWord[s.kind] ?? s.kind} · {scopeName(s)}{live.canWrite && <button aria-label={t("Remove scope")} onClick={() => set({ scopes: d.scopes.filter((x) => x !== s) })}>✕</button>}</span>)}{d.scopes.length === 0 && <span className="text-xs text-muted">{t("No targets — zero business targets.")}</span>}</div>
                  {live.canWrite && <div className="mt-2 flex flex-wrap items-end gap-2"><Field label={t("Add scope")}><Select value={scopeKind} onChange={(e) => { setScopeKind(e.target.value as typeof scopeKind); setScopeId(""); }}><option value="unit">{t("Unit")}</option><option value="property">{t("Property")}</option><option value="organization">{t("Customer organization")}</option></Select></Field><Field label={t("Target")}><Select value={scopeId} onChange={(e) => setScopeId(e.target.value)}><option value="">{t("Select…")}</option>{targets.filter((x) => !d.scopes.some((s) => s.id === x.id)).map((x) => <option key={x.id} value={x.id}>{x.name}</option>)}</Select></Field><Btn size="sm" disabled={!scopeId} onClick={() => { set({ scopes: [...d.scopes, { kind: scopeKind, id: scopeId }] }); setScopeId(""); }}>{t("+ Add")}</Btn></div>}
                  {err("scopes") && <p className="mt-1 text-xs text-crit">{err("scopes")}</p>}
                </>}
            </section>
            <section>
              <h3 className="mb-2 text-[13px] font-bold">{t("Valid period")}</h3>
              <div className="grid-fluid" style={{ ["--min" as string]: "200px" }}>
                <Field label={t("Valid from")} error={err("validFrom")} hint={t("Times in {zone}", { zone })}><Input type="datetime-local" value={d.validFrom} disabled={!live.canWrite} onChange={(e) => set({ validFrom: e.target.value })} /></Field>
                <Field label={t("Valid until")} error={err("validUntil")} hint={d.role === "technician" && d.employment === "external" ? t("Required for external technicians") : t("Optional — revoking sets it to now")}><Input type="datetime-local" value={d.validUntil} disabled={!live.canWrite} onChange={(e) => set({ validUntil: e.target.value })} /></Field>
              </div>
            </section>
            <section>
              <h3 className="mb-2 text-[13px] font-bold">{t("Permissions · {n} of {total}", { n: d.permissions.length, total: allPermissions.length })}</h3>
              <div className="scroll-x"><table className="w-full min-w-[560px] text-left text-[13px]"><thead className="text-[11px] uppercase text-muted"><tr><th className="py-1.5 pr-3">{t("Resource")}</th><th className="pr-3">{t("permission::Read")}</th><th className="pr-3">{t("Write")}</th><th>{t("Actions")}</th></tr></thead><tbody>{permissionRows.map((r) => (
                <tr key={r.res} className="border-t border-line"><td className="py-2 pr-3 font-semibold">{resourceWord(r.res, t)}</td>
                  <td className="pr-3">{r.read ? <Check label={<span className="font-mono text-xs">{r.read}</span>} checked={has(r.read)} disabled={blocked(r.read) || has(r.write)} onChange={(v) => toggle(r.read!, v)} /> : <span className="text-muted">—</span>}</td>
                  <td className="pr-3">{r.write ? <Check label={<span className="font-mono text-xs">{r.write}</span>} checked={has(r.write)} disabled={blocked(r.write)} onChange={(v) => setWrite(r, v)} /> : <span className="text-muted">—</span>}</td>
                  <td><div className="flex flex-wrap gap-x-3 gap-y-1">{(r.actions ?? []).map((a) => <Check key={a} label={<span className="font-mono text-xs">{a}</span>} checked={has(a)} disabled={blocked(a)} onChange={(v) => toggle(a, v)} />)}{!r.actions && <span className="text-muted">—</span>}</div></td></tr>
              ))}</tbody></table></div>
              <p className="mt-2 text-[11px] text-muted">{t("Write includes Read (Read stays on while Write is on). Actions are independent.")}{self ? ` ${t("identity.write and restriction.override cannot be granted to yourself.")}` : ""}</p>
              {err("permissions") && <p className="mt-1 text-xs text-crit">{err("permissions")}</p>}
            </section>
            {live.canWrite && <>
              <Field label={t("Change reason · required")} error={err("reason")} hint={t("1–1000 characters, stored in the audit log")}><Textarea value={d.reason} maxLength={1000} onChange={(e) => set({ reason: e.target.value })} /></Field>
              <div className="flex flex-wrap justify-end gap-2">{sel?.active && <Btn variant="danger" disabled={pending} onClick={() => setRevoke("")}>{t("Revoke access…")}</Btn>}<Btn variant="primary" disabled={pending} onClick={save}>{sel ? t("Save changes") : t("Create membership")}</Btn></div>
            </>}
          </div>
        </Card>
      </div>
      <Modal open={revoke !== null} onClose={() => setRevoke(null)} title={t("Revoke access — {name}", { name: sel?.name ?? "" })} footer={<><Btn onClick={() => setRevoke(null)}>{t("Cancel")}</Btn><Btn variant="danger" disabled={pending || !revoke?.trim()} onClick={doRevoke}>{t("Revoke")}</Btn></>}>
        <p className="text-[13px]">{t("Revoking sets “Valid until” to now. The membership stays listed as inactive; nothing is deleted.")}</p>
        <Field label={t("Reason")}><Textarea value={revoke ?? ""} maxLength={1000} onChange={(e) => setRevoke(e.target.value)} /></Field>
      </Modal>
    </Page>
  );
}
