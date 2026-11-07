// /notifications (FR-X07): in API mode a Server Component reads the caller's own notifications through the DAL
// (newest first) and hands plain rows to the client view, titled and timed in the user's display language and time zone
// (FR-X01, IR258); the Phase 1A demo keeps the in-browser rows.
import { connection } from "next/server";
import { apiMode, coreDisplay, coreOp, verifySession } from "@ac/web/lib/dal";
import { inboxRow, type ApiNotification } from "@ac/web/lib/notifications";
import { NotificationsView } from "./_components/notifications-view";

export default async function NotificationsPage() {
  await connection(); // DATA_SOURCE is a runtime setting of the image (Next.js environment variables guide)
  if (!apiMode()) return <NotificationsView />;
  const session = await verifySession();
  const sort = { field: "occurredAt", direction: "desc" } as const;
  const list = (filters: { unreadOnly?: true }) => coreOp<{ items: ApiNotification[]; total: number }>("notifications.list", { filters, sort, limit: 100 });
  const [all, unread, display] = await Promise.all([list({}), list({ unreadOnly: true }), coreDisplay()]);
  const rowsOf = (items: ApiNotification[]) => items.map((n) => inboxRow(n, session.role, display));
  return <NotificationsView inbox={{ rows: rowsOf(all.items), total: all.total, unreadRows: rowsOf(unread.items), unreadTotal: unread.total }} />;
}
