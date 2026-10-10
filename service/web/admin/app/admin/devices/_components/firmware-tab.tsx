"use client";

import { useState } from "react";
import { Badge, Banner, Btn, Card, Check, EmptyState, Field, Input, ListRow, Modal, Select, SummaryList, Textarea, UtilBar } from "@ac/web/components/ui";
import { useI18n } from "@ac/web/components/I18n";
import { useAction } from "@ac/web/lib/useAction";
import { useUrlPatch } from "@ac/web/lib/useUrlPatch";
import { showTime } from "@ac/web/lib/i18n";
import { campaignErrors, campaignStart, type CampaignDraft, type CampaignRow } from "@ac/web/lib/devices";
import { controlCampaign, scheduleCampaign } from "../actions";
import type { DevicesLive } from "./devices-view";

const tone = (s: CampaignRow["state"]) => (s === "completed" ? "ok" : s === "running" ? "primary" : s === "paused" ? "warn" : s === "aborted" ? "crit" : "muted");
const stateWord: Record<CampaignRow["state"], string> = { scheduled: "scheduled", running: "running", paused: "paused", aborted: "aborted", completed: "completed" };
const resultWord: Record<string, string> = { pending: "pending", installing: "installing", succeeded: "succeeded", failed: "failed", skipped: "skipped" };

/** Firmware campaigns (DD-A20): the campaign list, the campaignId detail with wave progress and per-device results,
 * pause / resume / abort / retry, and scheduling a new campaign (start at least 24 hours ahead). Texts in the display
 * language; the start is typed and shown in the user's display time zone, the install window is the device's local
 * time (IR295). */
export function FirmwareTab({ live }: { live: DevicesLive }) {
  const i = useI18n(), { t } = i, zone = i.display.timeZone;
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
  const errors = campaignErrors(d, now, zone, t);
  const sel = live.campaign;
  const close = () => { setModal(null); setTried(false); };
  const schedule = () => {
    setTried(true);
    if (Object.keys(errors).length > 0) return;
    run(() => scheduleCampaign(d, campaignStart(d, zone)), t("Campaign scheduled"), (id) => { close(); setD(blank()); nav({ campaignId: id }); });
  };
  const done = { pause: "Campaign paused", resume: "Campaign resumed", abort: "Campaign aborted", retry_device: "Device queued again" } as const;
  const control = (action: "pause" | "resume" | "abort" | "retry_device", deviceId?: string) =>
    sel && run(() => controlCampaign(sel.id, sel.version, action, deviceId, action === "abort" ? reason.trim() : undefined), t(done[action]), close);
  const serial = new Map(live.devices.map((x) => [x.id, x.serial]));

  return (
    <div className="split-rev">
      <Card title={t("Campaigns")} action={live.canWrite && <Btn size="sm" variant="primary" onClick={() => { setD(blank()); setModal("new"); }}>{t("+ New campaign")}</Btn>} className="self-start">
        {(live.campaigns ?? []).length === 0 ? <EmptyState title={t("No campaigns")}>{t("Schedule a firmware campaign for a model.")}</EmptyState> : <div className="flex flex-col gap-2">{live.campaigns!.map((c) => <ListRow key={c.id} selected={sel?.id === c.id} onClick={() => nav({ campaignId: c.id })}><div className="min-w-0 flex-1"><b className="text-[13px]">{c.model}</b><div className="text-[11px] text-muted">{c.versions} · {t(c.devices === 1 ? "1 device" : "{n} devices", { n: c.devices })} · {t("starts {time}", { time: c.start })}</div></div><Badge tone={tone(c.state)}>{t(stateWord[c.state])}</Badge></ListRow>)}</div>}
      </Card>
      {!sel ? <Card title={t("Campaign")}><EmptyState title={t("Nothing selected")}>{t("Choose or schedule a campaign.")}</EmptyState></Card> : (
        <Card title={`${sel.model} · ${sel.versions}`} sub={t("Auto-pause if failures exceed {n} % within a wave · window {from}–{to} device local time · starts {time}", { n: sel.c.autoPauseFailurePercent, from: sel.c.window.startLocal, to: sel.c.window.endLocal, time: sel.start })} action={live.canWrite && <span className="flex gap-2">{(sel.state === "running" || sel.state === "scheduled") && <Btn size="sm" disabled={pending} onClick={() => control("pause")}>{t("Pause")}</Btn>}{sel.state === "paused" && <Btn size="sm" disabled={pending} onClick={() => control("resume")}>{t("Resume")}</Btn>}{["scheduled", "running", "paused"].includes(sel.state) && <Btn size="sm" variant="danger" disabled={pending} onClick={() => { setReason(""); setModal("abort"); }}>{t("Abort")}</Btn>}</span>}>
          <div className="flex flex-col gap-3">{sel.c.waves.map((w, n) => {
            const inWave = sel.c.results.filter((r) => r.waveIndex === n);
            const ok = inWave.filter((r) => r.status === "succeeded").length;
            const pct = inWave.length ? Math.round((ok * 100) / inWave.length) : 0;
            return <div key={w.label}><div className="mb-1 flex justify-between text-xs"><b>{t("Wave {n} · {percent} %", { n: n + 1, percent: w.percent })}</b><span className="text-muted">{t("{done} / {total} succeeded", { done: ok, total: inWave.length })}</span></div><UtilBar pct={pct} tone={pct >= 95 ? "ok" : "primary"} /></div>;
          })}</div>
          <div className="mt-3"><SummaryList items={[[t("Succeeded / installing / pending"), `${sel.c.progress.succeeded} / ${sel.c.progress.installing} / ${sel.c.progress.pending}`], [t("Failed"), t("{n} · failed devices keep their old version", { n: sel.c.progress.failed })], [t("Skipped"), t("{n} · busy, offline or tampered", { n: sel.c.progress.skipped })], ...(sel.c.reason ? [[t("Reason"), sel.c.reason] as [string, string]] : [])]} /></div>
          <h3 className="mb-1 mt-4 text-[13px] font-bold">{t("Devices")}</h3>
          <div className="flex flex-col gap-1">{sel.c.results.map((r) => <div key={r.deviceId} className="flex items-center justify-between gap-2 border-t border-line py-1.5 text-[13px]"><span>{serial.get(r.deviceId) ?? r.deviceId.slice(0, 8)} <span className="text-xs text-muted">{t("wave {n}", { n: r.waveIndex + 1 })}{r.reasonKey ? ` · ${r.reasonKey}` : ""}</span></span><span className="flex items-center gap-2"><Badge tone={r.status === "succeeded" ? "ok" : r.status === "failed" ? "crit" : r.status === "skipped" ? "warn" : "muted"}>{t(resultWord[r.status] ?? r.status)}</Badge>{live.canWrite && (r.status === "failed" || r.status === "skipped") && ["running", "paused"].includes(sel.state) && <Btn size="sm" disabled={pending} onClick={() => control("retry_device", r.deviceId)}>{t("Retry")}</Btn>}</span></div>)}</div>
        </Card>
      )}
      <Modal open={modal === "new"} onClose={close} title={t("New firmware campaign")} footer={<><Btn onClick={close}>{t("Cancel")}</Btn><Btn variant="primary" disabled={pending} onClick={schedule}>{t("campaign::Schedule")}</Btn></>}>
        <div className="grid-fluid" style={{ ["--min" as string]: "180px" }}>
          <Field label={t("Model")} error={tried ? errors.modelId : undefined}><Select value={d.modelId} onChange={(e) => setD({ ...d, modelId: e.target.value, targetVersion: "", deviceIds: [] })}>{live.models.map((m) => <option key={m.id} value={m.id}>{m.name}</option>)}</Select></Field>
          <Field label={t("Target version")} error={tried ? errors.targetVersion : undefined} hint={t("A firmware candidate of the model")}><Select value={d.targetVersion} onChange={(e) => setD({ ...d, targetVersion: e.target.value })}><option value="">{t("Select…")}</option>{model?.cap.firmwareCandidates.map((v) => <option key={v} value={v}>{v}</option>)}</Select></Field>
        </div>
        <div><p className="mb-1 text-[13px] font-semibold">{t("Devices of this model")}</p>{modelDevices.length === 0 ? <p className="text-xs text-muted">{t("No device is bound to a unit of this model.")}</p> : <div className="flex flex-col gap-1">{modelDevices.map((x) => <Check key={x.id} label={<span>{x.serial} <span className="text-xs text-muted">{x.unit} · {x.fw}{x.tamper ? ` · ${t("tamper (skipped)")}` : ""}</span></span>} checked={d.deviceIds.includes(x.id)} onChange={(on) => setD({ ...d, deviceIds: on ? [...d.deviceIds, x.id] : d.deviceIds.filter((y) => y !== x.id) })} />)}</div>}{tried && errors.deviceIds && <p className="mt-1 text-xs text-crit">{errors.deviceIds}</p>}</div>
        <Field label={t("Waves (ascending %, ends at 100)")} error={tried ? errors.waves : undefined}><Input value={d.waves} onChange={(e) => setD({ ...d, waves: e.target.value })} /></Field>
        <div className="grid-fluid" style={{ ["--min" as string]: "140px" }}>
          <Field label={t("Window start")} hint={t("Device local time")} error={tried ? errors.window : undefined}><Input type="time" value={d.startLocal} onChange={(e) => setD({ ...d, startLocal: e.target.value })} /></Field>
          <Field label={t("Window end")}><Input type="time" value={d.endLocal} onChange={(e) => setD({ ...d, endLocal: e.target.value })} /></Field>
          <Field label={t("Auto-pause above (%)")} error={tried ? errors.autoPause : undefined}><Input type="number" min="1" max="50" value={d.autoPause} onChange={(e) => setD({ ...d, autoPause: e.target.value })} /></Field>
        </div>
        <Field label={t("Start")} error={tried ? errors.startAt : undefined} hint={t("Now {time} — at least 24 h ahead · times in {zone}", { time: showTime(live.now, i.display), zone })}><Input type="datetime-local" value={d.startAt} onChange={(e) => setD({ ...d, startAt: e.target.value })} /></Field>
        <Banner>{t("Devices with an active diagnostic run, firmware operation or open tamper are skipped with a reason.")}</Banner>
      </Modal>
      <Modal open={modal === "abort"} onClose={close} title={t("Abort campaign")} footer={<><Btn onClick={close}>{t("Cancel")}</Btn><Btn variant="danger" disabled={pending || !reason.trim()} onClick={() => control("abort")}>{t("Abort")}</Btn></>}>
        <Field label={t("Reason")}><Textarea value={reason} maxLength={1000} onChange={(e) => setReason(e.target.value)} /></Field>
        <p className="text-[11px] text-muted">{t("Installed devices keep the new version; pending devices are not updated.")}</p>
      </Modal>
    </div>
  );
}
