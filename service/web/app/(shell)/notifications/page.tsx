// /notifications (FR-X07): in API mode a Server Component reads the caller's own notifications through the DAL
// (newest first) and hands plain rows to the client view; the Phase 1A demo keeps the in-browser rows.
import { connection } from "next/server";
import { apiMode, coreOp, verifySession } from "@/lib/dal";
import { inboxRow, type ApiNotification } from "@/lib/notifications";
import { NotificationsView } from "./_components/notifications-view";

export default async function NotificationsPage() {
  await connection(); // DATA_SOURCE is a runtime setting of the image (Next.js environment variables guide)
  if (!apiMode()) return <NotificationsView />;
  const session = await verifySession();
  const page = await coreOp<{ items: ApiNotification[] }>("notifications.list", { limit: 100, sort: { field: "occurredAt", direction: "desc" } });
  return <NotificationsView rows={page.items.map((n) => inboxRow(n, session.role))} />;
}
