"use client";

import Link from "next/link";
import { Fragment, useState } from "react";
import { Badge, Banner, Btn, Card, Check, Choice, Field, Input, Modal, SummaryList, UtilBar, cx } from "@ac/web/components/ui";
import { useAction } from "@ac/web/lib/useAction";
import type { ActionFailure } from "@ac/web/lib/actionMessage";
import { cleaningSymptom, DEFAULT_FILTER_HOURS, filterStatus, reminderErrors, reminderInput, type FilterGroup, type FilterRow, type ReminderForm } from "@ac/web/lib/customerFilterCare";
import { markCleaned, saveReminders } from "../actions";
import type { MaintenanceLive } from "../_lib/load";

type Filters = NonNullable<MaintenanceLive["filters"]>;
/** The New request modal opened from a row: the unit, type preventive and a symptom line. */
export type CleaningPrefill = { unitId: string; symptom: string };
type Props = { f: Filters; owner: boolean; onRequest: (p: CleaningPrefill) => void; onDone: (text: string) => void; onFail: (f: ActionFailure) => void };

/** Filter care (FR-C18, DD-C18, Figma Client 07j) from the Core API: run time since the cleaning per AC, areas with many
 * ACs folded into a summary row, Request cleaning / Mark cleaned and the owner's reminder settings. */
export function FilterCareTab({ f, owner, onRequest, onDone, onFail }: Props) {
  const [open, setOpen] = useState<string[]>([]);
  const [edit, setEdit] = useState(false);
  const [pending, run] = useAction();
  const mark = (r: FilterRow) => run(() => markCleaned(r.unitId), "Marked cleaned", () => onDone(`${r.unit}: marked cleaned — the counter restarts at 0 h and your technician sees it.`), onFail);
  const unitRow = (r: FilterRow, inner = false) => (
    <tr key={r.unitId} className="border-t border-line">
      <td className={cx("px-3 py-2.5 align-middle", inner && "pl-8")}>
        <b>{r.unit}</b>
        <div className="text-[11px] text-muted">{r.place ? `${r.place} · ` : ""}{r.last}{r.jobId && <> (<Link className="text-primary hover:underline" href={`/customer/maintenance?jobId=${r.jobId}`}>{r.jobId.slice(0, 8)}</Link>)</>}</div>
      </td>
      <td className="whitespace-nowrap px-3 py-2.5">{r.run}{r.runNote && <div className="text-[11px] text-muted">{r.runNote}</div>}</td>
      <td className="px-3 py-2.5 max-sm:hidden"><Progress pct={r.pct} tone={filterStatus[r.state].bar} text={r.progress} /></td>
      <td className="px-3 py-2.5"><Badge tone={filterStatus[r.state].badge}>{filterStatus[r.state].label}</Badge></td>
      <td className="px-3 py-2.5">
        <div className="flex justify-end gap-1.5 whitespace-nowrap">
          {r.state === "overdue" && <Btn size="sm" variant="primary" onClick={() => onRequest({ unitId: r.unitId, symptom: cleaningSymptom(r) })}>Request cleaning</Btn>}
          <Btn size="sm" disabled={pending} onClick={() => mark(r)} aria-label={`Mark ${r.unit} cleaned`}>Mark cleaned</Btn>
        </div>
      </td>
    </tr>
  );
  const groupRow = (g: FilterGroup) => {
    const shown = open.includes(g.key);
    return (
      <Fragment key={g.key}>
        <tr className="border-t border-line bg-surface2/40">
          <td className="px-3 py-2.5"><b>{g.title}</b><div className="text-[11px] text-muted">{g.place}</div></td>
          <td className="whitespace-nowrap px-3 py-2.5">{g.run}</td>
          <td className="px-3 py-2.5 max-sm:hidden"><Progress pct={g.pct} tone={g.badge.tone === "crit" ? "crit" : g.badge.tone === "warn" ? "warn" : "primary"} text={g.progress} /></td>
          <td className="px-3 py-2.5"><Badge tone={g.badge.tone}>{g.badge.label}</Badge></td>
          <td className="px-3 py-2.5 text-right">
            <button type="button" aria-expanded={shown} className="text-[13px] font-semibold text-primary hover:underline" onClick={() => setOpen((s) => (shown ? s.filter((k) => k !== g.key) : [...s, g.key]))}>{shown ? "Hide" : `View ${g.rows.length}`}</button>
          </td>
        </tr>
        {shown && g.rows.map((r) => unitRow(r, true))}
      </Fragment>
    );
  };
  return (
    <div className="grid grid-cols-1 gap-4 xl:grid-cols-[minmax(0,1fr)_minmax(0,360px)]">
      <Card title="Filters by AC" sub={`Run time since the last cleaning, from telemetry · reminder at ${f.settings.thresholdHours ?? `${DEFAULT_FILTER_HOURS} h (model default)`}${f.settings.thresholdHours ? " h" : ""}`}>
        <div className="scroll-x rounded-xl border border-line">
          <table className="w-full border-collapse text-left text-[13px]">
            <thead>
              <tr className="bg-surface2 text-[11px] uppercase tracking-wide text-muted">
                <th className="px-3 py-2 font-semibold">AC</th><th className="px-3 py-2 font-semibold">Run time</th><th className="px-3 py-2 font-semibold max-sm:hidden">Progress</th>
                <th className="px-3 py-2 font-semibold">Status</th><th className="px-3 py-2"><span className="sr-only">Actions</span></th>
              </tr>
            </thead>
            <tbody>
              {f.lines.map((l) => (l.kind === "unit" ? unitRow(l.row) : groupRow(l.group)))}
              {f.lines.length === 0 && <tr><td colSpan={5} className="px-3 py-6 text-center text-muted">No ACs in your account yet.</td></tr>}
            </tbody>
          </table>
        </div>
        <p className="mt-2 text-[11px] text-muted">Run time counts hours the AC was running (from telemetry). “Mark cleaned” is for cleaning you did yourself — it resets the counter and is shown to your technician. A technician visit that cleans the filter resets it too.</p>
      </Card>
      <Card title="Reminders" sub={f.settings.version === 0 ? "Default settings" : "Set by the account owner"} className="self-start">
        <SummaryList items={f.reminders} />
        <div className="mt-3 flex flex-wrap items-center gap-2">
          <Btn size="sm" disabled={!owner} onClick={() => setEdit(true)}>Edit reminders</Btn>
          {!owner && <span className="text-xs text-warn">Only the account owner can change the reminders.</span>}
        </div>
        <div className="mt-3"><Banner>Dirty filters raise power use by up to 15 % and can trigger the “Filter pressure” alert.</Banner></div>
      </Card>
      {edit && <ReminderModal init={f.form} onClose={(text) => { setEdit(false); if (text) onDone(text); }} onFail={(x) => { setEdit(false); onFail(x); }} />}
    </div>
  );
}

function Progress({ pct, tone, text }: { pct: number | null; tone: "ok" | "warn" | "crit" | "unknown" | "primary" | "muted"; text: string }) {
  return <div className="w-36"><UtilBar pct={pct} tone={tone} /><div className="mt-1 text-[11px] text-muted">{text}</div></div>;
}

function ReminderModal({ init, onClose, onFail }: { init: ReminderForm; onClose: (text?: string) => void; onFail: (f: ActionFailure) => void }) {
  const [form, setForm] = useState(init);
  const [tried, setTried] = useState(false);
  const [pending, run] = useAction();
  const err = reminderErrors(form);
  const set = (p: Partial<ReminderForm>) => setForm((s) => ({ ...s, ...p }));
  const save = () => {
    setTried(true);
    if (err.hours || err.days) return;
    run(() => saveReminders(reminderInput(form)), "Reminders saved", () => onClose("Reminder settings saved."), onFail);
  };
  return (
    <Modal open onClose={() => onClose()} title="Edit reminders" footer={<><Btn onClick={() => onClose()}>Cancel</Btn><Btn variant="primary" disabled={pending} onClick={save}>Save reminders</Btn></>}>
      <Field label="Remind at" error={tried ? err.hours : undefined} hint="Hours the AC runs before the filter needs cleaning">
        <div className="flex flex-wrap items-center gap-2">
          <Choice value={form.useDefault ? "default" : "custom"} onChange={(v) => set({ useDefault: v === "default" })} options={[{ id: "default", label: `Model default (${DEFAULT_FILTER_HOURS} h)` }, { id: "custom", label: "Custom" }]} />
          {!form.useDefault && <><Input aria-label="Hours of running" inputMode="numeric" className="w-24" value={form.hours} onChange={(e) => set({ hours: e.target.value })} /><span className="text-[13px] font-normal">h (50–2000)</span></>}
        </div>
      </Field>
      <Field label="Also remind every (days, when run time is unknown)" error={tried ? err.days : undefined} hint="7–180 days since the last cleaning — used while an AC is offline or has no power readings">
        <Input inputMode="numeric" className="w-24" value={form.days} onChange={(e) => set({ days: e.target.value })} />
      </Field>
      <Field label="Who">
        <Choice value={form.recipients} onChange={(v) => set({ recipients: v })} options={[{ id: "owners", label: "Owners of the location" }, { id: "all_users", label: "All users of the location" }]} />
      </Field>
      <Field label="Send by">
        <div className="flex flex-col gap-1.5 font-normal">
          <Check label="App notification (always on)" checked disabled />
          <Check label="E-mail" checked={form.email} onChange={(v) => set({ email: v })} />
        </div>
      </Field>
      <Banner>When an AC reaches the reminder you get a “Filter cleaning due” maintenance alert by the channels above.</Banner>
    </Modal>
  );
}
