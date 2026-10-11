"use client";

import Link from "next/link";
import { useState, useTransition } from "react";
import { usePathname, useRouter } from "next/navigation";
import { Btn, Card, Check, EmptyState, Field, ListRow, Modal, Page, Select, SeverityBadge, SummaryList, Tabs, Textarea, Timeline, useToast, cx } from "@ac/web/components/ui";
import { useI18n, useT } from "@ac/web/components/I18n";
import { EvidencePicker } from "@ac/web/components/EvidencePicker";
import { useUrlTab } from "@ac/web/lib/useUrlTab";
import { useUrlPatch } from "@ac/web/lib/useUrlPatch";
import { actionMessage } from "@ac/web/lib/actionMessage";
import { evidenceProblem, evidenceRefusal, evidenceRows, type ApiEvidenceCandidate } from "@ac/web/lib/alertEvidence";
import { invalidate, useNow, useOp } from "@ac/web/lib/useOp";
import type { AdminAlert, AdminPolicy, AlertState, Severity } from "@ac/web/lib/adminAlerts";
import { acknowledgeAlert, resolveAlert, type ActionResult } from "../actions";
import { demoCustomers, NewPolicyModal, PoliciesTab, seedPolicies, type PoliciesLive, type PolicySel } from "./policies-tab";
import { ScopeBar } from "../../jobs/_components/jobs-header";
import { Icon } from "@ac/web/components/Icon";

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
// the demo's remeasurements of the Bedroom AC after its alerts (Figma Admin 259:2)
const demoEvidence: ApiEvidenceCandidate[] = [
  { id: "measurement-0920", kind: "remeasurement", observedAt: "2026-09-14T01:20:00Z", metric: "temperature", value: 26.4, unit: "°C", origin: "measured", quality: "valid", eventType: null },
  { id: "measurement-0910", kind: "remeasurement", observedAt: "2026-09-14T01:10:00Z", metric: "temperature", value: 26.9, unit: "°C", origin: "measured", quality: "valid", eventType: null },
];
type Scope = { customers: { id: string; name: string }[]; properties: { id: string; name: string }[]; units: { id: string; name: string }[] };
type Query = { customerId?: string; propertyId?: string; unitId?: string; severity: Severity | null; status: AlertState; alertId: string | null };
type Live = { tab: "alerts" | "policies"; q: Query; scope: Scope; scopeName: string | null; alerts: AdminAlert[]; policies: AdminPolicy[]; pol: PoliciesLive };

/** HQ alerts and alert policies. `live` comes from the Server Component in API mode (writes are Server Actions that
 * re-render the route); the demo keeps its fixture rows in local state. Texts in the display language (IR292). */
export function AdminAlertsView({ live }: { live?: Live }) {
  const t = useT();
  const i18n = useI18n();
  const now = useNow();
  const toast = useToast();
  const router = useRouter();
  const pathname = usePathname();
  const patch = useUrlPatch();
  const [pending, startTransition] = useTransition();
  const [urlTab, setUrlTab] = useUrlTab<"alerts" | "policies">({ alerts: "alerts", policies: "policies" }, "alerts");
  const tab = live ? live.tab : urlTab;
  // the customer → property → unit scope is shared by both tabs, so switching keeps it (DD-A05 items 5 and 8)
  const scoped = (extra: Record<string, string>) => {
    const sp = new URLSearchParams({ ...Object.fromEntries((["customerId", "propertyId", "unitId"] as const).flatMap((k) => (live?.q[k] ? [[k, live.q[k]!]] : []))), ...extra });
    return sp.size ? `${pathname}?${sp}` : pathname;
  };
  const setTab = (v: "alerts" | "policies") => (live ? router.replace(scoped(v === "policies" ? { tab: "policies" } : {}), { scroll: false }) : setUrlTab(v));

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
  // policies: the selected one and a new one's owner customer, in the URL in API mode (DD-A05 items 5 and 8, IR345)
  const [demoSel, setDemoSel] = useState<PolicySel>({ policyId: null, newFor: null, from: null });
  const polSel: PolicySel = live ? { policyId: live.pol.policyId, newFor: live.pol.newFor, from: live.pol.from } : demoSel;
  const selectPolicy = (policyId: string | null, newFor: string | null = null) => (live ? patch({ policyId, new: newFor, from: null }) : setDemoSel({ policyId, newFor, from: null }));
  // “+ New policy” asks for the owner; a default rule's copy remembers the rule it starts from (`from`, DD-A05 item 9)
  const [asking, setAsking] = useState<{ from: string | null } | null>(null);
  const [showOff, setShowOff] = useState(true);
  const startPolicy = (customerId: string, from: string | null = null) => {
    setAsking(null);
    if (live) return router.replace(scoped({ tab: "policies", new: customerId, ...(from ? { from } : {}) }), { scroll: false });
    setUrlTab("policies");
    setDemoSel({ policyId: null, newFor: customerId, from });
  };
  // scoped to one active customer, “+ New policy for <customer>” opens its draft at once (Figma 268:2)
  const scopeOwner = live?.q.customerId ? live.pol.customers.find((c) => c.id === live.q.customerId) ?? null : null;
  const pols = live ? live.policies : seedPolicies;
  const scopeUnit = live?.q.unitId ? live.scope.units.find((u) => u.id === live.q.unitId)?.name ?? live.q.unitId : null;
  const [resolve, setResolve] = useState(false);
  const [reason, setReason] = useState("");
  const [picked, setPicked] = useState<string[]>([]);
  const [tried, setTried] = useState(false);
  // the evidence candidates, read only while the Resolve dialog is open (IR327); the demo lists its fixture readings
  const evidence = useOp<{ items: ApiEvidenceCandidate[] }, ApiEvidenceCandidate[]>("alerts.evidence", { alertId: a?.id ?? "", query: { limit: 100 } }, live ? [] : demoEvidence, (d) => d.items, resolve && !!a);
  const evRows = evidenceRows(evidence.data, (now ?? new Date()).getTime(), i18n);
  const sendable = picked.filter((id) => evRows.some((r) => r.id === id)); // a refreshed list drops what is no longer a candidate
  const evProblem = a ? evidenceProblem(a.policyless, sendable, t) : null;

  const act = (fn: () => Promise<ActionResult>, ok: string, after?: () => void, failed?: () => void) =>
    startTransition(async () => {
      const r = await fn();
      if (r.ok) {
        toast(ok);
        after?.();
      } else {
        toast(evidenceRefusal(r.fieldErrors, t) ?? actionMessage(r, t), "crit");
        failed?.();
      }
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
    if (!a || !reason.trim() || reason.length > 1000 || evProblem) return;
    // a refused evidence ID re-reads the candidates (invalidate)
    if (live) return act(() => resolveAlert(a.id, a.version, reason.trim(), sendable), t("Alert resolved"), () => { setResolve(false); setReason(""); setPicked([]); setTried(false); }, invalidate);
    demoState(a.id, "resolved");
    setResolve(false);
    toast(t("Alert resolved"));
  };
  const counts: [AlertState, number][] = [["open", al.filter((x) => x.state === "open").length], ["acknowledged", al.filter((x) => x.state === "acknowledged").length], ["resolved", al.filter((x) => x.state === "resolved").length]];
  const openish = al.filter((x) => x.state !== "resolved");
  const scopeName = live?.scopeName ?? t("all customers");

  return (
    <Page>
      <div className="flex flex-wrap items-center justify-between gap-2">
        <Tabs value={tab} onChange={setTab} tabs={[{ id: "alerts", label: t("Alerts"), count: openish.length }, { id: "policies", label: t("Policies"), count: pols.length }]} />
        {(live ? live.pol.canWrite : true) && (scopeOwner
          ? <Btn size="sm" variant="primary" onClick={() => startPolicy(scopeOwner.id)}><Icon name="add" /> {t("New policy for {name}", { name: scopeOwner.name })}</Btn>
          : <Btn size="sm" variant="primary" onClick={() => setAsking({ from: null })}><Icon name="add" /> {t("New policy")}</Btn>)}
      </div>
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
            <Card title={`${t(LIST[status])} · ${scopeName}`} sub={t("most severe first")} className="self-start">
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
                    <Btn size="sm" variant="primary" disabled={a.state === "resolved" || pending} onClick={() => { setPicked([]); setTried(false); setResolve(true); }}>{t("Resolve…")}</Btn>
                    {a.state !== "resolved" && <Link href={`/admin/jobs?new=${a.unitId}&alertId=${a.id}`} className="inline-flex items-center rounded-control border border-line bg-surface px-2.5 py-1 text-xs font-semibold hover:bg-surface2">{t("Request maintenance")}</Link>}
                    <Link href={a.customerId ? `/admin/units?customerId=${a.customerId}&unitId=${a.unitId}` : `/admin/units?unitId=${a.unitId}`} className="inline-flex items-center px-1 text-xs font-semibold text-primary">{t("Open unit")} <Icon name="arrowRight" /></Link>
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
      ) : (
        <>
          {live ? (
            <ScopeBar scope={live.scope} q={live.q} text={t(pols.length === 1 ? "1 policy in scope" : "{n} policies in scope", { n: pols.length })} clear={["policyId", "new", "from"]}>
              <Check label={t("Show disabled")} checked={showOff} onChange={setShowOff} />
            </ScopeBar>
          ) : <div className="flex justify-end"><Check label={t("Show disabled")} checked={showOff} onChange={setShowOff} /></div>}
          {scopeUnit && <p className="text-xs text-muted">{t("Showing the policies attached to {unit}: {names}.", { unit: scopeUnit, names: pols.map((x) => x.name).join(", ") })}</p>}
          <PoliciesTab pols={pols} live={live?.pol} sel={polSel} select={selectPolicy} showOff={showOff} copyFor={scopeOwner}
            onCopy={(ruleKey) => (scopeOwner ? startPolicy(scopeOwner.id, ruleKey) : setAsking({ from: ruleKey }))} />
        </>
      )}
      {asking && <NewPolicyModal customers={live ? live.pol.customers : demoCustomers} initial={live?.q.customerId} onClose={() => setAsking(null)} onStart={(id) => startPolicy(id, asking.from)} />}
      <Modal open={resolve} onClose={() => setResolve(false)} title={t("Resolve alert")} footer={<><Btn onClick={() => setResolve(false)}>{t("Cancel")}</Btn><Btn variant="primary" disabled={pending || evidence.loading} onClick={doResolve}>{t("Resolve alert")}</Btn></>}>
        {a && (
          <>
            <p className="text-xs text-muted">{a.id} · {a.title}</p>
            <EvidencePicker rows={evRows} policyless={a.policyless} picked={sendable} onChange={setPicked} loading={evidence.loading} error={evidence.error ? t("The evidence could not be loaded — close the dialog and try again.") : tried ? evProblem : null} />
          </>
        )}
        <Field label={t("Resolution reason · required")} hint={`${reason.length} / 1000`} error={tried && !reason.trim() ? t("A reason is required") : tried && reason.length > 1000 ? t("At most 1000 characters") : undefined}><Textarea value={reason} onChange={(e) => setReason(e.target.value)} /></Field>
        <p className="text-[11px] text-muted">{t("Resolved alerts stay in history. If the condition returns, a new alert is created.")}</p>
      </Modal>
    </Page>
  );
}
