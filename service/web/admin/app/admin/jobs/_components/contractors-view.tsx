"use client";

import { useRouter } from "next/navigation";
import { useState } from "react";
import { Badge, Banner, Btn, Card, EmptyState, Field, Input, Kpi, ListRow, Modal, Page, Select, SummaryList, Textarea } from "@ac/web/components/ui";
import { useAction } from "@ac/web/lib/useAction";
import { klTime } from "@ac/web/lib/devices";
import { areasOf, contractorRefusal, nextMonthStart, profileErrors, rateCardErrors, WORK_TYPES, type WorkType } from "@ac/web/lib/adminContractors";
import { fromKlInput, klInput } from "@ac/web/lib/adminPlans";
import type { ActionFailure } from "@ac/web/lib/actionMessage";
import { saveContractor, saveRateCard, setOfferStatus, verifyCertificate } from "../actions";
import type { ContractorsLive } from "../_lib/load";
import { JobsTabs } from "./jobs-header";
import { NewJobModal } from "./jobs-view";

type Detail = NonNullable<ContractorsLive["detail"]>;
type ModalState = null | { kind: "profile"; organizationId: string | null } | { kind: "status" } | { kind: "rates" } | { kind: "verify" } | { kind: "job" };

/** HQ contractor register (FR-A21, DD-A21, Figma Admin 06-9) from the Core API. */
export function ContractorsView({ live }: { live: ContractorsLive }) {
  const [modal, setModal] = useState<ModalState>(null);
  const [result, setResult] = useState<{ tone: "ok" | "crit" | "warn"; text: string } | null>(null);
  const d = live.detail;
  const close = (text?: string) => { setModal(null); if (text) setResult({ tone: "ok", text }); };
  const fail = (f: ActionFailure) => { setModal(null); setResult({ tone: f.code === "CONFLICT" ? "warn" : "crit", text: contractorRefusal(f) }); };
  return (
    <Page className="max-w-[1440px]">
      <JobsTabs tab="contractors" counts={live.counts} q={{}} action={<Btn variant="primary" size="sm" onClick={() => setModal({ kind: "job" })}>+ New job</Btn>} />
      {result && <Banner tone={result.tone}>{result.text}</Banner>}
      <div className="split-rev">
        <Card title="Contractors" className="self-start" action={<Btn size="sm" disabled={!live.withoutProfile.length} title={live.withoutProfile.length ? undefined : "Every contractor organization has a profile — add the organization in Access & roles first."} onClick={() => setModal({ kind: "profile", organizationId: live.withoutProfile[0]?.id ?? null })}>+ Add contractor</Btn>}>
          {live.rows.length === 0 ? <EmptyState title="No contractor organizations">Contractor organizations are created in Access & roles (A03).</EmptyState> : (
            <div className="flex flex-col gap-1.5">{live.rows.map((r) => (
              <ListRow key={r.id} selected={d?.org.id === r.id} href={`/admin/jobs?tab=contractors&contractorId=${r.id}`}>
                <div className="min-w-0 flex-1"><b className="block truncate text-[13px]">{r.name}</b><div className="text-[11px] text-muted">{r.sub}</div></div>
                <Badge tone={r.badge.tone}>{r.badge.label}</Badge>
              </ListRow>
            ))}</div>
          )}
          <p className="mt-2 text-[11px] text-muted">Contractor companies are partners, not users — their staff accounts are managed in Access & roles (A03).</p>
        </Card>
        {d ? <ContractorDetail key={`${d.org.id}:${d.profile?.version ?? 0}`} d={d} open={setModal} /> : <Card title="Contractor"><p className="text-[13px] text-muted">{"missing" in live ? "That contractor no longer exists." : "Pick a contractor."}</p></Card>}
      </div>
      {modal?.kind === "profile" && <ProfileModal live={live} d={d} organizationId={modal.organizationId} onClose={close} onFail={fail} />}
      {modal?.kind === "status" && d?.profile && <StatusModal d={d} onClose={close} onFail={fail} />}
      {modal?.kind === "rates" && d && <RatesModal d={d} now={Date.parse(live.now)} onClose={close} onFail={fail} />}
      {modal?.kind === "verify" && d && <VerifyModal d={d} onClose={close} onFail={fail} />}
      {modal?.kind === "job" && <NewJobModal live={{ now: live.now, units: live.units, q: {} }} onClose={() => setModal(null)} />}
    </Page>
  );
}

function ContractorDetail({ d, open }: { d: Detail; open: (m: ModalState) => void }) {
  const p = d.profile;
  const r = d.rates;
  return (
    <div className="flex min-w-0 flex-col gap-4">
      {d.kpis && <div className="grid-fluid" style={{ ["--min" as string]: "170px" }}>{d.kpis.map((k) => <Kpi key={k.label} label={k.label} value={k.value} sub={k.sub} tone={k.tone} />)}</div>}
      <div className="split">
        <Card title={d.org.name} sub={p ? undefined : "No contractor profile yet — offers still work; add one to record its registration, areas and insurance"}
          action={p ? <span className="flex gap-2"><Btn size="sm" onClick={() => open({ kind: "profile", organizationId: p.organizationId })}>Edit</Btn><Btn size="sm" variant={p.status === "active" ? "danger" : "primary"} onClick={() => open({ kind: "status" })}>{p.status === "active" ? "Suspend offers" : "Resume offers"}</Btn></span>
            : <Btn size="sm" variant="primary" onClick={() => open({ kind: "profile", organizationId: d.org.id })}>+ Add profile</Btn>}>
          {p?.status === "suspended" && <div className="mb-3"><Banner tone="crit">Offers suspended{p.suspendedReason ? ` — “${p.suspendedReason}”` : ""}. New offers to {p.name} are refused; open offers and jobs continue (IR111).</Banner></div>}
          {d.facts && <SummaryList items={d.facts} />}
          <p className="mt-3 text-[13px] font-bold">{r.current ? r.current.title : "No rate card in effect"}</p>
          {r.current && (
            <div className="scroll-x mt-1"><table className="w-full min-w-[420px] text-[13px]">
              <thead><tr className="bg-surface2 text-left text-[11px] uppercase text-muted"><th className="px-2 py-1.5">Work type</th><th className="px-2 text-right">{r.current.currency}</th></tr></thead>
              <tbody>{r.current.lines.map((l) => <tr key={l.label} className="border-t border-line"><td className="px-2 py-1.5 font-semibold">{l.label}{l.note && <span className="block text-[11px] font-normal text-muted">{l.note}</span>}</td><td className="px-2 text-right">{l.amount}</td></tr>)}</tbody>
            </table></div>
          )}
          {r.scheduled.map((s) => <p key={s.id} className="mt-2 text-xs text-muted">Next: {s.title} — {s.lines.map((l) => `${l.label} ${l.amount}`).join(" · ")}</p>)}
          <div className="mt-3"><Btn size="sm" onClick={() => open({ kind: "rates" })}>Edit rate card</Btn></div>
        </Card>
        <Card title="Technicians & certificates">
          {d.techs.length === 0 ? <p className="text-[13px] text-muted">No technician memberships in this organization.</p> : (
            <ul className="divide-y divide-line">{d.techs.map((t) => <li key={t.id} className="flex items-center justify-between gap-2 py-2"><span className="min-w-0"><b className="block text-[13px]">{t.name}</b><span className="text-[11px] text-muted">{t.sub}</span></span><Badge tone={t.badge.tone}>{t.badge.label}</Badge></li>)}</ul>
          )}
          <div className="mt-3"><Banner>{d.pending.length ? `${d.pending.length} renewal upload${d.pending.length === 1 ? " is" : "s are"} waiting for verification.` : "No renewal uploads are waiting for verification."} Certificates uploaded by the contractor need HQ verification before they count.</Banner></div>
          <div className="mt-3"><Btn size="sm" variant={d.pending.length ? "primary" : "secondary"} disabled={!d.pending.length} onClick={() => open({ kind: "verify" })}>Verify uploads ({d.pending.length})</Btn></div>
          <p className="mt-2 text-[11px] text-muted">KPIs: last 90 days. Customer rating comes from the Client “Confirm & rate” step after completion; SLA targets per plan are in the SLA tab.</p>
        </Card>
      </div>
    </div>
  );
}

type ModalProps = { onClose: (text?: string) => void; onFail: (f: ActionFailure) => void };

function ProfileModal({ live, d, organizationId, onClose, onFail }: ModalProps & { live: ContractorsLive; d: Detail | null; organizationId: string | null }) {
  const router = useRouter();
  const [pending, run] = useAction();
  const existing = d?.profile && d.profile.organizationId === organizationId ? d.profile : null;
  const [org, setOrg] = useState(organizationId ?? live.withoutProfile[0]?.id ?? "");
  const [f, setF] = useState({ registrationNo: existing?.registrationNo ?? "", serviceAreas: existing?.serviceAreas.join(", ") ?? "", contactEmail: existing?.contactEmail ?? "", insurance: existing?.insuranceValidUntil ? klInput(existing.insuranceValidUntil).slice(0, 10) : "" });
  const [tried, setTried] = useState(false);
  const errors = profileErrors(f);
  const choices = existing ? [{ id: existing.organizationId, name: existing.name }] : live.withoutProfile;
  const save = () => {
    setTried(true);
    if (Object.keys(errors).length || !org) return;
    const insuranceValidUntil = f.insurance ? fromKlInput(`${f.insurance}T00:00`) : null;
    run(() => saveContractor({ id: existing?.id ?? null, version: existing?.version ?? null, organizationId: org, registrationNo: f.registrationNo, serviceAreas: areasOf(f.serviceAreas), contactEmail: f.contactEmail, insuranceValidUntil }),
      existing ? "Profile saved" : "Contractor profile created", () => { onClose(existing ? "Profile saved." : "Contractor profile created — its KPIs, rate card and certificates show here."); if (!existing) router.push(`/admin/jobs?tab=contractors&contractorId=${org}`); }, onFail);
  };
  return (
    <Modal open onClose={() => onClose()} title={existing ? `Edit ${existing.name}` : "Add contractor"} footer={<><Btn onClick={() => onClose()}>Cancel</Btn><Btn variant="primary" disabled={pending} onClick={save}>{existing ? "Save" : "Add contractor"}</Btn></>}>
      <Field label="Contractor organization" hint={existing ? "A profile’s organization cannot change." : "Organizations of kind contractor without a profile (Access & roles creates them)."}><Select value={org} disabled={!!existing} onChange={(e) => setOrg(e.target.value)}>{choices.map((o) => <option key={o.id} value={o.id}>{o.name}</option>)}</Select></Field>
      <Field label="Registration number · required" error={tried ? errors.registrationNo : undefined}><Input value={f.registrationNo} maxLength={64} onChange={(e) => setF({ ...f, registrationNo: e.target.value })} placeholder="SSM 202301012345" /></Field>
      <Field label="Service areas · required" hint="Comma-separated, 1–20" error={tried ? errors.serviceAreas : undefined}><Input value={f.serviceAreas} onChange={(e) => setF({ ...f, serviceAreas: e.target.value })} placeholder="Kuala Lumpur, Selangor" /></Field>
      <div className="grid-fluid" style={{ ["--min" as string]: "200px" }}>
        <Field label="Contact e-mail · required" error={tried ? errors.contactEmail : undefined}><Input type="email" value={f.contactEmail} onChange={(e) => setF({ ...f, contactEmail: e.target.value })} placeholder="ops@contractor.example" /></Field>
        <Field label="Insurance valid until" hint="Empty: not recorded — the delegation then runs a year"><Input type="date" value={f.insurance} onChange={(e) => setF({ ...f, insurance: e.target.value })} /></Field>
      </div>
      <Banner>The delegation period runs from the profile’s creation to the insurance end (or one year).</Banner>
    </Modal>
  );
}

function StatusModal({ d, onClose, onFail }: ModalProps & { d: Detail }) {
  const p = d.profile!;
  const [pending, run] = useAction();
  const [reason, setReason] = useState("");
  const [tried, setTried] = useState(false);
  const suspend = p.status === "active";
  const confirm = () => {
    setTried(true);
    if (!reason.trim()) return;
    run(() => setOfferStatus(p.organizationId, p.version, suspend ? "suspended" : "active", reason), suspend ? "Offers suspended" : "Offers resumed",
      () => onClose(suspend ? `Offers to ${p.name} suspended — open offers and jobs continue.` : `Offers to ${p.name} resumed.`), onFail);
  };
  return (
    <Modal open onClose={() => onClose()} title={suspend ? `Suspend offers to ${p.name}` : `Resume offers to ${p.name}`} footer={<><Btn onClick={() => onClose()}>Cancel</Btn><Btn variant={suspend ? "danger" : "primary"} disabled={pending} onClick={confirm}>{suspend ? "Suspend offers" : "Resume offers"}</Btn></>}>
      <Field label="Reason · required (1–1000)" error={tried && !reason.trim() ? "A reason is required" : undefined}><Textarea value={reason} maxLength={1000} onChange={(e) => setReason(e.target.value)} placeholder={suspend ? "Insurance certificate expired; offers paused until renewal." : "Insurance renewed."} /></Field>
      <Banner tone={suspend ? "warn" : undefined}>{suspend ? "New offers to this contractor are refused (CONFLICT). Open offers, accepted jobs and their assignments are unchanged." : "HQ can offer jobs to this contractor again."}</Banner>
    </Modal>
  );
}

function RatesModal({ d, now, onClose, onFail }: ModalProps & { d: Detail; now: number }) {
  const [pending, run] = useAction();
  const base = d.rates.current;
  const [from, setFrom] = useState(klInput(nextMonthStart(now)).slice(0, 10));
  const [currency, setCurrency] = useState<"MYR" | "USD">((base?.currency as "MYR" | "USD") ?? "MYR");
  const [lines, setLines] = useState(() => WORK_TYPES.map((w) => {
    const l = base?.lines.find((x) => x.label === w.label);
    return { workType: w.id as WorkType, on: !!l || !base, amount: l ? l.amount.replace("− ", "") : "", note: l?.note ?? "" };
  }));
  const [tried, setTried] = useState(false);
  const chosen = lines.filter((l) => l.on);
  const effectiveFrom = from ? fromKlInput(`${from}T00:00`) : null;
  const errors = rateCardErrors({ effectiveFrom, lines: chosen }, now);
  const save = () => {
    setTried(true);
    if (Object.keys(errors).length) return;
    run(() => saveRateCard({ contractorOrgId: d.org.id, effectiveFrom: effectiveFrom!, currency, lines: chosen.map((l) => ({ workType: l.workType, amountMinor: Math.round(Number(l.amount) * 100), note: l.note.trim() || null })) }),
      "Rate card saved", () => onClose(`Rate card v${d.rates.nextVersion} saved — effective from ${from}.`), onFail);
  };
  return (
    <Modal open wide onClose={() => onClose()} title={`New rate card for ${d.org.name} · v${d.rates.nextVersion}`} footer={<><Btn onClick={() => onClose()}>Cancel</Btn><Btn variant="primary" disabled={pending} onClick={save}>Save rate card</Btn></>}>
      <p className="text-xs text-muted">{base ? `Starts from ${base.title.replace("Rate card ", "")}. ` : ""}A rate card never changes once saved — each edit is a new version from a future date.</p>
      <div className="grid-fluid" style={{ ["--min" as string]: "200px" }}>
        <Field label="Effective from · required" error={tried ? errors.effectiveFrom : undefined}><Input type="date" value={from} onChange={(e) => setFrom(e.target.value)} /></Field>
        <Field label="Currency"><Select value={currency} onChange={(e) => setCurrency(e.target.value as "MYR" | "USD")}><option value="MYR">MYR</option><option value="USD">USD</option></Select></Field>
      </div>
      <div className="flex flex-col gap-2">
        {lines.map((l, i) => (
          <div key={l.workType} className="flex flex-wrap items-end gap-2">
            <label className="mb-2 flex w-60 items-center gap-2 text-[13px] font-semibold"><input type="checkbox" checked={l.on} onChange={(e) => setLines(lines.map((x, k) => (k === i ? { ...x, on: e.target.checked } : x)))} />{WORK_TYPES[i].label}</label>
            <Field label={`Amount ${i + 1}`}><Input inputMode="decimal" disabled={!l.on} value={l.amount} onChange={(e) => setLines(lines.map((x, k) => (k === i ? { ...x, amount: e.target.value } : x)))} placeholder="0.00" /></Field>
            <Field label={`Note ${i + 1}`}><Input disabled={!l.on} value={l.note} maxLength={200} onChange={(e) => setLines(lines.map((x, k) => (k === i ? { ...x, note: e.target.value } : x)))} /></Field>
          </div>
        ))}
        {tried && errors.lines && <p className="text-xs text-crit">✕ {errors.lines}</p>}
      </div>
      <Banner>The rework deduction is entered as a positive amount and shown as “−”. Jobs keep the card in effect when they were offered; payouts are not made from here.</Banner>
    </Modal>
  );
}

function VerifyModal({ d, onClose, onFail }: ModalProps & { d: Detail }) {
  const [pending, run] = useAction();
  const [reasons, setReasons] = useState<Record<string, string>>({});
  const [tried, setTried] = useState<string | null>(null);
  const decide = (c: Detail["pending"][number], decision: "approve" | "reject") => {
    if (decision === "reject") { setTried(c.id); if (!reasons[c.id]?.trim()) return; }
    run(() => verifyCertificate(c.id, c.version, decision, reasons[c.id] ?? ""), decision === "approve" ? "Certificate approved" : "Certificate rejected",
      () => onClose(decision === "approve" ? `Approved ${c.name} for ${c.technician} — it now counts as the qualification.` : `Rejected ${c.name} for ${c.technician}.`), onFail);
  };
  return (
    <Modal open wide onClose={() => onClose()} title={`Verify uploads · ${d.org.name}`} footer={<Btn onClick={() => onClose()}>Close</Btn>}>
      {d.pending.map((c) => (
        <Card key={c.id} title={`${c.technician} · ${c.name}`} sub={`${c.code.replace(/^demo_/, "")} · no. ${c.number} · ${klTime(c.issuedAt).slice(0, 10)} → ${klTime(c.expiresAt).slice(0, 10)}${c.renewal ? " · renewal" : ""}${c.fileName ? ` · ${c.fileName}` : ""}`}>
          <Field label="Reason (to reject)" error={tried === c.id && !reasons[c.id]?.trim() ? "A reason is required to reject" : undefined}><Input value={reasons[c.id] ?? ""} maxLength={1000} onChange={(e) => setReasons({ ...reasons, [c.id]: e.target.value })} /></Field>
          <div className="mt-2 flex justify-end gap-2"><Btn size="sm" variant="danger" disabled={pending} onClick={() => decide(c, "reject")}>Reject</Btn><Btn size="sm" variant="primary" disabled={pending} onClick={() => decide(c, "approve")}>Approve</Btn></div>
        </Card>
      ))}
    </Modal>
  );
}
