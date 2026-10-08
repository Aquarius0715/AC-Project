"use client";

import { useState } from "react";
import { Badge, Banner, Btn, Card, Check, EmptyState, Field, Input, ListRow, Modal, Select, SummaryList, Textarea, UtilBar } from "@ac/web/components/ui";
import { useAction } from "@ac/web/lib/useAction";
import { useUrlPatch } from "@ac/web/lib/useUrlPatch";
import { campaignErrors, klTime, type CampaignDraft, type CampaignRow } from "@ac/web/lib/devices";
import { controlCampaign, scheduleCampaign } from "../actions";
import type { DevicesLive } from "./devices-view";

const tone = (s: CampaignRow["state"]) => (s === "completed" ? "ok" : s === "running" ? "primary" : s === "paused" ? "warn" : s === "aborted" ? "crit" : "muted");

/** Firmware campaigns (DD-A20): the campaign list, the campaignId detail with wave progress and per-device results,
 * pause / resume / abort / retry, and scheduling a new campaign (start at least 24 hours ahead). */
export function FirmwareTab({ live }: { live: DevicesLive }) {
  const nav = useUrlPatch();
  const [pending, run] = useAction();
  const [modal, setModal] = useState<null | "new" | "abort">(null);
  const [tried, setTried] = useState(false);
  const [reason, setReason] = useState("");
  const now = new Date(live.now);
  const blank = (): CampaignDraft => ({ modelId: live.models[0]?.id ?? "", targetVersion: "", deviceIds: [], waves: "10, 50, 100", startLocal: "02:00", endLocal: "05:00", autoPause: "5", startAt: "" });
  const [d, setD] = useState<CampaignDraft>(blank);
  const model = live.models.find((m) => m.id === d.modelId);
  const modelUnits = new Set(live.units.filter((u) => u.modelId === d.modelId).map((u) => u.id));
  const modelDevices = live.devices.filter((x) => x.unitId && modelUnits.has(x.unitId));
  const errors = campaignErrors(d, now);
  const sel = live.campaign;
  const close = () => { setModal(null); setTried(false); };
  const schedule = () => {
    setTried(true);
    if (Object.keys(errors).length > 0) return;
    run(() => scheduleCampaign(d), "Campaign scheduled", (id) => { close(); setD(blank()); nav({ campaignId: id }); });
  };
  const control = (action: "pause" | "resume" | "abort" | "retry_device", deviceId?: string) =>
    sel && run(() => controlCampaign(sel.id, sel.version, action, deviceId, action === "abort" ? reason.trim() : undefined), action === "retry_device" ? "Device queued again" : `Campaign ${action === "pause" ? "paused" : action === "resume" ? "resumed" : "aborted"}`, close);
  const serial = new Map(live.devices.map((x) => [x.id, x.serial]));

  return (
    <div className="split-rev">
      <Card title="Campaigns" action={live.canWrite && <Btn size="sm" variant="primary" onClick={() => { setD(blank()); setModal("new"); }}>+ New campaign</Btn>} className="self-start">
        {(live.campaigns ?? []).length === 0 ? <EmptyState title="No campaigns">Schedule a firmware campaign for a model.</EmptyState> : <div className="flex flex-col gap-2">{live.campaigns!.map((c) => <ListRow key={c.id} selected={sel?.id === c.id} onClick={() => nav({ campaignId: c.id })}><div className="min-w-0 flex-1"><b className="text-[13px]">{c.model}</b><div className="text-[11px] text-muted">{c.versions} · {c.devices} device{c.devices === 1 ? "" : "s"} · starts {c.start}</div></div><Badge tone={tone(c.state)}>{c.state}</Badge></ListRow>)}</div>}
      </Card>
      {!sel ? <Card title="Campaign"><EmptyState title="Nothing selected">Choose or schedule a campaign.</EmptyState></Card> : (
        <Card title={`${sel.model} · ${sel.versions}`} sub={`Auto-pause if failures exceed ${sel.c.autoPauseFailurePercent} % within a wave · window ${sel.c.window.startLocal}–${sel.c.window.endLocal} · starts ${sel.start}`} action={live.canWrite && <span className="flex gap-2">{(sel.state === "running" || sel.state === "scheduled") && <Btn size="sm" disabled={pending} onClick={() => control("pause")}>Pause</Btn>}{sel.state === "paused" && <Btn size="sm" disabled={pending} onClick={() => control("resume")}>Resume</Btn>}{["scheduled", "running", "paused"].includes(sel.state) && <Btn size="sm" variant="danger" disabled={pending} onClick={() => { setReason(""); setModal("abort"); }}>Abort</Btn>}</span>}>
          <div className="flex flex-col gap-3">{sel.c.waves.map((w, i) => {
            const inWave = sel.c.results.filter((r) => r.waveIndex === i);
            const done = inWave.filter((r) => r.status === "succeeded").length;
            const pct = inWave.length ? Math.round((done * 100) / inWave.length) : 0;
            return <div key={w.label}><div className="mb-1 flex justify-between text-xs"><b>{w.label}</b><span className="text-muted">{done} / {inWave.length} succeeded</span></div><UtilBar pct={pct} tone={pct >= 95 ? "ok" : "primary"} /></div>;
          })}</div>
          <div className="mt-3"><SummaryList items={[["Succeeded / installing / pending", `${sel.c.progress.succeeded} / ${sel.c.progress.installing} / ${sel.c.progress.pending}`], ["Failed", `${sel.c.progress.failed} · failed devices keep their old version`], ["Skipped", `${sel.c.progress.skipped} · busy, offline or tampered`], ...(sel.c.reason ? [["Reason", sel.c.reason] as [string, string]] : [])]} /></div>
          <h3 className="mb-1 mt-4 text-[13px] font-bold">Devices</h3>
          <div className="flex flex-col gap-1">{sel.c.results.map((r) => <div key={r.deviceId} className="flex items-center justify-between gap-2 border-t border-line py-1.5 text-[13px]"><span>{serial.get(r.deviceId) ?? r.deviceId.slice(0, 8)} <span className="text-xs text-muted">wave {r.waveIndex + 1}{r.reasonKey ? ` · ${r.reasonKey}` : ""}</span></span><span className="flex items-center gap-2"><Badge tone={r.status === "succeeded" ? "ok" : r.status === "failed" ? "crit" : r.status === "skipped" ? "warn" : "muted"}>{r.status}</Badge>{live.canWrite && (r.status === "failed" || r.status === "skipped") && ["running", "paused"].includes(sel.state) && <Btn size="sm" disabled={pending} onClick={() => control("retry_device", r.deviceId)}>Retry</Btn>}</span></div>)}</div>
        </Card>
      )}
      <Modal open={modal === "new"} onClose={close} title="New firmware campaign" footer={<><Btn onClick={close}>Cancel</Btn><Btn variant="primary" disabled={pending} onClick={schedule}>Schedule</Btn></>}>
        <div className="grid-fluid" style={{ ["--min" as string]: "180px" }}>
          <Field label="Model" error={tried ? errors.modelId : undefined}><Select value={d.modelId} onChange={(e) => setD({ ...d, modelId: e.target.value, targetVersion: "", deviceIds: [] })}>{live.models.map((m) => <option key={m.id} value={m.id}>{m.name}</option>)}</Select></Field>
          <Field label="Target version" error={tried ? errors.targetVersion : undefined} hint="A firmware candidate of the model"><Select value={d.targetVersion} onChange={(e) => setD({ ...d, targetVersion: e.target.value })}><option value="">Select…</option>{model?.cap.firmwareCandidates.map((v) => <option key={v} value={v}>{v}</option>)}</Select></Field>
        </div>
        <div><p className="mb-1 text-[13px] font-semibold">Devices of this model</p>{modelDevices.length === 0 ? <p className="text-xs text-muted">No device is bound to a unit of this model.</p> : <div className="flex flex-col gap-1">{modelDevices.map((x) => <Check key={x.id} label={<span>{x.serial} <span className="text-xs text-muted">{x.unit} · {x.fw}{x.tamper ? " · tamper (skipped)" : ""}</span></span>} checked={d.deviceIds.includes(x.id)} onChange={(on) => setD({ ...d, deviceIds: on ? [...d.deviceIds, x.id] : d.deviceIds.filter((y) => y !== x.id) })} />)}</div>}{tried && errors.deviceIds && <p className="mt-1 text-xs text-crit">{errors.deviceIds}</p>}</div>
        <Field label="Waves (ascending %, ends at 100)" error={tried ? errors.waves : undefined}><Input value={d.waves} onChange={(e) => setD({ ...d, waves: e.target.value })} /></Field>
        <div className="grid-fluid" style={{ ["--min" as string]: "140px" }}>
          <Field label="Window start" error={tried ? errors.window : undefined}><Input type="time" value={d.startLocal} onChange={(e) => setD({ ...d, startLocal: e.target.value })} /></Field>
          <Field label="Window end"><Input type="time" value={d.endLocal} onChange={(e) => setD({ ...d, endLocal: e.target.value })} /></Field>
          <Field label="Auto-pause above (%)" error={tried ? errors.autoPause : undefined}><Input type="number" min="1" max="50" value={d.autoPause} onChange={(e) => setD({ ...d, autoPause: e.target.value })} /></Field>
        </div>
        <Field label="Start (Kuala Lumpur)" error={tried ? errors.startAt : undefined} hint={`Now ${klTime(live.now)} — at least 24 h ahead`}><Input type="datetime-local" value={d.startAt} onChange={(e) => setD({ ...d, startAt: e.target.value })} /></Field>
        <Banner>Devices with an active diagnostic run, firmware operation or open tamper are skipped with a reason.</Banner>
      </Modal>
      <Modal open={modal === "abort"} onClose={close} title="Abort campaign" footer={<><Btn onClick={close}>Cancel</Btn><Btn variant="danger" disabled={pending || !reason.trim()} onClick={() => control("abort")}>Abort</Btn></>}>
        <Field label="Reason"><Textarea value={reason} maxLength={1000} onChange={(e) => setReason(e.target.value)} /></Field>
        <p className="text-[11px] text-muted">Installed devices keep the new version; pending devices are not updated.</p>
      </Modal>
    </div>
  );
}
