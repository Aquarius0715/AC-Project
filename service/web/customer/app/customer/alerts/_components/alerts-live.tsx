"use client";

import { useState, useTransition } from "react";
import { Badge, Banner, Btn, Card, Check, Modal, Page, Select, SeverityBadge, SummaryList, Tabs, cx } from "@ac/web/components/ui";
import { useUrlPatch } from "@ac/web/lib/useUrlPatch";
import type { InboxAlert } from "@ac/web/lib/alerts";
import { readAlert } from "../actions";
import type { AlertsLive } from "../_lib/load";
import { PoliciesPanel } from "./alerts-view";

/** The customer alert inbox (FR-C08, DD-C08, Figma Client 06a/06b) from the Core API: filters in the URL, read state from
 * the membership's notifications (opening an alert marks them read; the alert itself stays as it is). */
export function AlertsLiveView({ live }: { live: AlertsLive }) {
  const patch = useUrlPatch();
  const [open, setOpen] = useState<InboxAlert | null>(null);
  const [failed, setFailed] = useState<string | null>(null);
  const [, start] = useTransition();
  const attn = live.rows.filter((a) => a.group === "attn");
  const info = live.rows.filter((a) => a.group === "info");
  const show = (a: InboxAlert) => {
    setOpen(a);
    if (a.unread.length) start(async () => setFailed(await readAlert(a.unread)));
  };
  const Item = ({ a }: { a: InboxAlert }) => (
    <button onClick={() => show(a)} className="flex w-full items-start gap-3 border-t border-line px-1 py-3 text-left first:border-0 hover:bg-surface2/50">
      <span className="mt-0.5">{a.group === "attn" ? "⚠" : "ⓘ"}</span>
      <span className="min-w-0 flex-1">
        <span className="flex flex-wrap items-center gap-2">
          <b className={cx(a.unread.length > 0 && "text-primary")}>{a.title}</b><SeverityBadge s={a.severity} /><Badge tone="muted">{a.kind}</Badge>
          <Badge tone={a.status.tone}>{a.status.text}</Badge>{a.unread.length > 0 ? <Badge tone="primary">Unread</Badge> : a.notes > 0 && <Badge tone="ok" icon="✓">Read</Badge>}
        </span>
        <span className="block text-xs text-muted">{a.where}</span><span className="mt-1 block text-xs">{a.evidence}</span>
      </span>
      <span className="text-muted">›</span>
    </button>
  );
  return (
    <Page>
      <Tabs value={live.tab} onChange={(t) => patch({ tab: t === "policies" ? "policies" : null })} tabs={[{ id: "alerts", label: "Alerts", count: live.total }, { id: "policies", label: "Alert policies" }]} />
      {live.tab === "alerts" ? (
        <>
          <div className="flex flex-wrap items-center gap-3 text-xs text-muted">
            <Select aria-label="Severity" className="w-auto text-xs" value={live.severity ?? ""} onChange={(e) => patch({ severity: e.target.value || null })}>
              <option value="">Severity: All</option><option value="critical">Severity: Critical</option><option value="warning">Severity: Warning</option>
            </Select>
            <Check label="Unread only" checked={live.unreadOnly} onChange={(v) => patch({ unreadOnly: v ? "true" : null })} />
            <span>{live.alertCount} unresolved warning{live.alertCount === 1 ? "" : "s"} · {live.unread} unread</span>
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
        <>
          <Banner tone="warn">Alert policies are not connected to the Core API yet — illustrative data (FR-C15, next round).</Banner>
          <PoliciesPanel />
        </>
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
