"use client";

import { useEffect, useRef } from "react";
import { useRouter } from "next/navigation";

function runWaiting(waiting: { current: (() => void) | null }): void {
  const done = waiting.current;
  waiting.current = null;
  done?.();
}

/** After a change that succeeded: makes the page read its data again and runs `then` once the new screen is shown (IR681). `pending` is the flag of the transition the change runs in: it stays true until the new read is on the screen, so a "saved" notice never stands on a screen that still shows the old values (IR680). A component the new read removes (a deleted row) runs `then` as it goes (IR682). When the component that asked is removed by that new read, `then` runs as it goes (IR682). Returns the function to call on success. */
export function useRefreshThen(pending: boolean): (then: () => void) => void {
  const router = useRouter();
  const waiting = useRef<(() => void) | null>(null);
  useEffect(() => {
    if (!pending) runWaiting(waiting);
  }, [pending]);
  // a deleted row is gone with the new read: its component is removed while the transition is still on, and the
  // notice must still come (the toast belongs to the page, not to this component)
  useEffect(() => () => { runWaiting(waiting); }, []);
  return (then) => {
    waiting.current = then;
    router.refresh();
  };
}
