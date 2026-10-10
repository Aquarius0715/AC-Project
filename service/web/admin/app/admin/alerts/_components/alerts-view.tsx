"use client";

import Link from "next/link";
import { useState, useTransition } from "react";
import { usePathname, useRouter } from "next/navigation";
import { Badge, Banner, Btn, Card, Check, Choice, EmptyState, Field, Input, ListRow, Modal, Page, Select, SeverityBadge, SummaryList, Tabs, Textarea, Timeline, LineChart, useToast, cx } from "@ac/web/components/ui";
import { useT } from "@ac/web/components/I18n";
import { useUrlTab } from "@ac/web/lib/useUrlTab";
import { useUrlPatch } from "@ac/web/lib/useUrlPatch";
import { actionMessage } from "@ac/web/lib/actionMessage";
import { HQ_METRICS, metricLabel, opSymbol, policyErrors, type AdminAlert, type AdminPolicy, type AlertState, type Channel, type Severity } from "@ac/web/lib/adminAlerts";
import { acknowledgeAlert, resolveAlert, savePolicy, type ActionResult } from "../actions";
import { ScopeBar } from "../../jobs/_components/jobs-header";

const ST: Record<AlertState, string> = { open: "Open", acknowledged: "Acknowledged", resolved: "Resolved" };
const LIST: Record<AlertState, string> = { open: "Open alerts", acknowledged: "Acknowledged alerts", resolved: "Resolved alerts" };
const SEVERITY: [Severity, string][] = [["critical", "Severity: Critical"], ["warning", "Severity: Warning"], ["normal", "Severity: Info"]];
// the Phase 1A demo's fixture rows (their data stays English, as the other demo screens)
const demoTimeline = (time: string, st: AlertState): AdminAlert["timeline"] => [
  { time, title: "Alert detected", detail: "Load increase with stable outdoor temperature", tone: "warn" },
  { time: "08:50", title: "Notified customer-a · In-app", detail: "deliveryState: simulated" },
  { time: "08:52", title: "customer-a read the notification", detail: "Reading a notification does not acknowledge or resolve the alert" },
  { time: "—", title: st === "resolved" ? "Resolved" : st === "acknowledged" ? "Acknowledged — waiting for resolution" : "Waiting for acknowledgement", detail: "", tone: st === "resolved" ? "ok" : undefined },
];
const demoAlert = (id: string, title: string, time: string, sev: Severity, meta: string): AdminAlert => ({
  id, version: 1, title, time, sev, meta, state: "open", st: ST.open, unitId: "unit-online-rto", customerId: "customer-a", policyless: id !== "alert-temp-a", inferred: true,
  unit: "Bedroom AC · unit-online-rto", context: "customer-a · Home A › 1F › Bedroom · Running · Online",
  evidence: "inferred — “Demo: cooling load rose while the outdoor temperature was stable.”", cause: "window_open (suspected)", observed: `${time} MYT`, detected: `${time} MYT`,
  evidenceRecords: "none attached", timeline: demoTimeline(time, "open"),
});
const seedAlerts: AdminAlert[] = [
  demoAlert("alert-c08-critical", "High temperature observed", "08:55", "critical", "customer-a · Bedroom AC · no policy"),
  demoAlert("alert-insulation-a", "Poor insulation suspected", "08:45", "warning", "customer-a · Bedroom AC"),
  demoAlert("alert-temp-a", "Temperature ≥ 30 °C for 60 s", "08:58", "warning", "customer-a · Bedroom AC"),
  demoAlert("alert-unknown-a", "Cooling load increase", "08:48", "warning", "customer-a · Bedroom AC"),
  demoAlert("alert-window-a", "Possible open window", "08:50", "warning", "customer-a · Bedroom AC"),
];
const demoRecipients = [{ id: "customer-a", label: "customer-a", role: "client" }, { id: "hq-operator", label: "hq-operator", role: "admin" }];
const demoPolicy = (id: string, name: string, sub: string, metric: string, threshold: number, recovery: number, duration: number, on: boolean, units: string): AdminPolicy => ({
  id, version: 1, kind: "alert", name, group: "CUSTOMER-A", sub, on, units, customerId: "customer-a", customerName: "customer-a", priority: 50, timezone: "Asia/Kuala_Lumpur",
  metric, operator: "gte", threshold, recovery, duration, severity: "warning", cooldown: 5, escalate: 60, channels: ["inApp", "email"], recipients: demoRecipients,
  recipientIds: [], activeWindow: null,
});
const seedPolicies: AdminPolicy[] = [
  { ...demoPolicy("policy-default", "Default policy", "6 rules · ventilation (CO₂, PM2.5) + fault causes", "co2", 1000, 900, 600, true, "every unit"), kind: "default_alert", group: "DEFAULT · ON EVERY UNIT", customerId: null },
  demoPolicy("policy-temp-a", "Bedroom too hot", "Temperature ≥ 30 °C for 60 s", "temperature", 30, 28, 60, true, "2 units"),
  demoPolicy("policy-co2-a", "Stuffy office", "CO₂ ≥ 1200 ppm · 15 min · weekdays", "co2", 1200, 1000, 900, true, "2 units"),
  demoPolicy("policy-humidity-a", "Night humidity", "Humidity ≥ 70 % · 30 min · 22–06", "humidity", 70, 65, 1800, false, "0 units"),
];

type Scope = { customers: { id: string; name: string }[]; properties: { id: string; name: string }[]; units: { id: string; name: string }[] };
type Query = { customerId?: string; propertyId?: string; unitId?: string; severity: Severity | null; status: AlertState; alertId: string | null };
type Live = { tab: "alerts" | "policies"; q: Query; scope: Scope; scopeName: string | null; alerts: AdminAlert[]; policies: AdminPolicy[] };

/** HQ alerts and alert policies. `live` comes from the Server Component in API mode (writes are Server Actions that
 * re-render the route); the demo keeps its fixture rows in local state. Texts in the display language (IR292). */
export function AdminAlertsView({ live }: { live?: Live }) {
  const t = useT();
  const toast = useToast();
  const router = useRouter();
  const pathname = usePathname();
  const patch = useUrlPatch();
  const [pending, startTransition] = useTransition();
  const [urlTab, setUrlTab] = useUrlTab<"alerts" | "policies">({ alerts: "alerts", policies: "policies" }, "alerts");
  const tab = live ? live.tab : urlTab;
  const setTab = (v: "alerts" | "policies") => (live ? router.replace(v === "policies" ? `${pathname}?tab=policies` : pathname, { scroll: false }) : setUrlTab(v));

  // alerts: the demo changes its local copy; API mode shows the server rows
  const [demoList, setDemoList] = useState(seedAlerts);
  const al = live ? live.alerts : demoList;
  // API mode keeps the status, scope, severity and selection in the URL (DD-A05 item 5); the demo in local state
  const [demoStatus, setDemoStatus] = useState<AlertState>("open");
  const [demoAlertId, setDemoAlertId] = useState<string | null>(null);
  const status = live ? live.q.status : demoStatus;
  const shown = al.filter((x) => x.state === status);
  const a = al.find((x) => x.id === (live ? live.q.alertId : demoAlertId)) ?? shown[0] ?? null;
  const pick = (id: string) => (live ? patch({ alertId: id }) : setDemoAlertId(id));
  const pickStatus = (s: AlertState) => (live ? patch({ status: s === "open" ? null : s, alertId: null }) : (setDemoStatus(s), setDemoAlertId(null)));
  const [resolve, setResolve] = useState(false);
  const [reason, setReason] = useState("");
  const [tried, setTried] = useState(false);

  // policies: the editor holds a draft of the selected policy, restarted when the selection or its version changes
  const pols = live ? live.policies : seedPolicies;
  const [polId, setPolId] = useState<string | null>(null);
  const p = pols.find((x) => x.id === polId) ?? pols.find((x) => x.kind === "alert") ?? pols[0] ?? null;
  const [draft, setDraft] = useState<AdminPolicy | null>(p);
  const [draftOf, setDraftOf] = useState(p ? `${p.id}@${p.version}` : "");
  if (p && draftOf !== `${p.id}@${p.version}`) {
    setDraftOf(`${p.id}@${p.version}`);
    setDraft(p);
  }
  const set = <K extends keyof AdminPolicy>(k: K, v: AdminPolicy[K]) => setDraft((d) => (d ? { ...d, [k]: v } : d));
  const errors = draft ? policyErrors(draft, t) : {};

  const act = (fn: () => Promise<ActionResult>, ok: string, after?: () => void) =>
    startTransition(async () => {
      const r = await fn();
      if (r.ok) {
        toast(ok);
        after?.();
      } else toast(actionMessage(r, t), "crit");
    });
  const demoState = (id: string, state: AlertState) => setDemoList((l) => l.map((x) => (x.id === id ? { ...x, state, st: t(ST[state]), timeline: demoTimeline(x.time, state) } : x)));
  const acknowledge = () => {
    if (!a) return;
    if (live) return act(() => acknowledgeAlert(a.id, a.version), t("Acknowledged"));
    demoState(a.id, "acknowledged");
    toast(t("Acknowledged"));
  };
  const doResolve = () => {
    setTried(true);
    if (!a || !reason.trim() || reason.length > 1000) return;
    if (live) return act(() => resolveAlert(a.id, a.version, reason.trim()), t("Alert resolved"), () => { setResolve(false); setReason(""); setTried(false); });
    demoState(a.id, "resolved");
    setResolve(false);
    toast(t("Alert resolved"));
  };
  const save = () => {
    setTried(true);
    if (!draft || Object.keys(errors).length) return toast(t("Fix the validation errors first"), "crit");
    if (live) return act(() => savePolicy(draft), t("Policy saved"), () => setTried(false));
    toast(t("Policy saved"));
  };
  const toggleChannel = (c: Channel, on: boolean) => draft && set("channels", on ? [...new Set([...draft.channels, c])] : draft.channels.filter((x) => x !== c));
  const groups = [...new Set(pols.map((x) => x.group))];
  const counts: [AlertState, number][] = [["open", al.filter((x) => x.state === "open").length], ["acknowledged", al.filter((x) => x.state === "acknowledged").length], ["resolved", al.filter((x) => x.state === "resolved").length]];
  const openish = al.filter((x) => x.state !== "resolved");
  const scopeName = live?.scopeName ?? t("all customers");

  return (
    <Page>
      <Tabs value={tab} onChange={setTab} tabs={[{ id: "alerts", label: t("Alerts"), count: openish.length }, { id: "policies", label: t("Policies"), count: pols.length }]} />
      {tab === "alerts" ? (
        <>
          {live && <ScopeBar scope={live.scope} q={live.q} text={t(al.length === 1 ? "1 alert in scope" : "{n} alerts in scope", { n: al.length })} clear="alertId" />}
          <div className="flex flex-wrap items-stretch gap-3">
            {counts.map(([s, n]) => (
              <button key={s} type="button" aria-pressed={status === s} onClick={() => pickStatus(s)} className={cx("min-w-[140px] flex-1 rounded-2xl border bg-surface p-4 text-left", status === s ? "border-primary ring-1 ring-primary" : "border-line hover:bg-surface2")}>
                <div className="text-xs text-muted">{t(ST[s])}</div><div className="text-2xl font-bold">{n}</div>
              </button>
            ))}
            {live && (
              <Select aria-label={t("Severity")} className="w-auto self-center" value={live.q.severity ?? ""} onChange={(e) => patch({ severity: e.target.value || null, alertId: null })}>
                <option value="">{t("Severity: All")}</option>{SEVERITY.map(([v, l]) => <option key={v} value={v}>{t(l)}</option>)}
              </Select>
            )}
          </div>
          <div className="split-rev">
            <Card title={`${t(LIST[status])} · ${scopeName}`} sub={t("severity ↓")} className="self-start">
              <div className="flex flex-col gap-2">
                {shown.length === 0 ? <EmptyState title={t("No alerts in this scope")}>{t("Change the scope, severity or status.")}</EmptyState> : shown.map((x) => (
                  <ListRow key={x.id} selected={a?.id === x.id} onClick={() => pick(x.id)}>
                    <div className="min-w-0 flex-1"><div className="flex items-center justify-between gap-2"><b className="text-[13px]">{x.title}</b><span className="text-[11px] text-muted">{x.time}</span></div><div className="mt-1 flex flex-wrap items-center gap-2"><SeverityBadge s={x.sev} /></div><div className="mt-1 truncate text-[11px] text-muted">{x.meta}</div></div>
                  </ListRow>
                ))}
              </div>
            </Card>
            {!a ? <Card title={t("Alert")}><EmptyState title={t("No alert selected")}>{t("Pick an alert in the list.")}</EmptyState></Card> : (
              <div className="flex min-w-0 flex-col gap-4">
                <Card title={a.title} sub={`${a.id} · ${t("detected {time}", { time: a.detected })}`} action={
                  <div className="flex flex-wrap justify-end gap-2">
                    <Btn size="sm" disabled={a.state !== "open" || pending} onClick={acknowledge}>{t("Acknowledge")}</Btn>
                    <Btn size="sm" variant="primary" disabled={a.state === "resolved" || pending} onClick={() => setResolve(true)}>{t("Resolve…")}</Btn>
                    {a.state !== "resolved" && <Link href={`/admin/jobs?new=${a.unitId}&alertId=${a.id}`} className="inline-flex items-center rounded-control border border-line bg-surface px-2.5 py-1 text-xs font-semibold hover:bg-surface2">{t("Request maintenance")}</Link>}
                    <Link href={a.customerId ? `/admin/units?customerId=${a.customerId}&unitId=${a.unitId}` : `/admin/units?unitId=${a.unitId}`} className="inline-flex items-center px-1 text-xs font-semibold text-primary">{t("Open unit →")}</Link>
                  </div>
                }>
                  <SummaryList items={[[t("Unit"), a.unit], [t("Context"), a.context], [t("Severity"), <SeverityBadge key="s" s={a.sev} />], [t("Status"), a.st]]} />
                </Card>
                <Card title={t("Why we think this")}>
                  <SummaryList items={[[t("Evidence"), a.evidence], [t("Cause"), a.cause], [t("Observed at"), a.observed], [t("Evidence records"), a.evidenceRecords]]} />
                  {a.inferred && <p className="mt-2 text-xs text-muted">{t("This is an inference, not a confirmed cause. Ask the customer to check windows, or request an inspection to record evidence.")}</p>}
                </Card>
                <Card title={t("Activity")}>
                  <Timeline items={a.timeline} />
                  <p className="mt-2 text-[11px] text-muted">{t("Reading a notification does not acknowledge or resolve the alert")}{a.policyless ? ` · ${t("No policy escalation — this alert was not raised by a policy")}` : ""}</p>
                  <p className="mt-1 text-[11px] text-muted">{t("Resolving requires a reason and evidence (remeasurement or a recorded confirmation). Completing maintenance work does not resolve the alert by itself.")}</p>
                </Card>
              </div>
            )}
          </div>
        </>
      ) : !p || !draft ? (
        <Card title={t("Alert policies")}><EmptyState title={t("No alert policies")}>{t("Customers have no alert policies yet.")}</EmptyState></Card>
      ) : (
        <div className="split-rev">
          <Card title={t("Alert policies")} sub={t("by customer")} action={!live && <Btn size="sm" variant="primary" onClick={() => toast(t("New policy draft created"))}>{t("+ New")}</Btn>} className="self-start">
            {groups.map((g) => <div key={g} className="mb-3"><div className="mb-1 text-[10px] font-bold tracking-wide text-muted">{g}</div><div className="flex flex-col gap-2">{pols.filter((x) => x.group === g).map((x) => <ListRow key={x.id} selected={p.id === x.id} onClick={() => { setPolId(x.id); setTried(false); }}><div className="min-w-0 flex-1"><b className="text-[13px]">{x.name}</b><div className="text-[11px] text-muted">{x.sub}</div><div className="text-[11px] text-muted">{x.units}{!x.on && ` · ${t("Off")}`}</div></div></ListRow>)}</div></div>)}
          </Card>
          <Card title={p.name} sub={`${p.id}${p.customerName ? ` · ${p.customerName}` : ""}`} action={<Badge tone={p.on ? "ok" : "muted"}>{t(p.on ? "Enabled" : "Disabled")}</Badge>}>
            {p.kind === "default_alert" ? <Banner>{t("Default policy rules are set by HQ for every unit. Per-customer on/off is controlled by the customer.")}</Banner> : (
              <div className="flex flex-col gap-5">
                <section><h3 className="mb-2 text-[13px] font-bold">{t("Basics")}</h3><div className="grid-fluid" style={{ ["--min" as string]: "180px" }}><Field label={t("Name")} error={tried ? errors.name : undefined}><Input value={draft.name} onChange={(e) => set("name", e.target.value)} /></Field><Field label={t("Priority")} hint="0–100"><Input type="number" value={draft.priority} onChange={(e) => set("priority", +e.target.value)} /></Field><Field label={t("Timezone")} hint={t("The zone the policy’s active window is evaluated in")}><Select value={draft.timezone} onChange={(e) => set("timezone", e.target.value)}><option>Asia/Kuala_Lumpur</option></Select></Field></div></section>
                <section><h3 className="mb-1 text-[13px] font-bold">{t("Owner & units")}</h3><p className="text-xs text-muted">{t("A policy belongs to one customer. Units pick it up on their own page (Customers & units › unit › Alert policies). Attached to {units} of {customer} · read-only here.", { units: p.units, customer: p.customerName })}</p></section>
                <section><h3 className="mb-2 text-[13px] font-bold">{t("Condition")}</h3><p className="mb-2 text-xs text-muted">{t("Missing or stale readings never count toward the condition — they raise a data-quality notice instead.")}</p><div className="grid-fluid" style={{ ["--min" as string]: "130px" }}><Field label={t("Metric")}><Select value={draft.metric} onChange={(e) => set("metric", e.target.value)}>{HQ_METRICS.map((k) => <option key={k} value={k}>{t(metricLabel[k])}</option>)}</Select></Field><Field label={t("Alert when {op}", { op: opSymbol[draft.operator] })}><Input type="number" value={draft.threshold} onChange={(e) => set("threshold", +e.target.value)} /></Field><Field label={t("For (s)")} error={errors.duration}><Input type="number" value={draft.duration} onChange={(e) => set("duration", +e.target.value)} /></Field><Field label={t("Recover")} error={errors.recovery}><Input type="number" value={draft.recovery} onChange={(e) => set("recovery", +e.target.value)} /></Field></div>
                  <div className="mt-2"><LineChart points={[20, 24, 28, 31, 32, 31, 29, 27, 25]} min={18} max={34} threshold={draft.threshold} height={90} /></div>
                  <p className={cx("text-xs", errors.recovery ? "text-crit" : "text-ok")}>{errors.recovery ? `✕ ${errors.recovery}` : t("✓ Recovery {recovery} is on the safe side of the {threshold} threshold ({op})", { recovery: draft.recovery, threshold: draft.threshold, op: opSymbol[draft.operator] })}</p></section>
                <section><h3 className="mb-1 text-[13px] font-bold">{t("Severity")}</h3><Choice value={draft.severity} onChange={(v) => set("severity", v)} options={[{ id: "normal", label: t("Info") }, { id: "warning", label: t("Warning") }, { id: "critical", label: t("Critical") }]} /><p className="mt-1 text-[11px] text-muted">{t("A severity change on an open alert counts as a new notification reason.")}</p></section>
                <section><h3 className="mb-2 text-[13px] font-bold">{t("Notify")}</h3><p className="mb-2 text-xs text-muted">{t("Recipients come from notifications.recipients — only people allowed to see these units.")}</p>
                  {draft.recipients.length === 0 ? <p className="text-xs text-muted">{t("No recipients.")}</p> : draft.recipients.map((r) => <div key={r.id} className="flex flex-wrap items-center justify-between gap-2 border-t border-line py-2 text-[13px]"><span><b>{r.label}</b><span className="block text-xs text-muted">{r.role}</span></span></div>)}
                  <div className="flex flex-wrap gap-3 border-t border-line pt-2"><span className="text-xs text-muted">{t("Channels for every recipient:")}</span><Check label={t("In-app")} checked={draft.channels.includes("inApp")} onChange={(v) => toggleChannel("inApp", v)} /><Check label={t("Email")} checked={draft.channels.includes("email")} onChange={(v) => toggleChannel("email", v)} /><Check label="WhatsApp" checked={draft.channels.includes("whatsapp")} onChange={(v) => toggleChannel("whatsapp", v)} /></div>
                  <p className="text-[11px] text-muted">{t("WhatsApp reaches only recipients who allowed that channel.")}</p></section>
                <section className="grid-fluid" style={{ ["--min" as string]: "180px" }}><Field label={t("Cooldown (min)")} hint={t("Same severity is not re-sent within this window (1–1440)")} error={errors.cooldown}><Input type="number" value={draft.cooldown} onChange={(e) => set("cooldown", +e.target.value)} /></Field><Field label={t("Escalate after (min)")} hint={t("Escalate if still unacknowledged (1–1440)")} error={errors.escalate}><Input type="number" value={draft.escalate} onChange={(e) => set("escalate", +e.target.value)} /></Field></section>
                <div className="flex justify-end"><Btn variant="primary" disabled={pending} onClick={save}>{t("Save policy")}</Btn></div>
              </div>
            )}
          </Card>
        </div>
      )}
      <Modal open={resolve} onClose={() => setResolve(false)} title={t("Resolve alert")} footer={<><Btn onClick={() => setResolve(false)}>{t("Cancel")}</Btn><Btn variant="primary" disabled={pending} onClick={doResolve}>{t("Resolve")}</Btn></>}><p className="text-xs text-muted">{t("Resolving requires a reason and evidence. Completing maintenance work does not resolve the alert by itself.")}</p><Field label={t("Reason (required)")} error={tried && !reason.trim() ? t("A reason is required") : tried && reason.length > 1000 ? t("At most 1000 characters") : undefined}><Textarea value={reason} onChange={(e) => setReason(e.target.value)} /></Field></Modal>
    </Page>
  );
}
