// The failure a Server Action returns for whatever its Core API call threw. A refused call keeps its code, key and field
// errors; a call that got no answer (the Core API is unreachable or restarting) reads as unavailable, so the page that
// called the action stays and keeps what the user has typed, instead of falling to the error boundary. Next's own control
// flow (the redirect to sign-in of the DAL, not-found) is not a failure and goes on.
import "server-only";
import { unstable_rethrow } from "next/navigation";
import { CoreError } from "@ac/web/lib/dal";
import type { ActionFailure } from "@ac/web/lib/actionMessage";

export function actionFailure(error: unknown): { ok: false } & ActionFailure {
  if (error instanceof CoreError) {
    return { ok: false, code: error.error.code, messageKey: error.error.messageKey, fieldErrors: error.error.fieldErrors, retryAfterSeconds: error.error.retryAfterSeconds };
  }
  unstable_rethrow(error);
  return { ok: false, code: "UNAVAILABLE", messageKey: "error.unavailable", fieldErrors: {} };
}
