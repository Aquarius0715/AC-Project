"use client";

import Link from "next/link";
import { useState } from "react";
import { Badge, Banner, Btn, Card, ConnBadge, Field, Input, ListRow, Modal, Page, Search, SummaryList, Tabs, Textarea, Timeline, cx, useToast } from "@/components/ui";

const devices = [
  { id: "ac-001", serial: "AC-001", dev: "device-ac-001", unit: "unit-t11-new · ventilation-demo v3", fw: "fw v1 → v2", conn: "online" as const, tamper: false },
  { id: "device-online-rto", serial: "AC-DEMO-0001", dev: "device-online-rto", unit: "unit-online-rto · ventilation-demo v3", fw: "fw v1", conn: "online" as const, tamper: false },
  { id: "device-tamper", serial: "AC-DEMO-0002", dev: "device-tamper", unit: "unit-non-rto · cap-split-std v1", fw: "fw v1", conn: "offline" as const, tamper: true },
  { id: "device-offline-rto", serial: "AC-DEMO-0003", dev: "device-offline-rto", unit: "unit-offline-rto · cap-split-std v1", fw: "fw v1", conn: "offline" as const, tamper: false },
];
type F = "all" | "online" | "offline" | "tamper";

export function DevicesView({ initial }: { initial?: string }) {
  const toast = useToast();
  const [filter, setFilter] = useState<F>("all");
  const [q, setQ] = useState("");
  const [sel, setSel] = useState(devices.find((d) => d.id === initial) ?? devices[0]);
  const [fw, setFw] = useState<"idle" | "running" | "failed" | "ok">(sel.id === "ac-001" ? "running" : "idle");
  const [reg, setReg] = useState(false);
  const [cal, setCal] = useState<string | null>(null);
  const [rebind, setRebind] = useState(false);
  const [ack, setAck] = useState(false);
  const [f, setF] = useState({ serial: "", unit: "", ref: "25.0", meas: "25.3", reason: "" });
  const [hist, setHist] = useState<string[]>(["temperature · ref 25.0 °C / measured 25.3 °C · offset −0.3 °C · 2026-09-21 09:40 · tech-internal-a · job-t11"]);
  const [tried, setTried] = useState(false);
  const shown = devices.filter((d) => (filter === "all" || (filter === "tamper" ? d.tamper : d.conn === filter)) && (!q || (d.serial + d.dev + d.unit).toLowerCase().includes(q.toLowerCase())));
  const count = (k: F) => devices.filter((d) => k === "all" || (k === "tamper" ? d.tamper : d.conn === k)).length;
  const offline = sel.conn === "offline";
  const pick = (d: (typeof devices)[number]) => { setSel(d); setFw(d.id === "ac-001" ? "running" : "idle"); };
  const events: { t: string; title: string; detail: string; tone?: "warn" | "crit" }[] = [
    { t: "09-22 08:41", title: "Cover opened while powered · tamper_signal · alert-tamper-a", detail: ack ? "acknowledged with response note" : "open · unacknowledged", tone: "crit" },
    { t: "09-22 08:33", title: "Dedicated demo power signal lost · power_signal", detail: "open", tone: "warn" },
    { t: "09-22 08:12", title: "Missed 3 heartbeats · heartbeat (not proof of power loss)", detail: "open", tone: "warn" },
    { t: "09-20 10:02", title: "check · DEVICE_TIMEOUT", detail: "retry succeeded 10:05" },
  ];
  return (
    <Page>
      <Tabs value={filter} onChange={setFilter} tabs={(["all", "online", "offline", "tamper"] as F[]).map((k) => ({ id: k, label: k[0].toUpperCase() + k.slice(1), count: count(k) }))} />
      <div className="split-rev">
        <Card title="Devices — assigned units" sub="your jobs only" action={<Btn size="sm" onClick={() => setReg(true)}>+ Register</Btn>} className="self-start">
          <div className="mb-2"><Search placeholder="Search serial, device or unit…" value={q} onChange={setQ} /></div>
          <div className="flex flex-col gap-2">{shown.map((d) => <ListRow key={d.id} selected={sel.id === d.id} onClick={() => pick(d)}><div className="min-w-0 flex-1"><div className="flex items-center justify-between gap-2"><b>{d.serial}</b><span className="flex gap-1">{d.tamper && <Badge tone="crit">Tamper</Badge>}<ConnBadge s={d.conn} /></span></div><div className="truncate text-[11px] text-muted">{d.dev}</div><div className="truncate text-[11px] text-muted">{d.unit} · {d.fw}</div></div></ListRow>)}{shown.length === 0 && <p className="text-xs text-muted">No devices match.</p>}</div>
        </Card>
        <div className="flex min-w-0 flex-col gap-4">
          <Card title={sel.serial} sub={`${sel.dev} · registered by tech-internal-a · job-t11`} action={<Link className="text-xs font-semibold text-primary" href={`/technician/devices/${sel.id}`}>Device events →</Link>}>
            <div className="grid-fluid" style={{ ["--min"as string]: "140px" }}>
              <div className="rounded-xl bg-surface2 p-3"><div className="text-[11px] text-muted">Connection</div><ConnBadge s={sel.conn} /><div className="text-[10px] text-muted">{offline ? "lost 08:12" : "last seen 30 s ago"}</div></div>
              <div className="rounded-xl bg-surface2 p-3"><div className="text-[11px] text-muted">Power signal</div><b>{offline ? "Lost" : "ON"}</b><div className="text-[10px] text-muted">from device</div></div>
              <div className="rounded-xl bg-surface2 p-3"><div className="text-[11px] text-muted">Tamper</div><b className={sel.tamper ? "text-crit" : ""}>{sel.tamper ? "Detected" : "Clear"}</b></div>
              <div className="rounded-xl bg-surface2 p-3"><div className="text-[11px] text-muted">Firmware</div><b>v1</b><div className="text-[10px] text-muted">{sel.id === "ac-001" ? "v2 available" : "latest"}</div></div>
            </div>
            <p className="mt-3 text-[13px]"><span className="text-[11px] font-bold tracking-wide text-muted">BOUND TO UNIT</span><br /><b>{sel.unit.split(" · ")[0]}</b> · {sel.unit.split(" · ")[1]}</p>
            <div className="mt-3 flex flex-wrap gap-2"><Btn size="sm" onClick={() => toast(offline ? "No response — device offline" : "Ping OK (42 ms)", offline ? "warn" : "ok")}>Check connection</Btn><Btn size="sm" onClick={() => setCal("temperature")}>Calibrate sensor</Btn><Btn size="sm" disabled={fw === "running" || sel.id !== "ac-001"} onClick={() => { if (offline) return toast("Firmware update rejected — device offline", "crit"); setFw("running"); setTimeout(() => setFw("ok"), 2500); }}>{fw === "running" ? "Update running…" : "Update firmware"}</Btn><Btn size="sm" onClick={() => setRebind(true)}>Rebind</Btn></div>
            {fw === "running" && <div className="mt-3"><Banner>Firmware update v1 → v2 · Started 09:14 by tech-internal-a · control commands locked (D05) · fails if the device does not confirm by 09:29. <Btn size="sm" className="ml-2" onClick={() => setFw("failed")}>Simulate failure</Btn></Banner></div>}
            {fw === "failed" && <div className="mt-3"><Banner tone="crit">Firmware update failed — old version kept (v1).</Banner></div>}
            {fw === "ok" && <div className="mt-3"><Banner tone="ok">Firmware v2.0 succeeded — history appended.</Banner></div>}
          </Card>
          <Card title="Sensors (5)" sub="from the unit’s capability v3">
            <div className="scroll-x"><table className="w-full min-w-[420px] text-[13px]"><thead className="text-left text-[11px] uppercase text-muted"><tr><th>Metric</th><th>Unit</th><th>Stale after</th><th>Last calibrated</th><th /></tr></thead><tbody>{[["temperature", "°C"], ["humidity", "%"], ["co2", "ppm"], ["pm25", "µg/m³"], ["power", "kW"]].map(([m, u], i) => <tr key={m} className="border-t border-line"><td className="py-2">{m}</td><td>{u}</td><td>120 s</td><td>{i === 0 && sel.id === "ac-001" ? "2026-09-21 09:40" : "Never"}</td><td><button className="text-xs font-semibold text-primary" onClick={() => setCal(m)}>Calibrate →</button></td></tr>)}</tbody></table></div>
          </Card>
          <div className="split-even">
            <Card title="Operation history"><div className="text-[13px]">{[["firmware", "2026-09-21 09:14", "v1 → v2"], ["check", "2026-09-21 09:12", "heartbeat confirmed"]].map(([k, t, d]) => <div key={t} className="flex flex-wrap justify-between gap-2 border-t border-line py-2 first:border-0"><span><b>{k}</b> · {d}</span><span className="text-xs text-muted">{t}</span></div>)}</div></Card>
            <Card title="Calibration history">{hist.map((h) => <p key={h} className="border-t border-line py-2 text-xs first:border-0">{h}</p>)}<p className="text-[11px] text-muted">Appended — earlier readings unchanged. Calibration values are demo records.</p></Card>
          </div>
          {sel.tamper && <Card title="Device events" sub="connection, power and tamper are separate events" tone="crit" action={!ack && <Btn size="sm" variant="primary" onClick={() => { setAck(true); toast("alert-tamper-a acknowledged"); }}>Acknowledge alert-tamper-a →</Btn>}><Timeline items={events.map((e) => ({ time: e.t, title: e.title, detail: e.detail, tone: e.tone }))} /></Card>}
        </div>
      </div>
      <Modal open={reg} onClose={() => setReg(false)} title="Register device" footer={<><Btn onClick={() => setReg(false)}>Cancel</Btn><Btn variant="primary" onClick={() => { setTried(true); if (!f.serial.trim()) return; if (devices.some((d) => d.serial.toLowerCase() === f.serial.toLowerCase())) return; toast("Device registered"); setReg(false); }}>Register</Btn></>}>
        <Field label="Serial" error={tried && !f.serial.trim() ? "Serial is required" : devices.some((d) => d.serial.toLowerCase() === f.serial.toLowerCase()) ? "Duplicate serial (CONFLICT)" : undefined}><Input value={f.serial} onChange={(e) => setF({ ...f, serial: e.target.value })} placeholder="AC-002" /></Field>
        <Field label="Unit to bind"><Input value={f.unit} onChange={(e) => setF({ ...f, unit: e.target.value })} placeholder="unit-t11-new" /></Field>
      </Modal>
      <Modal open={!!cal} onClose={() => setCal(null)} title={`Calibrate ${cal}`} footer={<><Btn onClick={() => setCal(null)}>Cancel</Btn><Btn variant="primary" onClick={() => { setHist((h) => [`${cal} · ref ${f.ref} / measured ${f.meas} · offset ${(+f.ref - +f.meas).toFixed(1)} · just now · you`, ...h]); setCal(null); toast("Calibration recorded"); }}>Record calibration</Btn></>}>
        <div className="grid-fluid" style={{ ["--min"as string]: "140px" }}><Field label="Reference value"><Input type="number" value={f.ref} onChange={(e) => setF({ ...f, ref: e.target.value })} /></Field><Field label="Measured value"><Input type="number" value={f.meas} onChange={(e) => setF({ ...f, meas: e.target.value })} /></Field></div>
      </Modal>
      <Modal open={rebind} onClose={() => setRebind(false)} title="Rebind device" footer={<><Btn onClick={() => setRebind(false)}>Cancel</Btn><Btn variant="primary" onClick={() => { setTried(true); if (!f.reason.trim()) return; setRebind(false); toast("Device rebound"); }}>Rebind</Btn></>}>
        <Field label="New unit"><Input placeholder="unit-…" /></Field>
        <Field label="Reason (required)" error={tried && !f.reason.trim() ? "A reason is required" : undefined}><Textarea value={f.reason} onChange={(e) => setF({ ...f, reason: e.target.value })} /></Field>
      </Modal>
    </Page>
  );
}
