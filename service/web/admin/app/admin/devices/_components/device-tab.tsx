"use client";

import { useState } from "react";
import { Badge, Btn, Card, Check, ConnBadge, EmptyState, Field, Input, ListRow, Modal, Search, Select, SummaryList, Textarea, Timeline } from "@ac/web/components/ui";
import { useT } from "@ac/web/components/I18n";
import { useAction } from "@ac/web/lib/useAction";
import { useUrlPatch } from "@ac/web/lib/useUrlPatch";
import { metrics, metricUnit, type Metric } from "@ac/web/lib/devices";
import { bindDevice, calibrateSensor, checkDevice, registerDevice, updateFirmware } from "../actions";
import type { DevicesLive } from "./devices-view";
import type { UnitSymbol } from "@ac/web/lib/contracts.gen";

const powerWord: Record<string, string> = { on: "on", off: "off", unknown: "unknown" };
const opWord: Record<string, string> = { check: "Connection check", calibrate: "Calibration", firmware: "Firmware update" };
const statusWord: Record<string, string> = { queued: "queued", running: "running", succeeded: "succeeded", failed: "failed" };

/** IoT devices (DD-A04): the device list and the deviceId detail with binding, sensors, operations, calibrations and
 * events; register, bind, connection check, calibration and firmware update are Server Actions. Texts in the display
 * language; times in the user's display time zone; metric and evidence codes stay codes (IR295). */
export function DeviceTab({ live }: { live: DevicesLive }) {
  const t = useT();
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
    run(() => registerDevice(reg.serial.trim(), reg.sensors, reg.unitId), t("Device registered"), (id) => { close(); setReg({ serial: "", unitId: "", sensors: [] }); nav({ deviceId: id }); });
  };
  const rebind = () => {
    setTried(true);
    if (!dv || !bind.unitId || !bind.reason.trim()) return;
    run(() => bindDevice(dv.detail.id, dv.detail.version, bind.unitId, bind.reason.trim()), t("Device bound — new sensor IDs issued"), close);
  };
  const calibrate = () => {
    setTried(true);
    const s = dv?.detail.sensors.find((x) => x.id === cal.sensorId);
    if (!dv || !s || cal.reference.trim() === "" || cal.measured.trim() === "" || !Number.isFinite(+cal.reference) || !Number.isFinite(+cal.measured)) return;
    run(() => calibrateSensor(dv.detail.id, dv.detail.version, s.id, s.metric, s.unit as UnitSymbol, +cal.reference, +cal.measured), t("Calibration recorded (demo)"), close);
  };
  const firmware = () => {
    setTried(true);
    if (!dv || !fw) return;
    run(() => updateFirmware(dv.detail.id, dv.detail.version, fw), t("Firmware update to {version} queued", { version: fw }), close);
  };
  const active = dv?.detail.activeOperation;

  return (
    <div className="split-rev">
      <Card title={t("IoT devices")} action={live.canWrite && <Btn size="sm" onClick={() => setModal("register")}>{t("+ Register device")}</Btn>} className="self-start">
        <div className="mb-2"><Search placeholder={t("Search serial or unit…")} value={q} onChange={setQ} /></div>
        {shown.length === 0 ? <EmptyState title={t("No devices")}>{t(q ? "No device matches this search." : "Register the first device.")}</EmptyState> : <div className="flex flex-col gap-2">{shown.map((x) => <ListRow key={x.id} selected={dv?.row.id === x.id} onClick={() => nav({ deviceId: x.id })}><div className="min-w-0 flex-1"><div className="flex items-center justify-between gap-2"><b className="text-[13px]">{x.serial}</b><span className="flex gap-1">{x.tamper && <Badge tone="crit">{t("Tamper")}</Badge>}<ConnBadge s={x.conn} /></span></div><div className="text-[11px] text-muted">{t("{unit} · firmware {version}", { unit: x.unit, version: x.fw })}</div></div></ListRow>)}</div>}
      </Card>
      {!dv ? <Card title={t("Device")}><EmptyState title={t("No device selected")}>{t("Choose a device.")}</EmptyState></Card> : (
        <div className="flex min-w-0 flex-col gap-4">
          <Card title={dv.row.serial} sub={t("Device v{v}", { v: dv.detail.version })} action={live.canWrite && <div className="flex flex-wrap gap-2"><Btn size="sm" disabled={pending || busy} onClick={() => run(() => checkDevice(dv.detail.id, dv.detail.version), t("Connection check queued"))}>{t("Check connection")}</Btn><Btn size="sm" disabled={pending || busy || dv.detail.sensors.length === 0} onClick={() => { setCal({ sensorId: dv.detail.sensors[0]?.id ?? "", reference: "", measured: "" }); setModal("calibrate"); }}>{t("Calibrate sensor")}</Btn><Btn size="sm" disabled={pending || busy} onClick={() => { setFw(dv.firmware.find((v) => v !== dv.detail.firmwareVersion) ?? ""); setModal("firmware"); }}>{t("Update firmware")}</Btn><Btn size="sm" disabled={pending} onClick={() => { setBind({ unitId: "", reason: "" }); setModal("bind"); }}>{t("Rebind")}</Btn></div>}>
            <div className="grid-fluid" style={{ ["--min" as string]: "130px" }}>
              <div className="rounded-xl bg-surface2 p-3"><div className="text-[11px] text-muted">{t("Connection")}</div><ConnBadge s={dv.detail.connection} /></div>
              <div className="rounded-xl bg-surface2 p-3"><div className="text-[11px] text-muted">{t("Power signal")}</div><b>{t(powerWord[dv.detail.powerSignal] ?? dv.detail.powerSignal)}</b></div>
              <div className="rounded-xl bg-surface2 p-3"><div className="text-[11px] text-muted">{t("Tamper")}</div><b className={dv.row.tamper ? "text-crit" : ""}>{t(dv.row.tamper ? "Detected" : "tamper::Clear")}</b></div>
              <div className="rounded-xl bg-surface2 p-3"><div className="text-[11px] text-muted">{t("Firmware")}</div><b>{dv.detail.firmwareVersion}</b></div>
              <div className="rounded-xl bg-surface2 p-3"><div className="text-[11px] text-muted">{t("Bound unit")}</div><b className="text-xs">{dv.row.unit}</b></div>
            </div>
            <div className="mt-3"><SummaryList items={[[t("Last seen"), dv.texts.lastSeen ?? "—"], [t("Active operation"), active ? t("{operation} · {status} — other operations wait", { operation: t(opWord[active.kind] ?? active.kind), status: t(statusWord[active.status] ?? active.status) }) : t("none")]]} /></div>
            {busy && <p className="mt-2 text-[11px] text-muted">{t("One operation at a time: check, calibration and firmware update are disabled until the active one finishes.")}</p>}
          </Card>
          <Card title={t("Sensors")} sub={t("Each binding issues its own sensor IDs (SR24)")}>
            {dv.detail.sensors.length === 0 ? <p className="text-xs text-muted">{t("This device has no sensors.")}</p> : <div className="scroll-x"><table className="w-full min-w-[360px] text-[13px]"><thead className="text-left text-[11px] uppercase text-muted"><tr><th>{t("Metric")}</th><th>{t("Unit")}</th><th>{t("Stale after")}</th><th>{t("Calibrated")}</th></tr></thead><tbody>{dv.detail.sensors.map((s) => <tr key={s.id} className="border-t border-line"><td className="py-1.5">{s.metric}</td><td>{s.unit}</td><td>{t("{n} s", { n: s.staleAfterSeconds })}</td><td className="text-xs">{dv.texts.calibrated[s.id] ?? t("never")}</td></tr>)}</tbody></table></div>}
          </Card>
          <div className="grid-fluid" style={{ ["--min" as string]: "300px" }}>
            <Card title={t("Operations")}>{dv.operations.length === 0 ? <p className="text-xs text-muted">{t("No operations yet.")}</p> : <Timeline items={dv.operations} />}</Card>
            <Card title={t("Calibrations")}>{dv.calibrations.length === 0 ? <p className="text-xs text-muted">{t("No calibrations recorded.")}</p> : <Timeline items={dv.calibrations} />}</Card>
          </div>
          <Card title={t("Device events")} sub={t("Connection, power and tamper are separate")} tone={dv.row.tamper ? "crit" : undefined}>{dv.events.length === 0 ? <p className="text-xs text-muted">{t("No device events.")}</p> : <Timeline items={dv.events} />}</Card>
        </div>
      )}
      <Modal open={modal === "register"} onClose={close} title={t("Register device")} footer={<><Btn onClick={close}>{t("Cancel")}</Btn><Btn variant="primary" disabled={pending} onClick={register}>{t("Register")}</Btn></>}>
        <Field label={t("Serial")} error={tried && !reg.serial.trim() ? t("Serial is required") : undefined}><Input value={reg.serial} onChange={(e) => setReg({ ...reg, serial: e.target.value })} placeholder="AC-DEMO-0006" /></Field>
        <Field label={t("Bind to unit")} error={tried && !reg.unitId ? t("Choose a unit") : undefined}><Select value={reg.unitId} onChange={(e) => setReg({ ...reg, unitId: e.target.value })}><option value="">{t("Select…")}</option>{live.units.map((u) => <option key={u.id} value={u.id}>{u.label}</option>)}</Select></Field>
        <div><p className="mb-1 text-[13px] font-semibold">{t("Sensors")}</p><p className="mb-2 text-xs text-muted">{t("Each must be defined in the unit model’s capability; none is allowed.")}</p><div className="flex flex-wrap gap-x-5 gap-y-1.5">{metrics.map((m) => <Check key={m} label={`${m} (${metricUnit[m]})`} checked={reg.sensors.includes(m)} onChange={(on) => setReg({ ...reg, sensors: on ? [...reg.sensors, m] : reg.sensors.filter((x) => x !== m) })} />)}</div></div>
      </Modal>
      <Modal open={modal === "bind"} onClose={close} title={t("Rebind device")} footer={<><Btn onClick={close}>{t("Cancel")}</Btn><Btn variant="primary" disabled={pending} onClick={rebind}>{t("Rebind")}</Btn></>}>
        <Field label={t("New unit")} error={tried && !bind.unitId ? t("Choose a unit") : undefined}><Select value={bind.unitId} onChange={(e) => setBind({ ...bind, unitId: e.target.value })}><option value="">{t("Select…")}</option>{live.units.filter((u) => u.id !== dv?.row.unitId).map((u) => <option key={u.id} value={u.id}>{u.label}</option>)}</Select></Field>
        <Field label={t("Reason")} error={tried && !bind.reason.trim() ? t("A reason is required") : undefined}><Textarea value={bind.reason} maxLength={1000} onChange={(e) => setBind({ ...bind, reason: e.target.value })} /></Field>
        <p className="text-[11px] text-muted">{t("History stays with the previous binding; the device gets new sensor IDs on the new unit.")}</p>
      </Modal>
      <Modal open={modal === "calibrate"} onClose={close} title={t("Calibrate sensor")} footer={<><Btn onClick={close}>{t("Cancel")}</Btn><Btn variant="primary" disabled={pending} onClick={calibrate}>{t("Record")}</Btn></>}>
        <Field label={t("Sensor")}><Select value={cal.sensorId} onChange={(e) => setCal({ ...cal, sensorId: e.target.value })}>{dv?.detail.sensors.map((s) => <option key={s.id} value={s.id}>{s.metric} ({s.unit})</option>)}</Select></Field>
        <div className="grid-fluid" style={{ ["--min" as string]: "140px" }}><Field label={t("Reference")} error={tried && (cal.reference.trim() === "" || !Number.isFinite(+cal.reference)) ? t("A number") : undefined}><Input type="number" value={cal.reference} onChange={(e) => setCal({ ...cal, reference: e.target.value })} /></Field><Field label={t("Measured")} error={tried && (cal.measured.trim() === "" || !Number.isFinite(+cal.measured)) ? t("A number") : undefined}><Input type="number" value={cal.measured} onChange={(e) => setCal({ ...cal, measured: e.target.value })} /></Field></div>
        <p className="text-[11px] text-muted">{t("Recorded at the current time as a demo calibration.")}</p>
      </Modal>
      <Modal open={modal === "firmware"} onClose={close} title={t("Update firmware")} footer={<><Btn onClick={close}>{t("Cancel")}</Btn><Btn variant="primary" disabled={pending || !fw} onClick={firmware}>{t("Queue update")}</Btn></>}>
        {dv && dv.firmware.length === 0 ? <p className="text-xs text-muted">{t("The unit model lists no firmware candidates.")}</p> : <Field label={t("Target version")} error={tried && !fw ? t("Choose a version") : undefined}><Select value={fw} onChange={(e) => setFw(e.target.value)}><option value="">{t("Select…")}</option>{dv?.firmware.map((v) => <option key={v} value={v} disabled={v === dv.detail.firmwareVersion}>{v === dv.detail.firmwareVersion ? t("{version} (current)", { version: v }) : v}</option>)}</Select></Field>}
        <p className="text-[11px] text-muted">{t("Only an online device accepts an update; a failure keeps the current version.")}</p>
      </Modal>
    </div>
  );
}
