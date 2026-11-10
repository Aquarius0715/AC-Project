"use client";

import { useEffect, useState } from "react";
import { Badge, Btn, Card, Check, ConnBadge, cx, EmptyState, Field, Input, ListRow, Modal, Search, Select, Textarea, Plain, ScrollX } from "@ac/web/components/ui";
import { useT } from "@ac/web/components/I18n";
import { useAction } from "@ac/web/lib/useAction";
import { useUrlPatch } from "@ac/web/lib/useUrlPatch";
import { metrics, metricUnit, mqttNodeValue, type Metric } from "@ac/web/lib/devices";
import { connectionDot, filterOf, firmwareCardLook, firmwareSub, followFilter, HQ_FILTERS, operationTone, type Filter, metricText } from "@ac/web/lib/techDevices";
import { bindDevice, calibrateSensor, checkDevice, deleteDevice, registerDevice, setMqttNode, unbindDevice, updateFirmware } from "../actions";
import type { DevicesLive } from "./devices-view";
import { Icon } from "@ac/web/components/Icon";

const dot: Record<string, string> = { ok: "bg-ok", crit: "bg-crit", warn: "bg-warn", primary: "bg-primary", muted: "bg-line" };

/** IoT devices (DD-A04, Figma Admin 246:2): the list sorted by serial with the state chips and the search, and the
deviceId detail as the technician's device screen draws it — tiles, the bound unit, the actions, the firmware card,
sensors with Calibrate… (a small button like the device actions, IR576), the operation and calibration history and the events. Register, bind, connection check,
calibration and firmware update are Server Actions. Texts in the display language; times in the user's display time
zone; metric and evidence codes stay codes (IR295, IR319). */
export function DeviceTab({ live }: Readonly<{ live: DevicesLive }>) {
  const t = useT();
  const nav = useUrlPatch();
  const [pending, run] = useAction();
  const [filter, setFilter] = useState<Filter>("all");
  const [q, setQ] = useServerSearch(live.page?.q);
  const [modal, setModal] = useState<null | "register" | "bind" | "unbind" | "delete" | "calibrate" | "firmware" | "node">(null);
  const [tried, setTried] = useState(false);
  const [reg, setReg] = useState({ serial: "", unitId: "", sensors: [] as Metric[], node: "" });
  const [node, setNode] = useState("");
  const [bind, setBind] = useState({ unitId: "", reason: "" });
  const reasonMissing = t("A reason is required");
  const [why, setWhy] = useState(""); // the reason of an unbind or a delete (DEC-103)
  const [cal, setCal] = useState({ sensorId: "", reference: "", measured: "" });
  const [fw, setFw] = useState("");
  const rows = live.rows ?? [];
  const counts = HQ_FILTERS.map(({ id: f, label }) => ({ f, label: t(label), n: rows.filter(filterOf(f)).length }));
  const shown = rows.filter(filterOf(filter));
  const dv = live.device;
  const close = () => { setModal(null); setTried(false); };
  const isBusy = !!dv?.detail.activeOperation;

  const register = () => {
    setTried(true);
    const regNode = mqttNodeValue(reg.node);
    if (!reg.serial.trim() || !reg.unitId || regNode === undefined) return;
    run(() => registerDevice(reg.serial.trim(), reg.sensors, reg.unitId, regNode), t("Device registered — bind it to the unit once it is installed"), (id) => { close(); setReg({ serial: "", unitId: "", sensors: [], node: "" }); nav({ deviceId: id }); });
  };
  // the node ID the device's ESP32 sends: the IoT bridge feeds and commands the device by it (IR635)
  const saveNode = () => {
    setTried(true);
    const next = mqttNodeValue(node);
    if (!dv || next === undefined) return;
    run(() => setMqttNode(dv.detail.id, dv.detail.version, next), t(next === null ? "MQTT node ID cleared" : "MQTT node ID saved"), close);
  };
  // the first binding connects the device to its planned unit; a rebinding moves it (D05, IR467)
  const isFirst = !!dv && !dv.row.unitId;
  const rebind = () => {
    setTried(true);
    if (!dv || !bind.unitId || !bind.reason.trim()) return;
    run(() => bindDevice(dv.detail.id, dv.detail.version, bind.unitId, bind.reason.trim()), t(isFirst ? "Device bound to the unit — it is connected now" : "Device bound — new sensor IDs issued"), close);
  };
  const leave = (kind: "unbind" | "delete") => {
    setTried(true);
    if (!dv || !why.trim()) return;
    const act = kind === "unbind" ? unbindDevice : deleteDevice;
    run(() => act(dv.detail.id, dv.detail.version, why.trim()), t(kind === "unbind" ? "Device unbound — it stays registered" : "Device deleted"), () => { close(); setWhy(""); if (kind === "delete") nav({ deviceId: null }); });
  };
  const calibrate = () => {
    setTried(true);
    const s = dv?.detail.sensors.find((x) => x.id === cal.sensorId);
    if (!dv || !s || cal.reference.trim() === "" || cal.measured.trim() === "" || !Number.isFinite(Number(cal.reference)) || !Number.isFinite(Number(cal.measured))) return;
    run(() => calibrateSensor(dv.detail.id, dv.detail.version, s.id, s.metric, s.unit, Number(cal.reference), Number(cal.measured)), t("Calibration recorded (demo)"), close);
  };
  const firmware = () => {
    setTried(true);
    if (!dv || !fw) return;
    run(() => updateFirmware(dv.detail.id, dv.detail.version, fw), t("Firmware update to {version} queued", { version: fw }), close);
  };
  const calibrateOf = (sensorId: string) => { setCal({ sensorId, reference: "", measured: "" }); setModal("calibrate"); };

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div className="flex flex-wrap gap-2">{counts.map(({ f, label, n }) => (
          <Plain key={f} type="button" aria-pressed={filter === f} onClick={() => { setFilter(f); const next = followFilter(rows, f, dv?.row.id ?? null); if (next) nav({ deviceId: next }); }}
            className={cx("flex items-center gap-1.5 rounded-full border px-3 py-1 text-[13px] font-semibold", filter === f ? "border-ink bg-ink text-white" : "border-line bg-surface")}>
            {f !== "all" && f !== "nosensors" && <span className={cx("h-2 w-2 rounded-full", connectionDot(f))} />}{label} <span className="text-xs opacity-70">{n}</span>
          </Plain>
        ))}</div>
        <div className="flex flex-wrap items-center gap-2"><div className="w-64"><Search placeholder={t("Search serial, device or unit…")} value={q} onChange={setQ} /></div>
          {live.canWrite && <Btn variant="primary" onClick={() => { setModal("register"); }}><Icon name="add" /> {t("Register device")}</Btn>}</div>
      </div>
      <div className="split-rev">
        <Card title={t("Devices")} sub={t("sorted by serial")} className="self-start">
          {shown.length === 0 ? <EmptyState title={t("No devices")}>{t(q || filter !== "all" ? "No device matches." : "Register the first device.")}</EmptyState> : (
            <div className="flex flex-col gap-2">{shown.map((x) => (
              <ListRow key={x.id} selected={dv?.row.id === x.id} onClick={() => { nav({ deviceId: x.id }); }}>
                <div className="min-w-0 flex-1">
                  <div className="flex items-center justify-between gap-2"><b className="whitespace-nowrap text-[13px]">{x.serial}</b><span className="flex flex-wrap items-center justify-end gap-1"><ConnBadge s={x.conn} />{x.tamper && <Badge tone="warn">{t("Tamper")}</Badge>}{x.connText && <span className="text-[10px] text-muted-foreground">{x.connText}</span>}</span></div>
                  <div className="flex justify-between gap-2 text-[11px] text-muted-foreground"><span className="truncate">{x.unit}</span><span>{t(x.sensors ? "fw {version}" : "fw {version} · no sensors", { version: x.fw })}</span></div>
                </div>
              </ListRow>
            ))}</div>
          )}
                  {live.page && <RegistryPaging shown={rows.length} total={live.deviceTotal} page={live.page} />}
        </Card>
        {dv ? (
          <Card>
            <div className="flex flex-col gap-4">
              <div><h2 className="text-lg font-bold">{dv.row.serial}</h2><p className="text-xs text-muted-foreground">{dv.detail.id.slice(0, 8)} · {dv.row.unit} · {t("Device v{v}", { v: dv.detail.version })}</p></div>
              <div className="grid-fluid" style={{ ["--min" as string]: "150px" }}>
                {dv.tiles.map((x) => (
                  <div key={x.label} className={cx("rounded-xl border p-3", x.warn ? "border-[#fdba74] bg-warn-soft/50" : "border-line")}>
                    <div className="text-[11px] text-muted-foreground">{x.label}</div>
                    <div className="flex items-center gap-1.5 text-[15px] font-bold">{x.dot && <span className={cx("h-2 w-2 rounded-full", dot[x.dot])} />}<span className={x.warn ? "text-warn" : ""}>{x.value}</span></div>
                    <div className="text-[10px] text-muted-foreground">{x.sub}</div>
                  </div>
                ))}
              </div>
              <div className="flex flex-wrap items-center justify-between gap-2 rounded-xl border border-line p-3">
                <div className="text-[13px]"><div className="text-[11px] font-bold uppercase tracking-wide text-muted-foreground">{t(isFirst ? "Not bound yet" : "Bound to unit")}</div>
                  {dv.unit ? <><b>{dv.unit.name}</b><div className="text-xs text-muted-foreground">{dv.unit.place} · {t("model {model}", { model: dv.unit.model })}</div></> : <b>{dv.row.unit}</b>}
                  {isFirst && <div className="text-xs text-muted-foreground">{t("Registered only: bind it once it is installed, then the unit's data and controls use it.")}</div>}</div>
                {live.canWrite && (isFirst
                  ? <Btn size="sm" variant="primary" disabled={pending} onClick={() => { setBind({ unitId: dv.row.targetUnitId ?? "", reason: t("Installed on site") }); setModal("bind"); }}>{t("Bind…")}</Btn>
                  : <span className="flex flex-wrap gap-2"><Btn size="sm" disabled={pending} onClick={() => { setBind({ unitId: "", reason: "" }); setModal("bind"); }}>{t("Rebind…")}</Btn>
                    <Btn size="sm" disabled={pending || isBusy} onClick={() => { setWhy(""); setModal("unbind"); }}>{t("Unbind…")}</Btn></span>)}
              </div>
              <div className="flex flex-wrap items-center justify-between gap-2 rounded-xl border border-line p-3">
                <div className="text-[13px]"><div className="text-[11px] font-bold uppercase tracking-wide text-muted-foreground">{t("MQTT node ID")}</div>
                  {dv.row.mqttNode ? <b>{dv.row.mqttNode}</b> : <b className="text-muted-foreground">{t("Not set")}</b>}
                  <div className="text-xs text-muted-foreground">{t("The IoT bridge takes this node's readings in and sends its commands here, within 30 seconds of a change.")}</div></div>
                {live.canWrite && <Btn size="sm" disabled={pending} onClick={() => { setNode(dv.row.mqttNode ?? ""); setModal("node"); }}>{t("Edit")}…</Btn>}
              </div>
              {live.canWrite && (
                <div className="grid-fluid" style={{ ["--min" as string]: "200px" }}>
                  {[
                    { label: t("Check connection"), sub: t("ping the device"), act: () => { run(() => checkDevice(dv.detail.id, dv.detail.version), t("Connection check queued")); }, off: false },
                    { label: t("Calibrate sensor"), sub: t("reference vs measured"), act: () => { calibrateOf(dv.detail.sensors[0]?.id ?? ""); }, off: dv.detail.sensors.length === 0 },
                    { label: t("Update firmware"), sub: firmwareSub(dv.detail.activeOperation?.kind === "firmware", dv.firmware, t), act: () => { setFw(dv.firmware.at(-1) ?? ""); setModal("firmware"); }, off: dv.firmware.length === 0 },
                  ].map((a) => (
                    <Plain key={a.label} type="button" disabled={pending || isBusy || a.off} onClick={a.act} className="rounded-xl border border-line px-3 py-2 text-left hover:bg-surface2 disabled:cursor-not-allowed disabled:opacity-50">
                      <div className="text-[13px] font-semibold">{a.label}</div><div className="text-[11px] text-muted-foreground">{a.sub}</div>
                    </Plain>
                  ))}
                </div>
              )}
              {live.canWrite && (
                <div className="flex flex-wrap items-center justify-between gap-2 rounded-xl border border-line p-3">
                  <div className="text-[13px]"><div className="text-[11px] font-bold uppercase tracking-wide text-muted-foreground">{t("Delete device")}</div>
                    <div className="text-xs text-muted-foreground">{t(isFirst ? "Removes the device with its sensors, binding history, operations, events and calibrations. It cannot be undone." : "A bound device cannot be deleted: unbind it first.")}</div></div>
                  <Btn size="sm" variant="danger" disabled={pending || isBusy || !isFirst} onClick={() => { setWhy(""); setModal("delete"); }}>{t("Delete…")}</Btn>
                </div>
              )}
              {isBusy && <p className="-mt-2 text-xs text-muted-foreground">{t("One operation at a time: check, calibration and firmware update are disabled until the active one finishes.")}</p>}
              {dv.firmwareCard && (
                <div className={cx("rounded-xl border p-3", firmwareCardLook(dv.firmwareCard.tone))}>
                  <div className="flex items-center justify-between gap-2"><b className="text-[13px]">{dv.firmwareCard.title}</b><Badge tone={dv.firmwareCard.tone}>{dv.firmwareCard.status}</Badge></div>
                  {dv.firmwareCard.tone === "primary" && <div className="mt-2 h-1.5 rounded-full bg-primary" />}
                  <p className="mt-1 text-[11px] text-muted-foreground">{dv.firmwareCard.text}</p>
                </div>
              )}
              <div>
                <h3 className="text-[15px] font-bold">{t("Sensors ({n}) · from the unit’s capability", { n: dv.sensors.length })}</h3>
                {dv.sensors.length === 0 ? <p className="text-[13px] text-muted-foreground">{t("This device has no sensors.")}</p> : (
                  <ScrollX className="mt-2 rounded-xl border border-line"><table className="w-full min-w-[420px] text-[13px]">
                    <thead className="bg-surface2 text-left text-[11px] uppercase text-muted-foreground"><tr><th className="px-3 py-2">{t("Metric")}</th><th>{t("Unit")}</th><th>{t("Stale after")}</th><th>{t("Last calibrated")}</th><td /></tr></thead>
                    <tbody>{dv.sensors.map((x) => (
                      <tr key={x.id} className="border-t border-line"><td className="px-3 py-2 font-semibold">{x.metric}</td><td>{x.unit}</td><td>{x.stale}</td><td className="text-xs">{x.calibrated}</td>
                        <td className="pr-3 text-right">{live.canWrite && <Btn size="sm" disabled={pending || isBusy} onClick={() => { calibrateOf(x.id); }}>{t("Calibrate…")}</Btn>}</td></tr>
                    ))}</tbody>
                  </table></ScrollX>
                )}
              </div>
              <div>
                <h3 className="text-[15px] font-bold">{t("Operation history")}</h3>
                {dv.operations.length === 0 ? <p className="text-[13px] text-muted-foreground">{t("No operations yet.")}</p> : (
                  <ScrollX className="mt-2 rounded-xl border border-line"><table className="w-full min-w-[420px] text-[13px]">
                    <thead className="bg-surface2 text-left text-[11px] uppercase text-muted-foreground"><tr><th className="px-3 py-2">{t("Kind")}</th><th>{t("Started")}</th><th>{t("Detail")}</th><th>{t("Status")}</th></tr></thead>
                    <tbody>{dv.operations.map((o) => (
                      <tr key={o.id} className="border-t border-line"><td className="px-3 py-2 font-semibold">{o.kind}</td><td className="text-xs">{o.time}</td><td className="text-xs">{o.detail}</td>
                        <td className="pr-3"><Badge tone={operationTone(o.status)}>{o.statusText}</Badge></td></tr>
                    ))}</tbody>
                  </table></ScrollX>
                )}
              </div>
              <div>
                <h3 className="text-[15px] font-bold">{t("Calibration history")}</h3>
                <div className="mt-2 rounded-xl border border-dashed border-line px-3 py-2 text-xs text-muted-foreground">{dv.calibrations.length === 0 ? t("No calibrations recorded.") : dv.calibrations.map((c, k) => <div key={k}>{c}</div>)}</div>
              </div>
              <div>
                <div className="mb-2 flex items-baseline gap-2"><h3 className="text-[15px] font-bold">{t("Device events")}</h3><span className="text-xs text-muted-foreground">{t("connection, power and tamper are separate events")}</span></div>
                {dv.events.length === 0 ? <p className="text-[13px] text-muted-foreground">{t("No device events.")}</p> : (
                  <ScrollX className="rounded-xl border border-line"><table className="w-full min-w-[560px] text-[13px]">
                    <thead className="bg-surface2 text-left text-[11px] uppercase text-muted-foreground"><tr><th className="px-3 py-2">{t("Time")}</th><th>{t("Event")}</th><th>{t("Detail")}</th><th>{t("Recovery / response")}</th></tr></thead>
                    <tbody>{dv.events.map((e) => (
                      <tr key={e.id} className="border-t border-line align-top">
                        <td className="px-3 py-2 text-xs text-muted-foreground">{e.time}</td><td className="py-2"><Badge tone={e.tone === "muted" ? "muted" : e.tone}>{e.type}</Badge></td>
                        <td className="py-2">{e.detail}{e.notes.map((n) => <div key={n} className="flex items-center gap-1 text-[11px] text-muted-foreground"><Icon name="edit" />{n}</div>)}</td>
                        <td className="py-2 pr-3 text-xs">{e.recovery}</td>
                      </tr>
                    ))}</tbody>
                  </table></ScrollX>
                )}
              </div>
            </div>
          </Card>
        ) : <Card title={t("Device")}><EmptyState title={t("No device selected")}>{t("Choose a device.")}</EmptyState></Card>}
      </div>
      <Modal open={modal === "register"} onClose={close} title={t("Register device")} footer={<><Btn onClick={close}>{t("Cancel")}</Btn><Btn variant="primary" disabled={pending} onClick={register}>{t("Register")}</Btn></>}>
        <Field label={t("Serial")} error={tried && !reg.serial.trim() ? t("Serial is required") : undefined}><Input value={reg.serial} onChange={(e) => { setReg({ ...reg, serial: e.target.value }); }} placeholder="AC-DEMO-0006" /></Field>
        <Field label={t("MQTT node ID (optional)")} hint={t("The node ID the device sends in its MQTT topics. Once the device is bound, its readings and commands use it.")} error={tried && mqttNodeValue(reg.node) === undefined ? t("Use 1–64 letters, digits, hyphens or underscores") : undefined}><Input value={reg.node} onChange={(e) => { setReg({ ...reg, node: e.target.value }); }} placeholder="node_3" /></Field>
        <Field label={t("Planned unit")} hint={t("Registering reserves the device for this unit; Bind connects it once it is installed.")} error={tried && !reg.unitId ? t("Choose a unit") : undefined}><Select value={reg.unitId} onChange={(e) => { setReg({ ...reg, unitId: e.target.value }); }}><option value="">{t("Select…")}</option>{live.units.map((u) => <option key={u.id} value={u.id}>{u.label}</option>)}</Select></Field>
        <div><p className="mb-1 text-[13px] font-semibold">{t("Sensors")}</p><p className="mb-2 text-xs text-muted-foreground">{t("Each must be defined in the unit model’s capability; none is allowed.")}</p><div className="flex flex-wrap gap-x-5 gap-y-1.5">{metrics.map((m) => <Check key={m} label={`${m} (${metricUnit[m]})`} checked={reg.sensors.includes(m)} onChange={(on) => { setReg({ ...reg, sensors: on ? [...reg.sensors, m] : reg.sensors.filter((x) => x !== m) }); }} />)}</div></div>
      </Modal>
      <Modal open={modal === "node"} onClose={close} title={t("MQTT node ID")} footer={<><Btn onClick={close}>{t("Cancel")}</Btn><Btn variant="primary" disabled={pending} onClick={saveNode}>{t("Save")}</Btn></>}>
        <Field label={t("Node ID")} hint={t("Leave it empty to stop taking this device's readings in; another device cannot use the same ID.")} error={tried && mqttNodeValue(node) === undefined ? t("Use 1–64 letters, digits, hyphens or underscores") : undefined}><Input value={node} onChange={(e) => { setNode(e.target.value); }} placeholder="node_3" /></Field>
      </Modal>
      <Modal open={modal === "bind"} onClose={close} title={t(isFirst ? "Bind device" : "Rebind device")} footer={<><Btn onClick={close}>{t("Cancel")}</Btn><Btn variant="primary" disabled={pending} onClick={rebind}>{t(isFirst ? "Bind" : "Rebind")}</Btn></>}>
        <Field label={t(isFirst ? "Unit" : "New unit")} error={tried && !bind.unitId ? t("Choose a unit") : undefined}><Select value={bind.unitId} onChange={(e) => { setBind({ ...bind, unitId: e.target.value }); }}><option value="">{t("Select…")}</option>{live.units.filter((u) => u.id !== dv?.row.unitId).map((u) => <option key={u.id} value={u.id}>{u.label}</option>)}</Select></Field>
        <Field label={t("Reason")} error={tried && !bind.reason.trim() ? reasonMissing : undefined}><Textarea value={bind.reason} maxLength={1000} onChange={(e) => { setBind({ ...bind, reason: e.target.value }); }} /></Field>
        <p className="text-[11px] text-muted-foreground">{t(isFirst ? "The device's sensors start reporting for this unit." : "History stays with the previous binding; the device gets new sensor IDs on the new unit.")}</p>
      </Modal>
      <Modal open={modal === "unbind"} onClose={close} title={t("Unbind device")} footer={<><Btn onClick={close}>{t("Cancel")}</Btn><Btn variant="primary" disabled={pending} onClick={() => { leave("unbind"); }}>{t("Unbind")}</Btn></>}>
        <Field label={t("Reason")} error={tried && !why.trim() ? reasonMissing : undefined}><Textarea value={why} maxLength={1000} onChange={(e) => { setWhy(e.target.value); }} /></Field>
        <p className="text-[11px] text-muted-foreground">{t("The device leaves its unit and stays registered; history stays with the binding, and the unit has no device until you bind another.")}</p>
      </Modal>
      <Modal open={modal === "delete"} onClose={close} title={t("Delete device")} footer={<><Btn onClick={close}>{t("Cancel")}</Btn><Btn variant="danger" disabled={pending} onClick={() => { leave("delete"); }}>{t("Delete")}</Btn></>}>
        <p className="text-[13px]">{t("Delete {serial}? Its sensors, binding history, operations, events and calibrations are removed with it.", { serial: dv?.row.serial ?? "" })}</p>
        <Field label={t("Reason")} error={tried && !why.trim() ? reasonMissing : undefined}><Textarea value={why} maxLength={1000} onChange={(e) => { setWhy(e.target.value); }} /></Field>
      </Modal>
      <Modal open={modal === "calibrate"} onClose={close} title={t("Calibrate sensor")} footer={<><Btn onClick={close}>{t("Cancel")}</Btn><Btn variant="primary" disabled={pending} onClick={calibrate}>{t("Record")}</Btn></>}>
        <Field label={t("Sensor")}><Select value={cal.sensorId} onChange={(e) => { setCal({ ...cal, sensorId: e.target.value }); }}>{dv?.detail.sensors.map((s) => <option key={s.id} value={s.id}>{metricText(s.metric, t)} ({s.unit})</option>)}</Select></Field>
        <div className="grid-fluid" style={{ ["--min" as string]: "140px" }}><Field label={t("Reference")} error={tried && (cal.reference.trim() === "" || !Number.isFinite(Number(cal.reference))) ? t("Enter a number") : undefined}><Input type="number" value={cal.reference} onChange={(e) => { setCal({ ...cal, reference: e.target.value }); }} /></Field><Field label={t("Measured")} error={tried && (cal.measured.trim() === "" || !Number.isFinite(Number(cal.measured))) ? t("Enter a number") : undefined}><Input type="number" value={cal.measured} onChange={(e) => { setCal({ ...cal, measured: e.target.value }); }} /></Field></div>
        <p className="text-[11px] text-muted-foreground">{t("Recorded at the current time as a demo calibration.")}</p>
      </Modal>
      <Modal open={modal === "firmware"} onClose={close} title={t("Update firmware")} footer={<><Btn onClick={close}>{t("Cancel")}</Btn><Btn variant="primary" disabled={pending || !fw} onClick={firmware}>{t("Queue update")}</Btn></>}>
        {dv?.firmware.length === 0 ? <p className="text-xs text-muted-foreground">{t("The unit model lists no firmware candidates.")}</p> : <Field label={t("Target version")} error={tried && !fw ? t("Choose a version") : undefined}><Select value={fw} onChange={(e) => { setFw(e.target.value); }}><option value="">{t("Select…")}</option>{dv?.firmware.map((v) => <option key={v} value={v} disabled={v === dv.detail.firmwareVersion}>{v === dv.detail.firmwareVersion ? t("{version} (current)", { version: v }) : v}</option>)}</Select></Field>}
        <p className="text-[11px] text-muted-foreground">{t("Only an online device accepts an update; a failure keeps the current version.")}</p>
      </Modal>
    </div>
  );
}

/** The registry's paging (IR723): how many of the matching devices this page shows, the first page and the next. */
function RegistryPaging({ shown, total, page }: Readonly<{ shown: number; total: number; page: NonNullable<DevicesLive["page"]> }>) {
  const t = useT();
  const nav = useUrlPatch();
  const { cursor, next } = page;
  return (
    <div className="mt-3 flex flex-wrap items-center justify-between gap-2 text-[11px] text-muted-foreground">
      <span>{t("{n} of {total}", { n: shown, total })}</span>
      <span className="flex items-center gap-3">
        {cursor && <Plain className="font-semibold text-primary" onClick={() => { nav({ cursor: null, deviceId: null }); }}><Icon name="chevronLeft" /> {t("First page")}</Plain>}
        {next && <Plain className="font-semibold text-primary" onClick={() => { nav({ cursor: next, deviceId: null }); }}>{t("Next")} <Icon name="chevronRight" /></Plain>}
      </span>
    </div>
  );
}

/** The registry's search box: it runs on the server (serial, MQTT node ID or unit name), so the URL takes the text once
typing pauses, from the first page and without a selection (IR723). */
function useServerSearch(searched = ""): [string, (v: string) => void] {
  const nav = useUrlPatch();
  const [q, setQ] = useState(searched);
  useEffect(() => {
    const isChanged = q.trim() !== searched;
    const timer = isChanged ? setTimeout(() => { nav({ q: q.trim() || null, cursor: null, deviceId: null }); }, 400) : undefined;
    return () => { clearTimeout(timer); };
  }, [q, searched, nav]);
  return [q, setQ];
}
