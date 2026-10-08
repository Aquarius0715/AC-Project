"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { Badge, Banner, Btn, Card, Check, DataTable, EmptyState, Field, Input, Modal, Page, Search, Select, Stat, Tabs, Textarea, UtilBar, useToast } from "@/components/ui";
import { useUrlTab } from "@/lib/useUrlTab";
import { useBffSession, useNow, useOp, invalidate } from "@/lib/useOp";
import { callOp, OpError } from "@/lib/ops";

// DATA_SOURCE=api: technicians of the company (members.list, IR172 displayName), a week of members.capacity and
// members.setUnavailability. Qualification grants come from the memberships (contractors get no permissions/scopes, IR42).
type ApiMember = { id: string; displayName: string; role: string; employment: string | null; validFrom: string; validUntil: string | null; qualifications: { code: string; validFrom: string; validUntil: string; revokedAt: string | null }[] };
type ApiCapacity = { membershipId: string; date: string; availableMinutes: number | null; assignedMinutes: number; utilization: number | null; unavailability: string | null };
const day = 86400e3;
const isoDate = (d: Date) => d.toISOString().slice(0, 10);
const hours = (m: number | null) => (m === null ? "—" : `${Math.round((m / 60) * 10) / 10} h`);
const qualName = (c: string) => c.replace(/^demo_/, "").replace(/_/g, " ");

function ApiTeam() {
  const toast = useToast();
  const now = useNow();
  const [tab, setTab] = useUrlTab<"members" | "certs">({ members: "overview", certs: "certifications" }, "members");
  const [activeOnly, setActiveOnly] = useState(true);
  const [open, setOpen] = useState(false);
  const [form, setForm] = useState({ membershipId: "", from: "", to: "", type: "training", note: "" });
  const [err, setErr] = useState<string | null>(null);
  const members = useOp<{ items: ApiMember[] }, ApiMember[]>("members.list", { limit: 100 }, [], (p) => p.items.filter((m) => m.role === "technician"));
  const today = now ?? new Date();
  const monday = new Date(Date.UTC(today.getUTCFullYear(), today.getUTCMonth(), today.getUTCDate()) - ((today.getUTCDay() + 6) % 7) * day);
  const dates = Array.from({ length: 7 }, (_, i) => isoDate(new Date(monday.getTime() + i * day)));
  const [cap, setCap] = useState<Record<string, ApiCapacity[]>>({});
  const [rev, setRev] = useState(0);
  useEffect(() => {
    if (!now) return;
    let live = true;
    Promise.all(dates.map((d) => callOp<{ items: ApiCapacity[] }>("members.capacity", { date: d, query: { limit: 100 } }).then((p) => [d, p.items] as const).catch(() => [d, []] as const)))
      .then((rows) => live && setCap(Object.fromEntries(rows)));
    return () => { live = false; };
    // dates derive from now
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [now?.toISOString().slice(0, 10), rev]);
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
    try {
      await callOp("members.setUnavailability", { membershipId: form.membershipId || null, from: form.from, to: form.to, type: form.type, ...(form.note ? { note: form.note } : {}) }, { write: true });
      setOpen(false);
      toast("Unavailable days saved — availability updated");
      setRev((x) => x + 1);
      invalidate();
    } catch (e) {
      setErr(e instanceof OpError ? Object.values(e.error.fieldErrors)[0] ?? e.error.messageKey : "Not saved");
    }
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

const techs = [
  { id: "tech-external-a", quals: ["Split-unit refrigerant handling", "Electrical basics"], sub: "Refrigerant handling · Electrical basics · active since 2025-04", asg: "09:00–13:00 assigned", free: "13:00–17:00 free", used: 10, tot: 40, week: ["2 / 8 h", "4 / 8 h", "0 / 8 h", "4 / 8 h", "0 / 8 h", "—", "—"], active: true },
  { id: "tech-external-a2", quals: ["General maintenance"], sub: "General maintenance · active since 2026-02", asg: "13:00–17:00 assigned (job-p12)", free: "09:00–13:00 free", used: 8, tot: 40, week: ["0 / 8 h", "4 / 8 h", "4 / 8 h", "0 / 8 h", "0 / 8 h", "—", "—"], active: true },
  { id: "tech-external-old", quals: [], sub: "Expired membership", asg: "—", free: "—", used: 0, tot: 0, week: [], active: false },
];
const quals: [string, string][] = [["Split-unit refrigerant handling", "tech-external-a · valid to 2027-03-31"], ["Electrical basics", "tech-external-a · expires 2026-10-15"], ["General maintenance", "tech-external-a2 · valid to 2027-01-31"], ["Gas leak detection", "not held"]];

export default function Team() {
  const session = useBffSession();
  if (session?.dataSource === "api") return <ApiTeam />;
  return <MockTeam />;
}

function MockTeam() {
  const toast = useToast();
  const [tab, setTab] = useUrlTab<"members" | "certs">({ members: "overview", certs: "certifications" }, "members");
  const [modal, setModal] = useState<null | "unavail" | "upload">(null);
  const [activeOnly, setActiveOnly] = useState(true);
  const [q, setQ] = useState("");
  const list = techs.filter((t) => (!activeOnly || t.active) && (!q || t.quals.some((x) => x.toLowerCase().includes(q.toLowerCase())) || t.id.includes(q)));
  return (
    <Page>
      <div className="flex flex-wrap items-center justify-between gap-2"><Tabs value={tab} onChange={setTab} tabs={[{ id: "members", label: "Members" }, { id: "certs", label: "Certifications", count: 1 }]} /><div className="flex gap-2"><Btn size="sm" onClick={() => setModal("unavail")}>+ Unavailable days</Btn></div></div>
      {tab === "certs" ? <Certs onUpload={() => setModal("upload")} /> : <>
      <div className="flex flex-wrap items-center gap-4"><Check label="Active only" checked={activeOnly} onChange={setActiveOnly} /><div className="min-w-[220px] flex-1 sm:max-w-sm"><Search placeholder="Filter by qualification or technician" value={q} onChange={setQ} /></div></div>
      <div className="split">
        <div className="flex min-w-0 flex-col gap-4">
          <h2 className="text-[15px] font-bold">Technicians — contractor-a <span className="text-xs font-normal text-muted">· {techs.filter((t) => t.active).length} active</span></h2>
          {list.length === 0 && <EmptyState title="No match">No technician matches this qualification.</EmptyState>}
          {list.map((t) => {
            const pct = t.tot ? Math.round((t.used / t.tot) * 100) : null;
            return <Card key={t.id} title={t.id} sub={t.sub} action={<div className="text-right"><div className="text-xl font-bold">{pct === null ? "—" : pct + "%"}</div><div className="text-[11px] text-muted">utilization</div></div>}>
              <div className="mb-2 flex justify-between text-xs"><span>Week of 09-14</span><b>{t.used} h / {t.tot} h</b></div><UtilBar pct={pct} />
              <div className="mt-2 grid-fluid text-xs" style={{ ["--min"as string]: "180px" }}><span>{t.asg}</span><span className="text-ok">{t.free}</span></div>
            </Card>;
          })}
          <p className="text-[11px] text-muted">Utilization = assigned hours ÷ configured available hours (4 h / 8 h = 50%). Undefined available hours show “—”, not 0%. Read-only — membership changes are requested from HQ.</p>
          <Card title="Week of 09-14 — assigned / available hours" action={<Link className="text-xs font-semibold text-primary" href="/partner/schedule">Open schedule →</Link>}>
            <div className="scroll-x"><table className="w-full min-w-[560px] text-xs"><thead><tr className="text-left text-muted"><th /><th>Mon 14</th><th>Tue 15</th><th>Wed 16</th><th>Thu 17</th><th>Fri 18</th><th>Sat 19</th><th>Sun 20</th></tr></thead><tbody>{techs.filter((t) => t.week.length).map((t) => <tr key={t.id} className="border-t border-line"><td className="py-2 font-bold">{t.id}</td>{t.week.map((w, i) => <td key={i}>{w}</td>)}</tr>)}</tbody></table></div>
            <p className="mt-2 text-[11px] text-muted">Sat/Sun have no configured available hours, so utilization shows “—”.</p>
          </Card>
        </div>
        <div className="flex min-w-0 flex-col gap-4">
          <div className="grid-fluid" style={{ ["--min"as string]: "120px" }}><Stat label="Assigned" value="18 h" /><Stat label="Available" value="80 h" /><Stat label="Team utilization" value="23%" /><Stat label="Free hours Tue" value="8 h" /></div>
          <Card title="Qualifications">{quals.map(([a, b]) => <div key={a} className="flex flex-wrap items-center justify-between gap-2 border-t border-line py-2 text-[13px] first:border-0"><span>{a}</span><span className="text-xs text-muted">{b}</span></div>)}</Card>
          <p className="text-[11px] text-muted">{activeOnly ? "1 expired member hidden (Active only). " : ""}Qualification or membership changes are requested from HQ — contractors cannot grant permissions.</p>
        </div>
      </div>
      </>}
      <Modal open={modal === "unavail"} onClose={() => setModal(null)} title="Add unavailable days" footer={<><Btn onClick={() => setModal(null)}>Cancel</Btn><Btn variant="primary" onClick={() => { setModal(null); toast("Unavailable days saved — available hours set to 0"); }}>Save</Btn></>}>
        <Field label="Technician"><Select><option>tech-external-a</option><option>tech-external-a2</option></Select></Field>
        <div className="grid-fluid" style={{ ["--min" as string]: "160px" }}><Field label="From"><Input type="date" defaultValue="2026-10-01" /></Field><Field label="To"><Input type="date" defaultValue="2026-10-01" /></Field></div>
        <Field label="Type"><Select><option>Training</option><option>Annual leave</option><option>Public holiday</option><option>Sick</option><option>Other</option></Select></Field>
        <Banner tone="warn">Conflicts with 1 confirmed assignment (job-c07 · 10-01 10:00–12:00). Reassign it or ask HQ to propose a new time to the client — the client must approve any time change.</Banner>
      </Modal>
      <Modal open={modal === "upload"} onClose={() => setModal(null)} title="Upload renewal — Electrical basics" footer={<><Btn onClick={() => setModal(null)}>Cancel</Btn><Btn variant="primary" onClick={() => { setModal(null); toast("Renewal uploaded — pending HQ verification"); }}>Upload</Btn></>}>
        <Field label="Technician"><Input disabled value="tech-external-a" /></Field>
        <div className="grid-fluid" style={{ ["--min" as string]: "160px" }}><Field label="Certificate no."><Input defaultValue="EB-2026-0442" /></Field><Field label="Valid until"><Input type="date" defaultValue="2028-10-15" /></Field></div>
        <div className="rounded-xl border border-dashed border-line p-4 text-center text-xs text-muted">📄 Drop a PDF / image (≤ 5 MiB)</div>
        <p className="text-[11px] text-muted">A pending renewal does not extend eligibility until HQ verifies it.</p>
      </Modal>
    </Page>
  );
}

function Certs({ onUpload }: { onUpload: () => void }) {
  const rows = [
    { t: "tech-external-a", c: "Split-unit refrigerant handling", until: "2027-03-31", st: "Valid" },
    { t: "tech-external-a", c: "Electrical basics", until: "2026-10-15", st: "Expiring · 14 d" },
    { t: "tech-external-a2", c: "General maintenance", until: "2027-01-31", st: "Valid" },
    { t: "tech-external-b", c: "Refrigerant handling", until: "2026-09-01", st: "Expired" },
  ];
  return (
    <div className="flex flex-col gap-4">
      <div className="grid-fluid" style={{ ["--min" as string]: "160px" }}><Stat label="Valid" value="4" sub="certificates in date" /><Stat label="Expiring ≤ 60 days" value="1" sub="tech-external-a · 14 d" /><Stat label="Expired" value="1" sub="tech-external-b (membership ended)" /><Stat label="Pending HQ verification" value="0" sub="renewals you uploaded" /></div>
      <Card title="Certificates — contractor-a">
        <DataTable rowKey={(r) => r.t + r.c} rows={rows} cols={[
          { key: "t", label: "Technician", render: (r) => <b>{r.t}</b> }, { key: "c", label: "Certificate", render: (r) => r.c }, { key: "u", label: "Valid until", render: (r) => r.until },
          { key: "s", label: "Status", render: (r) => <Badge tone={r.st === "Valid" ? "ok" : r.st === "Expired" ? "muted" : "warn"}>{r.st}</Badge> },
          { key: "a", label: "", render: (r) => (r.st.startsWith("Expiring") ? <Btn size="sm" variant="primary" onClick={onUpload}>Upload renewal</Btn> : null) },
        ]} />
        <p className="mt-2 text-[11px] text-muted">Only valid, HQ-verified certificates make a technician eligible for offers that require them.</p>
      </Card>
    </div>
  );
}
