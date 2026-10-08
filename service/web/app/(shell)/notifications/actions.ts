"use server";

// Server Actions of the notification inbox (Next.js: Server Functions are public endpoints, so each one verifies the
// session through the DAL; the Core API then authorizes recipient-only access, IR58).
import { refresh } from "next/cache";
import { coreOp, CoreError } from "@/lib/dal";

const messages: Record<string, string> = {
  "error.versionConflict": "This notification changed — the latest state is shown.",
  "error.notFound": "This notification is no longer available.",
};

/** notifications.markRead with the row version; returns an error message or null. */
export async function markRead(id: string, version: number): Promise<string | null> {
  try {
    await coreOp("notifications.markRead", { id }, { write: true, expectedVersion: version });
    return null;
  } catch (e) {
    return e instanceof CoreError ? (messages[e.error.messageKey] ?? `Not marked as read (${e.error.code}).`) : "Not marked as read.";
  } finally {
    refresh(); // re-render the current route so the Server Component reads the updated list
  }
}
