"use client";

import { useCallback, useTransition } from "react";
import { usePathname, useRouter } from "next/navigation";

/** Replaces URL search params in place (API-mode screens keep their scope and selection in the URL); null or ""
removes a key. The server reads the new URL and re-renders. */
export function useUrlPatch() {
  const router = useRouter();
  const pathname = usePathname();
  // stable across renders, so effects and callbacks that depend on it do not re-run on every render
  return useCallback((patch: Record<string, string | null>) => {
    const q = new URLSearchParams(location.search);
    for (const [k, v] of Object.entries(patch)) {
      if (v) q.set(k, v);
      else q.delete(k);
    }
    router.replace(q.size > 0 ? `${pathname}?${q}` : pathname, { scroll: false });
  }, [router, pathname]);
}

/** useUrlPatch with the navigation's pending state: the patch runs in a transition, so `isPending` stays true until the
server's new render is on screen — a control can show the choice at once instead of waiting a second (UI/UX loop R74). */
export function useUrlPatchPending(): [(patch: Record<string, string | null>) => void, boolean] {
  const patch = useUrlPatch();
  const [isPending, startTransition] = useTransition();
  const run = useCallback((p: Record<string, string | null>) => { startTransition(() => { patch(p); }); }, [patch]);
  return [run, isPending];
}
