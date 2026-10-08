"use client";

import { useState, useTransition } from "react";
import { usePathname, useRouter } from "next/navigation";
import { Badge, Banner, Btn, Card, Check, Choice, EmptyState, Field, Input, ListRow, Modal, Page, Select, SeverityBadge, SummaryList, Tabs, Textarea, Timeline, LineChart, useToast, cx } from "@ac/web/components/ui";
import { useUrlTab } from "@ac/web/lib/useUrlTab";
import { metricLabel, opSymbol, recoveryError, type AdminAlert, type AdminPolicy, type Channel, type Severity } from "@ac/web/lib/adminAlerts";
import { acknowledgeAlert, resolveAlert, savePolicy, type ActionResult } from "../actions";

const demoTimeline = (time: string, st: AdminAlert["st"]): AdminAlert["timeline"] => [
  { time, title: "Alert detected", detail: "Load increase with stable outdoor temperature", tone: "warn" },
  { time: "08:50", title: "Notified customer-a · In-app", detail: "deliveryState: simulated" },
  { time: "08:52", title: "customer-a read the notification", detail: "Reading a notification does not acknowledge or resolve the alert" },
  { time: "—", title: st === "Resolved" ? "Resolved" : st === "Acknowledged" ? "Acknowledged — waiting for resolution" : "Waiting for acknowledgement", detail: "", tone: st === "Resolved" ? "ok" : undefined },
];
const demoAlert = (id: string, title: string, time: string, sev: Severity, meta: string): AdminAlert => ({
  id, version: 1, title, time, sev, meta, st: "Open", unit: "Bedroom AC · unit-online-rto", context: "customer-a · Home A › 1F › Bedroom · Running · Online",
  evidence: "inferred — “Demo: cooling load rose while the outdoor temperature was stable.”", cause: "window_open (suspected)", observed: `${time} MYT`,
  evidenceRecords: "none attached", timeline: demoTimeline(time, "Open"),
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
const message = (r: Extract<ActionResult, { ok: false }>) =>
  r.code === "CONFLICT" ? "Someone changed this in the meantime — the screen now shows the latest version" : Object.values(r.fieldErrors)[0] ?? `${r.code}: ${r.messageKey}`;

type Live = { tab: "alerts" | "policies"; alerts: AdminAlert[]; policies: AdminPolicy[] };

/** HQ alerts and alert policies. `live` comes from the Server Component in API mode (writes are Server Actions that
 * re-render the route); the demo keeps its fixture rows in local state. */
export function AdminAlertsView({ live }: { live?: Live }) {
  const toast = useToast();
  const router = useRouter();
  const pathname = usePathname();
  const [pending, startTransition] = useTransition();
  const [urlTab, setUrlTab] = useUrlTab<"alerts" | "policies">({ alerts: "alerts", policies: "policies" }, "alerts");
  const tab = live ? live.tab : urlTab;
  const setTab = (t: "alerts" | "policies") => (live ? router.replace(t === "policies" ? `${pathname}?tab=policies` : pathname, { scroll: false }) : setUrlTab(t));

  // alerts: the demo changes its local copy; API mode shows the server rows
  const [demoList, setDemoList] = useState(seedAlerts);
  const al = live ? live.alerts : demoList;
  const [alertId, setAlertId] = useState<string | null>(null);
  const a = al.find((x) => x.id === alertId) ?? al.find((x) => x.st !== "Resolved") ?? al[0] ?? null;
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
  const recErr = draft ? recoveryError(draft.operator, draft.threshold, draft.recovery) : undefined;
  const durErr = draft && (draft.duration < 1 || draft.duration > 86400) ? "Duration must be 1–86400 s" : undefined;
  const nameErr = draft && (!draft.name.trim() || draft.name.length > 120) ? "1–120 characters" : undefined;
  const coolErr = draft && (draft.cooldown < 1 || draft.cooldown > 1440) ? "1–1440" : undefined;
  const escErr = draft && (draft.escalate < 1 || draft.escalate > 1440) ? "1–1440" : undefined;

  const act = (fn: () => Promise<ActionResult>, ok: string, after?: () => void) =>
    startTransition(async () => {
      const r = await fn();
      if (r.ok) {
        toast(ok);
        after?.();
      } else toast(message(r), "crit");
    });
  const acknowledge = () => {
    if (!a) return;
    if (live) return act(() => acknowledgeAlert(a.id, a.version), "Acknowledged");
    setDemoList((l) => l.map((x) => (x.id === a.id ? { ...x, st: "Acknowledged", timeline: demoTimeline(x.time, "Acknowledged") } : x)));
    toast("Acknowledged");
  };
  const doResolve = () => {
    setTried(true);
    if (!a || !reason.trim() || reason.length > 1000) return;
    if (live) return act(() => resolveAlert(a.id, a.version, reason.trim()), "Alert resolved", () => { setResolve(false); setReason(""); setTried(false); });
    setDemoList((l) => l.map((x) => (x.id === a.id ? { ...x, st: "Resolved", timeline: demoTimeline(x.time, "Resolved") } : x)));
    setResolve(false);
    toast("Alert resolved");
  };
  const save = () => {
    setTried(true);
    if (!draft || recErr || durErr || nameErr || coolErr || escErr) return toast("Fix the validation errors first", "crit");
    if (live) return act(() => savePolicy(draft), "Policy saved", () => setTried(false));
    toast("Policy saved");
  };
  const toggleChannel = (c: Channel, on: boolean) => draft && set("channels", on ? [...new Set([...draft.channels, c])] : draft.channels.filter((x) => x !== c));
  const groups = [...new Set(pols.map((x) => x.group))];
  const counts = { Open: al.filter((x) => x.st === "Open").length, Acknowledged: al.filter((x) => x.st === "Acknowledged").length, Resolved: al.filter((x) => x.st === "Resolved").length + (live ? 0 : 2) };
  const openish = al.filter((x) => x.st !== "Resolved");

  return (
    <Page>
      <Tabs value={tab} onChange={setTab} tabs={[{ id: "alerts", label: "Alerts", count: openish.length }, { id: "policies", label: "Policies", count: pols.length }]} />
      {tab === "alerts" ? (
        <>
          <div className="grid-fluid" style={{ ["--min" as string]: "140px" }}>{(Object.entries(counts) as [string, number][]).map(([l, n]) => <div key={l} className="rounded-2xl border border-line bg-surface p-4"><div className="text-xs text-muted">{l}</div><div className="text-2xl font-bold">{n}</div></div>)}</div>
          {!a ? <Card title="Open alerts · all customers"><EmptyState title="No alerts">No unit has raised an alert.</EmptyState></Card> : (
            <div className="split-rev">
              <Card title="Open alerts · all customers" sub="severity ↓" className="self-start"><div className="flex flex-col gap-2">{openish.length === 0 ? <EmptyState title="No open alerts">Every alert is resolved.</EmptyState> : openish.map((x) => <ListRow key={x.id} selected={a.id === x.id} onClick={() => setAlertId(x.id)}><div className="min-w-0 flex-1"><div className="flex items-center justify-between gap-2"><b className="text-[13px]">{x.title}</b><span className="text-[11px] text-muted">{x.time}</span></div><div className="flex items-center gap-2 text-[11px] text-muted"><SeverityBadge s={x.sev} />{x.st !== "Open" && <Badge tone="primary">{x.st}</Badge>}</div><div className="truncate text-[11px] text-muted">{x.meta} · {x.id}</div></div></ListRow>)}</div></Card>
              <div className="flex min-w-0 flex-col gap-4">
                <Card title={a.title} sub={`${a.id} · detected ${a.observed}`} action={<div className="flex gap-2"><Btn size="sm" disabled={a.st !== "Open" || pending} onClick={acknowledge}>Acknowledge</Btn><Btn size="sm" variant="primary" disabled={a.st === "Resolved" || pending} onClick={() => setResolve(true)}>Resolve…</Btn></div>}>
                  <SummaryList items={[["Unit", a.unit], ["Context", a.context], ["Evidence", a.evidence], ["Cause", a.cause], ["Observed at", a.observed], ["Evidence records", a.evidenceRecords]]} />
                  <p className="mt-2 text-xs text-muted">This is an inference, not a confirmed cause. Ask the customer to check windows, or request an inspection to record evidence.</p>
                </Card>
                <Card title="Timeline"><Timeline items={a.timeline} /><p className="mt-2 text-[11px] text-muted">Resolving requires a reason and evidence (remeasurement or a recorded confirmation). Completing maintenance work does not resolve the alert by itself.</p></Card>
              </div>
            </div>
          )}
        </>
      ) : !p || !draft ? (
        <Card title="Alert policies"><EmptyState title="No alert policies">Customers have no alert policies yet.</EmptyState></Card>
      ) : (
        <div className="split-rev">
          <Card title="Alert policies" sub="by customer" action={!live && <Btn size="sm" variant="primary" onClick={() => toast("New policy draft created")}>+ New</Btn>} className="self-start">
            {groups.map((g) => <div key={g} className="mb-3"><div className="mb-1 text-[10px] font-bold tracking-wide text-muted">{g}</div><div className="flex flex-col gap-2">{pols.filter((x) => x.group === g).map((x) => <ListRow key={x.id} selected={p.id === x.id} onClick={() => { setPolId(x.id); setTried(false); }}><div className="min-w-0 flex-1"><b className="text-[13px]">{x.name}</b><div className="text-[11px] text-muted">{x.sub}</div><div className="text-[11px] text-muted">{x.units}{!x.on && " · Off"}</div></div></ListRow>)}</div></div>)}
          </Card>
          <Card title={p.name} sub={`${p.id}${p.customerName ? ` · ${p.customerName}` : ""}`} action={<Badge tone={p.on ? "ok" : "muted"}>{p.on ? "Enabled" : "Disabled"}</Badge>}>
            {p.kind === "default_alert" ? <Banner>Default policy rules are set by HQ for every unit. Per-customer on/off is controlled by the customer.</Banner> : (
              <div className="flex flex-col gap-5">
                <section><h3 className="mb-2 text-[13px] font-bold">Basics</h3><div className="grid-fluid" style={{ ["--min" as string]: "180px" }}><Field label="Name" error={tried ? nameErr : undefined}><Input value={draft.name} onChange={(e) => set("name", e.target.value)} /></Field><Field label="Priority" hint="0–100"><Input type="number" value={draft.priority} onChange={(e) => set("priority", +e.target.value)} /></Field><Field label="Timezone"><Select value={draft.timezone} onChange={(e) => set("timezone", e.target.value)}><option>Asia/Kuala_Lumpur</option></Select></Field></div></section>
                <section><h3 className="mb-1 text-[13px] font-bold">Owner & units</h3><p className="text-xs text-muted">A policy belongs to one customer. Units pick it up on their own page (Customers & units › unit › Alert policies). Attached to {p.units} of {p.customerName} · read-only here.</p></section>
                <section><h3 className="mb-2 text-[13px] font-bold">Condition</h3><p className="mb-2 text-xs text-muted">Missing or stale readings never count toward the condition — they raise a data-quality notice instead.</p><div className="grid-fluid" style={{ ["--min" as string]: "130px" }}><Field label="Metric"><Select value={draft.metric} onChange={(e) => set("metric", e.target.value)}>{Object.entries(metricLabel).map(([k, l]) => <option key={k} value={k}>{l}</option>)}</Select></Field><Field label={`Alert when ${opSymbol[draft.operator]}`}><Input type="number" value={draft.threshold} onChange={(e) => set("threshold", +e.target.value)} /></Field><Field label="For (s)" error={durErr}><Input type="number" value={draft.duration} onChange={(e) => set("duration", +e.target.value)} /></Field><Field label="Recover" error={recErr}><Input type="number" value={draft.recovery} onChange={(e) => set("recovery", +e.target.value)} /></Field></div>
                  <div className="mt-2"><LineChart points={[20, 24, 28, 31, 32, 31, 29, 27, 25]} min={18} max={34} threshold={draft.threshold} height={90} /></div>
                  <p className={cx("text-xs", recErr ? "text-crit" : "text-ok")}>{recErr ? `✕ ${recErr}` : `✓ Recovery ${draft.recovery} is on the safe side of the ${draft.threshold} threshold (${opSymbol[draft.operator]})`}</p></section>
                <section><h3 className="mb-1 text-[13px] font-bold">Severity</h3><Choice value={draft.severity} onChange={(v) => set("severity", v)} options={[{ id: "normal", label: "Info" }, { id: "warning", label: "Warning" }, { id: "critical", label: "Critical" }]} /><p className="mt-1 text-[11px] text-muted">A severity change on an open alert counts as a new notification reason.</p></section>
                <section><h3 className="mb-2 text-[13px] font-bold">Notify</h3><p className="mb-2 text-xs text-muted">Recipients come from notifications.recipients — only people allowed to see these units.</p>
                  {draft.recipients.length === 0 ? <p className="text-xs text-muted">No recipients.</p> : draft.recipients.map((r) => <div key={r.id} className="flex flex-wrap items-center justify-between gap-2 border-t border-line py-2 text-[13px]"><span><b>{r.label}</b><span className="block text-xs text-muted">{r.role}</span></span></div>)}
                  <div className="flex flex-wrap gap-3 border-t border-line pt-2"><span className="text-xs text-muted">Channels for every recipient:</span><Check label="In-app" checked={draft.channels.includes("inApp")} onChange={(v) => toggleChannel("inApp", v)} /><Check label="Email" checked={draft.channels.includes("email")} onChange={(v) => toggleChannel("email", v)} /><Check label="WhatsApp" checked={draft.channels.includes("whatsapp")} onChange={(v) => toggleChannel("whatsapp", v)} /></div>
                  <p className="text-[11px] text-muted">WhatsApp reaches only recipients who allowed that channel.</p></section>
                <section className="grid-fluid" style={{ ["--min" as string]: "180px" }}><Field label="Cooldown (min)" hint="Same severity is not re-sent within this window (1–1440)" error={coolErr}><Input type="number" value={draft.cooldown} onChange={(e) => set("cooldown", +e.target.value)} /></Field><Field label="Escalate after (min)" hint="Escalate if still unacknowledged (1–1440)" error={escErr}><Input type="number" value={draft.escalate} onChange={(e) => set("escalate", +e.target.value)} /></Field></section>
                <div className="flex justify-end"><Btn variant="primary" disabled={pending} onClick={save}>Save policy</Btn></div>
              </div>
            )}
          </Card>
        </div>
      )}
      <Modal open={resolve} onClose={() => setResolve(false)} title="Resolve alert" footer={<><Btn onClick={() => setResolve(false)}>Cancel</Btn><Btn variant="primary" disabled={pending} onClick={doResolve}>Resolve</Btn></>}><p className="text-xs text-muted">Resolving requires a reason and evidence. Completing maintenance work does not resolve the alert by itself.</p><Field label="Reason (required)" error={tried && !reason.trim() ? "A reason is required" : tried && reason.length > 1000 ? "At most 1000 characters" : undefined}><Textarea value={reason} onChange={(e) => setReason(e.target.value)} /></Field></Modal>
    </Page>
  );
}
