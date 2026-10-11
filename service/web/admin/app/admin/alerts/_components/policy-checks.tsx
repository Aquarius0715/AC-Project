"use client";

import { useState, useTransition } from "react";
import { Btn, Input, Select, SeverityBadge, useToast, cx } from "@ac/web/components/ui";
import { useI18n, useT } from "@ac/web/components/I18n";
import { actionMessage } from "@ac/web/lib/actionMessage";
import { useOp } from "@ac/web/lib/useOp";
import { inboxRow, type ApiNotification } from "@ac/web/lib/notifications";
import { metricLabel, metricUnit, recipientRole, testOutcome, type ApiRecipient, type NotificationDecision, type Operator, type PolicyRecipient, type Severity } from "@ac/web/lib/adminAlerts";
import { testPolicy } from "../actions";
import { Icon } from "@ac/web/components/Icon";

type Unit = { id: string; name: string };
const TONE = { ok: "text-ok", warn: "text-warn", crit: "text-crit", muted: "text-muted-foreground" } as const;

/** Notification preview (DD-A05 item 7, Figma Admin 38:5 / 268:2, IR348): what one recipient is shown for one of the
 * policy's units — `notifications.preview` of the alert template with the policy's (or rule's) severity, as the
 * notifier sends it. Read while the section is shown; nothing is stored. The demo shows the same row from its fixture. */
export function NotificationPreview({ recipients, severity, customerName, unit, demo }: { recipients: PolicyRecipient[]; severity: Severity; customerName: string; unit: Unit | null; demo: boolean }) {
  const t = useT();
  const i18n = useI18n();
  const [as, setAs] = useState("");
  const who = recipients.find((r) => r.id === as) ?? recipients[0] ?? null;
  const fixture: ApiNotification | null = demo && unit ? {
    id: "preview", version: 1, type: "fault", sourceAlertId: null, target: { kind: "unit", id: unit.id }, templateKey: "alert", severity,
    params: { targetName: unit.name, at: "2026-09-14T01:00:00Z", status: "open", reason: null, message: null }, channel: "inApp", occurredAt: "2026-09-14T01:00:00Z", readAt: null,
  } : null;
  const preview = useOp<ApiNotification, ApiNotification | null>("notifications.preview",
    { target: { kind: "unit", id: unit?.id ?? "" }, templateKey: "alert", channel: "inApp", recipientMembershipId: who?.id ?? "", severity }, fixture, (d) => d, !!unit && !!who);
  const row = preview.data ? inboxRow(preview.data, "client", i18n.display) : null;
  return (
    <section>
      <div className="mb-1 flex flex-wrap items-center justify-between gap-2">
        <h3 className="text-[13px] font-bold">{t("Notification preview")}</h3>
        {recipients.length > 1 && (
          <Select aria-label={t("Preview as")} className="w-auto" value={who?.id ?? ""} onChange={(e) => setAs(e.target.value)}>
            {recipients.map((r) => <option key={r.id} value={r.id}>{t("Preview as {name}", { name: r.label })}</option>)}
          </Select>
        )}
      </div>
      {!unit ? <p className="text-xs text-muted-foreground">{t("The preview needs a unit of {customer}, and it has none yet.", { customer: customerName })}</p>
        : !who ? <p className="text-xs text-muted-foreground">{t("Add a recipient to see what they are shown.")}</p>
        : (
          <>
            <p className="mb-2 text-xs text-muted-foreground">{t("What {name} sees for {unit} (notifications.preview · in-app).", { name: who.label, unit: unit.name })}</p>
            {preview.loading ? <p className="text-xs text-muted-foreground">{t("Loading the preview…")}</p>
              : preview.error ? <p className="text-xs text-crit">{preview.error.error.messageKey === "errors.recipient_ineligible" ? t("{name} cannot get this notification: they may not see {unit}.", { name: who.label, unit: unit.name }) : t("The preview could not be loaded — try again.")}</p>
              : row && preview.data && (
                <div className="flex items-start gap-3 rounded-xl border border-line bg-surface2/60 px-3 py-2.5">
                  {preview.data.severity && <SeverityBadge s={preview.data.severity} />}
                  <div className="min-w-0 flex-1 text-[13px]">
                    <div className="flex flex-wrap items-baseline justify-between gap-2"><b>{row.t}</b><span className="text-[11px] text-muted-foreground">{row.w}</span></div>
                    {row.d && <div className="text-xs text-muted-foreground">{row.d}</div>}
                    <div className="mt-1 text-xs text-muted-foreground">{t("Open unit")} <Icon name="arrowRight" /> · {t("Request maintenance")} <Icon name="arrowRight" /></div>
                  </div>
                </div>
              )}
          </>
        )}
    </section>
  );
}

/** What the test needs of a saved policy, or of the default policy's rule: the condition and whether it is on. */
export type Tested = { id: string; version: number; metric: string; operator: Operator; threshold: number; on: boolean };

/** The demo's outcome without the Core API: the policy's own condition on the reading. */
const demoDecision = (p: Tested, unitId: string, value: number): NotificationDecision => {
  const meets = p.operator === "gt" ? value > p.threshold : p.operator === "gte" ? value >= p.threshold : p.operator === "lt" ? value < p.threshold : value <= p.threshold;
  return { unitId, policyId: p.id, decision: !p.on || !meets ? "suppressed" : "selected", reason: !p.on ? "disabled" : meets ? null : "not_due" };
};

/** Test with demo data (DD-A05 item 7, Figma Admin 38:5 / 268:2, IR348): the saved policy — or the default policy's saved
 * rule — evaluated against one synthetic, valid reading on one of its units (`automations.simulate`; the demo evaluation
 * takes the reading as lasting the duration). Nothing is stored. */
export function DemoTest({ saved, dirty, unit, demo }: { saved: Tested; dirty: boolean; unit: Unit | null; demo: boolean }) {
  const t = useT();
  const toast = useToast();
  const [pending, startTransition] = useTransition();
  const [value, setValue] = useState(String(saved.threshold));
  const [result, setResult] = useState<{ tone: keyof typeof TONE; text: string } | null>(null);
  const [of, setOf] = useState(`${saved.id}@${saved.version}`);
  if (of !== `${saved.id}@${saved.version}`) { // another policy or a newer version: start again
    setOf(`${saved.id}@${saved.version}`);
    setValue(String(saved.threshold));
    setResult(null);
  }
  const unitSymbol = metricUnit[saved.metric] ?? "";
  const run = () => {
    if (!unit) return;
    const v = Number(value);
    if (value.trim() === "" || !Number.isFinite(v)) return setResult({ tone: "crit", text: t("Enter a reading first.") });
    if (demo) return setResult(testOutcome(demoDecision(saved, unit.id, v), saved, v, unit.name, t));
    startTransition(async () => {
      const r = await testPolicy(saved.id, unit.id, saved.metric, v);
      if (!r.ok) return toast(actionMessage(r, t), "crit");
      setResult(testOutcome(r.decision, saved, v, unit.name, t));
    });
  };
  return (
    <section>
      <h3 className="mb-1 text-[13px] font-bold">{t("Test with demo data")}</h3>
      <p className="mb-2 text-xs text-muted-foreground">{t("Evaluates the saved policy against a synthetic reading on one of its units (automations.simulate). The reading counts as lasting the policy's duration; nothing is stored.")}</p>
      {!saved.id ? <p className="text-xs text-muted-foreground">{t("Save the policy first: the test evaluates the saved version.")}</p>
        : !unit ? <p className="text-xs text-muted-foreground">{t("Attach the policy to a unit first: the test runs on one of its units.")}</p>
        : (
          <>
            {dirty && <p className="mb-1 text-xs text-warn">{t("Unsaved changes are not tested: the saved version is.")}</p>}
            <div className="flex flex-wrap items-center gap-2 text-[13px]">
              <span>{t("Reading of {metric}", { metric: metricLabel[saved.metric] ? t(metricLabel[saved.metric]) : saved.metric })}</span>
              <Input aria-label={t("Reading")} type="number" className="w-28" value={value} onChange={(e) => setValue(e.target.value)} />
              <span>{unitSymbol}</span><span>{t("on {name}", { name: unit.name })}</span>
              <Btn size="sm" disabled={pending} onClick={run}>{t("Evaluate (demo)")}</Btn>
            </div>
            {result && <p role="status" className={cx("mt-1 flex items-start gap-1.5 text-xs", TONE[result.tone])}><Icon name={result.tone === "ok" ? "success" : result.tone === "crit" ? "error" : result.tone === "warn" ? "warning" : "info"} className="mt-px" />{result.text}</p>}
          </>
        )}
    </section>
  );
}

/** The default policy notifies the active users of the unit's customer in-app (IR155 item 3): they are previewed. */
export function DefaultRulePreview({ unit, customerName, severity, demo }: { unit: Unit; customerName: string; severity: Severity; demo: boolean }) {
  const t = useT();
  const users = useOp<{ items: ApiRecipient[] }, ApiRecipient[]>("notifications.recipients", { target: { kind: "unit", id: unit.id }, templateKey: "alert", channel: "inApp", role: "client", query: { limit: 20 } },
    demo ? [{ id: "customer-a", role: "client", displayLabel: "customer-a", allowedChannels: ["inApp", "email"] }] : [], (d) => d.items);
  return <NotificationPreview recipients={users.data.map((r) => ({ id: r.id, label: r.displayLabel, role: recipientRole(r.role, t) }))} severity={severity} customerName={customerName} unit={unit} demo={demo} />;
}
