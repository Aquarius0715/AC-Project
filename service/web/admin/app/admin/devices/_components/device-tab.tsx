"use client";

import { useState } from "react";
import { Badge, Btn, Card, Check, ConnBadge, cx, EmptyState, Field, Input, ListRow, Modal, Search, Select, Textarea } from "@ac/web/components/ui";
import { useT } from "@ac/web/components/I18n";
import { useAction } from "@ac/web/lib/useAction";
import { useUrlPatch } from "@ac/web/lib/useUrlPatch";
import { metrics, metricUnit, type Metric } from "@ac/web/lib/devices";
import { filterOf, HQ_FILTERS, type Filter } from "@ac/web/lib/techDevices";
import { bindDevice, calibrateSensor, checkDevice, registerDevice, updateFirmware } from "../actions";
import type { DevicesLive } from "./devices-view";

const dot: Record<string, string> = { ok: "bg-ok", crit: "bg-crit", warn: "bg-warn", primary: "bg-primary", muted: "bg-line" };

/** IoT devices (DD-A04, Figma Admin 246:2): the list sorted by serial with the state chips and the search, and the
 * deviceId detail as the technician's device screen draws it — tiles, the bound unit, the actions, the firmware card,
 * sensors with Calibrate →, the operation and calibration history and the events. Register, bind, connection check,
 * calibration and firmware update are Server Actions. Texts in the display language; times in the user's display time
 * zone; metric and evidence codes stay codes (IR295, IR319). */
export function DeviceTab({ live }: { live: DevicesLive }) {
  const t = useT();
  const nav = useUrlPatch();
  const [pending, run] = useAction();
  const [filter, setFilter] = useState<Filter>("all");
  const [q, setQ] = useState("");
  const [modal, setModal] = useState<null | "register" | "bind" | "calibrate" | "firmware">(null);
  const [tried, setTried] = useState(false);
  const [reg, setReg] = useState({ serial: "", unitId: "", sensors: [] as Metric[] });
  const [bind, setBind] = useState({ unitId: "", reason: "" });
  const [cal, setCal] = useState({ sensorId: "", reference: "", measured: "" });
  const [fw, setFw] = useState("");
  const rows = live.rows ?? [];
  const counts = HQ_FILTERS.map(({ id: f, label }) => ({ f, label: t(label), n: rows.filter(filterOf(f)).length }));
  const shown = rows.filter(filterOf(filter)).filter((x) => !q || `${x.serial} ${x.id} ${x.unit}`.toLowerCase().includes(q.toLowerCase()));
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
    run(() => calibrateSensor(dv.detail.id, dv.detail.version, s.id, s.metric, s.unit, +cal.reference, +cal.measured), t("Calibration recorded (demo)"), close);
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
          <button key={f} type="button" aria-pressed={filter === f} onClick={() => setFilter(f)}
            className={cx("flex items-center gap-1.5 rounded-full border px-3 py-1 text-[13px] font-semibold", filter === f ? "border-ink bg-ink text-white" : "border-line bg-surface")}>
            {f !== "all" && f !== "nosensors" && <span className={cx("h-2 w-2 rounded-full", f === "online" ? "bg-ok" : f === "offline" ? "bg-crit" : "bg-warn")} />}{label} <span className="text-xs opacity-70">{n}</span>
          </button>
        ))}</div>
        <div className="flex flex-wrap items-center gap-2"><div className="w-64"><Search placeholder={t("Search serial, device or unit…")} value={q} onChange={setQ} /></div>
          {live.canWrite && <Btn variant="primary" onClick={() => setModal("register")}>{t("+ Register device")}</Btn>}</div>
      </div>
      <div className="split-rev">
        <Card title={t("Devices")} sub={t("sorted by serial")} className="self-start">
          {shown.length === 0 ? <EmptyState title={t("No devices")}>{t(q || filter !== "all" ? "No device matches." : "Register the first device.")}</EmptyState> : (
            <div className="flex flex-col gap-2">{shown.map((x) => (
              <ListRow key={x.id} selected={dv?.row.id === x.id} onClick={() => nav({ deviceId: x.id })}>
                <div className="min-w-0 flex-1">
                  <div className="flex items-center justify-between gap-2"><b className="whitespace-nowrap text-[13px]">{x.serial}</b><span className="flex flex-wrap items-center justify-end gap-1"><ConnBadge s={x.conn} />{x.tamper && <Badge tone="warn">{t("Tamper")}</Badge>}{x.connText && <span className="text-[10px] text-muted">{x.connText}</span>}</span></div>
                  <div className="flex justify-between gap-2 text-[11px] text-muted"><span className="truncate">{x.unit}</span><span>{x.sensors ? t("fw {version}", { version: x.fw }) : t("fw {version} · no sensors", { version: x.fw })}</span></div>
                </div>
              </ListRow>
            ))}</div>
          )}
        </Card>
        {!dv ? <Card title={t("Device")}><EmptyState title={t("No device selected")}>{t("Choose a device.")}</EmptyState></Card> : (
          <Card>
            <div className="flex flex-col gap-4">
              <div><h2 className="text-lg font-bold">{dv.row.serial}</h2><p className="text-xs text-muted">{dv.detail.id.slice(0, 8)} · {dv.row.unit} · {t("Device v{v}", { v: dv.detail.version })}</p></div>
              <div className="grid-fluid" style={{ ["--min" as string]: "150px" }}>
                {dv.tiles.map((x) => (
                  <div key={x.label} className={cx("rounded-xl border p-3", x.warn ? "border-[#fdba74] bg-warn-soft/50" : "border-line")}>
                    <div className="text-[11px] text-muted">{x.label}</div>
                    <div className="flex items-center gap-1.5 text-[15px] font-bold">{x.dot && <span className={cx("h-2 w-2 rounded-full", dot[x.dot])} />}<span className={x.warn ? "text-warn" : ""}>{x.value}</span></div>
                    <div className="text-[10px] text-muted">{x.sub}</div>
                  </div>
                ))}
              </div>
              <div className="flex flex-wrap items-center justify-between gap-2 rounded-xl border border-line p-3">
                <div className="text-[13px]"><div className="text-[11px] font-bold uppercase tracking-wide text-muted">{t("Bound to unit")}</div>
                  {dv.unit ? <><b>{dv.unit.name}</b><div className="text-xs text-muted">{dv.unit.place} · {t("model {model}", { model: dv.unit.model })}</div></> : <b>{dv.row.unit}</b>}</div>
                {live.canWrite && <Btn size="sm" disabled={pending} onClick={() => { setBind({ unitId: "", reason: "" }); setModal("bind"); }}>{t("Rebind…")}</Btn>}
              </div>
              {live.canWrite && (
                <div className="grid-fluid" style={{ ["--min" as string]: "200px" }}>
                  {[
                    { label: t("Check connection"), sub: t("ping the device"), act: () => run(() => checkDevice(dv.detail.id, dv.detail.version), t("Connection check queued")), off: false },
                    { label: t("Calibrate sensor"), sub: t("reference vs measured"), act: () => calibrateOf(dv.detail.sensors[0]?.id ?? ""), off: dv.detail.sensors.length === 0 },
                    { label: t("Update firmware"), sub: dv.detail.activeOperation?.kind === "firmware" ? t("running…") : dv.firmware.length ? t("{version} available", { version: dv.firmware.join(", ") }) : t("no newer candidate"), act: () => { setFw(dv.firmware[dv.firmware.length - 1] ?? ""); setModal("firmware"); }, off: dv.firmware.length === 0 },
                  ].map((a) => (
                    <button key={a.label} type="button" disabled={pending || busy || a.off} onClick={a.act} className="rounded-xl border border-line px-3 py-2 text-left hover:bg-surface2 disabled:cursor-not-allowed disabled:opacity-50">
                      <div className="text-[13px] font-semibold">{a.label}</div><div className="text-[11px] text-muted">{a.sub}</div>
                    </button>
                  ))}
                </div>
              )}
              {busy && <p className="-mt-2 text-xs text-muted">{t("One operation at a time: check, calibration and firmware update are disabled until the active one finishes.")}</p>}
              {dv.firmwareCard && (
                <div className={cx("rounded-xl border p-3", dv.firmwareCard.tone === "crit" ? "border-crit bg-crit-soft/40" : dv.firmwareCard.tone === "ok" ? "border-[#86efac] bg-ok-soft/40" : "border-primary bg-primary-soft/40")}>
                  <div className="flex items-center justify-between gap-2"><b className="text-[13px]">{dv.firmwareCard.title}</b><Badge tone={dv.firmwareCard.tone}>{dv.firmwareCard.status}</Badge></div>
                  {dv.firmwareCard.tone === "primary" && <div className="mt-2 h-1.5 rounded-full bg-primary" />}
                  <p className="mt-1 text-[11px] text-muted">{dv.firmwareCard.text}</p>
                </div>
              )}
              <div>
                <h3 className="text-[15px] font-bold">{t("Sensors ({n}) · from the unit’s capability", { n: dv.sensors.length })}</h3>
                {dv.sensors.length === 0 ? <p className="text-[13px] text-muted">{t("This device has no sensors.")}</p> : (
                  <div className="scroll-x mt-2 rounded-xl border border-line"><table className="w-full min-w-[420px] text-[13px]">
                    <thead className="bg-surface2 text-left text-[11px] uppercase text-muted"><tr><th className="px-3 py-2">{t("Metric")}</th><th>{t("Unit")}</th><th>{t("Stale after")}</th><th>{t("Last calibrated")}</th><th /></tr></thead>
                    <tbody>{dv.sensors.map((x) => (
                      <tr key={x.id} className="border-t border-line"><td className="px-3 py-2 font-semibold">{x.metric}</td><td>{x.unit}</td><td>{x.stale}</td><td className="text-xs">{x.calibrated}</td>
                        <td className="pr-3 text-right">{live.canWrite && <button type="button" disabled={pending || busy} className="text-xs font-semibold text-primary disabled:opacity-50" onClick={() => calibrateOf(x.id)}>{t("Calibrate →")}</button>}</td></tr>
                    ))}</tbody>
                  </table></div>
                )}
              </div>
              <div>
                <h3 className="text-[15px] font-bold">{t("Operation history")}</h3>
                {dv.operations.length === 0 ? <p className="text-[13px] text-muted">{t("No operations yet.")}</p> : (
                  <div className="scroll-x mt-2 rounded-xl border border-line"><table className="w-full min-w-[420px] text-[13px]">
                    <thead className="bg-surface2 text-left text-[11px] uppercase text-muted"><tr><th className="px-3 py-2">{t("Kind")}</th><th>{t("Started")}</th><th>{t("Detail")}</th><th>{t("Status")}</th></tr></thead>
                    <tbody>{dv.operations.map((o) => (
                      <tr key={o.id} className="border-t border-line"><td className="px-3 py-2 font-semibold">{o.kind}</td><td className="text-xs">{o.time}</td><td className="text-xs">{o.detail}</td>
                        <td className="pr-3"><Badge tone={o.status === "succeeded" ? "ok" : o.status === "failed" ? "crit" : "primary"}>{o.statusText}</Badge></td></tr>
                    ))}</tbody>
                  </table></div>
                )}
              </div>
              <div>
                <h3 className="text-[15px] font-bold">{t("Calibration history")}</h3>
                <div className="mt-2 rounded-xl border border-dashed border-line px-3 py-2 text-xs text-muted">{dv.calibrations.length === 0 ? t("No calibrations recorded.") : dv.calibrations.map((c, k) => <div key={k}>{c}</div>)}</div>
              </div>
              <div>
                <div className="mb-2 flex items-baseline gap-2"><h3 className="text-[15px] font-bold">{t("Device events")}</h3><span className="text-xs text-muted">{t("connection, power and tamper are separate events")}</span></div>
                {dv.events.length === 0 ? <p className="text-[13px] text-muted">{t("No device events.")}</p> : (
                  <div className="scroll-x rounded-xl border border-line"><table className="w-full min-w-[560px] text-[13px]">
                    <thead className="bg-surface2 text-left text-[11px] uppercase text-muted"><tr><th className="px-3 py-2">{t("Time")}</th><th>{t("Event")}</th><th>{t("Detail")}</th><th>{t("Recovery / response")}</th></tr></thead>
                    <tbody>{dv.events.map((e) => (
                      <tr key={e.id} className="border-t border-line align-top">
                        <td className="px-3 py-2 text-xs text-muted">{e.time}</td><td className="py-2"><Badge tone={e.tone === "muted" ? "muted" : e.tone}>{e.type}</Badge></td>
                        <td className="py-2">{e.detail}{e.notes.map((n) => <div key={n} className="text-[11px] text-muted">✎ {n}</div>)}</td>
                        <td className="py-2 pr-3 text-xs">{e.recovery}</td>
                      </tr>
                    ))}</tbody>
                  </table></div>
                )}
              </div>
            </div>
          </Card>
        )}
      </div>
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
