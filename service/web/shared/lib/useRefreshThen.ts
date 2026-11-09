"use client";

import { useEffect, useRef } from "react";
import { useRouter } from "next/navigation";

/** After a change that succeeded: makes the page read its data again and runs `then` once the new screen is shown (IR681). `pending` is the flag of the transition the change runs in: it stays true until the new read is on the screen, so a "saved" notice never stands on a screen that still shows the old values (IR680). Returns the function to call on success. */
export function useRefreshThen(pending: boolean): (then: () => void) => void {
  const router = useRouter();
  const waiting = useRef<(() => void) | null>(null);
  useEffect(() => {
    if (pending || !waiting.current) return;
    const done = waiting.current;
    waiting.current = null;
    done();
  }, [pending]);
  return (then) => {
    waiting.current = then;
    router.refresh();
  };
}
