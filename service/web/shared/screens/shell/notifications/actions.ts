"use server";

// Server Actions of the notification inbox (Next.js: Server Functions are public endpoints, so each one verifies the
// session through the DAL; the Core API then authorizes recipient-only access, IR58).
import { refresh } from "next/cache";
import { coreOp } from "@ac/web/lib/dal";
import { actionFailure } from "@ac/web/lib/actionFailure";
import type { ActionFailure } from "@ac/web/lib/actionMessage";
import type { NotificationsMarked } from "@ac/web/lib/contracts.gen";

/** notifications.markRead with the row version; returns why it was not marked (the view words it, IR380) or null. */
export async function markRead(id: string, version: number): Promise<ActionFailure | null> {
  try {
    await coreOp("notifications.markRead", { id }, { write: true, expectedVersion: version });
    return null;
  } catch (error) {
    return actionFailure(error);
  } finally {
    refresh(); // re-render the current route so the Server Component reads the updated list
  }
}

/** “Mark all as read” (Figma Client 10c): the listed unread notifications in one notifications.markAllRead write, so the
click spends one write of the member's limit (IR375, IR379); returns how many were marked and how many were skipped
(read meanwhile or no longer available), or why nothing was marked. */
export async function markAllRead(ids: string[]): Promise<{ marked: number; skipped: number; failure: ActionFailure | null }> {
  try {
    const r = await coreOp<NotificationsMarked>("notifications.markAllRead", { ids }, { write: true });
    return { marked: r.markedIds.length, skipped: r.skippedIds.length, failure: null };
  } catch (error) {
    return { marked: 0, skipped: 0, failure: actionFailure(error) }; // everything stays unread
  } finally {
    refresh();
  }
}
