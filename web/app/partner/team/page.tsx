"use client";

import { useState } from "react";
import Link from "next/link";
import { Badge, Banner, Card, Check, DataTable, EmptyState, Page, Search, Stat, UtilBar } from "@/components/ui";

const techs = [
  { id: "tech-external-a", quals: ["Split-unit refrigerant handling", "Electrical basics"], sub: "Refrigerant handling · Electrical basics · active since 2025-04", asg: "09:00–13:00 assigned", free: "13:00–17:00 free", used: 10, tot: 40, week: ["2 / 8 h", "4 / 8 h", "0 / 8 h", "4 / 8 h", "0 / 8 h", "—", "—"], active: true },
  { id: "tech-external-a2", quals: ["General maintenance"], sub: "General maintenance · active since 2026-02", asg: "13:00–17:00 assigned (job-p12)", free: "09:00–13:00 free", used: 8, tot: 40, week: ["0 / 8 h", "4 / 8 h", "4 / 8 h", "0 / 8 h", "0 / 8 h", "—", "—"], active: true },
  { id: "tech-external-old", quals: [], sub: "Expired membership", asg: "—", free: "—", used: 0, tot: 0, week: [], active: false },
];
const quals: [string, string][] = [["Split-unit refrigerant handling", "tech-external-a · valid to 2027-03-31"], ["Electrical basics", "tech-external-a · expires 2026-10-15"], ["General maintenance", "tech-external-a2 · valid to 2027-01-31"], ["Gas leak detection", "not held"]];

export default function Team() {
  const [activeOnly, setActiveOnly] = useState(true);
  const [q, setQ] = useState("");
  const list = techs.filter((t) => (!activeOnly || t.active) && (!q || t.quals.some((x) => x.toLowerCase().includes(q.toLowerCase())) || t.id.includes(q)));
  return (
    <Page>
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
    </Page>
  );
}
