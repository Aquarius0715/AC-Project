"use client";

import { useEffect, useState } from "react";
import { CURRENT_CLIENT } from "@/lib/clientUsers";
import { Badge, Banner, Btn, Card, Check, Choice, EmptyState, ErrorState, Field, Input, Modal, OnOffBadge, Page, Select, SeverityBadge, SummaryList, Tabs, Toggle, cx, useToast } from "@/components/ui";
import { useUrlTab } from "@/lib/useUrlTab";
import { useOp } from "@/lib/useOp";

type Alert = { id: string; title: string; sev: "warning" | "normal"; kind: string; icon: string; where: string; ev: string; group: "attn" | "info"; read: boolean };
const seed: Alert[] = [
  { id: "alert-window-a", title: "Possible open window", sev: "warning", kind: "⌂ Load cause (possible)", icon: "⚠", where: "Bedroom AC · Home A › 1F › Bedroom · detected 09:12 today", ev: "Evidence (inferred): room 3.2°C above setting for 20 min while compressor at full load. Possible cause — not confirmed.", group: "attn", read: false },
  { id: "alert-short", title: "Compressor short-cycling", sev: "warning", kind: "✕ Fault", icon: "⚠", where: "Kitchen AC · Home A › 1F › Kitchen · yesterday 22:40", ev: "Evidence (detected): 4 compressor cycles/hour vs ≤ 1 expected, over 3 hours.", group: "attn", read: true },
  { id: "alert-filter", title: "Filter cleaning reminder", sev: "normal", kind: "◷ Maintenance reminder", icon: "○", where: "Study AC · Home A › 2F · 5 days ago", ev: "Cleaning due (250 h of run time since the last cleaning). Not a fault.", group: "info", read: true },
  { id: "alert-insulation-a", title: "Poor insulation suspected", sev: "warning", kind: "✎ Inspection record", icon: "⚠", where: "Bedroom AC · logged Sep 10 by technician", ev: "Possible cause of higher load, from inspection notes. Not confirmed.", group: "info", read: true },
  { id: "alert-fw", title: "Firmware update available", sev: "normal", kind: "↻ System information", icon: "ⓘ", where: "Living room AC · v2.4.1 ready · 3 days ago", ev: "Installed by HQ/technician. No action needed.", group: "info", read: true },
];

/** Alert of service-contracts.ts (fields shown on this screen). */
type ApiAlert = { id: string; unitId: string; type: string; severity: "critical" | "warning" | "normal"; status: string; causeCode: string; evidenceKind: string; evidenceText: string; detectedAt: string };

const causeTitle: Record<string, string> = { window_open: "Possible open window", insulation_loss: "Poor insulation suspected" };
const typeTitle: Record<string, string> = { maintenance: "Maintenance reminder", quality: "Air quality alert", tamper: "Device tamper", reconciliation_required: "Restriction check needed", sensor: "Sensor alert" };
const evidenceLabel: Record<string, string> = { inferred: "Evidence (inferred)", inspection: "Inspection record", demo_observation: "Evidence (demo observation)" };

// DATA_SOURCE=api: one row per alert; unresolved critical/warning alerts need attention (IR51), the rest are information
function alertRow(a: ApiAlert): Alert {
  const attn = a.status !== "resolved" && a.severity !== "normal";
  const at = new Date(a.detectedAt).toLocaleString("en-MY", { dateStyle: "medium", timeStyle: "short", timeZone: "Asia/Kuala_Lumpur" });
  return {
    id: a.id,
    title: causeTitle[a.causeCode] ?? typeTitle[a.type] ?? "Alert",
    sev: a.severity === "normal" ? "normal" : "warning",
    kind: a.evidenceKind === "inspection" ? "✎ Inspection record" : a.type === "maintenance" ? "◷ Maintenance reminder" : "✕ Fault",
    icon: attn ? "⚠" : "ⓘ",
    where: `detected ${at}${a.status === "acknowledged" ? " · acknowledged" : ""}`,
    ev: `${evidenceLabel[a.evidenceKind] ?? "Evidence"}: ${a.evidenceText}`,
    group: attn ? "attn" : "info",
    read: a.status !== "open",
  };
}

const defaultRules = [["Ventilation", "CO₂ ≥ 1000 ppm for 10 min", "Warning", true], ["Dust / filter", "PM2.5 ≥ 35 µg/m³ for 30 min", "Warning", true], ["Refrigerant low pressure", "Refrigerant pressure ≤ 350 kPa for 120 s", "Critical", true], ["Compressor short-cycling", "≥ 4 cycles / h for 3 h", "Warning", true], ["Clogged filter", "Airflow drop ≥ 30 % vs. baseline for 1 h", "Info", true], ["AC offline", "No heartbeat for 15 min", "Warning", false]] as const;
const seedPolicies = [
  { id: "p1", name: "Bedroom too hot", when: "Room temperature ≥ 30 °C for 60 s · recover below 28 °C", then: "Warning · notify in-app + email", att: "Attached to 2 ACs: Bedroom AC, Bedroom AC #2", on: true },
  { id: "p2", name: "Stuffy office", when: "CO₂ ≥ 1200 ppm for 15 min · only 08:00–19:00 on weekdays", then: "Warning · notify in-app", att: "Attached to 2 ACs: Workstations AC 1, Workstations AC 2", on: true },
  { id: "p3", name: "Night humidity", when: "Humidity ≥ 70 % for 30 min · only 22:00–06:00", then: "Info · notify in-app", att: "Not attached to any AC yet — attach it from an AC’s page", on: false },
];

export default function Alerts() {
  const toast = useToast();
  const [tab, setTab] = useUrlTab<"alerts" | "policies">({ alerts: "overview", policies: "policies" }, "alerts");
  const remote = useOp<{ items: ApiAlert[] }, Alert[]>("alerts.list", { limit: 100 }, seed, (p) => p.items.map(alertRow));
  const [list, setList] = useState(seed);
  useEffect(() => setList(remote.data), [remote.data]);
  const [unread, setUnread] = useState(false);
  const [open, setOpen] = useState<Alert | null>(null);
  const [failed, setFailed] = useState(false);
  const [rules, setRules] = useState(defaultRules.map((r) => r[3] as boolean));
  const [policies, setPolicies] = useState(seedPolicies);
  const [edit, setEdit] = useState<string | null>(null);
  const [f, setF] = useState({ name: "", metric: "co2", op: "≥", val: "1200", dur: "15", rec: "1000", sev: "warning" });
  const show = (a: Alert) => (unread ? !a.read : true);
  const attn = list.filter((a) => a.group === "attn" && show(a));
  const info = list.filter((a) => a.group === "info" && show(a));
  const openAlert = (a: Alert) => { setOpen(a); setList((l) => l.map((x) => (x.id === a.id ? { ...x, read: true } : x))); };
  const recOk = +f.rec < +f.val;
  // policies.setDefaultRule is owner-only in the customer app (FR-C15, IR115); members see the rules read-only.
  const isOwner = CURRENT_CLIENT.role === "owner";

  const Item = ({ a }: { a: Alert }) => (
    <button onClick={() => openAlert(a)} className="flex w-full items-start gap-3 border-t border-line px-1 py-3 text-left first:border-0 hover:bg-surface2/50">
      <span className="mt-0.5">{a.icon}</span>
      <span className="min-w-0 flex-1"><span className="flex flex-wrap items-center gap-2"><b className={cx(!a.read && "text-primary")}>{a.title}</b><Badge tone="muted">{a.kind}</Badge>{a.read && <Badge tone="ok" icon="✓">Read</Badge>}</span><span className="block text-xs text-muted">{a.where}</span><span className="mt-1 block text-xs">{a.ev}</span></span>
      <span className="text-muted">›</span>
    </button>
  );

  return (
    <Page>
      <Tabs value={tab} onChange={setTab} tabs={[{ id: "alerts", label: "Alerts", count: list.length }, { id: "policies", label: "Alert policies", count: policies.length + 1 }]} />
      {tab === "alerts" ? (
        <>
          <div className="flex flex-wrap items-center gap-3 text-xs text-muted"><Check label="Unread only" checked={unread} onChange={setUnread} /><span>Updated 09:13 · {list.filter((a) => !a.read).length} unread</span><button className="underline" onClick={() => setFailed((x) => !x)}>{failed ? "show list" : "show load error"}</button></div>
          {failed ? <ErrorState title="Couldn’t load alerts" onRetry={() => setFailed(false)}>This is not “0 alerts”. Previous values are not shown as current. Retry or open units directly.</ErrorState> : (
            <>
              <Card title="Needs attention" sub="Unresolved warnings & faults — stays here until resolved, even after you read it" action={<Badge tone="warn">{attn.length}</Badge>}>{attn.length ? attn.map((a) => <Item key={a.id} a={a} />) : <p className="text-xs text-muted">Nothing here.</p>}</Card>
              <Card title="Reminders & information" sub="Maintenance reminders, inspection records and system notices" action={<Badge tone="muted">{info.length}</Badge>}>{info.length ? info.map((a) => <Item key={a.id} a={a} />) : <p className="text-xs text-muted">Nothing here.</p>}</Card>
            </>
          )}
        </>
      ) : (
        <>
          <div className="flex flex-wrap items-start justify-between gap-3"><div><h2 className="text-[15px] font-bold">Alert policies</h2><p className="max-w-xl text-xs text-muted">A policy is a set of limits. It only runs on the ACs you attach it to (open an AC › Alert policies). Every AC also has the default policy.</p></div><Btn variant="primary" onClick={() => { setEdit("new"); setF({ name: "", metric: "co2", op: "≥", val: "1200", dur: "15", rec: "1000", sev: "warning" }); }}>+ Create policy</Btn></div>
          <Card title="Default policy" sub="Limits set by HQ · on / off is set by the account owner">
            <p className="mb-3 text-xs text-muted">Covers ventilation and the usual causes of breakdowns. It is attached to every AC and cannot be detached or edited, but the account owner can switch each rule on or off for your account (all your ACs). Ask HQ if a limit should change.</p>
            {!isOwner && <p className="mb-3 text-xs text-warn">Only the account owner can change this.</p>}
            <div className="scroll-x"><table className="w-full text-left text-[13px]"><thead className="text-[11px] uppercase text-muted"><tr><th className="py-1.5 pr-3">Rule</th><th className="pr-3">Condition</th><th className="pr-3">Severity</th><th>On</th></tr></thead><tbody>{defaultRules.map((r, i) => <tr key={r[0]} className="border-t border-line"><td className="py-2 pr-3 font-semibold">{r[0]}</td><td className="pr-3">{r[1]}</td><td className="pr-3"><SeverityBadge s={(r[2] === "Info" ? "normal" : r[2].toLowerCase()) as "warning"} /></td><td><Toggle on={rules[i]} disabled={!isOwner} onChange={(v) => setRules((s) => s.map((x, j) => (j === i ? v : x)))} label={r[0]} /></td></tr>)}</tbody></table></div>
            {!rules[5] && <p className="mt-2 text-xs text-warn">Turned off: “AC offline” — no alerts for heartbeat loss on any of your ACs. HQ still sees device status.</p>}
          </Card>
          <div className="text-[13px] font-bold">Your policies <span className="text-xs font-normal text-muted">{policies.length} · added on top of the default policy · a policy belongs to your account and can be attached to many ACs</span></div>
          <div className="grid-fluid" style={{ ["--min" as string]: "320px" }}>
            {policies.map((p) => (
              <Card key={p.id} title={p.name} action={<Toggle on={p.on} onChange={(v) => setPolicies((s) => s.map((x) => (x.id === p.id ? { ...x, on: v } : x)))} label={p.name} />}>
                <p className="text-[13px]"><span className="text-muted">When</span> {p.when}</p><p className="text-[13px]"><span className="text-muted">Then</span> {p.then}</p><p className="my-1 text-xs text-muted">{p.att}</p>
                <div className="mt-2 flex gap-2"><Btn size="sm" onClick={() => { setEdit(p.id); setF({ ...f, name: p.name }); }}>Edit</Btn><Btn size="sm" variant="danger" onClick={() => { setPolicies((s) => s.filter((x) => x.id !== p.id)); toast("Policy deleted (detached from its ACs)", "warn"); }}>Delete</Btn></div>
              </Card>
            ))}
          </div>
          <p className="text-xs text-muted">A unit can carry several policies; each rule is checked separately. Missing or stale readings never raise an alert — they show “not measured”. Air-quality limits (CO₂ ppm, PM2.5 µg/m³) are regular alert policies.</p>
        </>
      )}
      <Modal open={!!open} onClose={() => setOpen(null)} title={open?.title ?? ""} footer={<Btn onClick={() => setOpen(null)}>Back to list</Btn>}>
        {open && <><div className="flex gap-2"><SeverityBadge s={open.sev} /><Badge tone="ok" icon="✓">Read</Badge><Badge tone="warn">Unresolved</Badge></div><SummaryList items={[["Where", open.where], ["Evidence", open.ev], ["Status", "Opened → auto Read, still Unresolved"]]} /><p className="text-xs text-muted">Reading an alert does not resolve it. It stays under “Needs attention” until the cause is resolved.</p></>}
      </Modal>
      <Modal open={!!edit} onClose={() => setEdit(null)} wide title={edit === "new" ? "Create alert policy" : "Edit alert policy"} footer={<><Btn onClick={() => setEdit(null)}>Cancel</Btn><Btn variant="primary" disabled={!recOk || !f.name.trim()} onClick={() => { if (edit === "new") setPolicies((s) => [...s, { id: "n" + Date.now(), name: f.name, when: `${f.metric.toUpperCase()} ${f.op} ${f.val} for ${f.dur} min · recover below ${f.rec}`, then: `${f.sev} · notify in-app`, att: "Not attached to any AC yet — attach it from an AC’s page", on: true }]); toast("Policy saved"); setEdit(null); }}>Save policy</Btn></>}>
        <Field label="Name" error={!f.name.trim() ? "Name is required" : undefined}><Input value={f.name} onChange={(e) => setF({ ...f, name: e.target.value })} /></Field>
        <div className="grid-fluid" style={{ ["--min" as string]: "140px" }}>
          <Field label="What to watch"><Select value={f.metric} onChange={(e) => setF({ ...f, metric: e.target.value })}><option value="temp">Temperature (°C)</option><option value="hum">Humidity (%)</option><option value="co2">CO₂ (ppm)</option><option value="pm25">PM2.5 (µg/m³)</option><option value="pow">Power (W)</option></Select></Field>
          <Field label="Alert when"><Select value={f.op} onChange={(e) => setF({ ...f, op: e.target.value })}><option>≥</option><option>≤</option></Select></Field>
          <Field label="Limit"><Input type="number" value={f.val} onChange={(e) => setF({ ...f, val: e.target.value })} /></Field>
          <Field label="For (min)"><Input type="number" value={f.dur} onChange={(e) => setF({ ...f, dur: e.target.value })} /></Field>
          <Field label="Recover below"><Input type="number" value={f.rec} onChange={(e) => setF({ ...f, rec: e.target.value })} /></Field>
        </div>
        <p className={cx("text-xs", recOk ? "text-ok" : "text-crit")}>{recOk ? `✓ Recovery ${f.rec} is below the ${f.val} limit` : `✕ Recovery must be below the ${f.val} limit`}</p>
        <Field label="Severity"><Choice value={f.sev as "warning"} onChange={(v) => setF({ ...f, sev: v })} options={[{ id: "normal" as "warning", label: "Info" }, { id: "warning", label: "Warning" }, { id: "critical" as "warning", label: "Critical" }]} /></Field>
        <p className="text-[11px] text-muted">WhatsApp is only available after you allow it in Preferences. Missing or stale readings never trigger — they show “not measured”.</p>
      </Modal>
    </Page>
  );
}
