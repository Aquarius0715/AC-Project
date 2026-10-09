"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useEffect, useState } from "react";
import { Badge, Banner, Btn, Card, Check, ConnBadge, cx, EmptyState, Field, Input, ListRow, Modal, Page, Search, Select, Textarea } from "@ac/web/components/ui";
import { useAction } from "@ac/web/lib/useAction";
import { useUrlPatch } from "@ac/web/lib/useUrlPatch";
import { klTime, type ApiDeviceDetail, type Metric } from "@ac/web/lib/devices";
import { filterOf, openAlerts, type EventRow, type Filter, type OperationRow, type TechDeviceRow } from "@ac/web/lib/techDevices";
import type { ActionFailure } from "@ac/web/lib/actionMessage";
import { acknowledgeAlert, addResponseNote, bindDevice, calibrateSensor, checkDevice, registerDevice, updateFirmware } from "../actions";

export type TechDevicesLive = {
  rows: TechDeviceRow[]; canMaintain: boolean; canAcknowledge: boolean; events: boolean;
  register: { unitId: string; label: string; model: string; sensors: Metric[]; jobs: { id: string; label: string }[] }[];
  rebind: { unitId: string; label: string; jobId: string }[];
  device: null | {
    detail: ApiDeviceDetail; row: TechDeviceRow; jobId: string | null;
    tiles: { label: string; value: string; dot: string | null; sub: string; warn?: boolean }[];
    unit: { name: string; place: string; model: string } | null; firmware: string[];
    firmwareCard: { tone: "primary" | "crit" | "ok"; title: string; status: string; text: string } | null;
    operations: OperationRow[]; calibrations: string[];
    events: EventRow[] | null;
    /** The events page of a device whose unit the technician has no assignment on (devices.events FORBIDDEN, IR94). */
    eventsLocked: boolean;
  };
};

const refusal = (f: ActionFailure): string | null =>
  f.code === "OFFLINE" ? "OFFLINE — the device is not reachable; nothing was started (an update needs an online device)."
    : f.messageKey === "errors.duplicate_serial" || (f.code === "CONFLICT" && f.fieldErrors.serial) ? "CONFLICT — this serial is already registered."
    : f.code === "CONFLICT" ? "CONFLICT — another operation is running or the device changed; the page shows the latest state."
    : f.code === "FORBIDDEN" ? "FORBIDDEN — outside your assignment or work window, or without device permission." : null;
const fieldText: Record<string, string> = { "error.required": "Required", "error.length": "1–1000 characters", "error.invalid": "Not valid", "error.duplicate": "Already registered" };
const dot: Record<string, string> = { ok: "bg-ok", crit: "bg-crit", warn: "bg-warn", primary: "bg-primary", muted: "bg-line" };

/** IoT devices of the technician's assigned units (FR-T11) and one device's events (FR-T12): every write runs under the
 * unit's assigned job (IR94), one device operation at a time; events keep detection, evidence and response apart. */
export function TechDevicesView({ live }: { live: TechDevicesLive }) {
  const nav = useUrlPatch();
  const router = useRouter();
  const [pending, run] = useAction();
  const [filter, setFilter] = useState<Filter>("all");
  const [q, setQ] = useState("");
  const [modal, setModal] = useState<null | "register" | "rebind" | "calibrate" | "firmware" | "note">(null);
  const [errors, setErrors] = useState<Record<string, string>>({});
  const [refused, setRefused] = useState<string | null>(null);
  const [reg, setReg] = useState({ serial: "", unitId: live.register[0]?.unitId ?? "", sensors: [] as Metric[], jobId: live.register[0]?.jobs[0]?.id ?? "" });
  const [bind, setBind] = useState({ unitId: "", reason: "" });
  const [cal, setCal] = useState({ sensorId: "", reference: "", measured: "" });
  const [fw, setFw] = useState("");
  const [note, setNote] = useState({ eventId: "", text: "" });
  const dv = live.device;
  const counts = (["all", "online", "offline", "tamper"] as Filter[]).map((f) => ({ f, n: live.rows.filter(filterOf(f)).length }));
  const shown = live.rows.filter(filterOf(filter)).filter((r) => !q || `${r.serial} ${r.id} ${r.unit}`.toLowerCase().includes(q.toLowerCase()));
  const open = (id: string) => (live.events ? router.push(`/technician/devices/${id}`) : nav({ deviceId: id }));
  const close = () => { setModal(null); setErrors({}); };
  const failed = (f: ActionFailure) => {
    setErrors(Object.fromEntries(Object.entries(f.fieldErrors).map(([k, v]) => [k, fieldText[v] ?? v.replace(/^errors?\./, "").replace(/_/g, " ")])));
    setRefused(refusal(f));
  };
  const ok = (after?: () => void) => () => { setRefused(null); close(); after?.(); };
  const busy = !!dv?.detail.activeOperation;
  useEffect(() => { // an operation starts on the scheduler and finishes with the device's answer or at its deadline (IR67)
    if (!busy) return;
    const t = setInterval(() => router.refresh(), 4000);
    return () => clearInterval(t);
  }, [busy, router]);
  const canWrite = live.canMaintain && !!dv?.jobId;
  const regUnit = live.register.find((u) => u.unitId === reg.unitId);

  const register = () => {
    const e: Record<string, string> = {};
    if (!/^[A-Za-z0-9-]{3,64}$/.test(reg.serial.trim())) e.serial = "3–64 letters, digits or hyphens";
    if (!reg.unitId) e.unitId = "Choose a unit";
    if (!reg.jobId) e.jobId = "Choose the job";
    setErrors(e);
    if (Object.keys(e).length) return;
    run(() => registerDevice(reg.serial.trim(), reg.sensors, reg.unitId, reg.jobId), "Device registered and bound — run Check connection", ok(), failed);
  };
  const rebind = () => {
    const target = live.rebind.find((u) => u.unitId === bind.unitId);
    const e: Record<string, string> = {};
    if (!target) e.unitId = "Choose a unit";
    if (bind.reason.trim().length < 1 || bind.reason.trim().length > 1000) e.reason = "VALIDATION — a reason is required to rebind (1–1000 characters)";
    setErrors(e);
    if (Object.keys(e).length || !dv || !target) return;
    run(() => bindDevice(dv.detail.id, dv.detail.version, target.unitId, target.jobId, bind.reason), "Device rebound — new sensor IDs issued", ok(), failed);
  };
  const calibrate = () => {
    const s = dv?.detail.sensors.find((x) => x.id === cal.sensorId);
    const e: Record<string, string> = {};
    if (!s) e.sensorId = "Choose a sensor";
    if (cal.reference.trim() === "" || !Number.isFinite(+cal.reference)) e.referenceValue = "A number";
    if (cal.measured.trim() === "" || !Number.isFinite(+cal.measured)) e.measuredValue = "A number";
    setErrors(e);
    if (Object.keys(e).length || !dv || !s || !dv.jobId) return;
    run(() => calibrateSensor(dv.detail.id, dv.detail.version, s.id, s.metric, s.unit, +cal.reference, +cal.measured, dv.jobId!), "Calibration recorded — earlier readings unchanged", ok(), failed);
  };
  const firmware = () => {
    if (!dv || !fw || !dv.jobId) return setErrors({ firmwareVersion: "Choose a version" });
    run(() => updateFirmware(dv.detail.id, dv.detail.version, fw, dv.jobId!), `Firmware update to ${fw} requested`, ok(), failed);
  };
  const saveNote = () => {
    const ev = dv?.events?.find((x) => x.id === note.eventId);
    if (!ev || note.text.trim().length < 1 || note.text.trim().length > 1000) return setErrors({ responseNote: "A note of 1–1000 characters" });
    run(() => addResponseNote(dv!.detail.id, ev.id, ev.version, note.text), "Response note added — the device state is unchanged", ok(), failed);
  };

  return (
    <Page>
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div className="flex flex-wrap gap-2">{counts.map(({ f, n }) => (
          <button key={f} type="button" aria-pressed={filter === f} onClick={() => setFilter(f)}
            className={cx("flex items-center gap-1.5 rounded-full border px-3 py-1 text-[13px] font-semibold", filter === f ? "border-ink bg-ink text-white" : "border-line bg-surface")}>
            {f !== "all" && <span className={cx("h-2 w-2 rounded-full", f === "online" ? "bg-ok" : f === "offline" ? "bg-crit" : "bg-warn")} />}{f.charAt(0).toUpperCase() + f.slice(1)} <span className="text-xs opacity-70">{n}</span>
          </button>
        ))}</div>
        <div className="flex flex-wrap items-center gap-2"><div className="w-64"><Search placeholder="Search serial, device or unit…" value={q} onChange={setQ} /></div>
          {live.canMaintain && <Btn variant="primary" disabled={live.register.length === 0} title={live.register.length ? undefined : "Every assigned unit already has a device"} onClick={() => { setErrors({}); setModal("register"); }}>+ Register device</Btn>}</div>
      </div>
      {refused && <Banner tone="crit">{refused}</Banner>}
      <div className="split-rev">
        <Card title="Devices — assigned units" sub="your jobs only" className="self-start">
          {shown.length === 0 ? <EmptyState title="No devices">{q || filter !== "all" ? "No device matches." : "No device on your assigned units yet."}</EmptyState> : (
            <div className="flex flex-col gap-2">{shown.map((r) => (
              <ListRow key={r.id} selected={dv?.row.id === r.id} onClick={() => open(r.id)}>
                <div className="min-w-0 flex-1">
                  <div className="flex items-center justify-between gap-2"><b className="whitespace-nowrap text-[13px]">{r.serial}</b><span className="flex flex-wrap items-center justify-end gap-1"><ConnBadge s={r.conn} />{r.tamper && <Badge tone="warn">Tamper</Badge>}{r.connText && <span className="text-[10px] text-muted">{r.connText}</span>}</span></div>
                  <div className="truncate text-[11px] text-muted">{r.id.slice(0, 8)}</div>
                  <div className="flex justify-between gap-2 text-[11px] text-muted"><span className="truncate">{r.unit}</span><span>fw {r.fw}</span></div>
                </div>
              </ListRow>
            ))}</div>
          )}
        </Card>
        {!dv ? <Card title="Device"><EmptyState title="No device selected">Choose a device from the list.</EmptyState></Card> : (
          <Card>
            <div className="flex flex-col gap-4">
              <div className="flex flex-wrap items-start justify-between gap-2">
                <div><h2 className="text-lg font-bold">{dv.row.serial}</h2><p className="text-xs text-muted">{dv.detail.id.slice(0, 8)} · {dv.row.unit}{dv.jobId ? ` · job ${dv.jobId.slice(0, 8)} (your assignment)` : " · no assigned job on this unit"}</p></div>
                {live.events ? <Link href={`/technician/devices?deviceId=${dv.detail.id}`} className="rounded-control border border-line px-3 py-1.5 text-xs font-semibold">← Device detail</Link>
                  : <Link href={`/technician/devices/${dv.detail.id}`} className="rounded-control border border-line px-3 py-1.5 text-xs font-semibold">Device events →</Link>}
              </div>
              <div className="grid-fluid" style={{ ["--min" as string]: "150px" }}>
                {dv.tiles.map((t) => (
                  <div key={t.label} className={cx("rounded-xl border p-3", t.warn ? "border-[#fdba74] bg-warn-soft/50" : "border-line")}>
                    <div className="text-[11px] text-muted">{t.label}</div>
                    <div className="flex items-center gap-1.5 text-[15px] font-bold">{t.dot && <span className={cx("h-2 w-2 rounded-full", dot[t.dot])} />}<span className={t.warn ? "text-warn" : ""}>{t.value}</span></div>
                    <div className="text-[10px] text-muted">{t.sub}</div>
                  </div>
                ))}
              </div>
              <div className="flex flex-wrap items-center justify-between gap-2 rounded-xl border border-line p-3">
                <div className="text-[13px]"><div className="text-[11px] font-bold tracking-wide text-muted">BOUND TO UNIT</div>
                  {dv.unit ? <><b>{dv.unit.name}</b><div className="text-xs text-muted">{dv.unit.place} · model {dv.unit.model}{dv.jobId ? ` · job ${dv.jobId.slice(0, 8)}` : ""}</div></> : <b>{dv.row.unit}</b>}</div>
                {live.canMaintain && <Btn size="sm" disabled={pending || !canWrite} onClick={() => { setBind({ unitId: "", reason: "" }); setErrors({}); setModal("rebind"); }}>Rebind…</Btn>}
              </div>
              {live.canMaintain && (
                <div className="grid-fluid" style={{ ["--min" as string]: "200px" }}>
                  {[
                    { label: "Check connection", sub: "ping the device", act: () => run(() => checkDevice(dv.detail.id, dv.detail.version, dv.jobId!), "Connection check requested", ok(), failed), off: false },
                    { label: "Calibrate sensor", sub: "reference vs measured", act: () => { setCal({ sensorId: dv.detail.sensors[0]?.id ?? "", reference: "", measured: "" }); setErrors({}); setModal("calibrate"); }, off: dv.detail.sensors.length === 0 },
                    { label: "Update firmware", sub: dv.detail.activeOperation?.kind === "firmware" ? "running…" : dv.firmware.length ? `${dv.firmware.join(", ")} available` : "no newer candidate", act: () => { setFw(dv.firmware[dv.firmware.length - 1] ?? ""); setErrors({}); setModal("firmware"); }, off: dv.firmware.length === 0 },
                  ].map((a) => (
                    <button key={a.label} type="button" disabled={pending || busy || !canWrite || a.off} onClick={a.act} className="rounded-xl border border-line px-3 py-2 text-left hover:bg-surface2 disabled:cursor-not-allowed disabled:opacity-50">
                      <div className="text-[13px] font-semibold">{a.label}</div><div className="text-[11px] text-muted">{a.sub}</div>
                    </button>
                  ))}
                </div>
              )}
              {live.canMaintain && !dv.jobId && <p className="-mt-2 text-xs text-muted">Device work runs under your assigned job on this unit — there is none now, so the device is read-only.</p>}
              {busy && <p className="-mt-2 text-xs text-muted">One device operation at a time ({dv.detail.activeOperation?.kind} {dv.detail.activeOperation?.status}) — the others wait; control commands are locked during an update (D05).</p>}
              {dv.firmwareCard && (
                <div className={cx("rounded-xl border p-3", dv.firmwareCard.tone === "crit" ? "border-crit bg-crit-soft/40" : dv.firmwareCard.tone === "ok" ? "border-[#86efac] bg-ok-soft/40" : "border-primary bg-primary-soft/40")}>
                  <div className="flex items-center justify-between gap-2"><b className="text-[13px]">{dv.firmwareCard.title}</b><Badge tone={dv.firmwareCard.tone}>{dv.firmwareCard.status}</Badge></div>
                  {dv.firmwareCard.tone === "primary" && <div className="mt-2 h-1.5 rounded-full bg-primary" />}
                  <p className="mt-1 text-[11px] text-muted">{dv.firmwareCard.text}</p>
                </div>
              )}
              {live.events && dv.eventsLocked ? (
                <Banner>Device events open while you have an assignment on {dv.unit?.name ?? "this unit"} (IR94). Ask HQ or your coordinator if you need them now.</Banner>
              ) : dv.events ? (
                <div>
                  <div className="mb-2 flex items-baseline gap-2"><b className="text-[15px]">Device events</b><span className="text-xs text-muted">connection, power and tamper are separate events</span></div>
                  {dv.events.length === 0 ? <p className="text-[13px] text-muted">No device events.</p> : (
                    <div className="scroll-x rounded-xl border border-line"><table className="w-full min-w-[560px] text-[13px]">
                      <thead className="bg-surface2 text-left text-[11px] uppercase text-muted"><tr><th className="px-3 py-2">Time</th><th>Event</th><th>Detail</th><th>Recovery / response</th></tr></thead>
                      <tbody>{dv.events.map((e) => (
                        <tr key={e.id} className="border-t border-line align-top">
                          <td className="px-3 py-2 text-xs text-muted">{e.time}</td><td className="py-2"><Badge tone={e.tone === "muted" ? "muted" : e.tone}>{e.type}</Badge></td>
                          <td className="py-2">{e.detail}{e.notes.map((n) => <div key={n} className="text-[11px] text-muted">✎ {n}</div>)}</td>
                          <td className="py-2 pr-3 text-xs">{e.recovery}</td>
                        </tr>
                      ))}</tbody>
                    </table></div>
                  )}
                  {dv.events.length > 0 && (live.canMaintain || live.canAcknowledge) && (
                    <div className="mt-2 flex flex-wrap items-center gap-3">
                      {live.canMaintain && <Btn size="sm" disabled={pending || !canWrite} onClick={() => { setNote({ eventId: dv.events![0].id, text: "" }); setErrors({}); setModal("note"); }}>Add response note…</Btn>}
                      {live.canAcknowledge && openAlerts(dv.events).map((a) => (
                        <button key={a.id} type="button" disabled={pending} className="text-[13px] font-semibold text-primary disabled:opacity-50"
                          onClick={() => run(() => acknowledgeAlert(a.id, a.version), "Alert acknowledged — the tamper state stays until the device reports it clear", ok(), failed)}>Acknowledge {a.type} alert {a.id.slice(0, 8)} →</button>
                      ))}
                    </div>
                  )}
                </div>
              ) : (
                <>
                  <div>
                    <b className="text-[15px]">Sensors ({dv.detail.sensors.length}) · from the unit’s capability</b>
                    {dv.detail.sensors.length === 0 ? <p className="text-[13px] text-muted">This device has no sensors.</p> : (
                      <div className="scroll-x mt-2 rounded-xl border border-line"><table className="w-full min-w-[420px] text-[13px]">
                        <thead className="bg-surface2 text-left text-[11px] uppercase text-muted"><tr><th className="px-3 py-2">Metric</th><th>Unit</th><th>Stale after</th><th>Last calibrated</th><th /></tr></thead>
                        <tbody>{dv.detail.sensors.map((s) => (
                          <tr key={s.id} className="border-t border-line"><td className="px-3 py-2 font-semibold">{s.metric}</td><td>{s.unit}</td><td>{s.staleAfterSeconds} s</td><td className="text-xs">{s.calibratedAt ? klTime(s.calibratedAt) : "Never"}</td>
                            <td className="pr-3 text-right">{live.canMaintain && <button type="button" disabled={pending || busy || !canWrite} className="text-xs font-semibold text-primary disabled:opacity-50" onClick={() => { setCal({ sensorId: s.id, reference: "", measured: "" }); setErrors({}); setModal("calibrate"); }}>Calibrate →</button>}</td></tr>
                        ))}</tbody>
                      </table></div>
                    )}
                  </div>
                  <div>
                    <b className="text-[15px]">Operation history</b>
                    {dv.operations.length === 0 ? <p className="text-[13px] text-muted">{live.canMaintain && dv.jobId ? "No operations yet." : "Shown for your assigned job on this unit."}</p> : (
                      <div className="scroll-x mt-2 rounded-xl border border-line"><table className="w-full min-w-[420px] text-[13px]">
                        <thead className="bg-surface2 text-left text-[11px] uppercase text-muted"><tr><th className="px-3 py-2">Kind</th><th>Started</th><th>Detail</th><th>Status</th></tr></thead>
                        <tbody>{dv.operations.map((o) => (
                          <tr key={o.id} className="border-t border-line"><td className="px-3 py-2 font-semibold">{o.kind}</td><td className="text-xs">{o.time}</td><td className="text-xs">{o.detail}</td>
                            <td className="pr-3"><Badge tone={o.status === "succeeded" ? "ok" : o.status === "failed" ? "crit" : "primary"}>{o.status.charAt(0).toUpperCase() + o.status.slice(1)}</Badge></td></tr>
                        ))}</tbody>
                      </table></div>
                    )}
                  </div>
                  <div>
                    <b className="text-[15px]">Calibration history</b>
                    <div className="mt-2 rounded-xl border border-dashed border-line px-3 py-2 text-xs text-muted">{dv.calibrations.length === 0 ? "No calibrations recorded." : dv.calibrations.map((c, i) => <div key={i}>{c}</div>)}</div>
                  </div>
                </>
              )}
            </div>
          </Card>
        )}
      </div>
      <Modal open={modal === "register"} onClose={close} title="Register device" footer={<><Btn onClick={close}>Cancel</Btn><Btn variant="primary" disabled={pending} onClick={register}>Register & bind</Btn></>}>
        <p className="-mt-1 text-xs text-muted">Adds an IoT device to one of your assigned units and binds it in one step.</p>
        <Field label="Serial number · required" hint={`Trimmed and upper-cased before the uniqueness check${reg.serial.trim() ? ` → ${reg.serial.trim().toUpperCase()}` : ""}.`} error={errors.serial}><Input value={reg.serial} maxLength={64} placeholder="AC-002" onChange={(e) => setReg({ ...reg, serial: e.target.value })} /></Field>
        <Field label="Bind to unit · required" hint={`Only units of your active assignments without a device are listed.${regUnit ? ` Model: ${regUnit.model}.` : ""}`} error={errors.unitId}>
          <Select value={reg.unitId} onChange={(e) => { const u = live.register.find((x) => x.unitId === e.target.value); setReg({ ...reg, unitId: e.target.value, sensors: [], jobId: u?.jobs[0]?.id ?? "" }); }}>
            {live.register.length === 0 && <option value="">No unit without a device</option>}{live.register.map((u) => <option key={u.unitId} value={u.unitId}>{u.label}</option>)}
          </Select>
        </Field>
        <div><p className="mb-1 flex justify-between text-xs font-semibold"><span>Sensor types</span><span className="font-normal text-muted">Only metrics in the unit’s capability</span></p>
          <div className="flex flex-wrap gap-2">{(regUnit?.sensors ?? []).map((m) => <Check key={m} label={m} checked={reg.sensors.includes(m)} onChange={(v) => setReg({ ...reg, sensors: v ? [...reg.sensors, m] : reg.sensors.filter((x) => x !== m) })} />)}{regUnit && regUnit.sensors.length === 0 && <span className="text-xs text-muted">The model defines no sensors.</span>}</div>
          {errors.sensorTypes && <p className="mt-1 text-xs text-crit">✕ {errors.sensorTypes}</p>}</div>
        <Field label="Job · required" error={errors.jobId}><Select value={reg.jobId} onChange={(e) => setReg({ ...reg, jobId: e.target.value })}>{(regUnit?.jobs ?? []).map((j) => <option key={j.id} value={j.id}>{j.label}</option>)}</Select></Field>
        <Banner>After registering, run “Check connection”. The device stays Unknown until its first heartbeat.</Banner>
      </Modal>
      <Modal open={modal === "rebind"} onClose={close} title="Rebind device" footer={<><Btn onClick={close}>Cancel</Btn><Btn variant="primary" disabled={pending} onClick={rebind}>Rebind</Btn></>}>
        <Field label="New unit" error={errors.unitId}><Select value={bind.unitId} onChange={(e) => setBind({ ...bind, unitId: e.target.value })}><option value="">Select…</option>{live.rebind.filter((u) => u.unitId !== dv?.row.unitId).map((u) => <option key={u.unitId} value={u.unitId}>{u.label}</option>)}</Select></Field>
        <Field label="Reason (required)" error={errors.reason}><Textarea value={bind.reason} maxLength={1000} onChange={(e) => setBind({ ...bind, reason: e.target.value })} /></Field>
        <p className="text-[11px] text-muted">History stays with the previous binding; the device gets new sensor IDs on the new unit (SR24).</p>
      </Modal>
      <Modal open={modal === "calibrate"} onClose={close} title="Calibrate sensor" footer={<><Btn onClick={close}>Cancel</Btn><Btn variant="primary" disabled={pending} onClick={calibrate}>Record calibration</Btn></>}>
        <Field label="Sensor" error={errors.sensorId ?? errors.unit ?? errors.metric}><Select value={cal.sensorId} onChange={(e) => setCal({ ...cal, sensorId: e.target.value })}>{dv?.detail.sensors.map((s) => <option key={s.id} value={s.id}>{s.metric} ({s.unit})</option>)}</Select></Field>
        <div className="grid-fluid" style={{ ["--min" as string]: "140px" }}>
          <Field label="Reference value" error={errors.referenceValue}><Input inputMode="decimal" value={cal.reference} onChange={(e) => setCal({ ...cal, reference: e.target.value })} /></Field>
          <Field label="Measured value" error={errors.measuredValue}><Input inputMode="decimal" value={cal.measured} onChange={(e) => setCal({ ...cal, measured: e.target.value })} /></Field>
        </div>
        <p className="text-[11px] text-muted">Recorded now as a demo calibration in the sensor’s unit; earlier readings are not rewritten.</p>
      </Modal>
      <Modal open={modal === "firmware"} onClose={close} title="Update firmware" footer={<><Btn onClick={close}>Cancel</Btn><Btn variant="primary" disabled={pending || !fw} onClick={firmware}>Start update</Btn></>}>
        <Field label="Target version" error={errors.firmwareVersion}><Select value={fw} onChange={(e) => setFw(e.target.value)}>{dv?.firmware.map((v) => <option key={v} value={v}>{v}</option>)}</Select></Field>
        <p className="text-[11px] text-muted">Only supported versions from the model are offered. Only an online device accepts an update; a failure keeps {dv?.detail.firmwareVersion}.</p>
      </Modal>
      <Modal open={modal === "note"} onClose={close} title="Add response note" footer={<><Btn onClick={close}>Cancel</Btn><Btn variant="primary" disabled={pending} onClick={saveNote}>Add note</Btn></>}>
        <Field label="Event"><Select value={note.eventId} onChange={(e) => setNote({ ...note, eventId: e.target.value })}>{dv?.events?.map((e) => <option key={e.id} value={e.id}>{e.time} · {e.type}</option>)}</Select></Field>
        <Field label="What you checked or did" error={errors.responseNote}><Textarea value={note.text} maxLength={1000} onChange={(e) => setNote({ ...note, text: e.target.value })} /></Field>
        <p className="text-[11px] text-muted">A note does not change the device’s physical state or clear its alert.</p>
      </Modal>
    </Page>
  );
}
