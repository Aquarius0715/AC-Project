"use client";
// Re-reads the page's server data at an interval while it is shown (IR809): a change another user, an automation or
// the device made then appears without a reload. It pauses while `enabled` is false (the user is editing, a dialog
// is open or a command is being sent: a re-read would reset what they are doing) and while the tab is hidden.
import { useEffect } from "react";
import { useRouter } from "next/navigation";

export const AUTO_REFRESH_SECONDS = 30;

export function useAutoRefresh(enabled: boolean, seconds = AUTO_REFRESH_SECONDS): void {
  const router = useRouter();
  useEffect(() => {
    const id = enabled ? setInterval(() => { if (document.visibilityState === "visible") router.refresh(); }, seconds * 1000) : null;
    return () => { if (id !== null) clearInterval(id); };
  }, [enabled, seconds, router]);
}
