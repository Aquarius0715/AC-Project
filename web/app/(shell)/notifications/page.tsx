"use client";

import { useState } from "react";
import { Badge, Card, Page, Tabs, cx } from "@/components/ui";

const initial = [
  { id: 1, t: "Alert opened on Bedroom AC", d: "alert-window-a · unresolved · linked evidence available", w: "09:12 today", read: false },
  { id: 2, t: "Work report accepted — job-c02", d: "Quality-reviewed report now visible", w: "Yesterday, 17:20", read: false },
  { id: 3, t: "Invoice inv-0231 is now unpaid", d: "Read · restriction notice pending", w: "2 days ago", read: false },
  { id: 4, t: "Session extended by 30 minutes", d: "System · read", w: "3 days ago", read: true },
];

export default function Notifications() {
  const [tab, setTab] = useState<"all" | "unread">("all");
  const [items, setItems] = useState(initial);
  const shown = items.filter((i) => tab === "all" || !i.read);
  return (
    <Page className="max-w-3xl">
      <Tabs value={tab} onChange={setTab} tabs={[{ id: "all", label: "All", count: items.length }, { id: "unread", label: "Unread", count: items.filter((i) => !i.read).length }]} />
      <Card pad={false}>
        <ul className="divide-y divide-line">
          {shown.map((n) => (
            <li key={n.id}>
              <button onClick={() => setItems((s) => s.map((x) => (x.id === n.id ? { ...x, read: true } : x)))} className="flex w-full items-start gap-3 px-4 py-3 text-left hover:bg-surface2/60">
                <span className={cx("mt-1.5 h-2 w-2 shrink-0 rounded-full", n.read ? "bg-transparent" : "bg-primary")} />
                <span className="min-w-0 flex-1"><span className={cx("block text-[13px]", !n.read && "font-bold")}>{n.t}</span><span className="block text-xs text-muted">{n.d}</span></span>
                <span className="shrink-0 text-xs text-muted">{n.w}</span>
              </button>
            </li>
          ))}
          {shown.length === 0 && <li className="p-6 text-center text-muted">No unread notifications</li>}
        </ul>
      </Card>
      <p className="text-xs text-muted">Reading a notification is separate from resolving the linked alert (FR-X07, AT-X07-N). <Badge tone="muted">preview only</Badge></p>
    </Page>
  );
}
