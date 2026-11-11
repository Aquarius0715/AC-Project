"use client";

import { useState } from "react";
import { Badge, Btn, Card, Check, EmptyState, Field, Input, ListRow, Modal, Textarea, Timeline } from "@ac/web/components/ui";
import { useI18n } from "@ac/web/components/I18n";
import { useAction } from "@ac/web/lib/useAction";
import { byCount, showTime } from "@ac/web/lib/i18n";
import { capabilityChanges, capabilityDraft, capabilityErrors, capabilityInput, fans, modes, type CapabilityDraft, type Fan, type Mode } from "@ac/web/lib/devices";
import { capabilityHistory, deleteCapability, saveCapability, type HistoryEntry } from "../actions";
import type { DevicesLive } from "./devices-view";
import { Icon } from "@ac/web/components/Icon";
import { entryOf } from "@ac/web/lib/lookup";

const modeLabel: Record<Mode, string> = { cool: "Cool", dry: "Dry", fan: "Fan" };
const levelLabel: Record<Fan, string> = { low: "Low", mid: "Mid", high: "High" };
const toggle = <T,>(xs: T[], x: T, on: boolean) => (on ? [...new Set([...xs, x])] : xs.filter((y) => y !== x));

/** Aircon models: the capability register and editor (DD-A04): the controls of the air conditioner (DEC-105: the sensors
and firmware are in the IoT models). Saving an existing model writes the next capability version and moves its units to
it; the version history is audit.list of the capability, read on explicit open. Texts in the display language (IR295). */
export function ModelsTab({ live }: Readonly<{ live: DevicesLive }>) {
  const i = useI18n(), { t } = i;
  const [pending, run] = useAction();
  const [selId, setSelId] = useState<string>(live.models[0]?.id ?? "new");
  const sel = selId === "new" ? null : live.models.find((m) => m.id === selId) ?? live.models[0] ?? null;
  const key = sel ? `${sel.id}:${String(sel.version)}` : "new";
  const [source, setSource] = useState(key);
  const [d, setD] = useState<CapabilityDraft>(capabilityDraft(sel?.cap));
  const [tried, setTried] = useState(false);
  if (source !== key) { // restart the form from each saved version (adjust state during render)
    setSource(key);
    setD(capabilityDraft(sel?.cap));
    setTried(false);
  }
  const errors = capabilityErrors(d, !!sel, t);
  const set = (patch: Partial<CapabilityDraft>) => { setD((x) => ({ ...x, ...patch })); };
  const [history, setHistory] = useState<HistoryEntry[] | null>(null);
  const [removing, setRemoving] = useState<string | null>(null); // the reason being typed in the delete dialog (DEC-103)
  const units = sel ? live.units.filter((u) => u.modelId === sel.id) : [];
  // what the draft changes against the saved capability (Figma Admin 106:3: “Editing → v4”, “1 change · …”)
  const changes = sel ? capabilityChanges(d, capabilityDraft(sel.cap), t) : [];
  const isDirty = changes.length > 0;
  const firstModel = live.models[0];
  const save = () => {
    setTried(true);
    if (Object.keys(errors).length > 0) return;
    run(() => saveCapability(capabilityInput(d, sel?.id), sel?.version), (k) => (sel ? t("Saved — capability version {v}", { v: k.version }) : t("Model created")), (k) => { setSelId(k.id); });
  };
  const openHistory = () => { if (sel) run(() => capabilityHistory(sel.id), (xs) => t(xs.length === 1 ? "1 version entry" : "{n} version entries", { n: xs.length }), setHistory); };
  const remove = () => {
    setTried(true);
    if (!sel || !removing?.trim()) return;
    run(() => deleteCapability(sel.id, sel.version, removing.trim()), t("Model deleted"), () => { setRemoving(null); setTried(false); setSelId(live.models.find((m) => m.id !== sel.id)?.id ?? "new"); });
  };
  const fieldError = (k: string) => (tried ? errors[k] : undefined);

  return (
    <div className="split-rev">
      <Card title={t("Aircon models")} action={live.canWrite && <Btn size="sm" onClick={() => { setSelId("new"); }}><Icon name="add" /> {t("New model")}</Btn>} className="self-start">
        {live.models.length === 0 ? <EmptyState title={t("No aircon models")}>{t("Register the first aircon model.")}</EmptyState> : <div className="flex flex-col gap-2">{live.models.map((x) => <ListRow key={x.id} selected={sel?.id === x.id} onClick={() => { setSelId(x.id); }}><div className="min-w-0"><b className="text-[13px]">{x.name}</b> {x.ventilation && <Badge tone="primary">{t("Ventilation")}</Badge>}<div className="text-[11px] text-muted-foreground">{t("capability v{v} · updated {time}", { v: x.version, time: x.updated })}</div><div className="text-[11px] text-muted-foreground">{x.used}</div></div></ListRow>)}</div>}
        <p className="mt-3 text-[11px] text-muted-foreground">{t("Saving a change creates a new capability version. Unverified capabilities stay false/unknown.")}</p>
      </Card>
      <Card title={sel ? <span className="inline-flex flex-wrap items-center gap-2">{sel.name}<Badge>{t("Capability v{v}", { v: sel.version })}</Badge>{isDirty && <Badge tone="warn">{t("Editing → v{v}", { v: sel.version + 1 })}</Badge>}</span> : t("New model")}
        sub={sel ? `${t("updated {time}", { time: sel.updated })} · ${sel.used}` : t("Saving creates capability version 1")} action={sel && <span className="flex flex-wrap gap-2">{live.canAudit && <Btn size="sm" disabled={pending} onClick={openHistory}>{t("Version history")}</Btn>}{live.canWrite && <Btn size="sm" variant="danger" disabled={pending} onClick={() => { setRemoving(""); setTried(false); }}>{t("Delete model…")}</Btn>}</span>}>
        <div className="flex flex-col gap-5">
          <section><h3 className="mb-2 text-[13px] font-bold">{t("Identity")}</h3><div className="grid-fluid" style={{ ["--min" as string]: "200px" }}><Field label={t("Manufacturer")} error={fieldError("manufacturer")}><Input value={d.manufacturer} maxLength={120} onChange={(e) => { set({ manufacturer: e.target.value }); }} /></Field><Field label={t("Model")} hint={t("Manufacturer + model must be unique")} error={fieldError("model")}><Input value={d.model} maxLength={120} onChange={(e) => { set({ model: e.target.value }); }} /></Field></div></section>
          <section>
            <h3 className="mb-1 text-[13px] font-bold">{t("Controls")}</h3>
            <p className="mb-2 text-xs text-muted-foreground">{t("Only enable what has been verified on the real model — never infer from product category.")}</p>
            <div className="grid-fluid" style={{ ["--min" as string]: "180px" }}><Check label={t("Power control")} checked={d.control} onChange={(v) => { set({ control: v }); }} /><Check label={t("Mode control")} checked={d.modeControl} onChange={(v) => { set({ modeControl: v }); }} /><Check label={t("Fan control")} checked={d.fanControl} onChange={(v) => { set({ fanControl: v }); }} /><Check label={t("Temperature setpoint")} checked={d.temperatureOn} onChange={(v) => { set({ temperatureOn: v }); }} /></div>
            {fieldError("control") && <p className="mt-1 text-xs text-crit">{fieldError("control")}</p>}
          </section>
          {d.temperatureOn && <section><h3 className="mb-1 text-[13px] font-bold">{t("Temperature")}</h3><div className="grid-fluid" style={{ ["--min" as string]: "110px" }}><Field label={t("Min °C")} error={fieldError("temperature")}><Input type="number" value={d.min} onChange={(e) => { set({ min: e.target.value }); }} /></Field><Field label={t("Max °C")}><Input type="number" value={d.max} onChange={(e) => { set({ max: e.target.value }); }} /></Field><Field label={t("Step °C")}><Input type="number" step="0.5" value={d.step} onChange={(e) => { set({ step: e.target.value }); }} /></Field></div></section>}
          <section>
            <h3 className="mb-1 text-[13px] font-bold">{t("Modes & fan")}</h3>
            <div className="flex flex-wrap gap-x-6 gap-y-2 text-[13px]">{modes.map((m: Mode) => <Check key={m} label={t(modeLabel[m])} checked={d.modes.includes(m)} disabled={!d.modeControl} onChange={(v) => { set({ modes: toggle(d.modes, m, v) }); }} />)}{fans.map((f: Fan) => <Check key={f} label={t("Fan {level}", { level: t(entryOf(levelLabel, f)) })} checked={d.fanLevels.includes(f)} disabled={!d.fanControl} onChange={(v) => { set({ fanLevels: toggle(d.fanLevels, f, v) }); }} />)}</div>
            {(fieldError("modes") ?? fieldError("fanLevels")) !== undefined && <p className="mt-1 text-xs text-crit">{fieldError("modes") ?? fieldError("fanLevels")}</p>}
          </section>
          <section>
            <h3 className="mb-1 text-[13px] font-bold">{t("Ventilation")}</h3>
            <Check label={t("Ventilation supported")} checked={d.ventilation} onChange={(v) => { set({ ventilation: v, ventilationLevels: v ? d.ventilationLevels : [] }); }} />
            <p className="mb-1 mt-2 text-xs text-muted-foreground">{t("When enabled, levels must be non-empty and include Low.")}</p>
            <div className="flex flex-wrap gap-x-6 gap-y-2">{fans.map((k) => <Check key={k} label={t(entryOf(levelLabel, k))} checked={d.ventilationLevels.includes(k)} disabled={!d.ventilation} onChange={(v) => { set({ ventilationLevels: toggle(d.ventilationLevels, k, v) }); }} />)}</div>
            {fieldError("ventilationLevels") && <p className="mt-1 flex items-center gap-1 text-xs text-crit"><Icon name="error" />{fieldError("ventilationLevels")}</p>}
          </section>
          {sel && <section className="rounded-xl bg-surface2 p-3"><h3 className="mb-1 text-[13px] font-bold">{t("Impact of this change")}</h3><ul className="list-disc pl-5 text-xs"><li>{t(units.length === 1 ? "1 unit{list} moves to capability version {v}" : "{n} units{list} move to capability version {v}", { n: units.length, list: units.length > 0 ? ` — ${units.map((u) => u.label).join(", ")}` : "", v: sel.version + 1 })}</li><li>{t("Automation rules that no longer fit are disabled, with the reason that the capabilities changed")}</li><li>{t("Unfinished commands keep their original content")}</li></ul></section>}
          {sel && <Field label={t("Change reason · required for updates")} error={fieldError("changeReason")} hint={`${String(d.changeReason.length)} / 1000`}><Textarea value={d.changeReason} maxLength={1000} onChange={(e) => { set({ changeReason: e.target.value }); }} placeholder={t("Vendor bulletin: ventilation High not verified on the current batch.")} /></Field>}
          {live.canWrite && (
            <div className="flex flex-wrap items-center justify-end gap-2 border-t border-line pt-3">
              {sel && <span className="mr-auto text-[12px] text-muted-foreground">{isDirty ? `${byCount(changes.length, { one: t("1 change"), many: t("{n} changes", { n: changes.length }) })} · ${changes.join(", ")}` : t("No unsaved changes")}</span>}
              {!sel && firstModel && <Btn onClick={() => { setSelId(firstModel.id); }}>{t("Cancel")}</Btn>}
              {sel && <Btn disabled={pending || !isDirty} onClick={() => { setD(capabilityDraft(sel.cap)); setTried(false); }}>{t("Discard")}</Btn>}
              <Btn variant="primary" disabled={pending || (!!sel && !isDirty)} onClick={save}>{sel ? t("Save as capability v{v}", { v: sel.version + 1 }) : t("Create model")}</Btn>
            </div>
          )}
        </div>
      </Card>
      <Modal open={removing !== null} onClose={() => { setRemoving(null); setTried(false); }} title={t("Delete model")} footer={<><Btn onClick={() => { setRemoving(null); setTried(false); }}>{t("Cancel")}</Btn><Btn variant="danger" disabled={pending} onClick={remove}>{t("Delete")}</Btn></>}>
        <p className="text-[13px]">{t("Delete {name}? A model that units use cannot be deleted.", { name: sel?.name ?? "" })}</p>
        <Field label={t("Reason")} error={tried && !removing?.trim() ? t("A reason is required") : undefined}><Textarea value={removing ?? ""} maxLength={1000} onChange={(e) => { setRemoving(e.target.value); }} /></Field>
      </Modal>
      <Modal open={history !== null} onClose={() => { setHistory(null); }} title={t("Version history · {name}", { name: sel?.name ?? "" })} footer={<Btn onClick={() => { setHistory(null); }}>{t("Close")}</Btn>}>
        {history?.length === 0 ? <p className="text-xs text-muted-foreground">{t("No saved versions in the last 12 months.")}</p> : <Timeline items={(history ?? []).map((h) => ({ time: showTime(h.at, i.display), title: `v${String(h.from ?? "—")} → v${String(h.to ?? "—")}${h.reason ? ` — ${h.reason}` : ""}`, detail: `${h.actor}${h.changes.length > 0 ? ` · ${t("changed {fields}", { fields: h.changes.join(", ") })}` : ""}` }))} />}
        <p className="text-[11px] text-muted-foreground">{t("From the audit log (audit.read); values are masked as recorded.")}</p>
      </Modal>
    </div>
  );
}
