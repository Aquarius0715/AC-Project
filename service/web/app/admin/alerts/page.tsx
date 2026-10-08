"use client";

import { Suspense, useState } from "react";
import { useSearchParams } from "next/navigation";
import { Badge, Banner, Btn, Card, Check, Choice, DataTable, Field, Input, ListRow, Modal, Page, Select, SeverityBadge, SummaryList, Tabs, Textarea, Timeline, Toggle, LineChart, useToast, cx } from "@/components/ui";

const alerts = [
  { id: "alert-c08-critical", title: "High temperature observed", time: "08:55", sev: "critical" as const, meta: "customer-a · Bedroom AC · no policy", st: "Open" },
  { id: "alert-insulation-a", title: "Poor insulation suspected", time: "08:45", sev: "warning" as const, meta: "customer-a · Bedroom AC", st: "Open" },
  { id: "alert-temp-a", title: "Temperature ≥ 30 °C for 60 s", time: "08:58", sev: "warning" as const, meta: "customer-a · Bedroom AC", st: "Open" },
  { id: "alert-unknown-a", title: "Cooling load increase", time: "08:48", sev: "warning" as const, meta: "customer-a · Bedroom AC", st: "Open" },
  { id: "alert-window-a", title: "Possible open window", time: "08:50", sev: "warning" as const, meta: "customer-a · Bedroom AC", st: "Open" },
];
const pols = [
  { id: "policy-default", name: "Default policy", group: "DEFAULT · ON EVERY UNIT", sub: "6 rules · ventilation (CO₂, PM2.5) + fault causes", metric: "Ventilation / faults", on: true, units: "every unit" },
  { id: "policy-temp-a", name: "Bedroom too hot", group: "CUSTOMER-A", sub: "Temperature ≥ 30 °C for 60 s", metric: "Temperature", on: true, units: "2 units" },
  { id: "policy-co2-a", name: "Stuffy office", group: "CUSTOMER-A", sub: "CO₂ ≥ 1200 ppm · 15 min · weekdays", metric: "CO₂ (ppm)", on: true, units: "2 units" },
  { id: "policy-humidity-a", name: "Night humidity", group: "CUSTOMER-A", sub: "Humidity ≥ 70 % · 30 min · 22–06", metric: "Humidity", on: false, units: "0 units" },
];

function Inner() {
  const toast = useToast();
  const sp = useSearchParams();
  const [tab, setTab] = useState<"alerts" | "policies">(sp.get("tab") === "policies" ? "policies" : "alerts");
  const [al, setAl] = useState(alerts);
  const [a, setA] = useState(alerts[4]);
  const [p, setP] = useState(pols[1]);
  const [sev, setSev] = useState<"normal" | "warning" | "critical">("warning");
  const [th, setTh] = useState("30");
  const [rec, setRec] = useState("28");
  const [dur, setDur] = useState("60");
  const [name, setName] = useState("Bedroom too hot");
  const [resolve, setResolve] = useState(false);
  const [reason, setReason] = useState("");
  const [tried, setTried] = useState(false);
  const recErr = +rec >= +th ? `Recovery ${rec} must be below the ${th} threshold (direction for ≥)` : undefined;
  const durErr = +dur < 1 || +dur > 86400 ? "Duration must be 1–86400 s" : undefined;
  const setStatus = (st: string) => { setAl((l) => l.map((x) => (x.id === a.id ? { ...x, st } : x))); setA({ ...a, st }); };
  return (
    <Page>
      <Tabs value={tab} onChange={setTab} tabs={[{ id: "alerts", label: "Alerts", count: al.length }, { id: "policies", label: "Policies", count: pols.length }]} />
      {tab === "alerts" ? (
        <>
          <div className="grid-fluid" style={{ ["--min"as string]: "140px" }}>{[["Open", al.filter((x) => x.st === "Open").length], ["Acknowledged", al.filter((x) => x.st === "Acknowledged").length], ["Resolved", al.filter((x) => x.st === "Resolved").length + 2]].map(([l, n]) => <div key={l as string} className="rounded-2xl border border-line bg-surface p-4"><div className="text-xs text-muted">{l}</div><div className="text-2xl font-bold">{n}</div></div>)}</div>
          <div className="split-rev">
            <Card title="Open alerts · all customers" sub="severity ↓" className="self-start"><div className="flex flex-col gap-2">{al.map((x) => <ListRow key={x.id} selected={a.id === x.id} onClick={() => setA(x)}><div className="min-w-0 flex-1"><div className="flex items-center justify-between gap-2"><b className="text-[13px]">{x.title}</b><span className="text-[11px] text-muted">{x.time}</span></div><div className="flex items-center gap-2 text-[11px] text-muted"><SeverityBadge s={x.sev} />{x.st !== "Open" && <Badge tone={x.st === "Resolved" ? "ok" : "primary"}>{x.st}</Badge>}</div><div className="truncate text-[11px] text-muted">{x.meta} · {x.id}</div></div></ListRow>)}</div></Card>
            <div className="flex min-w-0 flex-col gap-4">
              <Card title={a.title} sub={`${a.id} · detected 2026-09-14 ${a.time} MYT · not linked to a policy`} action={<div className="flex gap-2"><Btn size="sm" disabled={a.st !== "Open"} onClick={() => { setStatus("Acknowledged"); toast("Acknowledged"); }}>Acknowledge</Btn><Btn size="sm" variant="primary" disabled={a.st === "Resolved"} onClick={() => setResolve(true)}>Resolve…</Btn></div>}>
                <SummaryList items={[["Unit", "Bedroom AC · unit-online-rto"], ["Context", "customer-a · Home A › 1F › Bedroom · Running · Online"], ["Evidence", "inferred — “Demo: cooling load rose while the outdoor temperature was stable.”"], ["Cause", "window_open (suspected)"], ["Observed at", `${a.time} MYT`], ["Evidence records", "none attached"]]} />
                <p className="mt-2 text-xs text-muted">This is an inference, not a confirmed cause. Ask the customer to check windows, or request an inspection to record evidence.</p>
              </Card>
              <Card title="Timeline"><Timeline items={[{ time: a.time, title: "Alert detected", detail: "Load increase with stable outdoor temperature", tone: "warn" }, { time: "08:50", title: "Notified customer-a · In-app", detail: "deliveryState: simulated" }, { time: "08:52", title: "customer-a read the notification", detail: "Reading a notification does not acknowledge or resolve the alert" }, { time: "—", title: a.st === "Resolved" ? "Resolved" : a.st === "Acknowledged" ? "Acknowledged — waiting for resolution" : "Waiting for acknowledgement", tone: a.st === "Resolved" ? "ok" : undefined }]} /><p className="mt-2 text-[11px] text-muted">Resolving requires a reason and evidence (remeasurement or a recorded confirmation). Completing maintenance work does not resolve the alert by itself.</p></Card>
            </div>
          </div>
        </>
      ) : (
        <div className="split-rev">
          <Card title="Alert policies" sub="by customer" action={<Btn size="sm" variant="primary" onClick={() => toast("New policy draft created")}>+ New</Btn>} className="self-start">
            {["DEFAULT · ON EVERY UNIT", "CUSTOMER-A"].map((g) => <div key={g} className="mb-3"><div className="mb-1 text-[10px] font-bold tracking-wide text-muted">{g}</div><div className="flex flex-col gap-2">{pols.filter((x) => x.group === g).map((x) => <ListRow key={x.id} selected={p.id === x.id} onClick={() => { setP(x); setName(x.name); }}><div className="min-w-0 flex-1"><b className="text-[13px]">{x.name}</b><div className="text-[11px] text-muted">{x.sub}</div><div className="text-[11px] text-muted">{x.units}{!x.on && " · Off"}</div></div></ListRow>)}</div></div>)}
          </Card>
          <Card title={p.name} sub={`${p.id} · customer-a · made by hq-operator · 1 open alert from this policy`} action={<Badge tone={p.on ? "ok" : "muted"}>{p.on ? "Enabled" : "Disabled"}</Badge>}>
            {p.id === "policy-default" ? <Banner>Default policy rules are set by HQ for every unit. Per-customer on/off is controlled by the customer.</Banner> : (
              <div className="flex flex-col gap-5">
                <section><h3 className="mb-2 text-[13px] font-bold">Basics</h3><div className="grid-fluid" style={{ ["--min"as string]: "180px" }}><Field label="Name" error={tried && !name.trim() ? "1–120 characters" : undefined}><Input value={name} onChange={(e) => setName(e.target.value)} /></Field><Field label="Priority" hint="0–100"><Input type="number" defaultValue={50} /></Field><Field label="Timezone"><Select><option>Asia/Kuala_Lumpur</option></Select></Field></div></section>
                <section><h3 className="mb-1 text-[13px] font-bold">Owner & units</h3><p className="text-xs text-muted">A policy belongs to one customer. Units pick it up on their own page (Customers & units › unit › Alert policies). Attached to {p.units} of customer-a · read-only here.</p></section>
                <section><h3 className="mb-2 text-[13px] font-bold">Condition</h3><p className="mb-2 text-xs text-muted">Missing or stale readings never count toward the condition — they raise a data-quality notice instead.</p><div className="grid-fluid" style={{ ["--min"as string]: "130px" }}><Field label="Metric"><Select defaultValue={p.metric}>{["Temperature", "Humidity", "CO₂ (ppm)", "PM2.5", "Refrigerant pressure", "Vibration", "Power"].map((m) => <option key={m}>{m}</option>)}</Select></Field><Field label="Alert when ≥"><Input type="number" value={th} onChange={(e) => setTh(e.target.value)} /></Field><Field label="For (s)" error={durErr}><Input type="number" value={dur} onChange={(e) => setDur(e.target.value)} /></Field><Field label="Recover below" error={recErr}><Input type="number" value={rec} onChange={(e) => setRec(e.target.value)} /></Field></div>
                  <div className="mt-2"><LineChart points={[20, 24, 28, 31, 32, 31, 29, 27, 25]} min={18} max={34} threshold={+th} height={90} /></div>
                  <p className={cx("text-xs", recErr ? "text-crit" : "text-ok")}>{recErr ? `✕ ${recErr}` : `✓ Recovery ${rec} is below the ${th} threshold (correct direction for ≥)`}</p></section>
                <section><h3 className="mb-1 text-[13px] font-bold">Severity</h3><Choice value={sev} onChange={setSev} options={[{ id: "normal", label: "Info" }, { id: "warning", label: "Warning" }, { id: "critical", label: "Critical" }]} /><p className="mt-1 text-[11px] text-muted">A severity change on an open alert counts as a new notification reason.</p></section>
                <section><h3 className="mb-2 text-[13px] font-bold">Notify</h3><p className="mb-2 text-xs text-muted">Recipients come from notifications.recipients — only people allowed to see these units.</p>{[["customer-a", "Client · unit owner", true, true], ["hq-operator", "Admin · HQ on-call", true, true]].map(([n, r, a1, e]) => <div key={n as string} className="flex flex-wrap items-center justify-between gap-2 border-t border-line py-2 text-[13px]"><span><b>{n as string}</b><span className="block text-xs text-muted">{r as string}</span></span><span className="flex gap-3"><Check label="In-app" checked={!!a1} /><Check label="Email" checked={!!e} /><Check label="WhatsApp" disabled /></span></div>)}<p className="text-[11px] text-muted">WhatsApp is greyed out when the recipient has not allowed that channel.</p></section>
                <section className="grid-fluid" style={{ ["--min"as string]: "180px" }}><Field label="Cooldown (min)" hint="Same severity is not re-sent within this window (1–1440)"><Input type="number" defaultValue={5} /></Field><Field label="Escalate after (min)" hint="Escalate if still unacknowledged (1–1440)"><Input type="number" defaultValue={60} /></Field></section>
                <div className="flex justify-end"><Btn variant="primary" onClick={() => { setTried(true); if (recErr || durErr || !name.trim()) return toast("Fix the validation errors first", "crit"); toast("Policy saved"); }}>Save policy</Btn></div>
              </div>
            )}
          </Card>
        </div>
      )}
      <Modal open={resolve} onClose={() => setResolve(false)} title="Resolve alert" footer={<><Btn onClick={() => setResolve(false)}>Cancel</Btn><Btn variant="primary" onClick={() => { setTried(true); if (!reason.trim()) return; setStatus("Resolved"); setResolve(false); toast("Alert resolved"); }}>Resolve</Btn></>}><p className="text-xs text-muted">Resolving requires a reason and evidence. Completing maintenance work does not resolve the alert by itself.</p><Field label="Reason (required)" error={tried && !reason.trim() ? "A reason is required" : undefined}><Textarea value={reason} onChange={(e) => setReason(e.target.value)} /></Field></Modal>
    </Page>
  );
}
export default function AdminAlerts() { return <Suspense><Inner /></Suspense>; }
