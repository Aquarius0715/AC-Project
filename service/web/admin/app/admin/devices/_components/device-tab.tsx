"use client";

import { useState } from "react";
import { Badge, Btn, Card, Check, ConnBadge, EmptyState, Field, Input, ListRow, Modal, Search, Select, SummaryList, Textarea, Timeline } from "@ac/web/components/ui";
import { useAction } from "@ac/web/lib/useAction";
import { useUrlPatch } from "@ac/web/lib/useUrlPatch";
import { klTime, metrics, metricUnit, type Metric } from "@ac/web/lib/devices";
import { bindDevice, calibrateSensor, checkDevice, registerDevice, updateFirmware } from "../actions";
import type { DevicesLive } from "./devices-view";

/** IoT devices (DD-A04): the device list and the deviceId detail with binding, sensors, operations, calibrations and
 * events; register, bind, connection check, calibration and firmware update are Server Actions. */
export function DeviceTab({ live }: { live: DevicesLive }) {
  const nav = useUrlPatch();
  const [pending, run] = useAction();
  const [q, setQ] = useState("");
  const [modal, setModal] = useState<null | "register" | "bind" | "calibrate" | "firmware">(null);
  const [tried, setTried] = useState(false);
  const [reg, setReg] = useState({ serial: "", unitId: "", sensors: [] as Metric[] });
  const [bind, setBind] = useState({ unitId: "", reason: "" });
  const [cal, setCal] = useState({ sensorId: "", reference: "", measured: "" });
  const [fw, setFw] = useState("");
  const shown = live.devices.filter((x) => !q || `${x.serial} ${x.unit}`.toLowerCase().includes(q.toLowerCase()));
  const dv = live.device;
  const close = () => { setModal(null); setTried(false); };
  const busy = !!dv?.detail.activeOperation;

  const register = () => {
    setTried(true);
    if (!reg.serial.trim() || !reg.unitId) return;
    run(() => registerDevice(reg.serial.trim(), reg.sensors, reg.unitId), "Device registered", (id) => { close(); setReg({ serial: "", unitId: "", sensors: [] }); nav({ deviceId: id }); });
  };
  const rebind = () => {
    setTried(true);
    if (!dv || !bind.unitId || !bind.reason.trim()) return;
    run(() => bindDevice(dv.detail.id, dv.detail.version, bind.unitId, bind.reason.trim()), "Device bound — new sensor IDs issued", close);
  };
  const calibrate = () => {
    setTried(true);
    const s = dv?.detail.sensors.find((x) => x.id === cal.sensorId);
    if (!dv || !s || cal.reference.trim() === "" || cal.measured.trim() === "" || !Number.isFinite(+cal.reference) || !Number.isFinite(+cal.measured)) return;
    run(() => calibrateSensor(dv.detail.id, dv.detail.version, s.id, s.metric, s.unit, +cal.reference, +cal.measured), "Calibration recorded (demo)", close);
  };
  const firmware = () => {
    setTried(true);
    if (!dv || !fw) return;
    run(() => updateFirmware(dv.detail.id, dv.detail.version, fw), `Firmware update to ${fw} queued`, close);
  };

  return (
    <div className="split-rev">
      <Card title="IoT devices" action={live.canWrite && <Btn size="sm" onClick={() => setModal("register")}>+ Register device</Btn>} className="self-start">
        <div className="mb-2"><Search placeholder="Search serial or unit…" value={q} onChange={setQ} /></div>
        {shown.length === 0 ? <EmptyState title="No devices">{q ? "No device matches this search." : "Register the first device."}</EmptyState> : <div className="flex flex-col gap-2">{shown.map((x) => <ListRow key={x.id} selected={dv?.row.id === x.id} onClick={() => nav({ deviceId: x.id })}><div className="min-w-0 flex-1"><div className="flex items-center justify-between gap-2"><b className="text-[13px]">{x.serial}</b><span className="flex gap-1">{x.tamper && <Badge tone="crit">Tamper</Badge>}<ConnBadge s={x.conn} /></span></div><div className="text-[11px] text-muted">{x.unit} · firmware {x.fw}</div></div></ListRow>)}</div>}
      </Card>
      {!dv ? <Card title="Device"><EmptyState title="No device selected">Choose a device.</EmptyState></Card> : (
        <div className="flex min-w-0 flex-col gap-4">
          <Card title={dv.row.serial} sub={`Device v${dv.detail.version}`} action={live.canWrite && <div className="flex flex-wrap gap-2"><Btn size="sm" disabled={pending || busy} onClick={() => run(() => checkDevice(dv.detail.id, dv.detail.version), "Connection check queued")}>Check connection</Btn><Btn size="sm" disabled={pending || busy || dv.detail.sensors.length === 0} onClick={() => { setCal({ sensorId: dv.detail.sensors[0]?.id ?? "", reference: "", measured: "" }); setModal("calibrate"); }}>Calibrate sensor</Btn><Btn size="sm" disabled={pending || busy} onClick={() => { setFw(dv.firmware.find((v) => v !== dv.detail.firmwareVersion) ?? ""); setModal("firmware"); }}>Update firmware</Btn><Btn size="sm" disabled={pending} onClick={() => { setBind({ unitId: "", reason: "" }); setModal("bind"); }}>Rebind</Btn></div>}>
            <div className="grid-fluid" style={{ ["--min" as string]: "130px" }}>
              <div className="rounded-xl bg-surface2 p-3"><div className="text-[11px] text-muted">Connection</div><ConnBadge s={dv.detail.connection} /></div>
              <div className="rounded-xl bg-surface2 p-3"><div className="text-[11px] text-muted">Power signal</div><b>{dv.detail.powerSignal}</b></div>
              <div className="rounded-xl bg-surface2 p-3"><div className="text-[11px] text-muted">Tamper</div><b className={dv.row.tamper ? "text-crit" : ""}>{dv.row.tamper ? "Detected" : "Clear"}</b></div>
              <div className="rounded-xl bg-surface2 p-3"><div className="text-[11px] text-muted">Firmware</div><b>{dv.detail.firmwareVersion}</b></div>
              <div className="rounded-xl bg-surface2 p-3"><div className="text-[11px] text-muted">Bound unit</div><b className="text-xs">{dv.row.unit}</b></div>
            </div>
            <div className="mt-3"><SummaryList items={[["Last seen", dv.detail.lastSeenAt ? klTime(dv.detail.lastSeenAt) : "—"], ["Active operation", dv.detail.activeOperation ? `${dv.detail.activeOperation.kind} · ${dv.detail.activeOperation.status} — other operations wait` : "none"]]} /></div>
            {busy && <p className="mt-2 text-[11px] text-muted">One operation at a time: check, calibration and firmware update are disabled until the active one finishes.</p>}
          </Card>
          <Card title="Sensors" sub="Each binding issues its own sensor IDs (SR24)">
            {dv.detail.sensors.length === 0 ? <p className="text-xs text-muted">This device has no sensors.</p> : <div className="scroll-x"><table className="w-full min-w-[360px] text-[13px]"><thead className="text-left text-[11px] uppercase text-muted"><tr><th>Metric</th><th>Unit</th><th>Stale after</th><th>Calibrated</th></tr></thead><tbody>{dv.detail.sensors.map((s) => <tr key={s.id} className="border-t border-line"><td className="py-1.5">{s.metric}</td><td>{s.unit}</td><td>{s.staleAfterSeconds} s</td><td className="text-xs">{s.calibratedAt ? klTime(s.calibratedAt) : "never"}</td></tr>)}</tbody></table></div>}
          </Card>
          <div className="grid-fluid" style={{ ["--min" as string]: "300px" }}>
            <Card title="Operations">{dv.operations.length === 0 ? <p className="text-xs text-muted">No operations yet.</p> : <Timeline items={dv.operations} />}</Card>
            <Card title="Calibrations">{dv.calibrations.length === 0 ? <p className="text-xs text-muted">No calibrations recorded.</p> : <Timeline items={dv.calibrations} />}</Card>
          </div>
          <Card title="Device events" sub="Connection, power and tamper are separate" tone={dv.row.tamper ? "crit" : undefined}>{dv.events.length === 0 ? <p className="text-xs text-muted">No device events.</p> : <Timeline items={dv.events} />}</Card>
        </div>
      )}
      <Modal open={modal === "register"} onClose={close} title="Register device" footer={<><Btn onClick={close}>Cancel</Btn><Btn variant="primary" disabled={pending} onClick={register}>Register</Btn></>}>
        <Field label="Serial" error={tried && !reg.serial.trim() ? "Serial is required" : undefined}><Input value={reg.serial} onChange={(e) => setReg({ ...reg, serial: e.target.value })} placeholder="AC-DEMO-0006" /></Field>
        <Field label="Bind to unit" error={tried && !reg.unitId ? "Choose a unit" : undefined}><Select value={reg.unitId} onChange={(e) => setReg({ ...reg, unitId: e.target.value })}><option value="">Select…</option>{live.units.map((u) => <option key={u.id} value={u.id}>{u.label}</option>)}</Select></Field>
        <div><p className="mb-1 text-[13px] font-semibold">Sensors</p><p className="mb-2 text-xs text-muted">Each must be defined in the unit model’s capability; none is allowed.</p><div className="flex flex-wrap gap-x-5 gap-y-1.5">{metrics.map((m) => <Check key={m} label={`${m} (${metricUnit[m]})`} checked={reg.sensors.includes(m)} onChange={(on) => setReg({ ...reg, sensors: on ? [...reg.sensors, m] : reg.sensors.filter((x) => x !== m) })} />)}</div></div>
      </Modal>
      <Modal open={modal === "bind"} onClose={close} title="Rebind device" footer={<><Btn onClick={close}>Cancel</Btn><Btn variant="primary" disabled={pending} onClick={rebind}>Rebind</Btn></>}>
        <Field label="New unit" error={tried && !bind.unitId ? "Choose a unit" : undefined}><Select value={bind.unitId} onChange={(e) => setBind({ ...bind, unitId: e.target.value })}><option value="">Select…</option>{live.units.filter((u) => u.id !== dv?.row.unitId).map((u) => <option key={u.id} value={u.id}>{u.label}</option>)}</Select></Field>
        <Field label="Reason" error={tried && !bind.reason.trim() ? "A reason is required" : undefined}><Textarea value={bind.reason} maxLength={1000} onChange={(e) => setBind({ ...bind, reason: e.target.value })} /></Field>
        <p className="text-[11px] text-muted">History stays with the previous binding; the device gets new sensor IDs on the new unit.</p>
      </Modal>
      <Modal open={modal === "calibrate"} onClose={close} title="Calibrate sensor" footer={<><Btn onClick={close}>Cancel</Btn><Btn variant="primary" disabled={pending} onClick={calibrate}>Record</Btn></>}>
        <Field label="Sensor"><Select value={cal.sensorId} onChange={(e) => setCal({ ...cal, sensorId: e.target.value })}>{dv?.detail.sensors.map((s) => <option key={s.id} value={s.id}>{s.metric} ({s.unit})</option>)}</Select></Field>
        <div className="grid-fluid" style={{ ["--min" as string]: "140px" }}><Field label="Reference" error={tried && (cal.reference.trim() === "" || !Number.isFinite(+cal.reference)) ? "A number" : undefined}><Input type="number" value={cal.reference} onChange={(e) => setCal({ ...cal, reference: e.target.value })} /></Field><Field label="Measured" error={tried && (cal.measured.trim() === "" || !Number.isFinite(+cal.measured)) ? "A number" : undefined}><Input type="number" value={cal.measured} onChange={(e) => setCal({ ...cal, measured: e.target.value })} /></Field></div>
        <p className="text-[11px] text-muted">Recorded at the current time as a demo calibration.</p>
      </Modal>
      <Modal open={modal === "firmware"} onClose={close} title="Update firmware" footer={<><Btn onClick={close}>Cancel</Btn><Btn variant="primary" disabled={pending || !fw} onClick={firmware}>Queue update</Btn></>}>
        {dv && dv.firmware.length === 0 ? <p className="text-xs text-muted">The unit model lists no firmware candidates.</p> : <Field label="Target version" error={tried && !fw ? "Choose a version" : undefined}><Select value={fw} onChange={(e) => setFw(e.target.value)}><option value="">Select…</option>{dv?.firmware.map((v) => <option key={v} value={v} disabled={v === dv.detail.firmwareVersion}>{v}{v === dv.detail.firmwareVersion ? " (current)" : ""}</option>)}</Select></Field>}
        <p className="text-[11px] text-muted">Only an online device accepts an update; a failure keeps the current version.</p>
      </Modal>
    </div>
  );
}
