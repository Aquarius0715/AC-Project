"use server";

// Server Actions of the notification inbox (Next.js: Server Functions are public endpoints, so each one verifies the
// session through the DAL; the Core API then authorizes recipient-only access, IR58).
import { refresh } from "next/cache";
import { coreOp, CoreError } from "@ac/web/lib/dal";
import type { ActionFailure } from "@ac/web/lib/actionMessage";
import type { NotificationsMarked } from "@ac/web/lib/contracts.gen";

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

/** “Mark all as read” (Figma Client 10c): the listed unread notifications in one notifications.markAllRead write, so the
 * click spends one write of the member's limit (IR375, IR379); returns how many were marked and how many were skipped
 * (read meanwhile or no longer available), or why nothing was marked. */
export async function markAllRead(ids: string[]): Promise<{ marked: number; skipped: number; failure: ActionFailure | null }> {
  try {
    const r = await coreOp<NotificationsMarked>("notifications.markAllRead", { ids }, { write: true });
    return { marked: r.markedIds.length, skipped: r.skippedIds.length, failure: null };
  } catch (e) {
    const failure = e instanceof CoreError
      ? { code: e.error.code, messageKey: e.error.messageKey, fieldErrors: e.error.fieldErrors, retryAfterSeconds: e.error.retryAfterSeconds }
      : { code: "UNAVAILABLE", messageKey: "error.unavailable", fieldErrors: {} }; // the Core API is unreachable: everything stays unread
    return { marked: 0, skipped: 0, failure };
  } finally {
    refresh();
  }
}
