"use client";

import { useState } from "react";
import { Badge, Banner, Btn, Card, Check, DataTable, EmptyState, Field, Input, Modal, Page, Select, Stat, Tabs, Textarea, UtilBar, useToast } from "@ac/web/components/ui";
import { useUrlTab } from "@ac/web/lib/useUrlTab";
import { setUnavailability } from "../actions";

export type ApiMember = { id: string; displayName: string; role: string; employment: string | null; validFrom: string; validUntil: string | null; qualifications: { code: string; validFrom: string; validUntil: string; revokedAt: string | null }[] };
export type ApiCapacity = { membershipId: string; date: string; availableMinutes: number | null; assignedMinutes: number; utilization: number | null; unavailability: string | null };
const day = 86400e3;
const hours = (m: number | null) => (m === null ? "—" : `${Math.round((m / 60) * 10) / 10} h`);
const qualName = (c: string) => c.replace(/^demo_/, "").replace(/_/g, " ");

export type TeamData = { members: ApiMember[]; dates: string[]; capacity: Record<string, ApiCapacity[]>; now: string };

/** Team screen in API mode: members and a week of capacity come from the Server Component. */
export function TeamView({ data }: { data: TeamData }) {
  const toast = useToast();
  const now = new Date(data.now);
  const [tab, setTab] = useUrlTab<"members" | "certs">({ members: "overview", certs: "certifications" }, "members");
  const [activeOnly, setActiveOnly] = useState(true);
  const [open, setOpen] = useState(false);
  const [form, setForm] = useState({ membershipId: "", from: "", to: "", type: "training", note: "" });
  const [err, setErr] = useState<string | null>(null);
  const members = { data: data.members, loading: false };
  const today = now;
  const dates = data.dates;
  const cap = data.capacity;
  const active = (m: ApiMember) => !m.validUntil || new Date(m.validUntil) > today;
  const list = members.data.filter((m) => !activeOnly || active(m));
  const of = (id: string, d: string) => cap[d]?.find((c) => c.membershipId === id);
  const sum = (f: (c: ApiCapacity) => number) => Object.values(cap).flat().reduce((a, c) => a + f(c), 0);
  const assigned = sum((c) => c.assignedMinutes);
  const available = sum((c) => c.availableMinutes ?? 0);
  const certs = members.data.flatMap((m) => m.qualifications.map((q) => ({ m, q, days: Math.ceil((new Date(q.validUntil).getTime() - today.getTime()) / day) })));
  const status = (x: { q: { revokedAt: string | null }; days: number }) => (x.q.revokedAt ? "Revoked" : x.days < 0 ? "Expired" : x.days <= 60 ? `Expiring · ${x.days} d` : "Valid");
  const save = async () => {
    setErr(null);
    const res = await setUnavailability({ membershipId: form.membershipId || null, from: form.from, to: form.to, type: form.type, note: form.note || undefined }); // Server Action
    if (!res.ok) return setErr(res.message);
    setOpen(false);
    toast("Unavailable days saved — availability updated");
  };
  return (
    <Page>
      <div className="flex flex-wrap items-center justify-between gap-2"><Tabs value={tab} onChange={setTab} tabs={[{ id: "members", label: "Members", count: members.data.length }, { id: "certs", label: "Certifications", count: certs.length }]} /><Btn size="sm" onClick={() => { setForm({ membershipId: members.data[0]?.id ?? "", from: dates[0], to: dates[0], type: "training", note: "" }); setOpen(true); }}>+ Unavailable days</Btn></div>
      {tab === "certs" ? (
        <Card title="Certificates">
          <DataTable rowKey={(r) => r.m.id + r.q.code} rows={certs} cols={[
            { key: "t", label: "Technician", render: (r) => <b>{r.m.displayName}</b> }, { key: "c", label: "Certificate", render: (r) => qualName(r.q.code) },
            { key: "u", label: "Valid until", render: (r) => r.q.validUntil.slice(0, 10) },
            { key: "s", label: "Status", render: (r) => <Badge tone={status(r) === "Valid" ? "ok" : status(r).startsWith("Expiring") ? "warn" : "muted"}>{status(r)}</Badge> },
          ]} />
          <p className="mt-2 text-[11px] text-muted">Only valid, HQ-verified certificates make a technician eligible for offers that require them. Renewals are requested from HQ.</p>
        </Card>
      ) : (
        <>
          <Check label="Active only" checked={activeOnly} onChange={setActiveOnly} />
          <div className="split">
            <div className="flex min-w-0 flex-col gap-4">
              {list.length === 0 && <EmptyState title={members.loading ? "Loading…" : "No technicians"}>No technician membership in your company.</EmptyState>}
              {list.map((m) => {
                const a = dates.reduce((x, d) => x + (of(m.id, d)?.assignedMinutes ?? 0), 0);
                const v = dates.reduce((x, d) => x + (of(m.id, d)?.availableMinutes ?? 0), 0);
                const pct = v ? Math.round((a / v) * 100) : null;
                return <Card key={m.id} title={m.displayName} sub={`${m.qualifications.map((q) => qualName(q.code)).join(" · ") || "No qualifications"} · ${active(m) ? "active" : "membership ended"}`} action={<div className="text-right"><div className="text-xl font-bold">{pct === null ? "—" : pct + "%"}</div><div className="text-[11px] text-muted">utilization</div></div>}>
                  <div className="mb-2 flex justify-between text-xs"><span>Week of {dates[0].slice(5)}</span><b>{hours(a)} / {hours(v)}</b></div><UtilBar pct={pct} />
                </Card>;
              })}
              <Card title={`Week of ${dates[0].slice(5)} — assigned / available hours`}>
                <div className="scroll-x"><table className="w-full min-w-[560px] text-xs"><thead><tr className="text-left text-muted"><th />{dates.map((d) => <th key={d}>{new Date(d).toLocaleDateString("en-MY", { weekday: "short", day: "numeric", timeZone: "UTC" })}</th>)}</tr></thead>
                  <tbody>{list.map((m) => <tr key={m.id} className="border-t border-line"><td className="py-1.5 font-semibold">{m.displayName}</td>{dates.map((d) => { const c = of(m.id, d); return <td key={d}>{c?.unavailability ? <Badge tone="muted">{c.unavailability.replace(/_/g, " ")}</Badge> : c ? `${hours(c.assignedMinutes)} / ${hours(c.availableMinutes)}` : "—"}</td>; })}</tr>)}</tbody></table></div>
                <p className="mt-2 text-[11px] text-muted">Utilization = assigned ÷ available hours; days without configured hours show “—”, not 0%.</p>
              </Card>
            </div>
            <div className="grid-fluid self-start" style={{ ["--min" as string]: "120px" }}><Stat label="Assigned" value={hours(assigned)} /><Stat label="Available" value={hours(available)} /><Stat label="Team utilization" value={available ? `${Math.round((assigned / available) * 100)}%` : "—"} /></div>
          </div>
        </>
      )}
      <Modal open={open} onClose={() => setOpen(false)} title="Add unavailable days" footer={<><Btn onClick={() => setOpen(false)}>Cancel</Btn><Btn variant="primary" onClick={save}>Save</Btn></>}>
        <Field label="Technician"><Select value={form.membershipId} onChange={(e) => setForm({ ...form, membershipId: e.target.value })}><option value="">Whole company (e.g. public holiday)</option>{members.data.map((m) => <option key={m.id} value={m.id}>{m.displayName}</option>)}</Select></Field>
        <div className="grid-fluid" style={{ ["--min" as string]: "160px" }}><Field label="From"><Input type="date" value={form.from} onChange={(e) => setForm({ ...form, from: e.target.value })} /></Field><Field label="To"><Input type="date" value={form.to} onChange={(e) => setForm({ ...form, to: e.target.value })} /></Field></div>
        <Field label="Type"><Select value={form.type} onChange={(e) => setForm({ ...form, type: e.target.value })}><option value="training">Training</option><option value="annual_leave">Annual leave</option><option value="public_holiday">Public holiday</option><option value="sick">Sick</option><option value="other">Other</option></Select></Field>
        <Field label="Note (optional)"><Textarea value={form.note} onChange={(e) => setForm({ ...form, note: e.target.value })} /></Field>
        {err && <Banner tone="crit">{err}</Banner>}
        <p className="text-[11px] text-muted">Days that overlap a confirmed assignment are saved with the conflict listed; reassign the job or ask HQ to propose a new time to the client.</p>
      </Modal>
    </Page>
  );
}
