// Why a command to an air conditioner failed, in words (IR799): the stored failure code (DEVICE_REJECTED, TIMEOUT …) is
// never shown as it is, as no customer, contractor or technician reads error codes (IR626); a code this list does not
// know reads as “no reason given”.
import { textOf, translator, type T } from "@ac/web/lib/i18n";

const en = translator("en");

const REASON: Record<string, string> = {
  DEVICE_REJECTED: "the AC refused it",
  TIMEOUT: "no answer from the AC in time",
  OFFLINE: "the AC was offline",
  UNAVAILABLE: "the AC reported a fault",
  CONFLICT: "another command was still running",
};

/** The reason of a failed command in the display language; “no reason given” for a code it does not know. */
export const commandFailure = (code: string, t: T = en): string => textOf(REASON, code, t) ?? t("no reason given");
