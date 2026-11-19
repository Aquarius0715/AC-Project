"use server";

// Server Actions of /settings/preferences (Next.js: Server Functions are public endpoints, so each one verifies the
// session through the DAL): the display language and time zone and, for a client, the monthly report e-mail
// (preferences.update; the Core API refuses the e-mail flag from other sessions), the location consent of a client
// (consents.update with the consent version; 0 when none is recorded), and two-step verification (twoFactor.enable
// returns the recovery codes once, twoFactor.disable; any 6 digits in this build, IR144 item 6).
import { refresh } from "next/cache";
import { coreOp, CoreError } from "@ac/web/lib/dal";
import { actionFailure } from "@ac/web/lib/actionFailure";
import type { ActionFailure } from "@ac/web/lib/actionMessage";
import type { Locale } from "@ac/web/lib/i18n";

type Result<T> = { ok: true; value: T } | ({ ok: false } & ActionFailure);
const run = async <T,>(fn: () => Promise<T>): Promise<Result<T>> => {
  try {
    const value = await fn();
    refresh();
    return { ok: true, value };
  } catch (error) {
    if (error instanceof CoreError) {
      if (error.error.code === "CONFLICT") refresh();
      return { ok: false, code: error.error.code, messageKey: error.error.messageKey, fieldErrors: error.error.fieldErrors, retryAfterSeconds: error.error.retryAfterSeconds };
    }
    return actionFailure(error);
  }
};

/** preferences.update with the display language and time zone (and a client's monthly report e-mail); a changed
location consent goes with it. When the preferences are saved but the consent is refused, the page is re-rendered
too, so it shows the saved preferences and only the consent stays to retry. */
export async function savePreferences(input: { locale: Locale; timezone: string; monthlyReportEmail?: boolean }, consent: { granted: boolean; version: number } | null) {
  const step = { saved: false }; // an object: a `let` set inside the callback would stay narrowed to false here
  const result = await run(async () => {
    await coreOp("preferences.update", input, { write: true });
    step.saved = true;
    if (consent) await coreOp("consents.update", { purpose: "location_automation", granted: consent.granted }, { write: true, expectedVersion: consent.version });
    return null;
  });
  if (!result.ok && step.saved &&result.code !== "CONFLICT") refresh(); // run re-rendered after a CONFLICT already
  return result;
}

/** twoFactor.enable with the 6-digit code: the recovery codes are shown once. */
export async function enableTwoFactor(code: string) {
  return run(async () => {
    const { recoveryCodes } = await coreOp<{ recoveryCodes: string[] }>("twoFactor.enable", { code }, { write: true });
    return recoveryCodes;
  });
}

/** twoFactor.disable with the 6-digit code. */
export async function disableTwoFactor(code: string) {
  return run(async () => { await coreOp("twoFactor.disable", { code }, { write: true }); return null; });
}
