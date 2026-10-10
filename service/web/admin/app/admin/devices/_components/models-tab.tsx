"use client";

import { useState } from "react";
import { Badge, Btn, Card, Check, EmptyState, Field, Input, ListRow, Modal, Select, Textarea, Timeline } from "@ac/web/components/ui";
import { useI18n } from "@ac/web/components/I18n";
import { useAction } from "@ac/web/lib/useAction";
import { showTime } from "@ac/web/lib/i18n";
import { capabilityDraft, capabilityErrors, capabilityInput, fans, metrics, metricUnit, modes, type CapabilityDraft, type Fan, type Metric, type Mode } from "@ac/web/lib/devices";
import { capabilityHistory, saveCapability, type HistoryEntry } from "../actions";
import type { DevicesLive } from "./devices-view";

const modeLabel: Record<Mode, string> = { cool: "Cool", dry: "Dry", fan: "Fan" };
const levelLabel: Record<Fan, string> = { low: "Low", mid: "Mid", high: "High" };
const toggle = <T,>(xs: T[], x: T, on: boolean) => (on ? [...new Set([...xs, x])] : xs.filter((y) => y !== x));

/** Models: the capability register and editor (DD-A04). Saving an existing model writes the next capability version
 * and moves its units to it; the version history is audit.list of the capability, read on explicit open. Texts in the
 * display language; metric codes stay codes (IR295). */
export function ModelsTab({ live }: { live: DevicesLive }) {
  const i = useI18n(), { t } = i;
  const [pending, run] = useAction();
  const [selId, setSelId] = useState<string | "new">(live.models[0]?.id ?? "new");
  const sel = selId === "new" ? null : live.models.find((m) => m.id === selId) ?? live.models[0] ?? null;
  const key = sel ? `${sel.id}:${sel.version}` : "new";
  const [source, setSource] = useState(key);
  const [d, setD] = useState<CapabilityDraft>(capabilityDraft(sel?.cap));
  const [tried, setTried] = useState(false);
  if (source !== key) { // restart the form from each saved version (adjust state during render)
    setSource(key);
    setD(capabilityDraft(sel?.cap));
    setTried(false);
  }
  const errors = capabilityErrors(d, !!sel, t);
  const set = (patch: Partial<CapabilityDraft>) => setD((x) => ({ ...x, ...patch }));
  const [history, setHistory] = useState<HistoryEntry[] | null>(null);
  const [newSensor, setNewSensor] = useState<Metric>("temperature");
  const units = sel ? live.units.filter((u) => u.modelId === sel.id) : [];
  const available = metrics.filter((m) => !d.sensors.some((s) => s.metric === m));
  const pick = available.includes(newSensor) ? newSensor : available[0];
  const save = () => {
    setTried(true);
    if (Object.keys(errors).length > 0) return;
    run(() => saveCapability(capabilityInput(d, sel?.id), sel?.version), (k) => (sel ? t("Saved — capability version {v}", { v: k.version }) : t("Model created")), (k) => setSelId(k.id));
  };
  const openHistory = () => sel && run(() => capabilityHistory(sel.id), (xs) => t(xs.length === 1 ? "1 version entry" : "{n} version entries", { n: xs.length }), setHistory);
  const err = (k: string) => (tried ? errors[k] : undefined);

  return (
    <div className="split-rev">
      <Card title={t("Models")} action={live.canWrite && <Btn size="sm" onClick={() => setSelId("new")}>{t("+ New")}</Btn>} className="self-start">
        {live.models.length === 0 ? <EmptyState title={t("No models")}>{t("Register the first model.")}</EmptyState> : <div className="flex flex-col gap-2">{live.models.map((x) => <ListRow key={x.id} selected={sel?.id === x.id} onClick={() => setSelId(x.id)}><div className="min-w-0"><b className="text-[13px]">{x.name}</b> {x.ventilation && <Badge tone="primary">{t("Ventilation")}</Badge>}<div className="text-[11px] text-muted">{t("capability v{v} · updated {time}", { v: x.version, time: x.updated })}</div><div className="text-[11px] text-muted">{x.used}</div></div></ListRow>)}</div>}
        <p className="mt-3 text-[11px] text-muted">{t("Saving a change creates a new capability version. Unverified capabilities stay false/unknown (BR-A04).")}</p>
      </Card>
      <Card title={sel ? sel.name : t("New model")} sub={sel ? t("capability v{v} · {used}", { v: sel.version, used: sel.used }) : t("Saving creates capability version 1")} action={sel && live.canAudit && <Btn size="sm" disabled={pending} onClick={openHistory}>{t("Version history")}</Btn>}>
        <div className="flex flex-col gap-5">
          <section><h3 className="mb-2 text-[13px] font-bold">{t("Identity")}</h3><div className="grid-fluid" style={{ ["--min" as string]: "200px" }}><Field label={t("Manufacturer")} error={err("manufacturer")}><Input value={d.manufacturer} maxLength={120} onChange={(e) => set({ manufacturer: e.target.value })} /></Field><Field label={t("Model")} hint={t("Manufacturer + model must be unique")} error={err("model")}><Input value={d.model} maxLength={120} onChange={(e) => set({ model: e.target.value })} /></Field></div></section>
          <section>
            <h3 className="mb-1 text-[13px] font-bold">{t("Controls")}</h3>
            <p className="mb-2 text-xs text-muted">{t("Only enable what has been verified on the real model — never infer from product category.")}</p>
            <div className="grid-fluid" style={{ ["--min" as string]: "180px" }}><Check label={t("Power control")} checked={d.control} onChange={(v) => set({ control: v })} /><Check label={t("Mode control")} checked={d.modeControl} onChange={(v) => set({ modeControl: v })} /><Check label={t("Fan control")} checked={d.fanControl} onChange={(v) => set({ fanControl: v })} /><Check label={t("Temperature setpoint")} checked={d.temperatureOn} onChange={(v) => set({ temperatureOn: v })} /></div>
            {err("control") && <p className="mt-1 text-xs text-crit">{err("control")}</p>}
          </section>
          {d.temperatureOn && <section><h3 className="mb-1 text-[13px] font-bold">{t("Temperature")}</h3><div className="grid-fluid" style={{ ["--min" as string]: "110px" }}><Field label={t("Min °C")} error={err("temperature")}><Input type="number" value={d.min} onChange={(e) => set({ min: e.target.value })} /></Field><Field label={t("Max °C")}><Input type="number" value={d.max} onChange={(e) => set({ max: e.target.value })} /></Field><Field label={t("Step °C")}><Input type="number" step="0.5" value={d.step} onChange={(e) => set({ step: e.target.value })} /></Field></div></section>}
          <section>
            <h3 className="mb-1 text-[13px] font-bold">{t("Modes & fan")}</h3>
            <div className="flex flex-wrap gap-x-6 gap-y-2 text-[13px]">{modes.map((m: Mode) => <Check key={m} label={t(modeLabel[m])} checked={d.modes.includes(m)} disabled={!d.modeControl} onChange={(v) => set({ modes: toggle(d.modes, m, v) })} />)}{fans.map((f: Fan) => <Check key={f} label={t("Fan {level}", { level: t(levelLabel[f]) })} checked={d.fanLevels.includes(f)} disabled={!d.fanControl} onChange={(v) => set({ fanLevels: toggle(d.fanLevels, f, v) })} />)}</div>
            {(err("modes") || err("fanLevels")) && <p className="mt-1 text-xs text-crit">{err("modes") ?? err("fanLevels")}</p>}
          </section>
          <section>
            <h3 className="mb-1 text-[13px] font-bold">{t("Ventilation")}</h3>
            <Check label={t("Ventilation supported")} checked={d.ventilation} onChange={(v) => set({ ventilation: v, ventilationLevels: v ? d.ventilationLevels : [] })} />
            <p className="mb-1 mt-2 text-xs text-muted">{t("When enabled, levels must be non-empty and include Low.")}</p>
            <div className="flex flex-wrap gap-x-6 gap-y-2">{fans.map((k) => <Check key={k} label={t(levelLabel[k])} checked={d.ventilationLevels.includes(k)} disabled={!d.ventilation} onChange={(v) => set({ ventilationLevels: toggle(d.ventilationLevels, k, v) })} />)}</div>
            {err("ventilationLevels") && <p className="mt-1 text-xs text-crit">✕ {err("ventilationLevels")}</p>}
          </section>
          <section>
            <h3 className="mb-2 text-[13px] font-bold">{t("Sensors")}</h3>
            <div className="scroll-x"><table className="w-full min-w-[420px] text-[13px]"><thead className="text-left text-[11px] uppercase text-muted"><tr><th>{t("Metric")}</th><th>{t("Unit")}</th><th>{t("Stale after (s)")}</th><th>{t("Boundary")}</th><th /></tr></thead><tbody>{d.sensors.map((s, n) => <tr key={s.metric} className="border-t border-line"><td className="py-1.5">{s.metric}</td><td>{metricUnit[s.metric]}</td><td><Input type="number" className="w-28" aria-label={t("Stale after (s)")} value={s.staleAfterSeconds} onChange={(e) => set({ sensors: d.sensors.map((x, j) => (j === n ? { ...x, staleAfterSeconds: e.target.value } : x)) })} /></td><td className="text-xs text-muted">{s.metric === "power" ? "ac_input_electricity" : "—"}</td><td><button type="button" className="text-xs text-crit" onClick={() => set({ sensors: d.sensors.filter((_, j) => j !== n) })}>{t("Remove")}</button></td></tr>)}</tbody></table></div>
            {available.length > 0 && <div className="mt-2 flex items-end gap-2"><Field label={t("Add sensor")}><Select value={pick} onChange={(e) => setNewSensor(e.target.value as Metric)}>{available.map((m) => <option key={m} value={m}>{m} ({metricUnit[m]})</option>)}</Select></Field><Btn size="sm" onClick={() => set({ sensors: [...d.sensors, { metric: pick, staleAfterSeconds: "120" }] })}>{t("Add")}</Btn></div>}
            {err("sensors") && <p className="mt-1 text-xs text-crit">{err("sensors")}</p>}
          </section>
          <section><Field label={t("Firmware candidates (comma separated)")} hint={t("Supported demo versions only")} error={err("firmware")}><Input value={d.firmware} onChange={(e) => set({ firmware: e.target.value })} placeholder="v1, v2" /></Field></section>
          {sel && <section className="rounded-xl bg-surface2 p-3"><h3 className="mb-1 text-[13px] font-bold">{t("Impact of this change")}</h3><ul className="list-disc pl-5 text-xs"><li>{t(units.length === 1 ? "1 unit{list} moves to capability version {v}" : "{n} units{list} move to capability version {v}", { n: units.length, list: units.length ? ` — ${units.map((u) => u.label).join(", ")}` : "", v: sel.version + 1 })}</li><li>{t("Automation rules that no longer fit are disabled with reason capability_changed")}</li><li>{t("Unfinished commands keep their original content")}</li></ul></section>}
          {sel && <Field label={t("Change reason · required for updates")} error={err("changeReason")} hint={`${d.changeReason.length} / 1000`}><Textarea value={d.changeReason} maxLength={1000} onChange={(e) => set({ changeReason: e.target.value })} placeholder={t("Vendor bulletin: ventilation High not verified on the current batch.")} /></Field>}
          {live.canWrite && <div className="flex justify-end gap-2">{!sel && live.models[0] && <Btn onClick={() => setSelId(live.models[0].id)}>{t("Cancel")}</Btn>}<Btn variant="primary" disabled={pending} onClick={save}>{t(sel ? "Save new version" : "Create model")}</Btn></div>}
        </div>
      </Card>
      <Modal open={history !== null} onClose={() => setHistory(null)} title={t("Version history · {name}", { name: sel?.name ?? "" })} footer={<Btn onClick={() => setHistory(null)}>{t("Close")}</Btn>}>
        {history && history.length === 0 ? <p className="text-xs text-muted">{t("No saved versions in the last 12 months.")}</p> : <Timeline items={(history ?? []).map((h) => ({ time: showTime(h.at, i.display), title: `v${h.from ?? "—"} → v${h.to ?? "—"}${h.reason ? ` — ${h.reason}` : ""}`, detail: `${h.actor}${h.changes.length ? ` · ${t("changed {fields}", { fields: h.changes.join(", ") })}` : ""}` }))} />}
        <p className="text-[11px] text-muted">{t("From the audit log (audit.read); values are masked as recorded.")}</p>
      </Modal>
    </div>
  );
}
