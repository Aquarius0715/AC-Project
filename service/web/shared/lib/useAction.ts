"use client";

import { useTransition } from "react";
import { useToast } from "@ac/web/components/ui";
import { useT } from "@ac/web/components/I18n";
import { actionMessage, type ActionFailure } from "@ac/web/lib/actionMessage";

type Result<T> = { ok: true; value: T } | ({ ok: false } & ActionFailure);

/** Runs a Server Action in a transition: a toast on success (text or from the value) and the readable failure in the
 * display language otherwise (IR269); `after` runs only on success. Returns [pending, run]. */
export function useAction() {
  const t = useT();
  const toast = useToast();
  const [pending, start] = useTransition();
  const run = <T,>(fn: () => Promise<Result<T>>, ok: string | ((v: T) => string), after?: (v: T) => void, failed?: (r: ActionFailure) => void) =>
    start(async () => {
      const r = await fn();
      if (r.ok) {
        toast(typeof ok === "string" ? ok : ok(r.value));
        after?.(r.value);
        return;
      }
      toast(actionMessage(r, t), "crit");
      failed?.(r);
    });
  return [pending, run] as const;
}
