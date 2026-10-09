"use server";

// Server Actions of /customer/alerts (FR-C08): opening an alert marks the signed-in membership's notifications about it
// read (notifications.markRead with each row version). The alert itself is never changed (DD-C08).
import { refresh } from "next/cache";
import { coreOp, CoreError } from "@ac/web/lib/dal";

/** Marks the alert's unread notifications read; returns an error message or null. */
export async function readAlert(notes: { id: string; version: number }[]): Promise<string | null> {
  try {
    for (const n of notes) await coreOp("notifications.markRead", { id: n.id }, { write: true, expectedVersion: n.version });
    return null;
  } catch (e) {
    return e instanceof CoreError ? `Not marked as read (${e.error.code}).` : "Not marked as read.";
  } finally {
    refresh();
  }
}
