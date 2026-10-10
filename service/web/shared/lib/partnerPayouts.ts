// Contractor payouts (FR-P10, DATA_SOURCE=api): statements, KPIs, the lines table with its job labels, open
// questions and a one-page PDF of the selected statement. Pure code shared by the Server Component and the client
// view. Texts in the display language (`t` / `i`, IR279): periods and pay dates are Kuala Lumpur business days, the
// moments a report was accepted or a statement paid are dates in the user's display time zone.
import { EN, intlTag, showDate, showDayMonth, translator, type I18n, type Locale, type T } from "@ac/web/lib/i18n";
import { businessDay } from "@ac/web/lib/clientBilling";
import { typeLabel } from "@ac/web/lib/partnerJobDetail";

const en = translator("en");

/** PayoutStatement of service-contracts.ts. */
export type ApiPayoutLine = { id: string; jobId: string; workType: "periodic_inspection" | "repair_base" | "emergency" | "rework_deduction"; acceptedAt: string | null; amountMinor: number; kind: "charge" | "deduction" | "adjustment"; note: string | null };
export type ApiPayoutQuestion = { id: string; lineId: string | null; topic: "amount" | "deduction" | "missing_job" | "other"; message: string; state: "open" | "answered" | "adjusted"; reply: string | null; adjustmentMinor: number | null };
export type ApiPayoutStatement = {
  id: string; version: number; contractorOrgId: string; period: string; status: "draft" | "approved" | "paid"; currency: string; grossMinor: number; deductionsMinor: number; netMinor: number;
  payDate: string; lines: ApiPayoutLine[]; queries: ApiPayoutQuestion[]; approvedByMembershipId: string | null; paidAt: string | null;
};
/** What the contractor may read about a line's job (jobs.get: a history snapshot after the access window, else the detail). */
export type JobLabel = { title: string; sub: string | null };
/** A delegated job still in quality review (jobs.list status submitted): “Not included · next statement”. */
export type ReviewingJob = { id: string; type: string; unitName: string | null; submittedAt?: string | null };

const plain = (s: string) => s.replace(/[   ]/g, " ");
/** “Sept 2026” from a YYYY-MM period in the user's language. */
export const periodLabel = (p: string, locale: Locale = "en") =>
  /^\d{4}-\d{2}$/.test(p) ? plain(new Date(`${p}-15T00:00:00Z`).toLocaleDateString(intlTag(locale), { month: "short", year: "numeric", timeZone: "UTC" })) : p;
/** “September” — the month of a period, inside a sentence. */
const monthName = (p: string, locale: Locale) => (/^\d{4}-\d{2}$/.test(p) ? new Date(`${p}-15T00:00:00Z`).toLocaleDateString(intlTag(locale), { month: "long", timeZone: "UTC" }) : p);
/** “MYR 1,990.00”; negative amounts read “− MYR 120.00”. */
export function money(minor: number, currency: string): string {
  const abs = (Math.abs(minor) / 100).toLocaleString("en-MY", { minimumFractionDigits: 2, maximumFractionDigits: 2 });
  return `${minor < 0 ? "− " : ""}${currency} ${abs}`;
}
const amount = (minor: number) => `${minor < 0 ? "− " : ""}${(Math.abs(minor) / 100).toLocaleString("en-MY", { minimumFractionDigits: 2, maximumFractionDigits: 2 })}`;
export const workLabel: Record<ApiPayoutLine["workType"], string> = { periodic_inspection: "Periodic inspection", repair_base: "Repair", emergency: "Emergency call-out", rework_deduction: "Rework deduction" };

export type StatementRow = { id: string; period: string; label: string; sub: string; badge: { label: string; tone: "primary" | "ok" } };
/** The statements list (newest period first): “Sept 2026 · stmt · MYR 1,990.00 · Approved / Paid 15 Sept”. */
export function statementRows(items: ApiPayoutStatement[], i: I18n = EN): StatementRow[] {
  const { t, display } = i;
  return items.map((s) => {
    const paid = s.status === "paid" && s.paidAt ? showDayMonth(s.paidAt, display) : null;
    return {
      id: s.id, period: s.period, label: `${periodLabel(s.period, display.locale)} · ${s.id.slice(0, 8)}`,
      sub: `${money(s.netMinor, s.currency)} · ${paid ? t("paid {date}", { date: paid }) : t("pays {date}", { date: businessDay(s.payDate, display.locale) })}`,
      badge: s.status === "paid" ? { label: paid ? t("Paid {date}", { date: paid }) : t("Paid"), tone: "ok" } : { label: t("Approved"), tone: "primary" },
    };
  });
}

export type Kpi = { label: string; value: string; sub: string; badge?: { label: string; tone: "primary" | "ok" } };
/** Jobs paid · Gross · Deductions · Net payable with the pay date (Figma 06-1). */
export function kpis(s: ApiPayoutStatement, rateCard: string | null, i: I18n = EN): Kpi[] {
  const { t, display } = i;
  const jobs = new Set(s.lines.filter((l) => l.kind === "charge").map((l) => l.jobId)).size;
  const deductions = s.lines.filter((l) => l.kind === "deduction" || (l.kind === "adjustment" && l.amountMinor < 0)).length;
  return [
    { label: t("Jobs paid"), value: String(jobs), sub: t("review-accepted in {month}", { month: monthName(s.period, display.locale) }) },
    { label: t("Gross"), value: money(s.grossMinor, s.currency), sub: rateCard ? t("rate card {card}", { card: rateCard }) : t("rate card set by HQ") },
    { label: t("Deductions"), value: money(-s.deductionsMinor, s.currency), sub: deductions ? t(deductions === 1 ? "1 rework deduction" : "{n} rework deductions", { n: deductions }) : t("none") },
    {
      label: t("Net payable"), value: money(s.netMinor, s.currency), sub: "",
      badge: s.status === "paid" ? { label: s.paidAt ? t("Paid {date}", { date: showDate(s.paidAt, display) }) : t("Paid"), tone: "ok" } : { label: t("Approved · pays {date}", { date: businessDay(s.payDate, display.locale) }), tone: "primary" },
    },
  ];
}

export type LineStatus = "accepted" | "deduction" | "adjustment" | "not_included";
export type LineRow = {
  key: string; lineId: string | null; jobId: string; job: string; work: string; sub: string | null; accepted: string; status: LineStatus; statusText: string;
  tone: "ok" | "warn" | "primary" | "muted"; amount: string; question: ApiPayoutQuestion | null; askable: boolean;
};
const STATUS_TEXT: Record<LineStatus, string> = { accepted: "Accepted", deduction: "Deduction", adjustment: "Adjustment", not_included: "Not included" };
/** The lines table (Figma 06-1): charges, deductions and adjustments of the statement, then the company's jobs
 * still in quality review as “Not included · next statement” (AT-P10-B). */
export function lineRows(s: ApiPayoutStatement, jobs: Map<string, JobLabel>, reviewing: ReviewingJob[], i: I18n = EN): LineRow[] {
  const { t, display } = i;
  const byLine = new Map(s.queries.filter((q) => q.lineId).map((q) => [q.lineId!, q]));
  const inStatement = new Set(s.lines.map((l) => l.jobId));
  const rows: LineRow[] = s.lines.map((l) => {
    const j = jobs.get(l.jobId);
    const status: LineStatus = l.kind === "deduction" ? "deduction" : l.kind === "adjustment" ? "adjustment" : "accepted";
    return {
      key: l.id, lineId: l.id, jobId: l.jobId, job: l.jobId.slice(0, 8),
      work: status === "deduction" ? t("Rework deduction") : status === "adjustment" ? t(l.note ? "Adjustment" : "Adjustment from HQ") : j?.title ?? t(workLabel[l.workType]),
      sub: status === "deduction" ? (l.note ?? t("report returned for rework")) : status === "adjustment" ? (l.note ?? null) : j?.sub ?? (l.note ?? null),
      accepted: l.acceptedAt ? showDayMonth(l.acceptedAt, display) : "—", status, statusText: t(STATUS_TEXT[status]),
      tone: status === "deduction" ? "warn" : status === "adjustment" ? "primary" : "ok",
      amount: amount(status === "deduction" ? -Math.abs(l.amountMinor) : l.amountMinor), question: byLine.get(l.id) ?? null, askable: s.status === "approved",
    };
  });
  for (const j of reviewing) {
    if (inStatement.has(j.id)) continue;
    rows.push({
      key: `review-${j.id}`, lineId: null, jobId: j.id, job: j.id.slice(0, 8), work: `${typeLabel(j.type, t)}${j.unitName ? ` · ${j.unitName}` : ""}`, sub: t("awaiting quality review"),
      accepted: "—", status: "not_included", statusText: t(STATUS_TEXT.not_included), tone: "muted", amount: "—", question: null, askable: false,
    });
  }
  return rows;
}

export const topicLabel: Record<ApiPayoutQuestion["topic"], string> = { amount: "Amount", deduction: "Deduction", missing_job: "Missing job", other: "Other" };
/** The question rows under the table: topic, line, message and HQ's reply or adjustment. */
export function questionRows(s: ApiPayoutStatement, rows: LineRow[], t: T = en) {
  return s.queries.map((q) => {
    const line = rows.find((r) => r.lineId === q.lineId);
    return {
      id: q.id, title: `${t(topicLabel[q.topic])} · ${line ? `${line.job} · ${line.work}` : t("statement")}`, message: q.message,
      state: q.state === "open" ? t("Waiting for HQ") : q.state === "adjusted" ? (q.adjustmentMinor !== null ? t("Adjusted {amount} on the next statement", { amount: money(q.adjustmentMinor, s.currency) }) : t("Adjusted on the next statement")) : t("Answered"),
      reply: q.reply, tone: (q.state === "open" ? "warn" : "ok") as "warn" | "ok",
    };
  });
}

/** A one-page PDF (Courier, PDF 1.4) of the statement: header, lines and totals in the user's language. ASCII only —
 * other characters print as “-” — the result is a byte string the browser wraps in a Blob. */
export function payoutPdf(s: ApiPayoutStatement, rows: LineRow[], company: string, t: T = en, locale: Locale = "en"): string {
  const col = (x: string, w: number) => (x.length > w ? x.slice(0, w - 1) + "…" : x).padEnd(w);
  const text = [
    `${t("PAYOUT STATEMENT")} ${s.id.slice(0, 8)}  ${periodLabel(s.period, locale)}`, `${company}  -  ${t(s.status === "paid" ? "Paid" : s.status === "approved" ? "Approved" : "Draft").toUpperCase()}  -  ${t("pay date {date}", { date: s.payDate.slice(0, 10) })}`, "",
    `${col(t("Job"), 10)} ${col(t("Work"), 36)} ${col(t("Accepted"), 9)} ${t("Amount").padStart(12)}`, "-".repeat(70),
    ...rows.filter((r) => r.lineId).map((r) => `${col(r.job, 10)} ${col(r.work, 36)} ${col(r.accepted, 9)} ${r.amount.padStart(12)}`), "-".repeat(70),
    `${t("Gross").padEnd(57)} ${money(s.grossMinor, s.currency).padStart(12)}`, `${t("Deductions").padEnd(57)} ${money(-s.deductionsMinor, s.currency).padStart(12)}`,
    `${t("Net payable").padEnd(57)} ${money(s.netMinor, s.currency).padStart(12)}`, "",
    t("Rates come from the contractor's rate card with HQ. Only review-accepted jobs are paid."), t("Demo document - not a tax invoice."),
  ];
  const esc = (x: string) => x.replace(/[^\x20-\x7e]/g, "-").replace(/[\\()]/g, (c) => "\\" + c);
  const stream = `BT /F1 9 Tf 40 800 Td 12 TL ${text.map((l) => `(${esc(l)}) Tj T*`).join(" ")} ET`;
  const objects = [
    "<< /Type /Catalog /Pages 2 0 R >>", "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
    "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 595 842] /Resources << /Font << /F1 4 0 R >> >> /Contents 5 0 R >>",
    "<< /Type /Font /Subtype /Type1 /BaseFont /Courier >>", `<< /Length ${stream.length} >>\nstream\n${stream}\nendstream`,
  ];
  let out = "%PDF-1.4\n";
  const offsets: number[] = [];
  objects.forEach((o, k) => { offsets.push(out.length); out += `${k + 1} 0 obj\n${o}\nendobj\n`; });
  const xref = out.length;
  out += `xref\n0 ${objects.length + 1}\n0000000000 65535 f \n${offsets.map((o) => `${String(o).padStart(10, "0")} 00000 n \n`).join("")}`;
  out += `trailer\n<< /Size ${objects.length + 1} /Root 1 0 R >>\nstartxref\n${xref}\n%%EOF\n`;
  return out;
}
