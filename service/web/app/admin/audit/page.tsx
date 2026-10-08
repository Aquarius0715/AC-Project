"use client";

import { useState } from "react";
import { Badge, Banner, Btn, Card, Choice, DataTable, EmptyState, Field, Input, ListRow, Page, Select, SummaryList, Tabs, Timeline } from "@/components/ui";
import { useUrlTab } from "@/lib/useUrlTab";

type L = { id: string; op: string; target: string; actor: string; at: string; corr: string; res: "Success" | "Denied" | "Failed" | "Pending" };
const logs: L[] = [
  { id: "1", op: "restriction.override", target: "restriction · restriction-a41", actor: "hq-operator (admin)", at: "09-22 09:14", corr: "corr-7f21a9", res: "Success" },
  { id: "2", op: "access.grant", target: "membership · membership-b09", actor: "contractor-lead-b (contractor)", at: "09-22 08:51", corr: "corr-2c88e0", res: "Denied" },
  { id: "3", op: "devices.calibrate", target: "device · device-online-rto", actor: "tech-ali (technician)", at: "09-21 16:30", corr: "corr-51d0aa", res: "Failed" },
  { id: "4", op: "commands.create", target: "command · cmd-3310", actor: "hq-operator (admin)", at: "09-20 10:14", corr: "corr-115e6b", res: "Pending" },
  { id: "5", op: "offsets.simulate · retire", target: "offset · offset-0231", actor: "hq-operator (admin)", at: "09-12 14:20", corr: "corr-9a10f4", res: "Success" },
];
const events = [{ t: "09-22 08:41", e: "tamper_signal — cover opened while powered", d: "device-online-rto" }, { t: "09-22 08:33", e: "power_signal lost", d: "device-online-rto" }, { t: "09-21 16:30", e: "calibration recorded", d: "device-online-rto" }];
const tone = (r: L["res"]) => (r === "Success" ? "ok" : r === "Denied" ? "warn" : r === "Failed" ? "crit" : "primary");

export default function Audit() {
  const [tab, setTab] = useUrlTab<"log" | "devices">({ log: "log", devices: "devices" }, "log");
  const [res, setRes] = useState<"All" | L["res"]>("All");
  const [corr, setCorr] = useState("");
  const [from, setFrom] = useState("2026-09-01");
  const [to, setTo] = useState("2026-09-30");
  const [sel, setSel] = useState(logs[0]);
  const [shown, setShown] = useState(5);
  const periodErr = from >= to ? "Period must be start < end, at most 366 days" : undefined;
  const rows = logs.filter((l) => (res === "All" || l.res === res) && (!corr || l.corr.includes(corr))).slice(0, shown);
  return (
    <Page>
      <Tabs value={tab} onChange={setTab} tabs={[{ id: "log", label: "Audit log", count: logs.length }, { id: "devices", label: "Device events" }]} />
      {tab === "log" ? (
        <>
          <Card title="Filter">
            <div className="grid-fluid" style={{ ["--min"as string]: "170px" }}><Field label="Correlation ID"><Input value={corr} onChange={(e) => setCorr(e.target.value)} placeholder="corr-…" /></Field><Field label="From" error={periodErr}><Input type="date" value={from} onChange={(e) => setFrom(e.target.value)} /></Field><Field label="To"><Input type="date" value={to} onChange={(e) => setTo(e.target.value)} /></Field></div>
            <div className="mt-3 flex flex-wrap items-center justify-between gap-2"><Choice value={res} onChange={setRes} options={(["All", "Success", "Denied", "Failed", "Pending"] as const).map((k) => ({ id: k, label: k }))} /><Btn size="sm" onClick={() => { setCorr(""); setRes("All"); setFrom("2026-09-01"); setTo("2026-09-30"); }}>Reset</Btn></div>
          </Card>
          <div className="split-rev">
            <Card title="Audit log · newest first" className="self-start">
              {rows.length === 0 ? <EmptyState title="No audit entries match these filters">No audit entries match this correlation ID.</EmptyState> : <div className="flex flex-col gap-2">{rows.map((l) => <ListRow key={l.id} selected={sel.id === l.id} onClick={() => setSel(l)}><div className="min-w-0 flex-1"><div className="flex items-center justify-between gap-2"><b className="truncate text-[13px]">{l.op}</b><Badge tone={tone(l.res)}>{l.res}</Badge></div><div className="truncate text-[11px] text-muted">{l.target}</div><div className="truncate text-[11px] text-muted">{l.actor} · {l.at} · {l.corr}</div></div></ListRow>)}</div>}
              <p className="mt-3 text-[11px] text-muted">{rows.length} of 5 · 25 per page</p>
            </Card>
            <div className="flex min-w-0 flex-col gap-4">
              <Card title={sel.op} sub={sel.target}>
                <SummaryList items={[["Actor", sel.actor.split(" ")[0]], ["Role at the time", `${sel.actor.match(/\((.*)\)/)?.[1]} — kept as recorded, not rewritten`], ["Occurred", `2026-${sel.at} (Asia/Kuala_Lumpur)`], ["Correlation ID", <button key="c" className="font-mono text-primary underline" onClick={() => setCorr(sel.corr)}>{sel.corr} · Filter by ID →</button>], ["Reason", "Customer requested a short exception while the invoice is under review."]]} />
                <h3 className="mt-4 mb-2 text-[13px] font-bold">Before / after (masked)</h3>
                <DataTable rows={[{ f: "exception.until", a: "null", b: "2026-09-29 00:00", c: true }, { f: "exception.reason", a: "null", b: "Invoice under review", c: true }, { f: "contactPhone", a: "***masked***", b: "***masked***", c: false }, { f: "state", a: "applied", b: "applied", c: false }]} rowKey={(r) => r.f} cols={[{ key: "f", label: "Field", render: (r) => <span className="font-mono text-xs">{r.f}</span> }, { key: "a", label: "Before", render: (r) => r.a }, { key: "b", label: "After", render: (r) => <span className={r.c ? "font-bold text-warn" : ""}>{r.b}</span> }]} />
                <p className="mt-2 text-[11px] text-muted">Changed fields are highlighted. Secrets are masked.</p>
              </Card>
              <Card title="Related records"><SummaryList items={[["Restriction", "restriction-a41 · applied · exception until 09-29"], ["Command", "cmd-3302 · same correlation ID"], ["Job", "No related job"]]} /><p className="mt-2 text-[11px] text-muted">Links open existing detail screens and export no new data.</p></Card>
            </div>
          </div>
          {rows.length < logs.length && <div className="text-center"><Btn onClick={() => setShown((s) => s + 25)}>Load more</Btn></div>}
        </>
      ) : (
        <Card title="Device events" sub="device-online-rto · connection, power and tamper are separate"><Timeline items={events.map((e) => ({ time: e.t, title: e.e, detail: e.d }))} /></Card>
      )}
    </Page>
  );
}
