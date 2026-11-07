"use client";

import Link from "next/link";
import { useState, useTransition } from "react";
import { Badge, Btn, Card, Page, Tabs, useToast, cx } from "@ac/web/components/ui";
import { useStoredRole } from "@ac/web/components/AppShell";
import { useT } from "@ac/web/components/I18n";
import { jobActions, useDemoInbox } from "@ac/web/lib/jobs";
import type { InboxRow } from "@ac/web/lib/notifications";
import { actionMessage, type ActionFailure } from "@ac/web/lib/actionMessage";
import { byCount, type T } from "@ac/web/lib/i18n";
import { markAllRead, markRead } from "../actions";
import { Button } from "@ac/web/components/ui/button";

// why a notification was not marked read, in the display language (the write limit says how long to wait, IR380)
const readFailure = (f: ActionFailure, t: T) => {
  if (f.messageKey === "error.versionConflict") return t("This notification changed — the latest state is shown.");
  return f.messageKey === "error.notFound" ? t("This notification is no longer available.") : t("Not marked as read: {reason}", { reason: actionMessage(f, t) });
};

/** The inbox. `rows` come from the Server Component in API mode; without them the Phase 1A demo rows are shown. */
export function NotificationsView({ rows }: Readonly<{ rows?: InboxRow[] }>) {
  const t = useT();
  const role = useStoredRole();
  const demo = useDemoInbox(role);
  const [tab, setTab] = useState<"all" | "unread">("all");
  const [pending, start] = useTransition();
  const [failed, setFailed] = useState<string | null>(null);
  const toast = useToast();
  const isApi = rows !== undefined;
  const all = isApi
    ? rows.map((n) => ({ ...n, live: true }))
    : [...demo.own.map((n) => ({ id: n.id, version: 0, t: n.title, d: n.detail, w: n.at, read: n.read, href: n.href, live: true })), ...demo.fixed.map((i) => ({ ...i, version: 0, live: false }))];
  const shown = all.filter((i) => tab === "all" || !i.read);
  const open = (n: { id: string; version: number; read: boolean; live: boolean }) => {
    if (isApi) {
      if (n.read) return;
      start(async () => { const f = await markRead(n.id, n.version); setFailed(f && readFailure(f, t)); }); return; // Server Action; the page re-renders with fresh rows
    }
    if (n.live) jobActions.markRead(n.id); else jobActions.markFixedRead([n.id]);
  };
  const unread = all.filter((i) => !i.read);
  // “Mark all as read” (Figma Client 10c): every unread notification of the list in one write (IR379)
  const readAll = () => {
    if (!isApi) {
      for (const n of unread) if (n.live) jobActions.markRead(n.id);
      jobActions.markFixedRead(unread.filter((n) => !n.live).map((n) => n.id));
      toast(t(unread.length === 1 ? "1 notification marked as read" : "{n} notifications marked as read", { n: unread.length })); return;
    }
    start(async () => {
      const r = await markAllRead(unread.map((n) => n.id));
      setFailed(null);
      if (r.failure) { toast(readFailure(r.failure, t), "crit"); return; }
      toast(r.skipped ? t("{n} marked as read · {skipped} skipped — read meanwhile or no longer available", { n: r.marked, skipped: r.skipped }) : byCount(r.marked, { one: t("1 notification marked as read"), many: t("{n} notifications marked as read", { n: r.marked }) }));
    });
  };
  return (
    <Page className="max-w-3xl">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <Tabs value={tab} onChange={setTab} tabs={[{ id: "all", label: t("All"), count: all.length }, { id: "unread", label: t("Unread"), count: unread.length }]} />
        {unread.length > 0 && <Btn size="sm" disabled={pending} onClick={readAll}>{t("Mark all as read")}</Btn>}
      </div>
      {failed && <p role="alert" className="text-xs text-crit">{failed}</p>}
      <Card pad={false} className={pending ? "opacity-70" : undefined}>
        <ul className="divide-y divide-line">
          {shown.map((n) => {
            const body = <><span className={cx("mt-1.5 h-2 w-2 shrink-0 rounded-full", n.read ? "bg-transparent" : "bg-primary")} /><span className="min-w-0 flex-1"><span className={cx("block text-[13px]", !n.read && "font-bold")}>{n.t}</span><span className="block text-xs text-muted-foreground">{n.d}</span></span><span className="shrink-0 text-xs text-muted-foreground">{n.w}</span></>;
            const cls = "flex w-full items-start gap-3 px-4 py-3 text-left hover:bg-surface2/60";
            return <li key={n.id}>{n.href ? <Link href={n.href} onClick={() => { open(n); }} className={cls}>{body}</Link> : <Button variant="ghost" onClick={() => { open(n); }} className={`h-auto justify-start whitespace-normal ${cls}`}>{body}</Button>}</li>;
          })}
          {shown.length === 0 && <li className="p-6 text-center text-muted-foreground">{t(tab === "unread" ? "No unread notifications" : "No notifications")}</li>}
        </ul>
      </Card>
      <p className="text-xs text-muted-foreground">{t("Reading a notification is separate from resolving the linked alert. Maintenance scheduling events (new request, time proposed, accepted/declined, new offer, new assignment) appear here for the role that has to act.")} {!isApi && <Badge tone="muted">{t("preview only")}</Badge>}</p>
    </Page>
  );
}
