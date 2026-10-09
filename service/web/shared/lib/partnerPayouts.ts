// Contractor payouts (FR-P10, DATA_SOURCE=api): statements, KPIs, the lines table with its job labels, open
// questions and a one-page PDF of the selected statement. Pure code shared by the Server Component and the client
// view.
import { klTime } from "@ac/web/lib/devices";

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

const months = ["Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"];
/** “Sep 2026” from a YYYY-MM period. */
export const periodLabel = (p: string) => `${months[+p.slice(5, 7) - 1] ?? p} ${p.slice(0, 4)}`;
/** “MYR 1,990.00”; negative amounts read “− MYR 120.00”. */
export function money(minor: number, currency: string): string {
  const abs = (Math.abs(minor) / 100).toLocaleString("en-MY", { minimumFractionDigits: 2, maximumFractionDigits: 2 });
  return `${minor < 0 ? "− " : ""}${currency} ${abs}`;
}
const amount = (minor: number) => `${minor < 0 ? "− " : ""}${(Math.abs(minor) / 100).toLocaleString("en-MY", { minimumFractionDigits: 2, maximumFractionDigits: 2 })}`;
export const workLabel: Record<ApiPayoutLine["workType"], string> = { periodic_inspection: "Periodic inspection", repair_base: "Repair", emergency: "Emergency call-out", rework_deduction: "Rework deduction" };
export const jobTypeLabel: Record<string, string> = { periodic: "Periodic inspection", reactive: "Repair", preventive: "Preventive maintenance" };

export type StatementRow = { id: string; period: string; label: string; sub: string; badge: { label: string; tone: "primary" | "ok" } };
/** The statements list (newest period first): “stmt · Sep 2026 · MYR 1,990.00 · Approved / Paid 09-15”. */
export function statementRows(items: ApiPayoutStatement[]): StatementRow[] {
  return items.map((s) => ({
    id: s.id, period: s.period, label: `${periodLabel(s.period)} · ${s.id.slice(0, 8)}`, sub: `${money(s.netMinor, s.currency)} · ${s.status === "paid" && s.paidAt ? `paid ${klTime(s.paidAt).slice(5, 10)}` : `pays ${s.payDate.slice(0, 10)}`}`,
    badge: s.status === "paid" ? { label: `Paid ${s.paidAt ? klTime(s.paidAt).slice(5, 10) : ""}`.trim(), tone: "ok" } : { label: "Approved", tone: "primary" },
  }));
}

export type Kpi = { label: string; value: string; sub: string; badge?: { label: string; tone: "primary" | "ok" } };
/** Jobs paid · Gross · Deductions · Net payable with the pay date (Figma 06-1). */
export function kpis(s: ApiPayoutStatement, rateCard: string | null): Kpi[] {
  const jobs = new Set(s.lines.filter((l) => l.kind === "charge").map((l) => l.jobId)).size;
  const deductions = s.lines.filter((l) => l.kind === "deduction" || (l.kind === "adjustment" && l.amountMinor < 0)).length;
  return [
    { label: "Jobs paid", value: String(jobs), sub: `review-accepted in ${months[+s.period.slice(5, 7) - 1]}` },
    { label: "Gross", value: money(s.grossMinor, s.currency), sub: rateCard ? `rate card ${rateCard}` : "rate card set by HQ" },
    { label: "Deductions", value: money(-s.deductionsMinor, s.currency), sub: deductions ? `${deductions} rework deduction${deductions === 1 ? "" : "s"}` : "none" },
    { label: "Net payable", value: money(s.netMinor, s.currency), sub: "", badge: s.status === "paid" ? { label: `Paid ${s.paidAt ? klTime(s.paidAt).slice(0, 10) : ""}`.trim(), tone: "ok" } : { label: `Approved · pays ${s.payDate.slice(0, 10)}`, tone: "primary" } },
  ];
}

export type LineRow = {
  key: string; lineId: string | null; jobId: string; job: string; work: string; sub: string | null; accepted: string; status: "Accepted" | "Deduction" | "Adjustment" | "Not included";
  tone: "ok" | "warn" | "primary" | "muted"; amount: string; question: ApiPayoutQuestion | null; askable: boolean;
};
/** The lines table (Figma 06-1): charges, deductions and adjustments of the statement, then the company's jobs
 * still in quality review as “Not included · next statement” (AT-P10-B). */
export function lineRows(s: ApiPayoutStatement, jobs: Map<string, JobLabel>, reviewing: ReviewingJob[]): LineRow[] {
  const byLine = new Map(s.queries.filter((q) => q.lineId).map((q) => [q.lineId!, q]));
  const inStatement = new Set(s.lines.map((l) => l.jobId));
  const rows: LineRow[] = s.lines.map((l) => {
    const j = jobs.get(l.jobId);
    const deduction = l.kind === "deduction";
    return {
      key: l.id, lineId: l.id, jobId: l.jobId, job: l.jobId.slice(0, 8),
      work: deduction ? "Rework deduction" : l.kind === "adjustment" ? `Adjustment${l.note ? "" : " from HQ"}` : j?.title ?? workLabel[l.workType],
      sub: deduction ? (l.note ?? "report returned for rework") : l.kind === "adjustment" ? (l.note ?? null) : j?.sub ?? (l.note ?? null),
      accepted: l.acceptedAt ? klTime(l.acceptedAt).slice(5, 10) : "—",
      status: deduction ? "Deduction" : l.kind === "adjustment" ? "Adjustment" : "Accepted", tone: deduction ? "warn" : l.kind === "adjustment" ? "primary" : "ok",
      amount: amount(deduction ? -Math.abs(l.amountMinor) : l.amountMinor), question: byLine.get(l.id) ?? null, askable: s.status === "approved",
    };
  });
  for (const j of reviewing) {
    if (inStatement.has(j.id)) continue;
    rows.push({ key: `review-${j.id}`, lineId: null, jobId: j.id, job: j.id.slice(0, 8), work: `${jobTypeLabel[j.type] ?? j.type}${j.unitName ? ` · ${j.unitName}` : ""}`, sub: "awaiting quality review",
      accepted: "—", status: "Not included", tone: "muted", amount: "—", question: null, askable: false });
  }
  return rows;
}

export const topicLabel: Record<ApiPayoutQuestion["topic"], string> = { amount: "Amount", deduction: "Deduction", missing_job: "Missing job", other: "Other" };
/** The question rows under the table: topic, line, message and HQ's reply or adjustment. */
export function questionRows(s: ApiPayoutStatement, rows: LineRow[]) {
  return s.queries.map((q) => {
    const line = rows.find((r) => r.lineId === q.lineId);
    return {
      id: q.id, title: `${topicLabel[q.topic]} · ${line ? `${line.job} · ${line.work}` : "statement"}`, message: q.message,
      state: q.state === "open" ? "Waiting for HQ" : q.state === "adjusted" ? `Adjusted ${q.adjustmentMinor !== null ? money(q.adjustmentMinor, s.currency) : ""} on the next statement` : "Answered",
      reply: q.reply, tone: (q.state === "open" ? "warn" : "ok") as "warn" | "ok",
    };
  });
}

/** A one-page PDF (Courier, PDF 1.4) of the statement: header, lines and totals. ASCII only — the result is a
 * byte string the browser wraps in a Blob. */
export function payoutPdf(s: ApiPayoutStatement, rows: LineRow[], company: string): string {
  const col = (t: string, w: number) => (t.length > w ? t.slice(0, w - 1) + "…" : t).padEnd(w);
  const text = [
    `PAYOUT STATEMENT ${s.id.slice(0, 8)}  ${periodLabel(s.period)}`, `${company}  -  ${s.status.toUpperCase()}  -  pay date ${s.payDate.slice(0, 10)}`, "",
    `${col("Job", 10)} ${col("Work", 36)} ${col("Accepted", 9)} ${"Amount".padStart(12)}`, "-".repeat(70),
    ...rows.filter((r) => r.lineId).map((r) => `${col(r.job, 10)} ${col(r.work, 36)} ${col(r.accepted, 9)} ${r.amount.padStart(12)}`), "-".repeat(70),
    `${"Gross".padEnd(57)} ${money(s.grossMinor, s.currency).padStart(12)}`, `${"Deductions".padEnd(57)} ${money(-s.deductionsMinor, s.currency).padStart(12)}`,
    `${"Net payable".padEnd(57)} ${money(s.netMinor, s.currency).padStart(12)}`, "",
    "Rates come from the contractor's rate card with HQ. Only review-accepted jobs are paid.", "Demo document - not a tax invoice.",
  ];
  const esc = (t: string) => t.replace(/[^\x20-\x7e]/g, "-").replace(/[\\()]/g, (c) => "\\" + c);
  const stream = `BT /F1 9 Tf 40 800 Td 12 TL ${text.map((l) => `(${esc(l)}) Tj T*`).join(" ")} ET`;
  const objects = [
    "<< /Type /Catalog /Pages 2 0 R >>", "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
    "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 595 842] /Resources << /Font << /F1 4 0 R >> >> /Contents 5 0 R >>",
    "<< /Type /Font /Subtype /Type1 /BaseFont /Courier >>", `<< /Length ${stream.length} >>\nstream\n${stream}\nendstream`,
  ];
  let out = "%PDF-1.4\n";
  const offsets: number[] = [];
  objects.forEach((o, i) => { offsets.push(out.length); out += `${i + 1} 0 obj\n${o}\nendobj\n`; });
  const xref = out.length;
  out += `xref\n0 ${objects.length + 1}\n0000000000 65535 f \n${offsets.map((o) => `${String(o).padStart(10, "0")} 00000 n \n`).join("")}`;
  out += `trailer\n<< /Size ${objects.length + 1} /Root 1 0 R >>\nstartxref\n${xref}\n%%EOF\n`;
  return out;
}
