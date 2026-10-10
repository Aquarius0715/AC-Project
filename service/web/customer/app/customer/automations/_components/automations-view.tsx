"use client";

import { useEffect, useState } from "react";
import { Badge, Banner, Btn, Card, cx, EmptyState, Field, Input, Modal, Page, PageHead, Select, Toggle } from "@ac/web/components/ui";
import { useAction } from "@ac/web/lib/useAction";
import { useOp } from "@ac/web/lib/useOp";
import { useUrlPatch } from "@ac/web/lib/useUrlPatch";
import {
  actionLabel, actionOf, actionOptions, apiErrors, compares, compareText, dayLong, dayShort, draftErrors, draftOf, eventTest, extraChoices, extraLabel, fitExtras, newDraft, newExtra, runClock, runText,
  saveInput, scheduleDraft, scheduleTest, summaryNote, summaryText, testFact, triggerInfo, triggers,
  type ApiAutomation, type ApiOccurrence, type Caps, type Draft, type ExtraDraft, type RuleCard, type TestRow,
} from "@ac/web/lib/clientAutomations";
import { useI18n, useT } from "@ac/web/components/I18n";
import { deleteAutomation, saveAutomation, testEvent, updateConsent } from "../actions";

type Ac = { id: string; name: string; property: string; path: string };
export type AutomationsLive = {
  rules: ApiAutomation[]; cards: RuleCard[]; acs: Ac[]; caps: Record<string, Caps | null>; timezone: string; editing: string | null;
  consent: { version: number | null; granted: boolean; title: string; text: string; tone: "ok" | "warn" | "muted" }; nextRuns: Record<string, string | null>;
};
type Notice = { title: string; text: string };

/** Automations & schedules (FR-C04/C05) in API mode: the rule list with location consent, and the editor at
 * ?automationId=<id|new>. Saving never sends a Command; the server runs schedules and events (IR54). */
export function AutomationsView({ live }: { live: AutomationsLive }) {
  const patch = useUrlPatch();
  const [notice, setNotice] = useState<Notice | null>(null);
  if (live.editing) {
    return <Editor key={live.editing} live={live} onDone={(n) => { setNotice(n); patch({ automationId: null }); }} onCancel={() => patch({ automationId: null })} />;
  }
  return <List live={live} notice={notice} setNotice={setNotice} create={() => { setNotice(null); patch({ automationId: "new" }); }} edit={(id) => { setNotice(null); patch({ automationId: id }); }} />;
}

function ConsentCard({ live, compact }: { live: AutomationsLive; compact?: boolean }) {
  const t = useT();
  const [pending, run] = useAction();
  const c = live.consent;
  const flip = () => c.version !== null && run(() => updateConsent(!c.granted, c.version!), t(c.granted ? "Location consent withdrawn — location automations are off" : "Location consent granted"));
  const badge = <Badge tone={c.tone === "ok" ? "ok" : c.tone === "warn" ? "warn" : "muted"} icon={c.granted ? "✓" : c.tone === "warn" ? "⊘" : "○"}>{c.title}</Badge>;
  const action = c.version !== null && <Btn size="sm" variant={c.granted ? "secondary" : "primary"} disabled={pending} onClick={flip}>{t(c.granted ? (compact ? "Withdraw" : "Withdraw consent") : (compact ? "Grant" : "Grant consent"))}</Btn>;
  return (
    <div className={cx("rounded-2xl border p-4", c.tone === "warn" ? "border-crit bg-warn-soft/60" : compact ? "border-line bg-primary-soft/40" : "border-line bg-surface")}>
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div className="flex flex-wrap items-center gap-2 text-[13px]"><b>{t(compact ? "Location consent" : "⌖ Location consent — for arrival/departure automations only")}</b>{badge}</div>
        {action}
      </div>
      <p className="mt-1.5 text-xs text-muted">{compact ? t("Required for this automation. Used only for arrival/departure automations; phase 1A uses predefined demo events — no real GPS history is collected.") : c.text}</p>
    </div>
  );
}

function List({ live, notice, setNotice, create, edit }: { live: AutomationsLive; notice: Notice | null; setNotice: (n: Notice | null) => void; create: () => void; edit: (id: string) => void }) {
  const t = useT();
  const [pending, run] = useAction();
  const [menu, setMenu] = useState<string | null>(null);
  const [del, setDel] = useState<RuleCard | null>(null);
  const on = live.cards.filter((c) => c.enabled).length;
  const rule = (id: string) => live.rules.find((r) => r.id === id)!;
  const toggle = (c: RuleCard) => run(() => saveAutomation({ ...saveInput(draftOf(rule(c.id))), enabled: !c.enabled }, c.version), t(c.enabled ? "“{name}” switched off" : "“{name}” switched on", { name: c.name }));
  const remove = (c: RuleCard) => run(() => deleteAutomation(c.id, c.version), t("“{name}” deleted", { name: c.name }), () => {
    setDel(null);
    setNotice({ title: t("Automation deleted"), text: t("“{name}” will no longer run. Other automations and manual control are unchanged.", { name: c.name }) });
  });
  return (
    <Page>
      <PageHead title={t("Your automations")} sub={t(live.cards.length === 1 ? "{n} automation · {on} on · each rule controls one AC · checked again when it runs" : "{n} automations · {on} on · each rule controls one AC · checked again when it runs", { n: live.cards.length, on })} action={<Btn variant="primary" onClick={create}>{t("+ Create automation")}</Btn>} />
      {notice && <Banner tone="ok" action={<Btn size="sm" variant="ghost" onClick={() => setNotice(null)}>{t("Dismiss")}</Btn>}><b>{notice.title}</b><div className="text-xs text-muted">{notice.text}</div></Banner>}
      <ConsentCard live={live} />
      {live.cards.length === 0 ? <EmptyState title={t("No automations yet")} action={<Btn size="sm" onClick={create}>{t("+ Create automation")}</Btn>}>{t("Create one to pre-cool before you arrive or to stop an empty room. Saving sends no command — the automation runs at its time or event.")}</EmptyState> : (
        <div className="flex flex-col gap-3">
          {live.cards.map((c) => (
            <Card key={c.id} className={cx(c.status.tone === "warn" && "border-crit")}>
              <div className="flex flex-wrap items-start justify-between gap-3">
                <div className="min-w-0 flex-1">
                  <div className="flex flex-wrap items-center gap-2"><b className="text-[15px]">{c.name}</b><Badge tone="primary">{triggerInfo[c.trigger].icon} {t(triggerInfo[c.trigger].label)}</Badge><Badge tone={c.status.tone} icon={c.status.tone === "ok" ? "●" : c.status.tone === "warn" ? "⊘" : "○"}>{c.status.text}</Badge></div>
                  <p className="mt-1 text-[13px]"><span className="text-muted">{t("When")}</span> {c.when}</p>
                  <p className="text-[13px]"><span className="text-muted">{t("Then")}</span> {c.then}</p>
                  <p className="text-xs text-muted">{c.place}</p>
                  {c.note && <p className="mt-1 text-xs font-semibold text-crit">⊘ {c.note}</p>}
                  {c.skipped && <p className="mt-1 text-xs text-muted">ⓘ {c.skipped}</p>}
                </div>
                <div className="relative flex flex-col items-end gap-2">
                  <span className="flex items-center gap-2 text-xs font-semibold">
                    <span className={c.enabled ? "text-ok" : c.status.tone === "warn" ? "text-warn" : "text-muted"}>{t(c.enabled ? "On" : c.status.tone === "warn" ? "Disabled" : "Off")}</span>
                    <span title={c.toggle.hint ?? undefined}><Toggle on={c.enabled} disabled={pending || !c.toggle.allowed} onChange={() => toggle(c)} label={t(c.enabled ? "Switch {name} off" : "Switch {name} on", { name: c.name })} /></span>
                  </span>
                  <span className="flex gap-2">
                    <Btn size="sm" onClick={() => edit(c.id)}>{t("Edit")}</Btn>
                    <Btn size="sm" variant="ghost" aria-label={t("More for {name}", { name: c.name })} aria-expanded={menu === c.id} onClick={() => setMenu(menu === c.id ? null : c.id)}>⋯</Btn>
                  </span>
                  {menu === c.id && (
                    <div className="absolute right-0 top-full z-10 mt-1 w-40 rounded-xl border border-line bg-surface p-1 shadow-lg">
                      <button type="button" className="w-full rounded-lg px-3 py-2 text-left text-[13px] font-semibold text-crit hover:bg-crit-soft" onClick={() => { setDel(c); setMenu(null); }}>{t("🗑 Delete")}</button>
                    </div>
                  )}
                </div>
              </div>
            </Card>
          ))}
        </div>
      )}
      <p className="text-xs text-muted">{t("Priority when rules overlap: device capabilities & active restrictions → HQ policy → your automations. List order is not run priority. Missing condition data never counts as a match.")}</p>
      <Modal open={!!del} onClose={() => setDel(null)} title={t("Delete automation?")} footer={<><Btn onClick={() => setDel(null)}>{t("Cancel")}</Btn><Btn variant="danger" disabled={pending} onClick={() => del && remove(del)}>{t("Delete automation")}</Btn></>}>
        <p className="text-[13px]">{t("“{name}” will no longer run.", { name: del?.name ?? "" })}</p>
        <p className="text-xs text-muted">{del && `${del.place.split(t(" · next run {when}", { when: "" }).trimEnd())[0]} · ${t(triggerInfo[del.trigger].label)} · ${del.when}.`} {t("Manual control and other automations are not affected. Commands it already sent stay in the history.")}</p>
      </Modal>
    </Page>
  );
}

/** Debounces a value (the schedule preview reads after typing pauses). */
function useDebounced<T>(value: T, ms: number): T {
  const [v, setV] = useState(value);
  useEffect(() => {
    const t = setTimeout(() => setV(value), ms);
    return () => clearTimeout(t);
  }, [value, ms]);
  return v;
}

function Editor({ live, onDone, onCancel }: { live: AutomationsLive; onDone: (n: Notice) => void; onCancel: () => void }) {
  const i18n = useI18n();
  const { t } = i18n;
  const rule = live.editing === "new" ? null : live.rules.find((r) => r.id === live.editing) ?? null;
  const [draft, setDraft] = useState<Draft>(() => (rule ? draftOf(rule) : newDraft(live.acs[0]?.id ?? "", live.timezone)));
  const [apiErr, setApiErr] = useState<Record<string, string>>({});
  const [shown, setShown] = useState(false); // client checks shown after the first Save / Test
  const [test, setTest] = useState<TestRow[] | null>(null);
  const [pending, run] = useAction();
  const set = (p: Partial<Draft>) => { setDraft((d) => ({ ...d, ...p })); setApiErr({}); };
  const local = draftErrors(draft, t);
  const errors = { ...(shown ? local : {}), ...apiErr };
  const sched = draft.trigger === "schedule";
  const scheduleOk = sched && !local.weekdays && !local.startLocal && !local.endLocal && !local.endsNextDay;
  const preview = useOp<ApiOccurrence[], ApiOccurrence[]>("automations.nextRuns", { draft: scheduleDraft(useDebounced(draft, 400)), count: 8 }, [], (d) => d, scheduleOk);
  if (live.editing !== "new" && !rule) {
    return <Page><EmptyState title={t("This automation no longer exists")} action={<Btn size="sm" onClick={onCancel}>{t("← Automations")}</Btn>}>{t("It may have been deleted on another device.")}</EmptyState></Page>;
  }
  const ac = live.acs.find((u) => u.id === draft.unitId);
  const options = actionOptions(live.caps[draft.unitId] ?? null, t);
  const supported = (k: string) => options.some((g) => g.options.some((o) => o.key === k));
  const unsupported = (k: string) => (draft.unitId && !supported(k) ? t("Not supported by this AC — choose another action") : undefined);
  const dirty = !rule || JSON.stringify(saveInput(draft)) !== JSON.stringify(saveInput(draftOf(rule)));
  const runsShown = preview.data.filter((o) => o.phase === "schedule_start").slice(0, 2).map((s) => ({ start: s, end: preview.data.find((o) => o.phase === "schedule_end" && o.at > s.at) }));
  const previewError = preview.error ? apiErrors(preview.error.error.fieldErrors, t) : {};
  const valid = Object.keys(local).length === 0 && ![draft.startAction, draft.endAction, draft.action].some((k, i) => (sched ? i < 2 : i === 2) && unsupported(k));
  const save = () => {
    setShown(true);
    if (!valid) return;
    run(() => saveAutomation(saveInput(draft), draft.version), t("Automation saved"), (a) => {
      const next = a.kind === "schedule" && a.enabled ? preview.data.find((o) => o.phase === "schedule_start") : undefined;
      onDone({ title: t("Automation saved"), text: next ? t("“{name}” saved · next run {when} ({zone}). Saving sends no command now.", { name: a.name, when: runText(next.at, a.timezone, i18n), zone: a.timezone })
        : t(a.enabled ? "“{name}” saved · on. Saving sends no command now." : "“{name}” saved · off — switch it on to run it. Saving sends no command now.", { name: a.name }) });
    }, (f) => setApiErr(apiErrors(f.fieldErrors, t)));
  };
  const runTest = () => {
    setShown(true);
    if (sched) {
      if (scheduleOk) setTest(scheduleTest(preview.data, draft, ac?.name ?? t("the AC"), i18n));
      return;
    }
    if (!rule) return;
    run(() => testEvent(draft.unitId, testFact(draft)), t("Simulated — no command was sent"), (r) =>
      setTest([eventTest(r.decision ?? undefined, rule.id, new Map(live.rules.map((x) => [x.id, x.name])), ac?.name ?? t("the AC"), actionOf(draft.action), r.at, i18n)]));
  };
  const eventTestHint = sched ? null : draft.trigger === "routine" ? t("A routine runs at its usual time — try it with Demo controls") : dirty ? t("Save first — the test runs your saved automations for this AC") : null;
  const actionSelect = (label: string, key: "startAction" | "endAction" | "action", hint?: string) => (
    <Field label={label} hint={hint} error={errors[key] ?? (shown ? unsupported(draft[key]) : undefined)}>
      <Select value={draft[key]} onChange={(e) => set({ [key]: e.target.value } as Partial<Draft>)}>
        {!supported(draft[key]) && <option value={draft[key]}>{t("{action} (not supported)", { action: actionLabel(actionOf(draft[key]), t) })}</option>}
        {options.map((g) => <optgroup key={g.group} label={g.group}>{g.options.map((o) => <option key={o.key} value={o.key}>{o.label}</option>)}</optgroup>)}
      </Select>
    </Field>
  );
  const step = (n: number, title: string, sub?: string) => <div className="flex items-start gap-2"><span className="mt-0.5 grid h-5 w-5 shrink-0 place-items-center rounded-full bg-ink text-[11px] font-bold text-white">{n}</span><div><div className="text-[15px] font-bold">{title}</div>{sub && <div className="text-xs text-muted">{sub}</div>}</div></div>;
  return (
    <Page>
      <div className="text-[13px] text-muted"><button type="button" className="font-semibold text-primary" onClick={onCancel}>{t("← Automations")}</button> › <b className="text-ink">{t(rule ? "Edit automation" : "Create automation")}</b></div>
      <div className="split">
        <Card>
          <div className="flex flex-col gap-4">
            <Field label={t("Name")} error={errors.name}><Input value={draft.name} maxLength={120} placeholder={t("e.g. Weekday pre-cool")} onChange={(e) => set({ name: e.target.value })} /></Field>
            {step(1, t("When …"), rule ? undefined : t("Choose what starts the automation"))}
            <div className="grid-fluid" style={{ ["--min" as string]: "150px" }} role="radiogroup" aria-label={t("Trigger")}>
              {triggers.map((tr) => (
                <button key={tr} type="button" role="radio" aria-checked={draft.trigger === tr} disabled={!!rule && (tr === "schedule") !== sched} onClick={() => set({ trigger: tr, extras: fitExtras({ ...draft, trigger: tr }) })}
                  className={cx("rounded-xl border px-3 py-2 text-left disabled:cursor-not-allowed disabled:opacity-50", draft.trigger === tr ? "border-primary bg-primary-soft text-primary ring-1 ring-primary" : "border-line bg-surface hover:bg-surface2")}>
                  <div className="text-[13px] font-semibold">{triggerInfo[tr].icon} {t(triggerInfo[tr].label)}</div><div className="text-[11px] text-muted">{t(triggerInfo[tr].sub)}</div>
                </button>
              ))}
            </div>
            {!!rule && <p className="-mt-2 text-[11px] text-muted">{t("A saved schedule stays a schedule and an event rule stays an event rule — create a new automation to switch.")}</p>}
            {sched && (
              <>
                <Field label={t("Weekdays (start day)")} error={errors.weekdays}>
                  <div className="flex flex-wrap gap-1.5">
                    {dayLong.map((d, k) => {
                      const on = draft.weekdays.includes(k + 1);
                      return <button key={d} type="button" aria-pressed={on} onClick={() => set({ weekdays: on ? draft.weekdays.filter((x) => x !== k + 1) : [...draft.weekdays, k + 1] })}
                        className={cx("rounded-full border px-3 py-1 text-[13px] font-semibold", on ? "border-primary bg-primary-soft text-primary" : "border-line bg-surface")}>{t(dayShort[k])}</button>;
                    })}
                  </div>
                </Field>
                <div className="grid grid-cols-[repeat(auto-fit,minmax(140px,1fr))] gap-3">
                  <Field label={t("Start")} hint={t("24-hour HH:mm")} error={errors.startLocal}><Input inputMode="numeric" maxLength={5} placeholder="HH:mm" value={draft.startLocal} onChange={(e) => set({ startLocal: e.target.value })} /></Field>
                  <Field label={t("End")} hint={t("24-hour HH:mm")} error={errors.endLocal}><Input inputMode="numeric" maxLength={5} placeholder="HH:mm" value={draft.endLocal} onChange={(e) => set({ endLocal: e.target.value })} /></Field>
                  <Field label={t("Time zone")} hint={t("From your preferences")}><Input value={draft.timezone} readOnly className="bg-surface2" /></Field>
                </div>
                <label className="flex items-center gap-2 text-xs text-muted"><input type="checkbox" checked={draft.endsNextDay} onChange={(e) => set({ endsNextDay: e.target.checked })} />{t("Ends next day (overnight) — off: end must be later than start; equal start/end is invalid")}</label>
                {errors.endsNextDay && <p className="-mt-2 text-xs font-medium text-crit">✕ {errors.endsNextDay}</p>}
              </>
            )}
            {draft.trigger === "presence" && (
              <Field label={t("Room")}><Select value={draft.occupied ? "occupied" : "empty"} onChange={(e) => set({ occupied: e.target.value === "occupied" })}><option value="empty">{t("No one detected (empty)")}</option><option value="occupied">{t("Someone is in the room")}</option></Select></Field>
            )}
            {draft.trigger === "location" && (
              <>
                <div className="grid grid-cols-[repeat(auto-fit,minmax(200px,1fr))] gap-3">
                  <Field label={t("Event")}><Select value={draft.event} onChange={(e) => set({ event: e.target.value as Draft["event"] })}><option value="arrival">{t("Everyone arrives")}</option><option value="departure">{t("Everyone leaves")}</option></Select></Field>
                  <Field label={t("Place")} hint={t("Where the target AC is")}><Input value={ac?.property ?? "—"} readOnly className="bg-surface2" /></Field>
                </div>
                {errors.condition && <p className="text-xs font-medium text-crit">✕ {errors.condition}</p>}
                <ConsentCard live={live} compact />
              </>
            )}
            {draft.trigger === "routine" && (
              <Field label={t("Your usual time")} hint={t("Demo routine — no real behaviour history is collected")} error={errors.localTime}><Input inputMode="numeric" maxLength={5} placeholder="HH:mm" value={draft.localTime} onChange={(e) => set({ localTime: e.target.value })} className="w-40" /></Field>
            )}
            {draft.trigger === "weather" && (
              <div className="grid grid-cols-[repeat(auto-fit,minmax(160px,1fr))] gap-3">
                <Field label={t("Outdoor temperature")}><Select value={draft.operator} onChange={(e) => set({ operator: e.target.value as Draft["operator"] })}>{compares.map((c) => <option key={c} value={c}>{compareText(c)}</option>)}</Select></Field>
                <Field label={t("°C (demo weather)")} error={errors.value}><Input inputMode="decimal" value={draft.value} onChange={(e) => set({ value: e.target.value })} /></Field>
              </div>
            )}
            {step(2, t("Only if … (optional)"), t("Extra conditions — all must be true"))}
            <OnlyIf draft={draft} errors={errors} set={set} />
            {step(3, t("Then …"), t("Action for one AC"))}
            <Field label={t("Target AC")} error={errors.unitId}>
              <Select value={draft.unitId} onChange={(e) => set({ unitId: e.target.value })}>
                {live.acs.length === 0 && <option value="">{t("No AC available")}</option>}
                {live.acs.map((u) => <option key={u.id} value={u.id}>{u.name} · {u.path}</option>)}
              </Select>
            </Field>
            {sched ? (
              <div className="grid grid-cols-[repeat(auto-fit,minmax(220px,1fr))] gap-3">
                {actionSelect(t("Start action (at {time})", { time: draft.startLocal }), "startAction")}
                {actionSelect(t("End action (at {time}) — required", { time: draft.endLocal }), "endAction", t("Never assumed — choose explicitly."))}
              </div>
            ) : actionSelect(t("Action"), "action")}
          </div>
        </Card>
        <div className="flex min-w-0 flex-col gap-4">
          <Card className="bg-primary-soft/40">
            <div className="text-[11px] font-bold uppercase tracking-wide text-muted">{t("Summary")}</div>
            <p className="mt-1 text-[13px] font-bold">{summaryText(draft, ac?.name ?? t("the AC"), ac?.property ?? t("the AC's place"), t)}</p>
            <p className="mt-1 text-[11px] text-muted">{t(summaryNote[draft.trigger])}</p>
            {sched && (
              <div className="mt-3 grid grid-cols-[90px_1fr] gap-2 border-t border-line pt-2 text-xs">
                <span className="text-muted">{t("Next runs")}</span>
                <span>{!scheduleOk ? t("Fix the schedule to preview") : preview.loading ? t("Checking…") : Object.keys(previewError).length ? <span className="text-crit">{Object.values(previewError)[0]}</span>
                  : runsShown.length === 0 ? t("None in the next weeks") : runsShown.map((r) => <span key={r.start.at} className="block">{runText(r.start.at, draft.timezone, i18n)}{r.end ? ` · ${runClock(r.end.at, draft.timezone, i18n)}` : ""}</span>)}</span>
              </div>
            )}
            <div className="mt-2 flex items-center justify-between gap-2 border-t border-line pt-2 text-xs">
              <span className="text-muted">{t("Status")}</span>
              <span className="flex items-center gap-2 font-semibold">{t(draft.enabled ? "Enabled on save" : "Off on save")}<Toggle on={draft.enabled} onChange={(v) => set({ enabled: v })} label={t("Enabled on save")} /></span>
            </div>
          </Card>
          <Card title={t("Review & test")} sub={t("Simulate an event to see what would happen. Test sends no command.")}>
            <div className="flex flex-col gap-2">
              <Btn onClick={runTest} disabled={pending || !!eventTestHint} title={eventTestHint ?? undefined}>{t("▶ Test / Simulate")}</Btn>
              {eventTestHint && <p className="text-[11px] text-muted">{eventTestHint}</p>}
              <Btn variant="primary" disabled={pending} onClick={save}>{t("Save automation")}</Btn>
              <Btn variant="ghost" onClick={onCancel}>{t("Cancel")}</Btn>
            </div>
          </Card>
        </div>
      </div>
      <Modal open={test !== null} onClose={() => setTest(null)} title={t("Test result — simulation only")} footer={<><Btn onClick={() => setTest(null)}>{t("Back to edit")}</Btn><Btn variant="primary" disabled={pending} onClick={() => { setTest(null); save(); }}>{t("Save automation")}</Btn></>}>
        <p className="text-xs text-muted"><Badge tone="primary" icon="ⓘ">{t("No command sent")}</Badge> {t("Simulated events for “{name}” ({zone}):", { name: draft.name.trim() || t("this automation"), zone: draft.timezone })}</p>
        {test?.length === 0 && <p className="text-[13px] text-muted">{t("No upcoming run in the next weeks.")}</p>}
        {test?.map((r) => (
          <div key={r.at + r.title} className={cx("rounded-xl px-3 py-2 text-[13px]", r.tone === "ok" ? "bg-ok-soft/70" : "bg-surface2")}>
            <b className={r.tone === "ok" ? "text-ok" : ""}>{r.tone === "ok" ? "✓" : "○"} {r.title}</b><div className="text-xs text-muted">{r.detail}</div>
          </div>
        ))}
        <p className="text-[11px] text-muted">{t("At run time the automation is re-checked (permissions, capabilities, restrictions).")}</p>
      </Modal>
    </Page>
  );
}

/** Step 2 “Only if …” (Figma 03b/03e, IR215): up to three extra conditions that must all hold when the rule runs. */
function OnlyIf({ draft, errors, set }: { draft: Draft; errors: Record<string, string>; set: (p: Partial<Draft>) => void }) {
  const t = useT();
  const [adding, setAdding] = useState(false);
  const extras = fitExtras(draft);
  const choices = extraChoices({ ...draft, extras });
  const put = (i: number, x: ExtraDraft) => set({ extras: extras.map((e, j) => (j === i ? x : e)) });
  return (
    <div className="flex flex-col gap-2">
      {extras.map((x, i) => (
        <div key={x.type} className="flex flex-wrap items-center gap-2 rounded-xl border border-line bg-surface2/60 px-3 py-2 text-[13px]">
          <b className="w-40 shrink-0">{t(extraLabel[x.type])}</b>
          {x.type === "occupancy" && (
            <Select className="w-auto py-1" value={x.occupied ? "yes" : "no"} onChange={(e) => put(i, { ...x, occupied: e.target.value === "yes" })}>
              <option value="yes">{t("Someone is home")}</option><option value="no">{t("No one is home")}</option>
            </Select>
          )}
          {x.type === "weather" && (
            <>
              <Select className="w-auto py-1" value={x.operator} onChange={(e) => put(i, { ...x, operator: e.target.value as typeof x.operator })}>{compares.map((c) => <option key={c} value={c}>{compareText(c)}</option>)}</Select>
              <Input className="w-24 py-1" inputMode="decimal" value={x.value} onChange={(e) => put(i, { ...x, value: e.target.value })} aria-label={t("Outdoor temperature °C")} /><span className="text-xs text-muted">{t("°C (demo weather)")}</span>
            </>
          )}
          {x.type === "weekday" && (
            <span className="flex flex-wrap gap-1">
              {dayLong.map((d, k) => {
                const on = x.weekdays.includes(k + 1);
                return <button key={d} type="button" aria-pressed={on} onClick={() => put(i, { ...x, weekdays: on ? x.weekdays.filter((w) => w !== k + 1) : [...x.weekdays, k + 1] })}
                  className={cx("rounded-full border px-2 py-0.5 text-xs font-semibold", on ? "border-primary bg-primary-soft text-primary" : "border-line bg-surface")}>{t(dayShort[k])}</button>;
              })}
            </span>
          )}
          <Btn size="sm" variant="ghost" className="ml-auto" aria-label={t("Remove {condition}", { condition: t(extraLabel[x.type]) })} onClick={() => set({ extras: extras.filter((_, j) => j !== i) })}>✕</Btn>
          {errors[`extra${i}`] && <span className="w-full text-xs font-medium text-crit">✕ {errors[`extra${i}`]}</span>}
        </div>
      ))}
      {errors.onlyIf && <p className="text-xs font-medium text-crit">✕ {errors.onlyIf}</p>}
      <div className="flex flex-wrap items-center gap-2">
        {adding && choices.length > 0 ? choices.map((c) => <Btn key={c} size="sm" onClick={() => { set({ extras: [...extras, newExtra(c)] }); setAdding(false); }}>{t(extraLabel[c])}</Btn>)
          : <Btn size="sm" disabled={choices.length === 0} onClick={() => setAdding(true)}>{t("+ Add condition")}</Btn>}
        <span className="text-xs text-muted">{t(choices.length === 0 && extras.length > 0 ? "No more condition types for this trigger" : "e.g. only if someone is home · only if outdoor ≥ 30°C")}</span>
      </div>
    </div>
  );
}
