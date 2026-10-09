"use server";

// Server Actions of the customer Energy & cost screen (FR-C16): the demo monthly report and the monthly e-mail copy.
import { coreOp, CoreError } from "@ac/web/lib/dal";

export type ActionResult<T = null> = { ok: true; value: T } | { ok: false; messageKey: string; code: string; fieldErrors: Record<string, string> };
const failure = (e: unknown): ActionResult<never> => (e instanceof CoreError
  ? { ok: false, messageKey: e.error.messageKey, code: e.error.code, fieldErrors: e.error.fieldErrors ?? {} }
  : { ok: false, messageKey: "error.unavailable", code: "UNAVAILABLE", fieldErrors: {} });

export type ReportFile = { fileName: string; mime: string; size: number; generatedAt: string; isDemo: boolean };
export type ReportInput = { month: string; propertyIds: string[]; sections: ("energy_cost" | "month_comparison" | "co2_offsets" | "alerts_maintenance")[]; format: "pdf" | "csv" };
/** energy.exportReport (the month must have ended; only the demo file's metadata comes back) and, when the monthly
 * e-mail choice changed, preferences.update with the stored locale and timezone. */
export async function exportReport(input: ReportInput, monthlyEmail: { locale: string; timezone: string; value: boolean } | null): Promise<ActionResult<ReportFile>> {
  try {
    const file = await coreOp<ReportFile>("energy.exportReport", input);
    if (monthlyEmail) await coreOp("preferences.update", { locale: monthlyEmail.locale, timezone: monthlyEmail.timezone, monthlyReportEmail: monthlyEmail.value }, { write: true });
    return { ok: true, value: file };
  } catch (e) {
    return failure(e);
  }
}
