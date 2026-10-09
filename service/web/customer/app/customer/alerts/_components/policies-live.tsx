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
  const [pending, run] = useAction();
  const [remove, setRemove] = useState<PolicyCard | null>(null);
  const d = p.defaults;
  const fail = (f: ActionFailure) => onFail(f);
  const toggleRule = (ruleKey: string, name: string, on: boolean, version: number) =>
    d && p.customerId && run(() => setDefaultRule(d.id, ruleKey, p.customerId!, on, version), on ? `“${name}” on` : `“${name}” off`, () => onDone(`Default rule “${name}” switched ${on ? "on" : "off"} for all your ACs.`), fail);
  return (
    <>
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div><h2 className="text-[15px] font-bold">Alert policies</h2><p className="max-w-xl text-xs text-muted">A policy is a set of limits. It only runs on the ACs you attach it to (open an AC › Alert policies). Every AC also has the default policy.</p></div>
        <Link href={`${base}&policyId=new`} className="inline-flex items-center rounded-control border border-primary bg-primary px-3.5 py-2 text-[13px] font-semibold text-white">+ Create policy</Link>
      </div>
      {d && (
        <Card title={<span className="flex flex-wrap items-center gap-2">Default policy <Badge tone="primary">Default · set by HQ</Badge><Badge tone="primary">{d.on} of {d.rows.length} rules on</Badge></span>} action={<span className="text-[11px] text-muted">Limits set by HQ · on / off is set by the account owner</span>}>
          <p className="mb-3 text-xs text-muted">Covers ventilation and the usual causes of breakdowns. It is attached to every AC and cannot be detached or edited, but the account owner can switch each rule on or off for your account (all your ACs); members see the rules read-only. Ask HQ if a limit should change.</p>
          {!p.owner && <p className="mb-3 text-xs text-warn">Only the account owner can change this.</p>}
          <div className="scroll-x"><table className="w-full text-left text-[13px]">
            <thead className="text-[11px] uppercase text-muted"><tr><th className="py-1.5 pr-3">Rule</th><th className="pr-3">Condition</th><th className="pr-3">Type</th><th className="pr-3">Severity</th><th>On</th></tr></thead>
            <tbody>{d.rows.map((r) => (
              <tr key={r.ruleKey} className={cx("border-t border-line", !r.enabled && "text-muted")}>
                <td className="py-2 pr-3 font-semibold">{r.name}</td><td className="pr-3">{r.condition}</td><td className="pr-3"><Badge tone={r.type === "Air quality" ? "primary" : "muted"}>{r.type}</Badge></td>
                <td className="pr-3"><SeverityBadge s={r.severity} /></td>
                <td><Toggle on={r.enabled} disabled={!p.owner || pending} onChange={(v) => toggleRule(r.ruleKey, r.name, v, r.version)} label={r.name} /></td>
              </tr>
            ))}</tbody>
          </table></div>
          {d.notes.map((n) => <p key={n} className="mt-2 text-xs text-muted">{n}</p>)}
        </Card>
      )}
      <div className="text-[13px] font-bold">Your policies <span className="text-xs font-normal text-muted">{p.cards.length} · added on top of the default policy · a policy belongs to your account and can be attached to many ACs</span></div>
      {p.cards.map((c) => (
        <Card key={c.id} title={<span className="flex flex-wrap items-center gap-2">{c.name}<Badge tone="muted">{c.badge}</Badge><Badge tone={c.enabled ? "ok" : "muted"}>{c.enabled ? "On" : "Off"}</Badge></span>}
          action={<span className="flex items-center gap-2 text-xs text-muted">{c.enabled ? "On" : "Off"}<Toggle on={c.enabled} disabled={pending} label={c.name} onChange={(v) => {
            const pol = p.list.find((x) => x.id === c.id)!;
            run(() => savePolicy(toggledInput(pol, v), pol.version), v ? "Policy on" : "Policy off", () => onDone(`“${c.name}” switched ${v ? "on" : "off"}.`), fail);
          }} /></span>}>
          <div className="flex flex-wrap items-end justify-between gap-3">
            <div className="text-[13px]">
              <p><span className="text-muted">When</span> {c.when}</p><p><span className="text-muted">Then</span> {c.then}</p>
              <p className={cx("mt-1 text-xs", c.attached.length ? "text-muted" : "text-warn")}>{c.attachedText}</p>
              {c.note && <p className="text-xs text-warn">{c.note}</p>}
            </div>
            <div className="flex gap-2"><Link href={`${base}&policyId=${c.id}`} className="inline-flex items-center rounded-control border border-line bg-surface px-3 py-1.5 text-xs font-semibold hover:bg-surface2">Edit</Link><Btn size="sm" variant="danger" onClick={() => setRemove(c)}>Delete</Btn></div>
          </div>
        </Card>
      ))}
      {p.cards.length === 0 && <p className="rounded-xl border border-line bg-surface p-4 text-center text-xs text-muted">No policies of your own yet — “+ Create policy” adds one; attach it on an AC’s page.</p>}
      <p className="text-xs text-muted">A unit can carry several policies; each rule is checked separately. Missing or stale readings never raise an alert — they show “not measured”. Air-quality limits (CO₂ ppm, PM2.5 µg/m³) are regular alert policies.</p>
      {remove && <DeleteModal c={remove} version={p.list.find((x) => x.id === remove.id)?.version ?? 0} onClose={(text) => { setRemove(null); if (text) onDone(text); }} onFail={(f) => { setRemove(null); onFail(f); }} />}
    </>
  );
}

function DeleteModal({ c, version, onClose, onFail }: { c: PolicyCard; version: number; onClose: (text?: string) => void; onFail: (f: ActionFailure) => void }) {
  const [pending, run] = useAction();
  const router = useRouter();
  return (
    <Modal open onClose={() => onClose()} title={`Delete “${c.name}”?`} footer={<><Btn onClick={() => onClose()}>Keep policy</Btn><Btn variant="danger" disabled={pending} onClick={() => run(() => deletePolicy(c.id, version), "Policy deleted", () => { onClose(`“${c.name}” deleted${c.attached.length ? ` and detached from ${c.attached.join(", ")}` : ""}.`); router.replace(base, { scroll: false }); }, onFail)}>Delete policy</Btn></>}>
      <p className="text-[13px]">{c.attached.length ? `It is detached from ${c.attached.length} AC${c.attached.length === 1 ? "" : "s"} first: ${c.attached.join(", ")}.` : "It is not attached to any AC."} The default policy can’t be deleted.</p>
    </Modal>
  );
}

const ops: Operator[] = ["gte", "gt", "lte", "lt"];
const days = ["Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"];

function PolicyEditor({ p, editor, onDone, onFail }: Props & { editor: NonNullable<Live["editor"]> }) {
  const router = useRouter();
  const [pending, run] = useAction();
  const [f, setF] = useState<PolicyForm>(editor.form);
  const [tried, setTried] = useState(false);
  const [remove, setRemove] = useState(false);
  const err = policyErrors(f);
  const set = (x: Partial<PolicyForm>) => setF((s) => ({ ...s, ...x }));
  const unit = metricUnit[f.metric] ?? "";
  const up = f.operator === "gt" || f.operator === "gte";
  const save = () => {
    setTried(true);
    if (Object.keys(err).length || !p.customerId) return;
    run(() => savePolicy(policyInput(f, editor.policy, { customerId: p.customerId!, membershipId: p.membershipId, timezone: p.timezone }), editor.policy?.version ?? null), "Policy saved",
      () => { onDone(`“${f.name.trim()}” saved${editor.policy ? "" : " — attach it on an AC’s page"}.`); router.replace(base, { scroll: false }); }, onFail);
  };
  return (
    <div className="grid grid-cols-1 gap-4 xl:grid-cols-[minmax(0,1fr)_minmax(0,320px)]">
      <Card>
        <div className="mb-3 text-[13px]"><Link href={base} className="font-semibold text-primary">← Alert policies</Link> › {editor.policy ? "Edit alert policy" : "Create alert policy"}</div>
        <Field label="Name" error={tried ? err.name : undefined}><Input value={f.name} maxLength={120} onChange={(e) => set({ name: e.target.value })} /></Field>
        <h3 className="mt-4 text-[13px] font-bold">1 · When … <span className="font-normal text-muted">What to watch</span></h3>
        <div className="mt-2 grid grid-cols-2 gap-2 sm:grid-cols-4">
          {metricGroups.map((g) => (
            <button key={g.id} type="button" aria-pressed={f.group === g.id} onClick={() => set({ group: g.id, metric: metricOfGroup(g.id) })} className={cx("rounded-xl border p-3 text-left", f.group === g.id ? "border-primary bg-primary-soft text-primary" : "border-line hover:bg-surface2")}>
              <b className="block text-[13px]">{g.label}</b><span className="text-[11px] text-muted">{g.sub}</span>
            </button>
          ))}
        </div>
        {f.group === "air" && <div className="mt-2"><Choice value={f.metric as "co2"} onChange={(v) => set({ metric: v })} options={[{ id: "co2", label: "CO₂ (ppm)" }, { id: "pm25" as "co2", label: "PM2.5 (µg/m³)" }]} /></div>}
        <div className="mt-3 flex flex-col gap-2 rounded-xl bg-surface2 p-3 text-[13px]">
          <div className="flex flex-wrap items-center gap-2">
            Alert when {f.metric === "co2" ? "CO₂" : f.metric === "pm25" ? "PM2.5" : f.group === "temperature" ? "room temperature" : f.group} is
            <Select aria-label="Operator" className="w-20" value={f.operator} onChange={(e) => set({ operator: e.target.value as Operator })}>{ops.map((o) => <option key={o} value={o}>{opSymbol[o]}</option>)}</Select>
            <Input aria-label="Threshold" inputMode="decimal" className="w-24" value={f.threshold} onChange={(e) => set({ threshold: e.target.value })} />{unit} continuously for
            <Input aria-label="Duration" inputMode="numeric" className="w-20" value={f.duration} onChange={(e) => set({ duration: e.target.value })} />
            <Select aria-label="Duration unit" className="w-24" value={f.unit} onChange={(e) => set({ unit: e.target.value as "s" | "min" })}><option value="min">min</option><option value="s">s</option></Select>
          </div>
          <div className="flex flex-wrap items-center gap-2">Recover when it {up ? "drops below" : "rises above"} <Input aria-label="Recovery" inputMode="decimal" className="w-24" value={f.recovery} onChange={(e) => set({ recovery: e.target.value })} />{unit}</div>
          {err.recovery || err.threshold || err.duration ? (tried || f.recovery !== editor.form.recovery || f.threshold !== editor.form.threshold) && <p className="text-xs text-crit">✕ {err.threshold ?? err.recovery ?? err.duration}</p>
            : <p className="text-xs text-ok">✓ Recovery {f.recovery} {unit} is {up ? "below" : "above"} the {f.threshold} {unit} limit · unit fixed to {unit} for this metric</p>}
        </div>
        <h3 className="mt-4 text-[13px] font-bold">2 · Only if … <span className="font-normal text-muted">(optional) Limit when the policy is checked</span></h3>
        <div className="mt-2 flex flex-col gap-2">
          <Check label="Only on some days and times" checked={f.windowOn} onChange={(v) => set({ windowOn: v })} />
          {f.windowOn && (
            <div className="flex flex-wrap items-center gap-2">
              <Btn size="sm" onClick={() => set({ weekdays: [1, 2, 3, 4, 5] })}>Mon–Fri</Btn><Btn size="sm" onClick={() => set({ weekdays: [1, 2, 3, 4, 5, 6, 7] })}>Every day</Btn>
              {days.map((d, i) => <button key={d} type="button" aria-pressed={f.weekdays.includes(i + 1)} onClick={() => set({ weekdays: f.weekdays.includes(i + 1) ? f.weekdays.filter((x) => x !== i + 1) : [...f.weekdays, i + 1] })} className={cx("rounded-control border px-2 py-1 text-xs font-semibold", f.weekdays.includes(i + 1) ? "border-primary bg-primary-soft text-primary" : "border-line")}>{d}</button>)}
              <Input aria-label="From" type="time" className="w-28" value={f.start} onChange={(e) => set({ start: e.target.value })} /> – <Input aria-label="To" type="time" className="w-28" value={f.end} onChange={(e) => set({ end: e.target.value })} />
              <span className="text-xs text-muted">{editor.policy?.timezone ?? p.timezone}</span>
            </div>
          )}
          {tried && err.window && <p className="text-xs text-crit">✕ {err.window}</p>}
        </div>
        <h3 className="mt-4 text-[13px] font-bold">3 · Then … <span className="font-normal text-muted">How loud the alert is and who hears about it</span></h3>
        <div className="mt-2 flex flex-col gap-2 text-[13px]">
          <div className="flex flex-wrap items-center gap-3">Severity <Choice value={f.severity} onChange={(v: Severity) => set({ severity: v })} options={[{ id: "normal", label: "Info" }, { id: "warning", label: "Warning" }, { id: "critical", label: "Critical" }]} /></div>
          <div className="flex flex-wrap items-center gap-3">Notify me by <Check label="In-app (always)" checked disabled /><Check label="Email" checked={f.email} onChange={(v) => set({ email: v })} /><Check label="WhatsApp" checked={false} disabled /></div>
          <p className="text-[11px] text-muted">WhatsApp is only available after you allow it in Preferences. Missing or stale readings never trigger — they show “not measured”.</p>
        </div>
      </Card>
      <div className="flex flex-col gap-4">
        <Card title="Summary"><p className="text-[13px] font-semibold">{summaryText(f)}</p></Card>
        <Card title="Attached to">
          {editor.attached.length ? <div className="flex flex-wrap gap-1.5">{editor.attached.map((a) => <Badge key={a} tone="primary">{a}</Badge>)}</div> : <p className="text-xs text-warn">Not attached to any AC yet.</p>}
          <p className="mt-2 text-[11px] text-muted">Attach or detach from each AC’s page (Units & locations › AC › Alert policies). Editing here changes it on every attached AC.</p>
        </Card>
        <Card>
          <Btn variant="primary" className="w-full" disabled={pending} onClick={save}>Save policy</Btn>
          <Link href={base} className="mt-2 block text-center text-[13px] font-semibold text-primary">Cancel</Link>
          {editor.policy && <><Btn variant="danger" className="mt-3 w-full" onClick={() => setRemove(true)}>Delete policy</Btn><p className="mt-2 text-[11px] text-muted">Deleting detaches it from {editor.attached.length ? `${editor.attached.length === 1 ? "its AC" : `all ${editor.attached.length} ACs`}` : "every AC"} first (you confirm). The default policy can’t be deleted.</p></>}
          {!p.customerId && <Banner tone="warn">Your customer account could not be read — saving is not possible.</Banner>}
        </Card>
      </div>
      {remove && editor.policy && (
        <DeleteModal c={{ id: editor.policy.id, version: editor.policy.version, name: editor.policy.name, badge: "", enabled: editor.policy.enabled, when: "", then: "", attached: editor.attached, attachedText: "", note: null }} version={editor.policy.version}
          onClose={(text) => { setRemove(false); if (text) onDone(text); }} onFail={(x) => { setRemove(false); onFail(x); }} />
      )}
    </div>
  );
}
