"use client";

import { usePathname, useRouter } from "next/navigation";

/** Replaces URL search params in place (API-mode screens keep their scope and selection in the URL); null or ""
 * removes a key. The server reads the new URL and re-renders. */
export function useUrlPatch() {
  const router = useRouter();
  const pathname = usePathname();
  return (patch: Record<string, string | null>) => {
    const q = new URLSearchParams(window.location.search);
    for (const [k, v] of Object.entries(patch)) {
      if (v) q.set(k, v);
      else q.delete(k);
    }
    router.replace(q.size ? `${pathname}?${q}` : pathname, { scroll: false });
  };
}
