"use client";

import Link from "next/link";
import { useState, useTransition } from "react";
import { Badge, Banner, Btn, Card, Check, Modal, Page, Select, SeverityBadge, SummaryList, Tabs, cx } from "@ac/web/components/ui";
import { useUrlPatch } from "@ac/web/lib/useUrlPatch";
import type { InboxAlert } from "@ac/web/lib/alerts";
import { policyRefusal } from "@ac/web/lib/customerPolicies";
import { readAlert } from "../actions";
import type { AlertsLive } from "../_lib/load";
import { PoliciesLive } from "./policies-live";

/** The customer alert inbox (FR-C08, DD-C08, Figma Client 06a/06b/06g) and the Alert policies tab (FR-C15, 06e/06f) from
 * the Core API: filters in the URL, read state from the membership's notifications (opening an alert marks them read; the
 * alert itself stays as it is). */
export function AlertsLiveView({ live }: { live: AlertsLive }) {
  const patch = useUrlPatch();
  const [open, setOpen] = useState<InboxAlert | null>(null);
  const [failed, setFailed] = useState<string | null>(null);
  const [result, setResult] = useState<{ tone: "ok" | "warn" | "crit"; text: string } | null>(null);
  // opening the policy editor starts without the list's last message (adjust state during render, not in an effect)
  const editorKey = live.policies.editor ? live.policies.editor.policy?.id ?? "new" : null;
  const [seenEditor, setSeenEditor] = useState(editorKey);
  if (seenEditor !== editorKey) {
    setSeenEditor(editorKey);
    if (editorKey) setResult(null);
  }
  const [, start] = useTransition();
  const updated = new Date(live.now).toLocaleTimeString("en-GB", { timeZone: "Asia/Kuala_Lumpur", hour: "2-digit", minute: "2-digit" });
  const attn = live.rows.filter((a) => a.group === "attn");
  const info = live.rows.filter((a) => a.group === "info");
  const show = (a: InboxAlert) => {
    setOpen(a);
    if (a.unread.length) start(async () => setFailed(await readAlert(a.unread)));
  };
  const Item = ({ a }: { a: InboxAlert }) => (
    <div className="flex w-full items-start gap-3 border-t border-line px-1 py-3 first:border-0">
      <span className="mt-0.5">{a.group === "attn" ? "⚠" : "ⓘ"}</span>
      <div className="min-w-0 flex-1">
        <button type="button" onClick={() => show(a)} className="block w-full text-left hover:opacity-80">
          <span className="flex flex-wrap items-center gap-2">
            <b className={cx(a.unread.length > 0 && "text-primary")}>{a.title}</b><SeverityBadge s={a.severity} /><Badge tone="muted">{a.kind}</Badge>
            <Badge tone={a.status.tone}>{a.status.text}</Badge>{a.unread.length > 0 ? <Badge tone="primary">Unread</Badge> : a.notes > 0 && <Badge tone="ok" icon="✓">Read</Badge>}
          </span>
          <span className="block text-xs text-muted">{a.where}</span><span className="mt-1 block text-xs">{a.evidence}</span>
        </button>
        <div className="mt-2 flex flex-wrap gap-2">
          <Link href={`/customer/units/${a.unitId}`} className="rounded-control bg-primary-soft px-2.5 py-1 text-xs font-semibold text-primary">View unit</Link>
          {a.type === "maintenance" && a.status.text !== "Resolved" && <Link href="/customer/maintenance?tab=filter-care" className="rounded-control bg-primary-soft px-2.5 py-1 text-xs font-semibold text-primary">Book cleaning</Link>}
          {a.type !== "maintenance" && a.group === "attn" && <Link href={`/customer/maintenance?new=${a.unitId}`} className="rounded-control bg-primary-soft px-2.5 py-1 text-xs font-semibold text-primary">Request repair</Link>}
        </div>
      </div>
      <button type="button" aria-label={`Open ${a.title}`} onClick={() => show(a)} className="text-muted">›</button>
    </div>
  );
  return (
    <Page>
      <Tabs value={live.tab} onChange={(t) => { setResult(null); patch({ tab: t === "policies" ? "policies" : null, policyId: null }); }} tabs={[{ id: "alerts", label: "Alerts", count: live.total }, { id: "policies", label: "Alert policies", count: live.policies.count }]} />
      {result && <Banner tone={result.tone}>{result.text}</Banner>}
      {live.tab === "alerts" ? (
        <>
          <div className="flex flex-wrap items-center gap-3 text-xs text-muted">
            <Select aria-label="Severity" className="w-auto text-xs" value={live.severity ?? ""} onChange={(e) => patch({ severity: e.target.value || null })}>
              <option value="">Severity: All</option><option value="critical">Severity: Critical</option><option value="warning">Severity: Warning</option>
            </Select>
            <Select aria-label="Unit" className="w-auto text-xs" value={live.unitId ?? ""} onChange={(e) => patch({ unitId: e.target.value || null })}>
              <option value="">Unit: All units</option>{live.units.map((u) => <option key={u.id} value={u.id}>Unit: {u.name}{u.place ? ` · ${u.place}` : ""}</option>)}
            </Select>
            <Check label="Unread only" checked={live.unreadOnly} onChange={(v) => patch({ unreadOnly: v ? "true" : null })} />
            <span className="ml-auto">{live.alertCount} unresolved warning{live.alertCount === 1 ? "" : "s"} · Updated {updated} · {live.unread} unread</span>
          </div>
          {failed && <Banner tone="warn">{failed}</Banner>}
          <Card title="Needs attention" sub="Unresolved warnings & faults — stays here until resolved, even after you read it" action={<Badge tone="warn">{attn.length}</Badge>}>
            {attn.length ? attn.map((a) => <Item key={a.id} a={a} />) : <p className="text-xs text-muted">{live.unreadOnly || live.severity ? "Nothing in this filter." : "Nothing needs your attention."}</p>}
          </Card>
          <Card title="Reminders & information" sub="Maintenance reminders, inspection records, notices and resolved alerts" action={<Badge tone="muted">{info.length}</Badge>}>
            {info.length ? info.map((a) => <Item key={a.id} a={a} />) : <p className="text-xs text-muted">Nothing here.</p>}
          </Card>
        </>
      ) : (
        <PoliciesLive p={live.policies} onDone={(text) => setResult({ tone: "ok", text })} onFail={(f) => setResult({ tone: f.code === "CONFLICT" || f.code === "FORBIDDEN" ? "warn" : "crit", text: policyRefusal(f) })} />
      )}
      <Modal open={!!open} onClose={() => setOpen(null)} title={open?.title ?? ""} footer={<Btn onClick={() => setOpen(null)}>Back to list</Btn>}>
        {open && (
          <>
            <div className="flex flex-wrap gap-2"><SeverityBadge s={open.severity} /><Badge tone={open.status.tone}>{open.status.text}</Badge>{open.notes > 0 && <Badge tone="ok" icon="✓">Read</Badge>}</div>
            <SummaryList items={[["Where", open.where], ["Evidence", open.evidence], ["Status", open.status.detail]]} />
            <p className="text-xs text-muted">{open.status.text === "Resolved" ? "This alert is resolved. It stays in the list as a record." : "Reading an alert does not resolve it. It stays under “Needs attention” until the cause is resolved."}</p>
          </>
        )}
      </Modal>
    </Page>
  );
}
