"use server";

// Server Actions of /settings/preferences (Next.js: Server Functions are public endpoints, so each one verifies the
// session through the DAL): the display language and time zone and, for a client, the monthly report e-mail
// (preferences.update; the Core API refuses the e-mail flag from other sessions), the location consent of a client
// (consents.update with the consent version; 0 when none is recorded), and two-step verification (twoFactor.enable
// returns the recovery codes once, twoFactor.disable; any 6 digits in this build, IR144 item 6).
import { refresh } from "next/cache";
import { coreOp, CoreError } from "@ac/web/lib/dal";
import type { ActionFailure } from "@ac/web/lib/actionMessage";

type Result<T> = { ok: true; value: T } | ({ ok: false } & ActionFailure);
const run = async <T,>(fn: () => Promise<T>): Promise<Result<T>> => {
  try {
    const value = await fn();
    refresh();
    return { ok: true, value };
  } catch (e) {
    if (e instanceof CoreError) {
      if (e.error.code === "CONFLICT") refresh();
      return { ok: false, code: e.error.code, messageKey: e.error.messageKey, fieldErrors: e.error.fieldErrors };
    }
    throw e;
  }
};

/** preferences.update with the display language and time zone (and a client's monthly report e-mail); a changed
 * location consent goes with it. */
export async function savePreferences(input: { locale: "en" | "ms"; timezone: string; monthlyReportEmail?: boolean }, consent: { granted: boolean; version: number } | null) {
  return run(async () => {
    await coreOp("preferences.update", input, { write: true });
    if (consent) await coreOp("consents.update", { purpose: "location_automation", granted: consent.granted }, { write: true, expectedVersion: consent.version });
    return null;
  });
}

/** twoFactor.enable with the 6-digit code: the recovery codes are shown once. */
export async function enableTwoFactor(code: string) {
  return run(async () => (await coreOp<{ recoveryCodes: string[] }>("twoFactor.enable", { code }, { write: true })).recoveryCodes);
}

/** twoFactor.disable with the 6-digit code. */
export async function disableTwoFactor(code: string) {
  return run(async () => { await coreOp("twoFactor.disable", { code }, { write: true }); return null; });
}
