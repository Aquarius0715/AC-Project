"use client";

import { useEffect, useRef, useTransition } from "react";
import { useRouter } from "next/navigation";
import { useToast } from "@ac/web/components/ui";
import { useT } from "@ac/web/components/I18n";
import { signInAgainHref, signedOutReason } from "@ac/web/lib/login";
import { actionMessage, type ActionFailure } from "@ac/web/lib/actionMessage";

/** Runs a Server Action in a transition: a toast on success (text or from the value) and the readable failure in the
display language otherwise (IR269); `after` runs only on success. A change that succeeded makes the page read its data
again (`router.refresh()`, whatever the action itself asked for), and the transition lasts until the new screen is
shown: the toast and `after` come then, so "saved" is never on a screen that still shows the old values (IR680). Returns
[pending, run]. */
export function useAction() {
  type Result<T> = { ok: true; value: T } | ({ ok: false } & ActionFailure);
  const t = useT();
  const toast = useToast();
  const router = useRouter();
  const [pending, start] = useTransition();
  const shown = useRef<(() => void) | null>(null); // the toast and `after` of a success, for when its screen is shown
  useEffect(() => {
    if (pending || !shown.current) return;
    const done = shown.current;
    shown.current = null;
    done();
  }, [pending]);
  const run = <T,>(fn: () => Promise<Result<T>>, ok: string | ((v: T) => string), after?: (v: T) => void, failed?: (r: ActionFailure) => void) => { start(async () => {
      const r = await fn();
      if (r.ok) {
        shown.current = () => { toast(typeof ok === "string" ? ok : ok(r.value)); after?.(r.value); };
        router.refresh();
        return;
      }
      if (r.code === "UNAUTHENTICATED") { // the session was refused: sign in again, then back here (IR385)
        location.assign(signInAgainHref(signedOutReason(r.messageKey), location.pathname + location.search));
        return;
      }
      toast(actionMessage(r, t), "crit");
      failed?.(r);
    }); };
  return [pending, run] as const;
}
