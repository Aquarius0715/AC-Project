"use client";

import { useState } from "react";
import { Badge, Banner, Btn, Card, Check, ConnBadge, Field, Input, ListRow, Modal, Page, Search, Select, SummaryList, Tabs, Textarea, Timeline, useToast, cx } from "@/components/ui";

const models = [
  { id: "m2", name: "DemoAir SPL-200V", cap: "ventilation-demo · v3", used: "1 unit · 1 device", tag: "Ventilation" },
  { id: "m1", name: "DemoAir SPL-100", cap: "cap-split-std · v1", used: "4 units · 4 devices", tag: "" },
];
const devs = [
  { id: "device-online-rto", serial: "AC-DEMO-0001", unit: "unit-online-rto", conn: "online" as const, fw: "v1", tamper: false },
  { id: "device-tamper", serial: "AC-DEMO-0002", unit: "unit-non-rto", conn: "offline" as const, fw: "v1", tamper: true },
  { id: "device-offline-rto", serial: "AC-DEMO-0003", unit: "unit-offline-rto", conn: "offline" as const, fw: "v1", tamper: false },
  { id: "device-ac-001", serial: "AC-001", unit: "unit-t11-new", conn: "online" as const, fw: "v1", tamper: false },
  { id: "device-b", serial: "AC-DEMO-0005", unit: "unit-limited", conn: "online" as const, fw: "v1", tamper: false },
];

export default function AdminDevices() {
  const toast = useToast();
  const [tab, setTab] = useState<"models" | "devices">("models");
  const [m, setM] = useState(models[0]);
  const [d, setD] = useState(devs[0]);
  const [q, setQ] = useState("");
  const [fans, setFans] = useState({ low: true, mid: true, high: true });
  const [vent, setVent] = useState({ low: true, mid: true, high: false });
  const [reason, setReason] = useState("");
  const [min, setMin] = useState("16");
  const [max, setMax] = useState("30");
  const [tried, setTried] = useState(false);
  const [modal, setModal] = useState<null | "register" | "calibrate" | "rebind" | "history" | "newModel">(null);
  const [mf, setMf] = useState({ manu: "", model: "", serial: "" });
  const rangeErr = +min > +max ? "min must be ≤ max" : undefined;
  const saveModel = () => { setTried(true); if (!reason.trim() || rangeErr) return; toast("Saved — new capability version created (v4)"); setReason(""); setTried(false); };
  const shown = devs.filter((x) => !q || (x.serial + x.id + x.unit).toLowerCase().includes(q.toLowerCase()));
  return (
    <Page>
      <Tabs value={tab} onChange={setTab} tabs={[{ id: "models", label: "Models", count: 2 }, { id: "devices", label: "IoT devices", count: 5 }]} />
      {tab === "models" ? (
        <div className="split-rev">
          <Card title="Models" sub="tenant-a" action={<Btn size="sm" onClick={() => setModal("newModel")}>+ New</Btn>} className="self-start">
            <div className="flex flex-col gap-2">{models.map((x) => <ListRow key={x.id} selected={m.id === x.id} onClick={() => setM(x)}><div className="min-w-0"><b className="text-[13px]">{x.name}</b> {x.tag && <Badge tone="primary">{x.tag}</Badge>}<div className="text-[11px] text-muted">{x.cap}</div><div className="text-[11px] text-muted">{x.used}</div></div></ListRow>)}</div>
            <p className="mt-3 text-[11px] text-muted">Saving a change creates a new capability version. Unverified capabilities stay false/unknown (BR-A04).</p>
          </Card>
          <div className="flex min-w-0 flex-col gap-4">
            <Card title={m.name} sub={`${m.cap} · updated 2026-09-01 by hq-operator · used by unit-online-rto`} action={<Btn size="sm" onClick={() => setModal("history")}>Version history</Btn>}>
              <div className="flex flex-col gap-5">
                <section><h3 className="mb-2 text-[13px] font-bold">Identity</h3><div className="grid-fluid" style={{ ["--min"as string]: "200px" }}><Field label="Manufacturer"><Input defaultValue="DemoAir" /></Field><Field label="Model" hint="Manufacturer + model must be unique"><Input defaultValue={m.name.split(" ")[1]} /></Field></div></section>
                <section><h3 className="mb-1 text-[13px] font-bold">Controls</h3><p className="mb-2 text-xs text-muted">Only enable what has been verified on the real model — never infer from product category.</p><div className="grid-fluid" style={{ ["--min"as string]: "180px" }}><Check label="Power control" checked /><Check label="Mode control" checked /><Check label="Fan control" checked /></div></section>
                <section><h3 className="mb-1 text-[13px] font-bold">Temperature</h3><div className="grid-fluid" style={{ ["--min"as string]: "110px" }}><Field label="Min °C" error={rangeErr}><Input type="number" value={min} onChange={(e) => setMin(e.target.value)} /></Field><Field label="Max °C"><Input type="number" value={max} onChange={(e) => setMax(e.target.value)} /></Field><Field label="Step °C"><Input type="number" defaultValue={1} /></Field></div></section>
                <section><h3 className="mb-1 text-[13px] font-bold">Modes & fan</h3><div className="flex flex-wrap gap-x-6 gap-y-2 text-[13px]"><Check label="Cool" checked /><Check label="Dry" checked /><Check label="Fan" checked />{(["low", "mid", "high"] as const).map((k) => <Check key={k} label={`Fan ${k}`} checked={fans[k]} onChange={(v) => setFans({ ...fans, [k]: v })} />)}</div></section>
                <section><h3 className="mb-1 text-[13px] font-bold">Ventilation</h3><p className="mb-1 text-xs text-muted">When enabled, levels must be non-empty and include Low.</p><div className="flex flex-wrap gap-x-6 gap-y-2">{(["low", "mid", "high"] as const).map((k) => <Check key={k} label={k[0].toUpperCase() + k.slice(1)} checked={vent[k]} onChange={(v) => setVent({ ...vent, [k]: v })} />)}</div>{!vent.low && <p className="mt-1 text-xs text-crit">✕ Ventilation levels must include Low.</p>}</section>
                <section><h3 className="mb-2 text-[13px] font-bold">Sensors</h3><div className="scroll-x"><table className="w-full min-w-[380px] text-[13px]"><thead className="text-left text-[11px] uppercase text-muted"><tr><th>Metric</th><th>Unit</th><th>Stale after</th></tr></thead><tbody>{[["temperature", "°C"], ["humidity", "%"], ["co2", "ppm"], ["pm25", "µg/m³"], ["power", "kW"]].map(([a, b]) => <tr key={a} className="border-t border-line"><td className="py-1.5">{a}</td><td>{b}</td><td>120 s</td></tr>)}</tbody></table></div></section>
                <section><h3 className="mb-1 text-[13px] font-bold">Firmware candidates</h3><p className="text-[13px]">v1 <span className="text-xs text-muted">current on 1 device</span> · v2 <Badge tone="primary">latest</Badge></p></section>
                <section className="rounded-xl bg-surface2 p-3"><h3 className="mb-1 text-[13px] font-bold">Impact of this change</h3><ul className="list-disc pl-5 text-xs"><li><b>1 unit</b> — unit-online-rto (customer-a · Bedroom) — ventilation “High” will no longer be offered</li><li><b>1 automation rule</b> — “Night CO₂ purge” will be disabled with reason capability_changed</li><li><b>0 unfinished commands</b> — pending commands keep their original content</li></ul></section>
                <Field label="Change reason · required for updates" error={tried && !reason.trim() ? "A reason is required" : undefined} hint={`${reason.length} / 1000`}><Textarea value={reason} onChange={(e) => setReason(e.target.value)} placeholder="Vendor bulletin: SPL-200V ventilation High not verified on current batch." /></Field>
                <div className="flex justify-end"><Btn variant="primary" onClick={saveModel}>Save new version</Btn></div>
              </div>
            </Card>
          </div>
        </div>
      ) : (
        <div className="split-rev">
          <Card title="IoT devices" action={<Btn size="sm" onClick={() => setModal("register")}>+ Register</Btn>} className="self-start">
            <div className="mb-2"><Search placeholder="Search serial, device or unit…" value={q} onChange={setQ} /></div>
            <div className="flex flex-col gap-2">{shown.map((x) => <ListRow key={x.id} selected={d.id === x.id} onClick={() => setD(x)}><div className="min-w-0 flex-1"><div className="flex items-center justify-between gap-2"><b className="text-[13px]">{x.serial}</b><ConnBadge s={x.conn} /></div><div className="text-[11px] text-muted">{x.id} · {x.unit}</div></div></ListRow>)}</div>
          </Card>
          <div className="flex min-w-0 flex-col gap-4">
            <Card title={d.serial} sub={d.id} action={<div className="flex flex-wrap gap-2"><Btn size="sm" onClick={() => setModal("calibrate")}>Calibrate sensor</Btn><Btn size="sm" onClick={() => setModal("rebind")}>Rebind</Btn></div>}>
              <div className="grid-fluid" style={{ ["--min"as string]: "130px" }}><div className="rounded-xl bg-surface2 p-3"><div className="text-[11px] text-muted">Connection</div><ConnBadge s={d.conn} /></div><div className="rounded-xl bg-surface2 p-3"><div className="text-[11px] text-muted">Tamper</div><b className={d.tamper ? "text-crit" : ""}>{d.tamper ? "Detected" : "Clear"}</b></div><div className="rounded-xl bg-surface2 p-3"><div className="text-[11px] text-muted">Firmware</div><b>{d.fw}</b></div><div className="rounded-xl bg-surface2 p-3"><div className="text-[11px] text-muted">Bound unit</div><b className="text-xs">{d.unit}</b></div></div>
            </Card>
            {d.tamper && <Card title="Tamper + device events" tone="crit"><Timeline items={[{ time: "09-22 08:41", title: "Cover opened while powered · tamper_signal", detail: "open · unacknowledged", tone: "crit" }, { time: "09-22 08:33", title: "Dedicated demo power signal lost", tone: "warn" }, { time: "09-22 08:12", title: "Missed 3 heartbeats", tone: "warn" }]} /></Card>}
          </div>
        </div>
      )}
      <Modal open={modal === "register"} onClose={() => setModal(null)} title="Register device" footer={<><Btn onClick={() => setModal(null)}>Cancel</Btn><Btn variant="primary" onClick={() => { setTried(true); if (!mf.serial.trim()) return; toast("Device registered"); setModal(null); setTried(false); }}>Register</Btn></>}><Field label="Serial" error={tried && !mf.serial.trim() ? "Serial is required" : undefined}><Input value={mf.serial} onChange={(e) => setMf({ ...mf, serial: e.target.value })} /></Field><Field label="Model"><Select>{models.map((x) => <option key={x.id}>{x.name}</option>)}</Select></Field><Field label="Bind to unit"><Input placeholder="unit-…" /></Field></Modal>
      <Modal open={modal === "calibrate"} onClose={() => setModal(null)} title="Calibrate sensor" footer={<><Btn onClick={() => setModal(null)}>Cancel</Btn><Btn variant="primary" onClick={() => { toast("Calibration recorded"); setModal(null); }}>Record</Btn></>}><div className="grid-fluid" style={{ ["--min"as string]: "140px" }}><Field label="Reference"><Input defaultValue="25.0" /></Field><Field label="Measured"><Input defaultValue="25.3" /></Field></div></Modal>
      <Modal open={modal === "rebind"} onClose={() => setModal(null)} title="Rebind device" footer={<><Btn onClick={() => setModal(null)}>Cancel</Btn><Btn variant="primary" onClick={() => { toast("Device rebound"); setModal(null); }}>Rebind</Btn></>}><Field label="New unit"><Input placeholder="unit-…" /></Field><Field label="Reason"><Textarea /></Field></Modal>
      <Modal open={modal === "history"} onClose={() => setModal(null)} title="Version history (audit.read)" footer={<Btn onClick={() => setModal(null)}>Close</Btn>}><Timeline items={[{ time: "2026-09-01", title: "v3 — ventilation levels added", detail: "hq-operator" }, { time: "2026-07-12", title: "v2 — firmware candidate v2", detail: "hq-operator" }, { time: "2026-05-02", title: "v1 — created", detail: "hq-operator" }]} /></Modal>
      <Modal open={modal === "newModel"} onClose={() => { setModal(null); setTried(false); }} title="New model" footer={<><Btn onClick={() => setModal(null)}>Cancel</Btn><Btn variant="primary" onClick={() => { setTried(true); if (!mf.manu.trim() || !mf.model.trim()) return; toast("Model created"); setModal(null); setTried(false); }}>Create</Btn></>}><Field label="Manufacturer" error={tried && !mf.manu.trim() ? "Required" : undefined}><Input value={mf.manu} onChange={(e) => setMf({ ...mf, manu: e.target.value })} /></Field><Field label="Model" error={tried && !mf.model.trim() ? "Required" : undefined}><Input value={mf.model} onChange={(e) => setMf({ ...mf, model: e.target.value })} /></Field></Modal>
    </Page>
  );
}
