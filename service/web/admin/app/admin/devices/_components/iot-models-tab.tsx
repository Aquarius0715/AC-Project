"use client";

import { useState } from "react";
import { Badge, Btn, Card, EmptyState, Field, Input, ListRow, Modal, Select, Textarea, Timeline, Plain, ScrollX } from "@ac/web/components/ui";
import { useI18n } from "@ac/web/components/I18n";
import { useAction } from "@ac/web/lib/useAction";
import { byCount, showTime } from "@ac/web/lib/i18n";
import { firmwareProblem, iotModelChanges, iotModelDraft, iotModelErrors, iotModelInput, metrics, metricUnit, type IoTModelDraft, type Metric } from "@ac/web/lib/devices";
import { metricText } from "@ac/web/lib/techDevices";
import { capabilityHistory, deleteIotModel, saveIotModel, type HistoryEntry } from "../actions";
import type { DevicesLive } from "./devices-view";
import { oneOf } from "@ac/web/lib/oneOf";
import { Icon } from "@ac/web/components/Icon";

/** IoT models: the register and editor of what a device of the model measures (sensors) and the firmware versions it
can be updated to (DEC-105, DD-A04). Saving an existing model writes the next version and moves its units and devices
to it; the version history is audit.list of the model, read on explicit open. Texts in the display language; metric
codes stay codes (IR295). */
export function IoTModelsTab({ live }: Readonly<{ live: DevicesLive }>) {
  const i = useI18n(), { t } = i;
  const [pending, run] = useAction();
  const [selId, setSelId] = useState<string>(live.iotModels[0]?.id ?? "new");
  const sel = selId === "new" ? null : live.iotModels.find((m) => m.id === selId) ?? live.iotModels[0] ?? null;
  const key = sel ? `${sel.id}:${String(sel.version)}` : "new";
  const [source, setSource] = useState(key);
  const [d, setD] = useState<IoTModelDraft>(iotModelDraft(sel?.model));
  const [tried, setTried] = useState(false);
  if (source !== key) { // restart the form from each saved version (adjust state during render)
    setSource(key);
    setD(iotModelDraft(sel?.model));
    setTried(false);
  }
  const errors = iotModelErrors(d, !!sel, t);
  const set = (patch: Partial<IoTModelDraft>) => { setD((x) => ({ ...x, ...patch })); };
  const [history, setHistory] = useState<HistoryEntry[] | null>(null);
  const [removing, setRemoving] = useState<string | null>(null); // the reason being typed in the delete dialog
  const [addedSensor, setAddedSensor] = useState<Metric>("temperature");
  const [adding, setAdding] = useState<string | null>(null); // the firmware version being typed after “+ Add version”
  const units = sel ? live.units.filter((u) => u.iotModelId === sel.id) : [];
  const changes = sel ? iotModelChanges(d, iotModelDraft(sel.model), t) : [];
  const isDirty = changes.length > 0;
  const runningOn = (v: string) => live.devices.filter((x) => sel !== null && x.iotModelId === sel.id && x.fw === v).length;
  const problemOnAdd = adding === null ? null : firmwareProblem(adding, d.firmware, t);
  const addVersion = () => {
    if (adding === null || problemOnAdd) return;
    set({ firmware: [...d.firmware, adding.trim()] });
    setAdding(null);
  };
  const available = metrics.filter((m) => d.sensors.every((s) => s.metric !== m));
  const pick = available.includes(addedSensor) ? addedSensor : available[0];
  const firstModel = live.iotModels[0];
  const save = () => {
    setTried(true);
    if (Object.keys(errors).length > 0) return;
    run(() => saveIotModel(iotModelInput(d, sel?.id), sel?.version), (k) => (sel ? t("Saved — IoT model version {v}", { v: k.version }) : t("IoT model created")), (k) => { setSelId(k.id); });
  };
  const openHistory = () => { if (sel) run(() => capabilityHistory(sel.id), (xs) => t(xs.length === 1 ? "1 version entry" : "{n} version entries", { n: xs.length }), setHistory); };
  const remove = () => {
    setTried(true);
    if (!sel || !removing?.trim()) return;
    run(() => deleteIotModel(sel.id, sel.version, removing.trim()), t("IoT model deleted"), () => { setRemoving(null); setTried(false); setSelId(live.iotModels.find((m) => m.id !== sel.id)?.id ?? "new"); });
  };
  const fieldError = (k: string) => (tried ? errors[k] : undefined);

  return (
    <div className="split-rev">
      <Card title={t("IoT models")} action={live.canWrite && <Btn size="sm" onClick={() => { setSelId("new"); }}><Icon name="add" /> {t("New IoT model")}</Btn>} className="self-start">
        {live.iotModels.length === 0 ? <EmptyState title={t("No IoT models")}>{t("Register the first IoT model.")}</EmptyState> : <div className="flex flex-col gap-2">{live.iotModels.map((x) => <ListRow key={x.id} selected={sel?.id === x.id} onClick={() => { setSelId(x.id); }}><div className="min-w-0"><b className="text-[13px]">{x.name}</b><div className="text-[11px] text-muted-foreground">{t("IoT model v{v} · updated {time}", { v: x.version, time: x.updated })}</div><div className="text-[11px] text-muted-foreground">{x.used}</div></div></ListRow>)}</div>}
        <p className="mt-3 text-[11px] text-muted-foreground">{t("Saving a change creates a new IoT model version. A unit's IoT model is optional.")}</p>
      </Card>
      <Card title={sel ? <span className="inline-flex flex-wrap items-center gap-2">{sel.name}<Badge>{t("IoT model v{v}", { v: sel.version })}</Badge>{isDirty && <Badge tone="warn">{t("Editing → v{v}", { v: sel.version + 1 })}</Badge>}</span> : t("New IoT model")}
        sub={sel ? `${t("updated {time}", { time: sel.updated })} · ${sel.used}` : t("Saving creates IoT model version 1")} action={sel && <span className="flex flex-wrap gap-2">{live.canAudit && <Btn size="sm" disabled={pending} onClick={openHistory}>{t("Version history")}</Btn>}{live.canWrite && <Btn size="sm" variant="danger" disabled={pending} onClick={() => { setRemoving(""); setTried(false); }}>{t("Delete IoT model…")}</Btn>}</span>}>
        <div className="flex flex-col gap-5">
          <section><h3 className="mb-2 text-[13px] font-bold">{t("Identity")}</h3><div className="grid-fluid" style={{ ["--min" as string]: "200px" }}><Field label={t("Manufacturer")} error={fieldError("manufacturer")}><Input value={d.manufacturer} maxLength={120} onChange={(e) => { set({ manufacturer: e.target.value }); }} /></Field><Field label={t("Model")} hint={t("Manufacturer + model must be unique")} error={fieldError("model")}><Input value={d.model} maxLength={120} onChange={(e) => { set({ model: e.target.value }); }} /></Field></div></section>
          <section>
            <h3 className="mb-2 text-[13px] font-bold">{t("Sensors")}</h3>
            <ScrollX><table className="w-full min-w-[420px] text-[13px]"><thead className="text-left text-[11px] uppercase text-muted-foreground"><tr><th>{t("Metric")}</th><th>{t("Unit")}</th><th>{t("Stale after (s)")}</th><th>{t("Boundary")}</th><td /></tr></thead><tbody>{d.sensors.map((s, n) => <tr key={s.metric} className="border-t border-line"><td className="py-1.5">{metricText(s.metric, t)}</td><td>{metricUnit[s.metric]}</td><td><Input type="number" className="w-28" aria-label={t("Stale after (s)")} value={s.staleAfterSeconds} onChange={(e) => { set({ sensors: d.sensors.map((x, j) => (j === n ? { ...x, staleAfterSeconds: e.target.value } : x)) }); }} /></td><td className="text-xs text-muted-foreground">{s.metric === "power" ? "ac_input_electricity" : "—"}</td><td><Plain touch type="button" className="text-xs text-crit" onClick={() => { set({ sensors: d.sensors.filter((_, j) => j !== n) }); }}>{t("Remove")}</Plain></td></tr>)}</tbody></table></ScrollX>
            {pick && <div className="mt-2 flex items-end gap-2"><Field label={t("Add sensor")}><Select value={pick} onChange={(e) => { setAddedSensor(oneOf(available, e.target.value, pick)); }}>{available.map((m) => <option key={m} value={m}>{metricText(m, t)} ({metricUnit[m]})</option>)}</Select></Field><Btn size="sm" onClick={() => { set({ sensors: [...d.sensors, { metric: pick, staleAfterSeconds: "120" }] }); }}>{t("Add")}</Btn></div>}
            {fieldError("sensors") && <p className="mt-1 text-xs text-crit">{fieldError("sensors")}</p>}
          </section>
          <section aria-label={t("Firmware candidates")}>
            <h3 className="mb-1 text-[13px] font-bold">{t("Firmware candidates")}</h3>
            <p className="mb-2 text-xs text-muted-foreground">{t("Only supported demo versions can be offered for updates.")}</p>
            <div className="flex flex-wrap items-center gap-2">
              {d.firmware.map((v, n) => {
                const on = runningOn(v);
                return (
                  <span key={v} className="inline-flex items-center gap-1.5 rounded-control border border-primary/40 bg-primary-soft px-2.5 py-1 text-xs">
                    <b>{v}</b><span className="text-muted-foreground">{byCount(on, { none: "", one: t("current on 1 device"), many: t("current on {n} devices", { n: on }) })}{!on && n === d.firmware.length - 1 && t("latest")}{!on && n !== d.firmware.length - 1 && ""}</span>
                    <Plain touch type="button" aria-label={t("Remove {name}", { name: v })} className="text-muted-foreground hover:text-ink" onClick={() => { set({ firmware: d.firmware.filter((x) => x !== v) }); }}><Icon name="close" /></Plain>
                  </span>
                );
              })}
              {adding === null ? <Btn size="sm" variant="ghost" onClick={() => { setAdding(""); }}><Icon name="add" /> {t("Add version")}</Btn> : (
                <span className="inline-flex items-center gap-1.5">
                  <Input aria-label={t("Firmware version")} className="w-28" ref={(el) => { el?.focus(); }} value={adding} maxLength={32} placeholder="v3" onChange={(e) => { setAdding(e.target.value); }} onKeyDown={(e) => { if (e.key === "Enter") { addVersion(); } else if (e.key === "Escape") { setAdding(null); } }} />
                  <Btn size="sm" disabled={!!problemOnAdd} onClick={addVersion}>{t("Add")}</Btn><Btn size="sm" variant="ghost" onClick={() => { setAdding(null); }}>{t("Cancel")}</Btn>
                </span>
              )}
            </div>
            {adding !== null && adding.trim() !== "" && problemOnAdd && <p className="mt-1 flex items-center gap-1 text-xs text-crit"><Icon name="error" />{problemOnAdd}</p>}
            {fieldError("firmware") && <p className="mt-1 flex items-center gap-1 text-xs text-crit"><Icon name="error" />{fieldError("firmware")}</p>}
          </section>
          {sel && <section className="rounded-xl bg-surface2 p-3"><h3 className="mb-1 text-[13px] font-bold">{t("Impact of this change")}</h3><ul className="list-disc pl-5 text-xs"><li>{t(units.length === 1 ? "1 unit{list} moves to IoT model version {v}" : "{n} units{list} move to IoT model version {v}", { n: units.length, list: units.length > 0 ? ` — ${units.map((u) => u.label).join(", ")}` : "", v: sel.version + 1 })}</li><li>{t("The devices registered with this model move to the new version")}</li></ul></section>}
          {sel && <Field label={t("Change reason · required for updates")} error={fieldError("changeReason")} hint={`${String(d.changeReason.length)} / 1000`}><Textarea value={d.changeReason} maxLength={1000} onChange={(e) => { set({ changeReason: e.target.value }); }} placeholder={t("Vendor bulletin: the PM2.5 sensor is not on the current batch.")} /></Field>}
          {live.canWrite && (
            <div className="flex flex-wrap items-center justify-end gap-2 border-t border-line pt-3">
              {sel && <span className="mr-auto text-[12px] text-muted-foreground">{isDirty ? `${byCount(changes.length, { one: t("1 change"), many: t("{n} changes", { n: changes.length }) })} · ${changes.join(", ")}` : t("No unsaved changes")}</span>}
              {!sel && firstModel && <Btn onClick={() => { setSelId(firstModel.id); }}>{t("Cancel")}</Btn>}
              {sel && <Btn disabled={pending || !isDirty} onClick={() => { setD(iotModelDraft(sel.model)); setTried(false); setAdding(null); }}>{t("Discard")}</Btn>}
              <Btn variant="primary" disabled={pending || (!!sel && !isDirty)} onClick={save}>{sel ? t("Save as IoT model v{v}", { v: sel.version + 1 }) : t("Create IoT model")}</Btn>
            </div>
          )}
        </div>
      </Card>
      <Modal open={removing !== null} onClose={() => { setRemoving(null); setTried(false); }} title={t("Delete IoT model")} footer={<><Btn onClick={() => { setRemoving(null); setTried(false); }}>{t("Cancel")}</Btn><Btn variant="danger" disabled={pending} onClick={remove}>{t("Delete")}</Btn></>}>
        <p className="text-[13px]">{t("Delete {name}? An IoT model that units or devices use, or that a firmware campaign names, cannot be deleted.", { name: sel?.name ?? "" })}</p>
        <Field label={t("Reason")} error={tried && !removing?.trim() ? t("A reason is required") : undefined}><Textarea value={removing ?? ""} maxLength={1000} onChange={(e) => { setRemoving(e.target.value); }} /></Field>
      </Modal>
      <Modal open={history !== null} onClose={() => { setHistory(null); }} title={t("Version history · {name}", { name: sel?.name ?? "" })} footer={<Btn onClick={() => { setHistory(null); }}>{t("Close")}</Btn>}>
        {history?.length === 0 ? <p className="text-xs text-muted-foreground">{t("No saved versions in the last 12 months.")}</p> : <Timeline items={(history ?? []).map((h) => ({ time: showTime(h.at, i.display), title: `v${String(h.from ?? "—")} → v${String(h.to ?? "—")}${h.reason ? ` — ${h.reason}` : ""}`, detail: `${h.actor}${h.changes.length > 0 ? ` · ${t("changed {fields}", { fields: h.changes.join(", ") })}` : ""}` }))} />}
        <p className="text-[11px] text-muted-foreground">{t("From the audit log (audit.read); values are masked as recorded.")}</p>
      </Modal>
    </div>
  );
}
