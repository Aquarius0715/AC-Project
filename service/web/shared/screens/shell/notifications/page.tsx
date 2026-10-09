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
  const [page, display] = await Promise.all([coreOp<{ items: ApiNotification[] }>("notifications.list", { limit: 100, sort: { field: "occurredAt", direction: "desc" } }), coreDisplay()]);
  return <NotificationsView rows={page.items.map((n) => inboxRow(n, session.role, display))} />;
}
