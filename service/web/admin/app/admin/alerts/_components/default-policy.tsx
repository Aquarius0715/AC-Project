"use client";

import { useState, useTransition } from "react";
import { Badge, Banner, Btn, Card, Choice, Field, ListRow, Modal, SearchSelect, Textarea, Toggle, useToast, cx } from "@ac/web/components/ui";
import { useI18n, useT } from "@ac/web/components/I18n";
import { actionMessage } from "@ac/web/lib/actionMessage";
import { conditionText } from "@ac/web/lib/assets";
import { conditionErrors, copyable, metricLabel, ruleChanges, settingText, type AdminPolicy, type DefaultRule } from "@ac/web/lib/adminAlerts";
import { saveDefaultRules, switchDefaultRule } from "../actions";
import { ConditionFields, WindowEditor } from "./policy-parts";
import { DefaultRulePreview, DemoTest } from "./policy-checks";
import { Icon } from "@ac/web/components/Icon";

const CATEGORY: Record<string, string> = { air_quality: "Air quality", fault: "Fault", maintenance: "Maintenance", connection: "Connection" };
const SHOWN = 20; // the customers listed before the search-select (DD-A05 item 9, Figma 268:2)

/** The default policy (DD-A05 item 9, Figma Admin 268:2, IR347): its six rules, the selected rule's condition, window and
 * severity as the HQ template — saved for every unit at the policy's version — and, for that rule, the switch of each
 * customer with an optional reason (policies.setDefaultRule). A rule whose metric a policy may watch can be copied as a
 * customer's policy. The key, name, category and metric of a rule are fixed (IR120 item 2). */
export function DefaultPolicyEditor({ p, customers, units, canWrite, copyFor, onCopy, demo }: {
  p: AdminPolicy; customers: { id: string; name: string }[]; units: { id: string; name: string; customerId: string | null }[]; canWrite: boolean;
  copyFor: { id: string; name: string } | null; onCopy: (ruleKey: string) => void; demo: boolean;
}) {
  const t = useT();
  const i18n = useI18n();
  const toast = useToast();
  const [pending, startTransition] = useTransition();
  const [rules, setRules] = useState(p.rules);
  const [of, setOf] = useState(`${p.id}@${p.version}`);
  const [tried, setTried] = useState(false);
  if (of !== `${p.id}@${p.version}`) {
    setOf(`${p.id}@${p.version}`);
    setRules(p.rules);
    setTried(false);
  }
  const [key, setKey] = useState(p.rules[0]?.ruleKey ?? "");
  const idx = Math.max(0, rules.findIndex((r) => r.ruleKey === key));
  const rule = rules[idx];
  const [more, setMore] = useState<string[]>([]); // customers past the first 20 picked in the search-select
  const [switching, setSwitching] = useState<{ customer: { id: string; name: string }; on: boolean } | null>(null);
  const [reason, setReason] = useState("");
  if (!rule) return <Card title={t("Default policy")}><p className="text-[13px] text-muted">{t("The default policy has no rules.")}</p></Card>;

  const edit = (patch: Partial<DefaultRule>) => setRules((rs) => rs.map((r, i) => (i === idx ? { ...r, ...patch } : r)));
  const errorsOf = (r: DefaultRule) => conditionErrors(r, t);
  const errors = errorsOf(rule);
  const broken = rules.filter((r) => Object.keys(errorsOf(r)).length);
  const changed = ruleChanges({ ...p, rules }, p);
  const status = tried && broken.length ? t(broken.length === 1 ? "VALIDATION · 1 rule to fix · nothing saved: {names}" : "VALIDATION · {n} rules to fix · nothing saved: {names}", { n: broken.length, names: broken.map((r) => r.name).join(", ") })
    : changed.length ? `${t(changed.length === 1 ? "1 rule changed" : "{n} rules changed", { n: changed.length })} · ${changed.join(", ")}` : t("No unsaved changes");
  const sorted = [...customers].sort((a, b) => a.name.localeCompare(b.name));
  const listed = [...sorted.slice(0, SHOWN), ...sorted.slice(SHOWN).filter((c) => more.includes(c.id))];
  const setting = (customerId: string) => p.settings.find((s) => s.ruleKey === rule.ruleKey && s.customerId === customerId);
  // the demo test runs the saved rule on a unit of the scoped customer, else of any customer (the default policy is on
  // every unit), whose customer has not switched the rule off — there it would never alert
  const savedRule = p.rules.find((r) => r.ruleKey === rule.ruleKey) ?? rule;
  const inScope = copyFor ? units.filter((u) => u.customerId === copyFor.id) : units;
  const testUnit = inScope.find((u) => setting(u.customerId ?? "")?.enabled !== false) ?? null;
  const previewUnit = testUnit ?? inScope[0] ?? null;
  const customerOf = (u: { customerId: string | null }) => customers.find((c) => c.id === u.customerId)?.name ?? "";

  const save = () => {
    setTried(true);
    if (broken.length) return toast(t("Fix the validation errors first"), "crit");
    if (demo) return toast(t("Default limits saved for every unit"));
    startTransition(async () => {
      const r = await saveDefaultRules({ ...p, rules });
      if (!r.ok) return toast(actionMessage(r, t), "crit");
      toast(t("Default limits saved for every unit"));
      setTried(false);
    });
  };
  const confirmSwitch = () => {
    if (!switching || reason.length > 500) return;
    const { customer, on } = switching;
    const done = () => { toast(t(on ? "{rule} switched on for {name}" : "{rule} switched off for {name}", { rule: rule.name, name: customer.name })); setSwitching(null); setReason(""); };
    if (demo) return done();
    startTransition(async () => {
      const r = await switchDefaultRule(p.id, rule.ruleKey, customer.id, on, reason, setting(customer.id)?.version ?? 0);
      if (!r.ok) return toast(actionMessage(r, t), "crit");
      done();
    });
  };

  return (
    <Card title={<span className="inline-flex flex-wrap items-center gap-2">{t("Default policy")} › {rule.name}<Badge tone="muted">{t("Policy v{n}", { n: p.version })}</Badge></span>}
      sub={`${p.id} · ${t("HQ template")} · ${t("rule {n} of {total}", { n: idx + 1, total: rules.length })}`} action={<Badge tone="ok">{t("Enabled (template)")}</Badge>}>
      <div className="flex flex-col gap-5">
        <Banner tone="warn" action={canWrite && copyable(rule) ? <Btn size="sm" onClick={() => onCopy(rule.ruleKey)}>{copyFor ? t("Copy as a {customer} policy", { customer: copyFor.name }) : t("Copy as a customer policy")}<Icon name="arrowRight" /></Btn> : undefined}>
          <span><b>{t("Default policy rule — runs on every unit of every customer")}</b><span className="block text-xs">{t("Saving changes affects all units. Customers and HQ can add their own policies on top; this rule cannot be detached from a unit.")}</span>
            {!copyable(rule) && <span className="block text-xs">{t("{metric} is not a metric a customer policy can watch, so this rule cannot be copied.", { metric: metricLabel[rule.metric] ? t(metricLabel[rule.metric]) : rule.metric })}</span>}</span>
        </Banner>
        {!canWrite && <Banner>{t("You can read the default policy. Changing it needs alert.policy.write.")}</Banner>}
        <section aria-label={t("Rules")}><h3 className="mb-2 text-[13px] font-bold">{t("Rules ({n})", { n: rules.length })}</h3>
          <div className="grid-fluid" style={{ ["--min" as string]: "240px" }}>
            {rules.map((r) => (
              <ListRow key={r.ruleKey} selected={r.ruleKey === rule.ruleKey} onClick={() => setKey(r.ruleKey)}>
                <div className="min-w-0 flex-1">
                  <b className="text-[13px]">{r.name}</b>{changed.includes(r.name) && <span className="ml-1 text-[11px] text-warn">· {t("edited")}</span>}
                  <div className="text-[11px] text-muted">{t(CATEGORY[r.category] ?? r.category)} · {conditionText({ metric: r.metric, operator: r.operator, threshold: r.threshold, durationSeconds: r.duration, activeWindow: r.activeWindow, severity: r.severity }, i18n)}</div>
                </div>
              </ListRow>
            ))}
          </div>
        </section>
        <fieldset disabled={!canWrite} className="flex min-w-0 flex-col gap-5">
          <section><h3 className="mb-2 text-[13px] font-bold">{t("Condition")}</h3><p className="mb-2 text-xs text-muted">{t("Missing or stale readings never count toward the condition — they raise a data-quality notice instead.")}</p>
            <ConditionFields c={rule} onChange={edit} errors={errors} tried={tried} fixedMetric />
          </section>
          <section><h3 className="mb-1 text-[13px] font-bold">{t("Only if … (optional) · active hours")}</h3>
            <p className="mb-2 text-xs text-muted">{t("Evaluated in the policy timezone. Outside these hours the condition is not checked — the same field as “Only if” in the client app.")}</p>
            <WindowEditor value={rule.activeWindow} onChange={(w) => edit({ activeWindow: w })} timezone={p.timezone} error={errors.activeWindow} />
          </section>
          <section><h3 className="mb-1 text-[13px] font-bold">{t("Severity")}</h3><Choice value={rule.severity} onChange={(v) => edit({ severity: v })} options={[{ id: "normal", label: t("Info") }, { id: "warning", label: t("Warning") }, { id: "critical", label: t("Critical") }]} /><p className="mt-1 text-[11px] text-muted">{t("A severity change on an open alert counts as a new notification reason.")}</p></section>
        </fieldset>
        <section><h3 className="mb-1 text-[13px] font-bold">{t("On / off per customer — this rule")}</h3>
          <p className="mb-2 text-xs text-muted">{t("Each customer (or HQ for them) can switch a default rule off. The limit itself stays the HQ template.")}</p>
          {listed.map((c) => {
            const s = setting(c.id);
            const on = s?.enabled ?? true;
            return (
              <div key={c.id} className="flex flex-wrap items-center gap-3 border-t border-line py-2 text-[13px]">
                <b className="min-w-[140px]">{c.name}</b>
                <span className={cx("min-w-0 flex-1 text-xs", on ? "text-ok" : "text-muted")}>{settingText(s, i18n)}</span>
                <Toggle on={on} label={t("{rule} for {name}", { rule: rule.name, name: c.name })} disabled={!canWrite || pending} onChange={(v) => { setReason(""); setSwitching({ customer: c, on: v }); }} />
              </div>
            );
          })}
          {sorted.length > SHOWN && (
            <div className="mt-2 max-w-sm"><SearchSelect label={t("More customers")} value="" placeholder={t("More customers: type to search")} onChange={(id) => setMore((m) => [...new Set([...m, id])])}
              options={sorted.slice(SHOWN).filter((c) => !more.includes(c.id)).map((c) => ({ id: c.id, label: c.name }))} /></div>
          )}
        </section>
        {previewUnit && <DefaultRulePreview key={previewUnit.id} unit={{ id: previewUnit.id, name: previewUnit.name }} customerName={customerOf(previewUnit)} severity={rule.severity} demo={demo} />}
        {copyable(rule) && testUnit ? (
          <DemoTest key={rule.ruleKey} saved={{ id: p.id, version: p.version, metric: savedRule.metric, operator: savedRule.operator, threshold: savedRule.threshold, on: true }}
            dirty={changed.includes(rule.name)} unit={{ id: testUnit.id, name: testUnit.name }} demo={demo} />
        ) : (
          <section><h3 className="mb-1 text-[13px] font-bold">{t("Test with demo data")}</h3><p className="text-xs text-muted">{t(!copyable(rule) ? "A synthetic reading cannot be entered for this rule's metric."
            : inScope.length ? "The rule is off for every customer in scope, so it cannot be tested here." : "There is no unit to test the rule on.")}</p></section>
        )}
        <section><h3 className="mb-1 text-[13px] font-bold">{t("Owner & units")}</h3>
          <div className="text-xs font-semibold">{t("Owner")}</div><div className="mt-1"><Badge tone="primary">{t("HQ default (all customers)")}</Badge></div>
          <p className="mt-2 rounded-xl bg-surface2 px-3 py-2 text-[13px]">{t("Attached to every unit of every customer — automatically, and it cannot be detached. To change a limit for one customer only, copy the rule as that customer’s policy.")}</p>
        </section>
        {canWrite && (
          <div className="flex flex-wrap items-center justify-end gap-2 border-t border-line pt-3">
            <span className={cx("mr-auto text-[12px]", tried && broken.length ? "text-crit" : "text-muted")}>{status}</span>
            <Btn disabled={pending || !changed.length} onClick={() => { setRules(p.rules); setTried(false); }}>{t("Discard")}</Btn>
            <Btn variant="primary" disabled={pending || !changed.length} onClick={save}>{t("Save policy")}</Btn>
          </div>
        )}
      </div>
      {switching && (
        <Modal open onClose={() => setSwitching(null)} title={t(switching.on ? "Switch “{rule}” on for {name}" : "Switch “{rule}” off for {name}", { rule: rule.name, name: switching.customer.name })}
          footer={<><Btn onClick={() => setSwitching(null)}>{t("Cancel")}</Btn><Btn variant="primary" disabled={pending || reason.length > 500} onClick={confirmSwitch}>{t(switching.on ? "Switch on" : "Switch off")}</Btn></>}>
          <p className="text-[13px] text-muted">{t(switching.on ? "The rule runs again on every unit of {name}." : "The rule stops on every unit of {name}; its limit stays the HQ template for everyone else.", { name: switching.customer.name })}</p>
          <Field label={t("Reason · optional")} hint={`${reason.length} / 500`} error={reason.length > 500 ? t("At most 500 characters") : undefined}><Textarea value={reason} onChange={(e) => setReason(e.target.value)} /></Field>
        </Modal>
      )}
    </Card>
  );
}
