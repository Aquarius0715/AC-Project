"use client";

import Link from "next/link";
import { useState } from "react";
import { Badge, Card, Page, Tabs, cx } from "@/components/ui";
import { useStoredRole } from "@/components/AppShell";
import { jobActions, useNotes } from "@/lib/jobs";

const initial = [
  { id: "s1", t: "Alert opened on Bedroom AC", d: "alert-window-a · unresolved · linked evidence available", w: "09:12 today", read: false, href: "" },
  { id: "s2", t: "Work report accepted — job-c02", d: "Quality-reviewed report now visible", w: "Yesterday, 17:20", read: false, href: "" },
  { id: "s3", t: "Invoice inv-0231 is now unpaid", d: "Read · restriction notice pending", w: "2 days ago", read: false, href: "" },
  { id: "s4", t: "Session extended by 30 minutes", d: "System · read", w: "3 days ago", read: true, href: "" },
];

export default function Notifications() {
  const role = useStoredRole();
  const live = useNotes(role);
  const [tab, setTab] = useState<"all" | "unread">("all");
  const [items, setItems] = useState(initial);
  const all = [...live.map((n) => ({ id: n.id, t: n.title, d: n.detail, w: n.at, read: n.read, href: n.href, live: true })), ...items.map((i) => ({ ...i, live: false }))];
  const shown = all.filter((i) => tab === "all" || !i.read);
  const open = (id: string, isLive: boolean) => (isLive ? jobActions.markRead(id) : setItems((s) => s.map((x) => (x.id === id ? { ...x, read: true } : x))));
  return (
    <Page className="max-w-3xl">
      <Tabs value={tab} onChange={setTab} tabs={[{ id: "all", label: "All", count: all.length }, { id: "unread", label: "Unread", count: all.filter((i) => !i.read).length }]} />
      <Card pad={false}>
        <ul className="divide-y divide-line">
          {shown.map((n) => {
            const body = <><span className={cx("mt-1.5 h-2 w-2 shrink-0 rounded-full", n.read ? "bg-transparent" : "bg-primary")} /><span className="min-w-0 flex-1"><span className={cx("block text-[13px]", !n.read && "font-bold")}>{n.t}</span><span className="block text-xs text-muted">{n.d}</span></span><span className="shrink-0 text-xs text-muted">{n.w}</span></>;
            const cls = "flex w-full items-start gap-3 px-4 py-3 text-left hover:bg-surface2/60";
            return <li key={n.id}>{n.href ? <Link href={n.href} onClick={() => open(n.id, n.live)} className={cls}>{body}</Link> : <button onClick={() => open(n.id, n.live)} className={cls}>{body}</button>}</li>;
          })}
          {shown.length === 0 && <li className="p-6 text-center text-muted">No unread notifications</li>}
        </ul>
      </Card>
      <p className="text-xs text-muted">Reading a notification is separate from resolving the linked alert (FR-X07). Maintenance scheduling events (new request, time proposed, accepted/declined, new offer, new assignment) appear here for the role that has to act (IR113). <Badge tone="muted">preview only</Badge></p>
    </Page>
  );
}
