"use client";

import { useState, useTransition } from "react";
import { Badge, Banner, Btn, Card, Check, Choice, EmptyState, Field, Input, LineChart, ListRow, Modal, SearchSelect, Select, SeverityBadge, TextLink, Toggle, useToast, cx } from "@ac/web/components/ui";
import { useT } from "@ac/web/components/I18n";
import { actionMessage } from "@ac/web/lib/actionMessage";
import { useOp } from "@ac/web/lib/useOp";
import { ZONES } from "@ac/web/lib/preferences";
import {
  HQ_METRICS, metricLabel, newPolicyDraft, opSymbol, policyChanges, policyErrors, recipientRole,
  type AdminPolicy, type ApiRecipient, type Channel, type Operator, type PolicyRecipient,
} from "@ac/web/lib/adminAlerts";
import { deletePolicy, savePolicy } from "../actions";

/** API mode: the selected policy (`policyId`) or the owner customer of a new one (`new`) from the URL (DD-A05 items 5
 * and 8), whether the session may write policies, the display time zone a new one starts in, the active customers that
 * may own one, and each customer's units (notifications.recipients lists a policy's recipients for one of them). */
export type PoliciesLive = {
  policyId: string | null; newFor: string | null; canWrite: boolean; timezone: string;
  customers: { id: string; name: string }[]; units: { id: string; customerId: string | null }[];
};

const OPERATORS: Operator[] = ["gte", "gt", "lte", "lt"];
const CHANNEL: Record<Channel, string> = { inApp: "In-app", email: "Email", whatsapp: "WhatsApp" };
// the Phase 1A demo's policies, customer and recipients (their data stays English, as the other demo screens)
const demoRecipients: PolicyRecipient[] = [{ id: "customer-a", label: "customer-a", role: "client" }, { id: "hq-operator", label: "hq-operator", role: "HQ" }];
const demoPolicy = (id: string, name: string, sub: string, metric: string, threshold: number, recovery: number, duration: number, on: boolean, units: string[]): AdminPolicy => ({
  id, version: 1, kind: "alert", name, group: "CUSTOMER-A", sub, on, units: units.length === 1 ? "1 unit" : `${units.length} units`, customerId: "customer-a", customerName: "customer-a",
  priority: 50, timezone: "Asia/Kuala_Lumpur", metric, operator: "gte", threshold, recovery, duration, severity: "warning", cooldown: 5, escalate: 60, channels: ["inApp", "email"],
  recipients: demoRecipients, activeWindow: null, madeBy: "hq-operator", unitIds: units, unitNames: units.map((u) => (u === "unit-online-rto" ? "Bedroom AC" : "Bedroom AC #2")),
});
export const seedPolicies: AdminPolicy[] = [
  { ...demoPolicy("policy-default", "Default policy", "6 rules · ventilation (CO₂, PM2.5) + fault causes", "co2", 1000, 900, 600, true, []), kind: "default_alert", group: "DEFAULT · ON EVERY UNIT", customerId: null, units: "every unit" },
  demoPolicy("policy-temp-a", "Bedroom too hot", "Temperature ≥ 30 °C for 60 s", "temperature", 30, 28, 60, true, ["unit-online-rto", "unit-online-rto-2"]),
  demoPolicy("policy-co2-a", "Stuffy office", "CO₂ ≥ 1200 ppm · 15 min · weekdays", "co2", 1200, 1000, 900, true, ["unit-online-rto", "unit-online-rto-2"]),
  { ...demoPolicy("policy-humidity-a", "Night humidity", "Humidity ≥ 70 % · 30 min · 22–06", "humidity", 70, 65, 1800, false, []), severity: "normal" },
];
export const demoCustomers = [{ id: "customer-a", name: "customer-a" }];
const demoUnits = [{ id: "unit-online-rto", customerId: "customer-a" }];
const demoCandidates: ApiRecipient[] = [
  { id: "customer-a", role: "client", displayLabel: "customer-a", allowedChannels: ["inApp", "email", "whatsapp"] },
  { id: "hq-operator", role: "admin", displayLabel: "hq-operator", allowedChannels: ["inApp", "email"] },
];

/** The selected policy, or the owner customer of a new one: URL state in API mode, local state in the demo. */
export type PolicySel = { policyId: string | null; newFor: string | null };

/** The Policies tab (DD-A05 items 7–9, Figma Admin 38:5 / 356:3714, IR345): the policies by customer and the editor of
 * the selected one. A new policy (`sel.newFor`, started by “+ New policy”) starts as DD-A05 says (display time zone,
 * off, priority 50) and is created by Save policy. Recipients are added from notifications.recipients; Delete policy
 * detaches it from every unit. The demo keeps its fixture policies and saves nothing. */
export function PoliciesTab({ pols, live, sel, select }: { pols: AdminPolicy[]; live?: PoliciesLive; sel: PolicySel; select: (policyId: string | null, newFor?: string | null) => void }) {
  const t = useT();
  const toast = useToast();
  const [pending, startTransition] = useTransition();
  const canWrite = live?.canWrite ?? true;
  const customers = live?.customers ?? demoCustomers;

  // the policy the editor starts from: a new one for `new`, else the selected one (a policyId the list does not hold
  // says so instead of opening another policy), else the first customer policy
  const owner = sel.newFor ? customers.find((c) => c.id === sel.newFor) ?? null : null;
  const missing = !sel.newFor && !!sel.policyId && !pols.some((x) => x.id === sel.policyId);
  const p = owner ? newPolicyDraft(owner, live?.timezone ?? "Asia/Kuala_Lumpur")
    : sel.newFor || missing ? null : pols.find((x) => x.id === sel.policyId) ?? pols.find((x) => x.kind === "alert") ?? pols[0] ?? null;
  const key = owner ? `new:${owner.id}` : p ? `${p.id}@${p.version}` : "";
  const [draft, setDraft] = useState<AdminPolicy | null>(p);
  const [draftOf, setDraftOf] = useState(key);
  const [tried, setTried] = useState(false);
  if (draftOf !== key) {
    setDraftOf(key);
    setDraft(p);
    setTried(false);
  }
  const [picking, setPicking] = useState(false);
  const [deleting, setDeleting] = useState(false);

  const set = <K extends keyof AdminPolicy>(k: K, v: AdminPolicy[K]) => setDraft((d) => (d ? { ...d, [k]: v } : d));
  const errors = draft ? policyErrors(draft, t) : {};
  const problems = Object.keys(errors).length;
  const isNew = !!draft && !draft.id;
  const changes = draft && p && !isNew ? policyChanges(draft, p, t) : [];
  const status = tried && problems ? t(problems === 1 ? "VALIDATION · 1 problem · nothing saved" : "VALIDATION · {n} problems · nothing saved", { n: problems })
    : isNew ? t("New policy · not saved yet")
    : changes.length ? `${t(changes.length === 1 ? "1 unsaved change" : "{n} unsaved changes", { n: changes.length })} · ${changes.join(", ")}` : t("No unsaved changes");
  // the unit notifications.recipients is asked about: one the policy is attached to, else one of its customer's
  const units = live?.units ?? demoUnits;
  const target = draft ? draft.unitIds[0] ?? units.find((u) => u.customerId === draft.customerId)?.id ?? null : null;

  const save = () => {
    setTried(true);
    if (!draft || problems) return toast(t("Fix the validation errors first"), "crit");
    if (!live) return toast(t("Policy saved"));
    startTransition(async () => {
      const r = await savePolicy(draft);
      if (!r.ok) return toast(actionMessage(r, t), "crit");
      toast(t(isNew ? "Policy created — attach it to units on each unit’s page" : "Policy saved"));
      setTried(false);
      if (isNew && r.id) select(r.id);
    });
  };
  const remove = () => {
    if (!draft || isNew) return;
    if (!live) return (setDeleting(false), toast(t("“{name}” deleted", { name: draft.name })));
    startTransition(async () => {
      const r = await deletePolicy(draft.id, draft.version);
      if (!r.ok) return toast(actionMessage(r, t), "crit");
      toast(t("“{name}” deleted", { name: draft.name }));
      setDeleting(false);
      select(null);
    });
  };
  const discard = () => (isNew ? select(null) : (setDraft(p), setTried(false)));
  const toggleChannel = (c: Channel, on: boolean) => draft && set("channels", on ? [...new Set([...draft.channels, c])] : draft.channels.filter((x) => x !== c));
  const groups = [...new Set([...pols.map((x) => x.group), ...(isNew && draft ? [draft.group] : [])])];

  return (
    <>
      <div className="split-rev">
        <Card title={t("Alert policies")} sub={t("by customer")} className="self-start">
          {groups.map((g) => (
            <div key={g} className="mb-3">
              <div className="mb-1 text-[10px] font-bold tracking-wide text-muted">{g}</div>
              <div className="flex flex-col gap-2">
                {isNew && draft?.group === g && (
                  <ListRow selected><div className="min-w-0 flex-1"><b className="text-[13px]">{draft.name.trim() || t("New alert policy")}</b><div className="text-[11px] text-muted">{t("not saved yet")}</div></div></ListRow>
                )}
                {pols.filter((x) => x.group === g).map((x) => (
                  <ListRow key={x.id} selected={!isNew && draft?.id === x.id} onClick={() => select(x.id)}>
                    <div className="min-w-0 flex-1">
                      <b className="text-[13px]">{x.name}</b><div className="text-[11px] text-muted">{x.sub}</div>
                      <div className="mt-1 flex flex-wrap items-center gap-2 text-[11px] text-muted">{x.kind === "alert" && <SeverityBadge s={x.severity} />}<span>{x.units}{!x.on && ` · ${t("Off")}`}</span></div>
                    </div>
                  </ListRow>
                ))}
              </div>
            </div>
          ))}
        </Card>
        {!draft ? (
          <Card title={t("Alert policy")}>
            <EmptyState title={t(missing ? "This policy is not in the list" : sel.newFor ? "No such active customer" : "No alert policies")}>
              {t(missing ? "It may have been deleted. Pick a policy in the list." : sel.newFor ? "A new policy needs an active owner customer — start it again with + New policy." : "Customers have no alert policies yet.")}
            </EmptyState>
          </Card>
        ) : draft.kind === "default_alert" ? (
          <Card title={draft.name} sub={draft.id} action={<Badge tone={draft.on ? "ok" : "muted"}>{t(draft.on ? "Enabled" : "Disabled")}</Badge>}>
            <Banner>{t("The default policy runs on every unit of every customer and cannot be detached. Its rules are switched on or off per customer on each customer’s Alert policies tab — the customer’s owner can do the same in the customer app.")}</Banner>
            <p className="mt-2"><TextLink href="/admin/units">{t("Open Customers & units ›")}</TextLink></p>
          </Card>
        ) : (
          <Card
            title={<span className="inline-flex flex-wrap items-center gap-2">{draft.name.trim() || t("New alert policy")}<Badge tone={isNew ? "warn" : "muted"}>{isNew ? t("Not saved") : t("Policy v{n}", { n: draft.version })}</Badge></span>}
            sub={[isNew ? t("new") : draft.id, draft.customerName, draft.madeBy && t("made by {name}", { name: draft.madeBy })].filter(Boolean).join(" · ")}
            action={<div className="flex items-center gap-2 text-xs font-semibold"><span aria-hidden>{t("Enabled")}</span><Toggle on={draft.on} onChange={(v) => set("on", v)} label={t("Enabled")} disabled={!canWrite} /></div>}>
            {!canWrite && <div className="mb-3"><Banner>{t("You can read this policy. Changing it needs alert.policy.write.")}</Banner></div>}
            <fieldset disabled={!canWrite} className="flex min-w-0 flex-col gap-5">
              <section><h3 className="mb-2 text-[13px] font-bold">{t("Basics")}</h3>
                <div className="grid-fluid" style={{ ["--min" as string]: "180px" }}>
                  <Field label={t("Name")} hint={t("1–120 characters")} error={tried ? errors.name : undefined}><Input value={draft.name} placeholder={t("e.g. Bedroom too hot")} onChange={(e) => set("name", e.target.value)} /></Field>
                  <Field label={t("Priority")} hint="0–100" error={tried ? errors.priority : undefined}><Input type="number" value={draft.priority} onChange={(e) => set("priority", +e.target.value)} /></Field>
                  <Field label={t("Timezone")} hint={t("The zone the policy’s active window is evaluated in")}><Select value={draft.timezone} onChange={(e) => set("timezone", e.target.value)}>{[...new Set([draft.timezone, ...ZONES])].map((z) => <option key={z}>{z}</option>)}</Select></Field>
                </div>
              </section>
              <section><h3 className="mb-1 text-[13px] font-bold">{t("Owner & units")}</h3>
                <p className="mb-2 text-xs text-muted">{t("A policy belongs to one customer. Units pick it up on their own page (Customers & units › unit › Alert policies) — units are never assigned from here.")}</p>
                <div className="text-xs font-semibold">{t("Owner (customer) · fixed after creation")}</div>
                <div className="mt-1"><Badge tone="primary">{draft.customerName}</Badge></div>
                <div className="mt-2 rounded-xl bg-surface2 px-3 py-2 text-[13px]">
                  <div>{draft.unitIds.length ? t("Attached to {units} of {customer}: {names}", { units: draft.units, customer: draft.customerName, names: draft.unitNames.join(", ") }) : t("Not attached to any unit yet")}</div>
                  {draft.customerId && <TextLink href={`/admin/units?customerId=${draft.customerId}&tab=policies`}>{t("Read-only here · attach / detach on each unit ›")}</TextLink>}
                </div>
              </section>
              <section><h3 className="mb-2 text-[13px] font-bold">{t("Condition")}</h3><p className="mb-2 text-xs text-muted">{t("Missing or stale readings never count toward the condition — they raise a data-quality notice instead.")}</p>
                <div className="grid-fluid" style={{ ["--min" as string]: "130px" }}>
                  <Field label={t("Metric")}><Select value={draft.metric} onChange={(e) => set("metric", e.target.value)}>{HQ_METRICS.map((k) => <option key={k} value={k}>{t(metricLabel[k])}</option>)}</Select></Field>
                  <Field label={t("Comparison")}><Select value={draft.operator} onChange={(e) => set("operator", e.target.value as Operator)}>{OPERATORS.map((o) => <option key={o} value={o}>{opSymbol[o]}</option>)}</Select></Field>
                  <Field label={t("Alert when {op}", { op: opSymbol[draft.operator] })}><Input type="number" value={draft.threshold} onChange={(e) => set("threshold", +e.target.value)} /></Field>
                  <Field label={t("For (s)")} error={tried ? errors.duration : undefined}><Input type="number" value={draft.duration} onChange={(e) => set("duration", +e.target.value)} /></Field>
                  <Field label={t("Recover")} error={errors.recovery}><Input type="number" value={draft.recovery} onChange={(e) => set("recovery", +e.target.value)} /></Field>
                </div>
                <div className="mt-2"><LineChart points={[20, 24, 28, 31, 32, 31, 29, 27, 25]} min={18} max={34} threshold={draft.threshold} height={90} /></div>
                <p className={cx("text-xs", errors.recovery ? "text-crit" : "text-ok")}>{errors.recovery ? `✕ ${errors.recovery}` : t("✓ Recovery {recovery} is on the safe side of the {threshold} threshold ({op})", { recovery: draft.recovery, threshold: draft.threshold, op: opSymbol[draft.operator] })}</p>
              </section>
              <section><h3 className="mb-1 text-[13px] font-bold">{t("Severity")}</h3><Choice value={draft.severity} onChange={(v) => set("severity", v)} options={[{ id: "normal", label: t("Info") }, { id: "warning", label: t("Warning") }, { id: "critical", label: t("Critical") }]} /><p className="mt-1 text-[11px] text-muted">{t("A severity change on an open alert counts as a new notification reason.")}</p></section>
              <section>
                <div className="mb-1 flex flex-wrap items-center justify-between gap-2"><h3 className="text-[13px] font-bold">{t("Notify")}</h3>{canWrite && <Btn size="sm" variant="ghost" onClick={() => setPicking(true)}>{t("+ Add recipient")}</Btn>}</div>
                <p className="mb-2 text-xs text-muted">{t("Recipients come from notifications.recipients — only people allowed to see these units.")}</p>
                {draft.recipients.map((r) => (
                  <div key={r.id} className="flex flex-wrap items-center justify-between gap-2 border-t border-line py-2 text-[13px]">
                    <span><b>{r.label}</b><span className="block text-xs text-muted">{r.role}</span></span>
                    {canWrite && <Btn size="sm" variant="ghost" aria-label={t("Remove {name}", { name: r.label })} onClick={() => set("recipients", draft.recipients.filter((x) => x.id !== r.id))}>✕</Btn>}
                  </div>
                ))}
                {(draft.recipients.length === 0 || (tried && errors.recipients)) && <p className={cx("py-1 text-xs", tried && errors.recipients ? "text-crit" : "text-muted")}>{tried && errors.recipients ? `✕ ${errors.recipients}` : t("No recipients yet — add at least one.")}</p>}
                <div className="flex flex-wrap gap-3 border-t border-line pt-2"><span className="text-xs text-muted">{t("Channels for every recipient:")}</span><Check label={t("In-app")} checked disabled /><Check label={t("Email")} checked={draft.channels.includes("email")} onChange={(v) => toggleChannel("email", v)} /><Check label="WhatsApp" checked={draft.channels.includes("whatsapp")} onChange={(v) => toggleChannel("whatsapp", v)} /></div>
                <p className="text-[11px] text-muted">{t("In-app is always on. WhatsApp reaches only recipients who allowed that channel.")}</p>
              </section>
              <section className="grid-fluid" style={{ ["--min" as string]: "180px" }}><Field label={t("Cooldown (min)")} hint={t("Same severity is not re-sent within this window (1–1440)")} error={tried ? errors.cooldown : undefined}><Input type="number" value={draft.cooldown} onChange={(e) => set("cooldown", +e.target.value)} /></Field><Field label={t("Escalate after (min)")} hint={t("Escalate if still unacknowledged (1–1440)")} error={tried ? errors.escalate : undefined}><Input type="number" value={draft.escalate} onChange={(e) => set("escalate", +e.target.value)} /></Field></section>
            </fieldset>
            {canWrite && (
              <div className="mt-5 flex flex-wrap items-center justify-end gap-2 border-t border-line pt-3">
                {!isNew && <Btn variant="danger" disabled={pending} onClick={() => setDeleting(true)}>{t("Delete policy")}</Btn>}
                <span className={cx("mr-auto text-[12px]", tried && problems ? "text-crit" : "text-muted")}>{status}</span>
                <Btn disabled={pending || (!isNew && !changes.length)} onClick={discard}>{t("Discard")}</Btn>
                <Btn variant="primary" disabled={pending || (!isNew && !changes.length)} onClick={save}>{t("Save policy")}</Btn>
              </div>
            )}
          </Card>
        )}
      </div>
      {picking && draft && <RecipientPicker unitId={target} customer={draft.customerName} chosen={draft.recipients.map((r) => r.id)} demo={!live} onClose={() => setPicking(false)}
        onAdd={(rs) => { set("recipients", [...draft.recipients, ...rs]); setPicking(false); }} />}
      {deleting && draft && (
        <Modal open onClose={() => setDeleting(false)} title={t("Delete policy")} footer={<><Btn onClick={() => setDeleting(false)}>{t("Cancel")}</Btn><Btn variant="danger" disabled={pending} onClick={remove}>{t("Delete policy")}</Btn></>}>
          <p className="text-[13px]">{draft.unitIds.length ? t("“{name}” is detached from {units} and deleted.", { name: draft.name, units: draft.units }) : t("“{name}” is deleted. It is not attached to any unit.", { name: draft.name })}</p>
          <p className="text-[11px] text-muted">{t("The alerts it raised stay in the history.")}</p>
        </Modal>
      )}
    </>
  );
}

/** “+ New policy” asks for the owner customer first — nobody is chosen for HQ unless the alerts are scoped to one
 * customer — and the owner cannot change once the policy is saved (DD-A05 item 8). */
export function NewPolicyModal({ customers, initial, onClose, onStart }: { customers: { id: string; name: string }[]; initial?: string; onClose: () => void; onStart: (customerId: string) => void }) {
  const t = useT();
  const [c, setC] = useState(customers.some((x) => x.id === initial) ? initial! : "");
  return (
    <Modal open onClose={onClose} title={t("New alert policy")} footer={<><Btn onClick={onClose}>{t("Cancel")}</Btn><Btn variant="primary" disabled={!c} onClick={() => onStart(c)}>{t("Start the policy")}</Btn></>}>
      <p className="text-[13px] text-muted">{t("A policy belongs to one customer, and only that customer’s units can carry it. The owner cannot change once the policy is saved.")}</p>
      {customers.length === 0 ? <EmptyState title={t("No active customers")}>{t("A policy needs an active customer to own it.")}</EmptyState> : (
        <Field label={t("Owner (customer)")}><SearchSelect label={t("Owner (customer)")} value={c} onChange={setC} options={customers.map((x) => ({ id: x.id, label: x.name }))} /></Field>
      )}
    </Modal>
  );
}

/** The recipients notifications.recipients lists for the alerts of one of the customer's units: its clients and HQ
 * members with alert.read (IR120 item 3, IR345), read only while the dialog is open. */
function RecipientPicker({ unitId, customer, chosen, demo, onClose, onAdd }: { unitId: string | null; customer: string; chosen: string[]; demo: boolean; onClose: () => void; onAdd: (rs: PolicyRecipient[]) => void }) {
  const t = useT();
  const cands = useOp<{ items: ApiRecipient[] }, ApiRecipient[]>("notifications.recipients", { target: { kind: "unit", id: unitId ?? "" }, templateKey: "alert", channel: "inApp", query: { limit: 100 } },
    demo ? demoCandidates : [], (d) => d.items, !!unitId);
  const [pick, setPick] = useState<string[]>([]);
  const options = cands.data.filter((r) => !chosen.includes(r.id));
  const add = () => onAdd(options.filter((r) => pick.includes(r.id)).map((r) => ({ id: r.id, label: r.displayLabel, role: recipientRole(r.role, t) })));
  return (
    <Modal open onClose={onClose} title={t("Add recipients")} footer={<><Btn onClick={onClose}>{t("Cancel")}</Btn><Btn variant="primary" disabled={!pick.length} onClick={add}>{pick.length ? t(pick.length === 1 ? "Add 1 recipient" : "Add {n} recipients", { n: pick.length }) : t("Add recipients")}</Btn></>}>
      {!unitId ? <EmptyState title={t("{customer} has no units yet", { customer })}>{t("Recipients are the people allowed to see the customer’s units. Add a unit first.")}</EmptyState>
        : cands.loading ? <p className="text-[13px] text-muted">{t("Loading the recipients…")}</p>
        : cands.error ? <p className="text-[13px] text-crit">{t("The recipients could not be loaded — close the dialog and try again.")}</p>
        : options.length === 0 ? <EmptyState title={t("Everyone is already a recipient")}>{t("Only {customer}’s users and HQ staff who read alerts can be recipients.", { customer })}</EmptyState>
        : (
          <div role="group" aria-label={t("Recipients")} className="flex flex-col gap-2">
            {options.map((r) => (
              <div key={r.id} className="rounded-xl border border-line px-3 py-2">
                <Check checked={pick.includes(r.id)} onChange={(on) => setPick((v) => (on ? [...v, r.id] : v.filter((x) => x !== r.id)))}
                  label={<span><b>{r.displayLabel}</b><span className="block text-xs text-muted">{recipientRole(r.role, t)} · {r.allowedChannels.map((c) => t(CHANNEL[c])).join(" · ")}</span></span>} />
              </div>
            ))}
          </div>
        )}
      <p className="text-[11px] text-muted">{t("Only {customer}’s users and HQ staff who read alerts can be recipients.", { customer })}</p>
    </Modal>
  );
}
