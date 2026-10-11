"use server";

// Server Actions of the notification inbox (Next.js: Server Functions are public endpoints, so each one verifies the
// session through the DAL; the Core API then authorizes recipient-only access, IR58).
import { refresh } from "next/cache";
import { coreOp, CoreError } from "@ac/web/lib/dal";
import type { ActionFailure } from "@ac/web/lib/actionMessage";
import type { NotificationsMarked } from "@ac/web/lib/contracts.gen";

// the Core API is unreachable: nothing was marked
const unreachable: ActionFailure = { code: "UNAVAILABLE", messageKey: "error.unavailable", fieldErrors: {} };
const failure = (e: unknown): ActionFailure => (e instanceof CoreError
  ? { code: e.error.code, messageKey: e.error.messageKey, fieldErrors: e.error.fieldErrors, retryAfterSeconds: e.error.retryAfterSeconds }
  : unreachable);

/** notifications.markRead with the row version; returns why it was not marked (the view words it, IR380) or null. */
export async function markRead(id: string, version: number): Promise<ActionFailure | null> {
  try {
    await coreOp("notifications.markRead", { id }, { write: true, expectedVersion: version });
    return null;
  } catch (e) {
    return failure(e);
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
    return { marked: 0, skipped: 0, failure: failure(e) }; // everything stays unread
  } finally {
    refresh();
  }
}
