"use client";

import { useTransition } from "react";
import { useRefreshThen } from "@ac/web/lib/useRefreshThen";
import { useToast } from "@ac/web/components/ui";
import { useT } from "@ac/web/components/I18n";
import { signInAgainHref, signedOutReason } from "@ac/web/lib/login";
import { actionMessage, type ActionFailure } from "@ac/web/lib/actionMessage";
import { reached } from "@ac/web/lib/unreachable";

/** Runs a Server Action in a transition: a toast on success (text or from the value) and the readable failure in the
display language otherwise (IR269); `after` runs only on success. A change that succeeded makes the page read its data
again (`router.refresh()`, whatever the action itself asked for), and the transition lasts until the new screen is
shown: the toast and `after` come then, so "saved" is never on a screen that still shows the old values (IR680). A change
that takes the screen away (a declined offer: the job leaves the company and its page would be not found) passes
`{ reload: false }`: the toast and `after` come at once and the page is not read again (IR685). Returns [pending, run]. */
export function useAction() {
  type Result<T> = { ok: true; value: T } | ({ ok: false } & ActionFailure);
  const t = useT();
  const toast = useToast();
  const [pending, start] = useTransition();
  const refreshThen = useRefreshThen(pending);
  const run = <T,>(fn: () => Promise<Result<T>>, ok: string | ((v: T) => string), after?: (v: T) => void, failed?: (r: ActionFailure) => void, options?: { reload?: boolean }) => { start(async () => {
      const r = await reached(fn); // no answer at all reads as unavailable (IR717)
      if (r.ok && options?.reload === false) {
        toast(typeof ok === "string" ? ok : ok(r.value));
        after?.(r.value);
        return;
      }
      if (r.ok) {
        refreshThen(() => { toast(typeof ok === "string" ? ok : ok(r.value)); after?.(r.value); });
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
