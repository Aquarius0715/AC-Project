"use client";

import { useRef, useState } from "react";
import { usePathname, useRouter } from "next/navigation";
import { Badge, Btn, Card, Choice, DataTable, EmptyState, Field, Input, ListRow, Page, SummaryList, Tabs, Timeline } from "@ac/web/components/ui";
import { useUrlTab } from "@ac/web/lib/useUrlTab";
import { periodError, type AuditFilters, type AuditResult, type AuditRow } from "@ac/web/lib/audit";

const seed: AuditRow[] = [
  { id: "1", op: "restriction.override", target: "restriction · restriction-a41", actor: "hq-operator", role: "admin", at: "09-22 09:14", occurred: "2026-09-22 09:14 (Asia/Kuala_Lumpur)", corr: "corr-7f21a9", res: "Success", reason: "Customer requested a short exception while the invoice is under review.",
    changes: [{ field: "exception.until", before: "null", after: "2026-09-29 00:00", changed: true }, { field: "exception.reason", before: "null", after: "Invoice under review", changed: true }, { field: "contactPhone", before: "***masked***", after: "***masked***", changed: false }, { field: "state", before: "applied", after: "applied", changed: false }] },
  { id: "2", op: "access.grant", target: "membership · membership-b09", actor: "contractor-lead-b", role: "contractor", at: "09-22 08:51", occurred: "2026-09-22 08:51 (Asia/Kuala_Lumpur)", corr: "corr-2c88e0", res: "Denied", reason: null, changes: [] },
  { id: "3", op: "devices.calibrate", target: "device · device-online-rto", actor: "tech-ali", role: "technician", at: "09-21 16:30", occurred: "2026-09-21 16:30 (Asia/Kuala_Lumpur)", corr: "corr-51d0aa", res: "Failed", reason: null, changes: [] },
  { id: "4", op: "commands.create", target: "command · cmd-3310", actor: "hq-operator", role: "admin", at: "09-20 10:14", occurred: "2026-09-20 10:14 (Asia/Kuala_Lumpur)", corr: "corr-115e6b", res: "Pending", reason: null, changes: [] },
  { id: "5", op: "offsets.simulate · retire", target: "offset · offset-0231", actor: "hq-operator", role: "admin", at: "09-12 14:20", occurred: "2026-09-12 14:20 (Asia/Kuala_Lumpur)", corr: "corr-9a10f4", res: "Success", reason: null, changes: [] },
];
const seedEvents = [{ time: "09-22 08:41", title: "tamper_signal — cover opened while powered", detail: "device-online-rto" }, { time: "09-22 08:33", title: "power_signal lost", detail: "device-online-rto" }, { time: "09-21 16:30", title: "calibration recorded", detail: "device-online-rto" }];
const demoFilters: AuditFilters = { from: "2026-09-01", to: "2026-09-30", correlationId: "", result: "All", limit: 25 };
const tone = (r: AuditResult) => (r === "Success" ? "ok" : r === "Denied" ? "warn" : r === "Failed" ? "crit" : "primary");

type Live = { rows: AuditRow[]; total: number; filters: AuditFilters; tab: "log" | "devices"; device?: { label: string; items: { time: string; title: string; detail: string }[] } };

/** The audit log. `live` comes from the Server Component in API mode, where every filter change replaces the URL and
 * the server reads audit.list again; the demo filters its seed rows locally. */
export function AuditView({ live }: { live?: Live }) {
  const router = useRouter();
  const pathname = usePathname();
  const nav = (patch: Record<string, string | null>) => {
    const q = new URLSearchParams(window.location.search);
    for (const [k, v] of Object.entries(patch)) {
      if (v === null || v === "") q.delete(k);
      else q.set(k, v);
    }
    router.replace(q.size ? `${pathname}?${q}` : pathname, { scroll: false });
  };
  const [urlTab, setUrlTab] = useUrlTab<"log" | "devices">({ log: "log", devices: "devices" }, "log");
  const tab = live ? live.tab : urlTab; // API mode: the server read the tab from the URL (and loaded its data)
  const setTab = (t: "log" | "devices") => (live ? nav({ tab: t === "devices" ? "devices" : null }) : setUrlTab(t));
  const initial = live?.filters ?? demoFilters;
  const [res, setRes] = useState<"All" | AuditResult>(initial.result);
  const [corr, setCorr] = useState(initial.correlationId);
  const [from, setFrom] = useState(initial.from);
  const [to, setTo] = useState(initial.to);
  const [shown, setShown] = useState(5);
  // the filters restart from each new server result: adjust state during render, not in an effect
  const [source, setSource] = useState(live?.filters);
  if (live && source !== live.filters) {
    setSource(live.filters);
    setRes(live.filters.result);
    setCorr(live.filters.correlationId);
    setFrom(live.filters.from);
    setTo(live.filters.to);
  }
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const periodErr = periodError({ from, to });
  const all = live ? live.rows : seed;
  const rows = live ? all : all.filter((l) => (res === "All" || l.res === res) && (!corr || l.corr.includes(corr))).slice(0, shown);
  const [selId, setSelId] = useState<string | null>(null);
  const sel = rows.find((r) => r.id === selId) ?? rows[0] ?? null;

  const changeCorr = (v: string) => {
    setCorr(v);
    if (!live) return;
    if (timer.current) clearTimeout(timer.current);
    timer.current = setTimeout(() => nav({ corr: v, limit: null }), 300);
  };
  const changeDate = (which: "from" | "to", v: string) => {
    if (which === "from") setFrom(v);
    else setTo(v);
    const next = { from: which === "from" ? v : from, to: which === "to" ? v : to };
    if (live && !periodError(next)) nav({ ...next, limit: null });
  };
  const changeRes = (v: "All" | AuditResult) => {
    setRes(v);
    if (live) nav({ result: v === "All" ? null : v, limit: null });
  };
  const reset = () => {
    if (live) {
      nav({ corr: null, result: null, from: null, to: null, limit: null });
      return;
    }
    setCorr(""); setRes("All"); setFrom(demoFilters.from); setTo(demoFilters.to);
  };
  const total = live ? live.total : seed.length;
  const more = live ? rows.length < live.total && live.filters.limit < 100 : rows.length < seed.length;

  return (
    <Page>
      <Tabs value={tab} onChange={setTab} tabs={[{ id: "log", label: "Audit log", count: total }, { id: "devices", label: "Device events" }]} />
      {tab === "log" ? (
        <>
          <Card title="Filter">
            <div className="grid-fluid" style={{ ["--min" as string]: "170px" }}><Field label="Correlation ID"><Input value={corr} onChange={(e) => changeCorr(e.target.value)} placeholder="corr-…" /></Field><Field label="From" error={periodErr}><Input type="date" value={from} onChange={(e) => changeDate("from", e.target.value)} /></Field><Field label="To"><Input type="date" value={to} onChange={(e) => changeDate("to", e.target.value)} /></Field></div>
            <div className="mt-3 flex flex-wrap items-center justify-between gap-2"><Choice value={res} onChange={changeRes} options={(["All", "Success", "Denied", "Failed", "Pending"] as const).map((k) => ({ id: k, label: k }))} /><Btn size="sm" onClick={reset}>Reset</Btn></div>
          </Card>
          <div className="split-rev">
            <Card title="Audit log · newest first" className="self-start">
              {rows.length === 0 ? <EmptyState title="No audit entries match these filters">{periodErr ?? "No audit entries match this period, correlation ID or result."}</EmptyState> : <div className="flex flex-col gap-2">{rows.map((l) => <ListRow key={l.id} selected={sel?.id === l.id} onClick={() => setSelId(l.id)}><div className="min-w-0 flex-1"><div className="flex items-center justify-between gap-2"><b className="truncate text-[13px]">{l.op}</b><Badge tone={tone(l.res)}>{l.res}</Badge></div><div className="truncate text-[11px] text-muted">{l.target}</div><div className="truncate text-[11px] text-muted">{l.actor} ({l.role}) · {l.at} · {l.corr}</div></div></ListRow>)}</div>}
              <p className="mt-3 text-[11px] text-muted">{rows.length} of {total} · 25 per page</p>
            </Card>
            {sel && (
              <div className="flex min-w-0 flex-col gap-4">
                <Card title={sel.op} sub={sel.target}>
                  <SummaryList items={[["Actor", sel.actor], ["Role at the time", `${sel.role} — kept as recorded, not rewritten`], ["Occurred", sel.occurred], ["Correlation ID", <button key="c" className="font-mono text-primary underline" onClick={() => changeCorr(sel.corr)}>{sel.corr} · Filter by ID →</button>], ["Reason", sel.reason ?? "—"]]} />
                  <h3 className="mt-4 mb-2 text-[13px] font-bold">Before / after (masked)</h3>
                  {sel.changes.length === 0 ? <p className="text-xs text-muted">No field values were recorded for this entry.</p> : <DataTable rows={sel.changes} rowKey={(r) => r.field} cols={[{ key: "f", label: "Field", render: (r) => <span className="font-mono text-xs">{r.field}</span> }, { key: "a", label: "Before", render: (r) => r.before }, { key: "b", label: "After", render: (r) => <span className={r.changed ? "font-bold text-warn" : ""}>{r.after}</span> }]} />}
                  <p className="mt-2 text-[11px] text-muted">Changed fields are highlighted. Secrets are masked.</p>
                </Card>
                {!live && <Card title="Related records"><SummaryList items={[["Restriction", "restriction-a41 · applied · exception until 09-29"], ["Command", "cmd-3302 · same correlation ID"], ["Job", "No related job"]]} /><p className="mt-2 text-[11px] text-muted">Links open existing detail screens and export no new data.</p></Card>}
              </div>
            )}
          </div>
          {more && <div className="text-center"><Btn onClick={() => (live ? nav({ limit: String(Math.min(100, live.filters.limit + 25)) }) : setShown((s) => s + 25))}>Load more</Btn></div>}
        </>
      ) : (
        <Card title="Device events" sub={live?.device?.label ?? "device-online-rto · connection, power and tamper are separate"}>{(live?.device?.items ?? seedEvents).length === 0 ? <EmptyState title="No device events">This device has no recorded events.</EmptyState> : <Timeline items={live?.device?.items ?? seedEvents} />}</Card>
      )}
    </Page>
  );
}
