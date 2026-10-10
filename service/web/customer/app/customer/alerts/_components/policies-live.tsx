"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useState } from "react";
import { Badge, Banner, Btn, Card, Check, Choice, Field, Input, Modal, Select, SeverityBadge, Toggle, cx } from "@ac/web/components/ui";
import { useAction } from "@ac/web/lib/useAction";
import type { ActionFailure } from "@ac/web/lib/actionMessage";
import { metricUnit, opSymbol, type Operator, type Severity } from "@ac/web/lib/adminAlerts";
import {
  metricGroups, metricOfGroup, policyErrors, policyInput, summaryText, toggledInput, type PolicyCard, type PolicyForm,
} from "@ac/web/lib/customerPolicies";
import { useT } from "@ac/web/components/I18n";
import { deletePolicy, savePolicy, setDefaultRule } from "../actions";
import type { AlertsLive } from "../_lib/load";

type Live = AlertsLive["policies"];
type Props = { p: Live; onDone: (text: string) => void; onFail: (f: ActionFailure) => void };
const base = "/customer/alerts?tab=policies";

/** The Alert policies tab (FR-C15, DD-C15, Figma Client 06e/06f) from the Core API: the default policy's rules switched
 * for the customer by the owner, the customer's own policies, and the editor (policyId in the URL). */
export function PoliciesLive({ p, onDone, onFail }: Props) {
  return p.editor ? <PolicyEditor p={p} editor={p.editor} onDone={onDone} onFail={onFail} /> : <PolicyList p={p} onDone={onDone} onFail={onFail} />;
}

function PolicyList({ p, onDone, onFail }: Props) {
  const t = useT();
  const [pending, run] = useAction();
  const [remove, setRemove] = useState<PolicyCard | null>(null);
  const d = p.defaults;
  const fail = (f: ActionFailure) => onFail(f);
  const toggleRule = (ruleKey: string, name: string, on: boolean, version: number) =>
    d && p.customerId && run(() => setDefaultRule(d.id, ruleKey, p.customerId!, on, version), t(on ? "“{name}” on" : "“{name}” off", { name }), () => onDone(t(on ? "Default rule “{name}” switched on for all your ACs." : "Default rule “{name}” switched off for all your ACs.", { name })), fail);
  return (
    <>
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div><h2 className="text-[15px] font-bold">{t("Alert policies")}</h2><p className="max-w-xl text-xs text-muted">{t("A policy is a set of limits. It only runs on the ACs you attach it to (open an AC › Alert policies). Every AC also has the default policy.")}</p></div>
        <Link href={`${base}&policyId=new`} className="inline-flex items-center rounded-control border border-primary bg-primary px-3.5 py-2 text-[13px] font-semibold text-white">{t("+ Create policy")}</Link>
      </div>
      {d && (
        <Card title={<span className="flex flex-wrap items-center gap-2">{t("Default policy")} <Badge tone="primary">{t("Default · set by HQ")}</Badge><Badge tone="primary">{t("{on} of {n} rules on", { on: d.on, n: d.rows.length })}</Badge></span>} action={<span className="text-[11px] text-muted">{t("Limits set by HQ · on / off is set by the account owner")}</span>}>
          <p className="mb-3 text-xs text-muted">{t("Covers ventilation and the usual causes of breakdowns. It is attached to every AC and cannot be detached or edited, but the account owner can switch each rule on or off for your account (all your ACs); members see the rules read-only. Ask HQ if a limit should change.")}</p>
          {!p.owner && <p className="mb-3 text-xs text-warn">{t("Only the account owner can change this.")}</p>}
          <div className="scroll-x"><table className="w-full text-left text-[13px]">
            <thead className="text-[11px] uppercase text-muted"><tr><th className="py-1.5 pr-3">{t("Rule")}</th><th className="pr-3">{t("Condition")}</th><th className="pr-3">{t("Type")}</th><th className="pr-3">{t("Severity")}</th><th>{t("On")}</th></tr></thead>
            <tbody>{d.rows.map((r) => (
              <tr key={r.ruleKey} className={cx("border-t border-line", !r.enabled && "text-muted")}>
                <td className="py-2 pr-3 font-semibold">{r.name}</td><td className="pr-3">{r.condition}</td><td className="pr-3"><Badge tone={r.category === "air_quality" ? "primary" : "muted"}>{r.type}</Badge></td>
                <td className="pr-3"><SeverityBadge s={r.severity} /></td>
                <td><Toggle on={r.enabled} disabled={!p.owner || pending} onChange={(v) => toggleRule(r.ruleKey, r.name, v, r.version)} label={r.name} /></td>
              </tr>
            ))}</tbody>
          </table></div>
          {d.notes.map((n) => <p key={n} className="mt-2 text-xs text-muted">{n}</p>)}
        </Card>
      )}
      <div className="text-[13px] font-bold">{t("Your policies")} <span className="text-xs font-normal text-muted">{t("{n} · added on top of the default policy · a policy belongs to your account and can be attached to many ACs", { n: p.cards.length })}</span></div>
      {p.cards.map((c) => (
        <Card key={c.id} title={<span className="flex flex-wrap items-center gap-2">{c.name}<Badge tone="muted">{c.badge}</Badge><Badge tone={c.enabled ? "ok" : "muted"}>{t(c.enabled ? "On" : "Off")}</Badge></span>}
          action={<span className="flex items-center gap-2 text-xs text-muted">{t(c.enabled ? "On" : "Off")}<Toggle on={c.enabled} disabled={pending} label={c.name} onChange={(v) => {
            const pol = p.list.find((x) => x.id === c.id)!;
            run(() => savePolicy(toggledInput(pol, v), pol.version), t(v ? "Policy on" : "Policy off"), () => onDone(t(v ? "“{name}” switched on." : "“{name}” switched off.", { name: c.name })), fail);
          }} /></span>}>
          <div className="flex flex-wrap items-end justify-between gap-3">
            <div className="text-[13px]">
              <p><span className="text-muted">{t("When")}</span> {c.when}</p><p><span className="text-muted">{t("Then")}</span> {c.then}</p>
              <p className={cx("mt-1 text-xs", c.attached.length ? "text-muted" : "text-warn")}>{c.attachedText}</p>
              {c.note && <p className="text-xs text-warn">{c.note}</p>}
            </div>
            <div className="flex gap-2"><Link href={`${base}&policyId=${c.id}`} className="inline-flex items-center rounded-control border border-line bg-surface px-3 py-1.5 text-xs font-semibold hover:bg-surface2">{t("Edit")}</Link><Btn size="sm" variant="danger" onClick={() => setRemove(c)}>{t("Delete")}</Btn></div>
          </div>
        </Card>
      ))}
      {p.cards.length === 0 && <p className="rounded-xl border border-line bg-surface p-4 text-center text-xs text-muted">{t("No policies of your own yet — “+ Create policy” adds one; attach it on an AC’s page.")}</p>}
      <p className="text-xs text-muted">{t("A unit can carry several policies; each rule is checked separately. Missing or stale readings never raise an alert — they show “not measured”. Air-quality limits (CO₂ ppm, PM2.5 µg/m³) are regular alert policies.")}</p>
      {remove && <DeleteModal c={remove} version={p.list.find((x) => x.id === remove.id)?.version ?? 0} onClose={(text) => { setRemove(null); if (text) onDone(text); }} onFail={(f) => { setRemove(null); onFail(f); }} />}
    </>
  );
}

function DeleteModal({ c, version, onClose, onFail }: { c: PolicyCard; version: number; onClose: (text?: string) => void; onFail: (f: ActionFailure) => void }) {
  const t = useT();
  const [pending, run] = useAction();
  const router = useRouter();
  const n = c.attached.length;
  return (
    <Modal open onClose={() => onClose()} title={t("Delete “{name}”?", { name: c.name })} footer={<><Btn onClick={() => onClose()}>{t("Keep policy")}</Btn><Btn variant="danger" disabled={pending} onClick={() => run(() => deletePolicy(c.id, version), t("Policy deleted"), () => { onClose(n ? t("“{name}” deleted and detached from {units}.", { name: c.name, units: c.attached.join(", ") }) : t("“{name}” deleted.", { name: c.name })); router.replace(base, { scroll: false }); }, onFail)}>{t("Delete policy")}</Btn></>}>
      <p className="text-[13px]">{n ? t(n === 1 ? "It is detached from {n} AC first: {units}." : "It is detached from {n} ACs first: {units}.", { n, units: c.attached.join(", ") }) : t("It is not attached to any AC.")} {t("The default policy can’t be deleted.")}</p>
    </Modal>
  );
}

const ops: Operator[] = ["gte", "gt", "lte", "lt"];
const days = ["Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"];

function PolicyEditor({ p, editor, onDone, onFail }: Props & { editor: NonNullable<Live["editor"]> }) {
  const t = useT();
  const router = useRouter();
  const [pending, run] = useAction();
  const [f, setF] = useState<PolicyForm>(editor.form);
  const [tried, setTried] = useState(false);
  const [remove, setRemove] = useState(false);
  const err = policyErrors(f, t);
  const set = (x: Partial<PolicyForm>) => setF((s) => ({ ...s, ...x }));
  const unit = metricUnit[f.metric] ?? "";
  const up = f.operator === "gt" || f.operator === "gte";
  const save = () => {
    setTried(true);
    if (Object.keys(err).length || !p.customerId) return;
    run(() => savePolicy(policyInput(f, editor.policy, { customerId: p.customerId!, membershipId: p.membershipId, timezone: p.timezone }), editor.policy?.version ?? null), t("Policy saved"),
      () => { onDone(t(editor.policy ? "“{name}” saved." : "“{name}” saved — attach it on an AC’s page.", { name: f.name.trim() })); router.replace(base, { scroll: false }); }, onFail);
  };
  return (
    <div className="grid grid-cols-1 gap-4 xl:grid-cols-[minmax(0,1fr)_minmax(0,320px)]">
      <Card>
        <div className="mb-3 text-[13px]"><Link href={base} className="font-semibold text-primary">{t("← Alert policies")}</Link> › {t(editor.policy ? "Edit alert policy" : "Create alert policy")}</div>
        <Field label={t("Name")} error={tried ? err.name : undefined}><Input value={f.name} maxLength={120} onChange={(e) => set({ name: e.target.value })} /></Field>
        <h3 className="mt-4 text-[13px] font-bold">{t("1 · When …")} <span className="font-normal text-muted">{t("What to watch")}</span></h3>
        <div className="mt-2 grid grid-cols-2 gap-2 sm:grid-cols-4">
          {metricGroups.map((g) => (
            <button key={g.id} type="button" aria-pressed={f.group === g.id} onClick={() => set({ group: g.id, metric: metricOfGroup(g.id) })} className={cx("rounded-xl border p-3 text-left", f.group === g.id ? "border-primary bg-primary-soft text-primary" : "border-line hover:bg-surface2")}>
              <b className="block text-[13px]">{t(g.label)}</b><span className="text-[11px] text-muted">{g.sub}</span>
            </button>
          ))}
        </div>
        {f.group === "air" && <div className="mt-2"><Choice value={f.metric as "co2"} onChange={(v) => set({ metric: v })} options={[{ id: "co2", label: "CO₂ (ppm)" }, { id: "pm25" as "co2", label: "PM2.5 (µg/m³)" }]} /></div>}
        <div className="mt-3 flex flex-col gap-2 rounded-xl bg-surface2 p-3 text-[13px]">
          <div className="flex flex-wrap items-center gap-2">
            {t("Alert when {what} is", { what: f.metric === "co2" ? "CO₂" : f.metric === "pm25" ? "PM2.5" : t(f.group === "temperature" ? "room temperature" : f.group === "humidity" ? "humidity" : "power") })}
            <Select aria-label={t("Operator")} className="w-20" value={f.operator} onChange={(e) => set({ operator: e.target.value as Operator })}>{ops.map((o) => <option key={o} value={o}>{opSymbol[o]}</option>)}</Select>
            <Input aria-label={t("Threshold")} inputMode="decimal" className="w-24" value={f.threshold} onChange={(e) => set({ threshold: e.target.value })} />{t("{unit} continuously for", { unit })}
            <Input aria-label={t("Duration")} inputMode="numeric" className="w-20" value={f.duration} onChange={(e) => set({ duration: e.target.value })} />
            <Select aria-label={t("Duration unit")} className="w-24" value={f.unit} onChange={(e) => set({ unit: e.target.value as "s" | "min" })}><option value="min">min</option><option value="s">s</option></Select>
          </div>
          <div className="flex flex-wrap items-center gap-2">{t(up ? "Recover when it drops below" : "Recover when it rises above")} <Input aria-label={t("Recovery")} inputMode="decimal" className="w-24" value={f.recovery} onChange={(e) => set({ recovery: e.target.value })} />{unit}</div>
          {err.recovery || err.threshold || err.duration ? (tried || f.recovery !== editor.form.recovery || f.threshold !== editor.form.threshold) && <p className="text-xs text-crit">✕ {err.threshold ?? err.recovery ?? err.duration}</p>
            : <p className="text-xs text-ok">{t(up ? "✓ Recovery {recovery} {unit} is below the {threshold} {unit} limit · unit fixed to {unit} for this metric" : "✓ Recovery {recovery} {unit} is above the {threshold} {unit} limit · unit fixed to {unit} for this metric", { recovery: f.recovery, threshold: f.threshold, unit })}</p>}
        </div>
        <h3 className="mt-4 text-[13px] font-bold">{t("2 · Only if …")} <span className="font-normal text-muted">{t("(optional) Limit when the policy is checked")}</span></h3>
        <div className="mt-2 flex flex-col gap-2">
          <Check label={t("Only on some days and times")} checked={f.windowOn} onChange={(v) => set({ windowOn: v })} />
          {f.windowOn && (
            <div className="flex flex-wrap items-center gap-2">
              <Btn size="sm" onClick={() => set({ weekdays: [1, 2, 3, 4, 5] })}>{t("Mon–Fri")}</Btn><Btn size="sm" onClick={() => set({ weekdays: [1, 2, 3, 4, 5, 6, 7] })}>{t("Every day")}</Btn>
              {days.map((d, i) => <button key={d} type="button" aria-pressed={f.weekdays.includes(i + 1)} onClick={() => set({ weekdays: f.weekdays.includes(i + 1) ? f.weekdays.filter((x) => x !== i + 1) : [...f.weekdays, i + 1] })} className={cx("rounded-control border px-2 py-1 text-xs font-semibold", f.weekdays.includes(i + 1) ? "border-primary bg-primary-soft text-primary" : "border-line")}>{t(d)}</button>)}
              <Input aria-label={t("From")} type="time" className="w-28" value={f.start} onChange={(e) => set({ start: e.target.value })} /> – <Input aria-label={t("To")} type="time" className="w-28" value={f.end} onChange={(e) => set({ end: e.target.value })} />
              <span className="text-xs text-muted">{editor.policy?.timezone ?? p.timezone}</span>
            </div>
          )}
          {tried && err.window && <p className="text-xs text-crit">✕ {err.window}</p>}
        </div>
        <h3 className="mt-4 text-[13px] font-bold">{t("3 · Then …")} <span className="font-normal text-muted">{t("How loud the alert is and who hears about it")}</span></h3>
        <div className="mt-2 flex flex-col gap-2 text-[13px]">
          <div className="flex flex-wrap items-center gap-3">{t("Severity")} <Choice value={f.severity} onChange={(v: Severity) => set({ severity: v })} options={[{ id: "normal", label: t("Info") }, { id: "warning", label: t("Warning") }, { id: "critical", label: t("Critical") }]} /></div>
          <div className="flex flex-wrap items-center gap-3">{t("Notify me by")} <Check label={t("In-app (always)")} checked disabled /><Check label={t("Email")} checked={f.email} onChange={(v) => set({ email: v })} /><Check label="WhatsApp" checked={false} disabled /></div>
          <p className="text-[11px] text-muted">{t("WhatsApp is only available after you allow it in Preferences. Missing or stale readings never trigger — they show “not measured”.")}</p>
        </div>
      </Card>
      <div className="flex flex-col gap-4">
        <Card title={t("Summary")}><p className="text-[13px] font-semibold">{summaryText(f, t)}</p></Card>
        <Card title={t("Attached to")}>
          {editor.attached.length ? <div className="flex flex-wrap gap-1.5">{editor.attached.map((a) => <Badge key={a} tone="primary">{a}</Badge>)}</div> : <p className="text-xs text-warn">{t("Not attached to any AC yet.")}</p>}
          <p className="mt-2 text-[11px] text-muted">{t("Attach or detach from each AC’s page (Units & locations › AC › Alert policies). Editing here changes it on every attached AC.")}</p>
        </Card>
        <Card>
          <Btn variant="primary" className="w-full" disabled={pending} onClick={save}>{t("Save policy")}</Btn>
          <Link href={base} className="mt-2 block text-center text-[13px] font-semibold text-primary">{t("Cancel")}</Link>
          {editor.policy && <><Btn variant="danger" className="mt-3 w-full" onClick={() => setRemove(true)}>{t("Delete policy")}</Btn><p className="mt-2 text-[11px] text-muted">{editor.attached.length === 0 ? t("Deleting detaches it from every AC first (you confirm). The default policy can’t be deleted.") : editor.attached.length === 1 ? t("Deleting detaches it from its AC first (you confirm). The default policy can’t be deleted.") : t("Deleting detaches it from all {n} ACs first (you confirm). The default policy can’t be deleted.", { n: editor.attached.length })}</p></>}
          {!p.customerId && <Banner tone="warn">{t("Your customer account could not be read — saving is not possible.")}</Banner>}
        </Card>
      </div>
      {remove && editor.policy && (
        <DeleteModal c={{ id: editor.policy.id, version: editor.policy.version, name: editor.policy.name, badge: "", enabled: editor.policy.enabled, when: "", then: "", attached: editor.attached, attachedText: "", note: null }} version={editor.policy.version}
          onClose={(text) => { setRemove(false); if (text) onDone(text); }} onFail={(x) => { setRemove(false); onFail(x); }} />
      )}
    </div>
  );
}
