import { describe, expect, it } from "vitest";
import { kpis, lineRows, money, payoutPdf, periodLabel, questionRows, statementRows, type ApiPayoutLine, type ApiPayoutStatement } from "@ac/web/lib/partnerPayouts";
import { i18nOf, translator } from "@ac/web/lib/i18n";
import { must } from "./must";

const JOB_BBBBBBBB = "job-bbbbbbbb";
const PERIODIC_INSPECTION = "Periodic inspection";
const JOB_EEEEEEEE = "job-eeeeeeee";

const MS = i18nOf({ locale: "ms", timeZone: "Asia/Tokyo" });
const ms = translator("ms");

const line = (over: Partial<ApiPayoutLine> = {}): ApiPayoutLine => ({ id: "l1", jobId: "job-aaaaaaaa", workType: "repair_base", acceptedAt: "2026-08-05T03:00:00Z", amountMinor: 45_000, kind: "charge", note: null, ...over });
const statement = (over: Partial<ApiPayoutStatement> = {}): ApiPayoutStatement => {
  return {
    id: "stmt-2026-08-aaaa", version: 2, contractorOrgId: "org-contractor-a", period: "2026-08", status: "approved", currency: "MYR", grossMinor: 128_000, deductionsMinor: 12_000, netMinor: 116_000,
    payDate: "2026-09-15T00:00:00Z", approvedByMembershipId: "hq", paidAt: null, queries: [],
    lines: [line(), line({ id: "l2", jobId: JOB_BBBBBBBB, workType: "periodic_inspection", amountMinor: 38_000, acceptedAt: "2026-08-12T03:00:00Z" }),
      line({ id: "l3", jobId: JOB_BBBBBBBB, workType: "rework_deduction", amountMinor: 12_000, kind: "deduction", acceptedAt: "2026-08-12T03:00:00Z" }),
      line({ id: "l4", jobId: "job-cccccccc", amountMinor: 45_000, acceptedAt: "2026-08-20T03:00:00Z" })],
    ...over,
  };
};

describe("labels and money", () => {
  it("prints periods and signed amounts", () => {
    expect([periodLabel("2026-09"), periodLabel("2026-09", "ms"), periodLabel("Q3")]).toStrictEqual(["Sept 2026", "Sep 2026", "Q3"]);
    expect(money(199_000, "MYR")).toBe("MYR 1,990.00");
    expect(money(-12_000, "MYR")).toBe("− MYR 120.00");
    expect(money(0, "USD")).toBe("USD 0.00");
  });
  it("lists statements newest first with their badge", () => {
    const rows = statementRows([statement(), statement({ id: "stmt-2026-07-bbbb", period: "2026-07", status: "paid", netMinor: 204_000, paidAt: "2026-08-15T02:00:00Z" })]);
    expect(rows[0]).toStrictEqual({ id: "stmt-2026-08-aaaa", period: "2026-08", label: "Aug 2026 · stmt-202", sub: "MYR 1,160.00 · pays 15 Sept 2026", badge: { label: "Approved", tone: "primary" } });
    expect(rows[1]).toMatchObject({ label: "Jul 2026 · stmt-202", sub: "MYR 2,040.00 · paid 15 Aug", badge: { label: "Paid 15 Aug", tone: "ok" } });
  });
});

describe("KPIs", () => {
  it("counts distinct paid jobs and deductions and shows the pay date", () => {
    const k = kpis(statement(), "e8c270a5 v3");
    expect(k.map((x) => [x.label, x.value])).toStrictEqual([["Jobs paid", "3"], ["Gross", "MYR 1,280.00"], ["Deductions", "− MYR 120.00"], ["Net payable", "MYR 1,160.00"]]);
    expect(k[0]?.sub).toBe("review-accepted in August");
    expect(k[1]?.sub).toBe("rate card e8c270a5 v3");
    expect(k[2]?.sub).toBe("1 rework deduction");
    expect(k[3]?.badge).toStrictEqual({ label: "Approved · pays 15 Sept 2026", tone: "primary" });
    expect(kpis(statement({ status: "paid", paidAt: "2026-09-15T02:00:00Z", deductionsMinor: 0, lines: [line()] }), null)[3]?.badge).toStrictEqual({ label: "Paid 15 Sept 2026", tone: "ok" });
    expect(kpis(statement({ lines: [line()] }), null)[2]?.sub).toBe("none");
    expect(kpis(statement(), null)[1]?.sub).toBe("rate card set by HQ");
  });
});

describe("lines table", () => {
  const jobs = new Map([["job-aaaaaaaa", { title: "Repair · Bedroom AC", sub: "Home A" }], [JOB_BBBBBBBB, { title: PERIODIC_INSPECTION, sub: "customer details closed with the delegation" }]]);
  it("prices charges, deductions and adjustments and appends the jobs still in review", () => {
    const s = statement({ lines: [...statement().lines, line({ id: "l5", jobId: "job-dddddddd", kind: "adjustment", amountMinor: -5000, note: null, acceptedAt: null })] });
    const rows = lineRows(s, jobs, [{ id: "job-cccccccc", type: "reactive", unitName: "Lobby AC" }, { id: JOB_EEEEEEEE, type: "periodic", unitName: null }]);
    expect(rows.map((r) => [r.job, r.work, r.accepted, r.status, r.statusText, r.amount])).toStrictEqual([
      ["job-aaaa", "Repair · Bedroom AC", "5 Aug", "accepted", "Accepted", "450.00"], ["job-bbbb", PERIODIC_INSPECTION, "12 Aug", "accepted", "Accepted", "380.00"],
      ["job-bbbb", "Rework deduction", "12 Aug", "deduction", "Deduction", "− 120.00"], ["job-cccc", "Repair", "20 Aug", "accepted", "Accepted", "450.00"],
      ["job-dddd", "Adjustment from HQ", "—", "adjustment", "Adjustment", "− 50.00"], ["job-eeee", PERIODIC_INSPECTION, "—", "not_included", "Not included", "—"],
    ]);
    expect(rows[0]?.sub).toBe("Home A");
    expect(rows[2]?.sub).toBe("report returned for rework");
    expect(rows[5]).toMatchObject({ lineId: null, askable: false, sub: "awaiting quality review", tone: "muted" });
    expect(rows.slice(0, 5).every((r) => r.askable)).toBe(true);
  });
  it("binds questions to their line and asks only on approved statements", () => {
    const q = { id: "q1", lineId: "l3", topic: "deduction" as const, message: "Why?", state: "open" as const, reply: null, adjustmentMinor: null };
    const rows = lineRows(statement({ status: "paid", queries: [q] }), new Map(), []);
    expect(rows.find((r) => r.lineId === "l3")?.question).toStrictEqual(q);
    expect(rows.every((r) => !r.askable)).toBe(true);
    const qs = questionRows(statement({ queries: [q, { ...q, id: "q2", lineId: "l1", state: "adjusted", reply: "Agreed", adjustmentMinor: 12_000 }, { ...q, id: "q3", lineId: null, state: "answered", reply: "No" }] }), rows);
    expect(qs.map((x) => [x.title, x.state, x.tone])).toStrictEqual([
      ["Deduction · job-bbbb · Rework deduction", "Waiting for HQ", "warn"], ["Deduction · job-aaaa · Repair", "Adjusted MYR 120.00 on the next statement", "ok"], ["Deduction · statement", "Answered", "ok"],
    ]);
  });
});

describe("payoutPdf", () => {
  it("builds a valid one-page PDF with ASCII text and correct xref offsets", () => {
    const rows = lineRows(statement(), new Map(), [{ id: JOB_EEEEEEEE, type: "periodic", unitName: null }]);
    const pdf = payoutPdf(statement(), rows, "Contractor (A) − demo");
    expect(pdf.startsWith("%PDF-1.4\n")).toBe(true);
    expect(pdf).toContain("/Type /Catalog");
    expect(pdf).toContain("/BaseFont /Courier");
    expect(pdf).toContain("Contractor \\(A\\) - demo"); // parentheses escaped, non-ASCII replaced
    expect(pdf).toContain("(Net payable");
    expect(pdf).not.toMatch(/\P{ASCII}/u);
    expect(pdf.match(/\) Tj/g) ?? []).toHaveLength(16); // 2 header + blank + column head + rule + 4 statement lines (Not included rows are left out) + rule + 3 totals + blank + 2 notes
    const xref = Number(must(/startxref\n(\d+)/.exec(pdf))[1]);
    expect(pdf.slice(xref, xref + 4)).toBe("xref");
    const offsets = pdf.matchAll(/(\d{10}) 00000 n/g).map((m) => Number(m[1])).toArray();
    for (const [i, o] of offsets.entries()) expect(pdf.slice(o, o + `${String(i + 1)} 0 obj`.length)).toBe(`${String(i + 1)} 0 obj`);
    const length = Number(must(/\/Length (\d+) >>\nstream\n/.exec(pdf))[1]);
    const stream = must(must(/stream\n([\s\S]*?)\nendstream/.exec(pdf))[1]);
    expect(stream).toHaveLength(length);
  });
  it("continues a long statement on further pages, so every line and the totals stay on a page (IR738)", () => {
    const lines = Array.from({ length: 120 }, (_, k) => line({ id: `l${String(k)}`, jobId: `job-${String(k).padStart(8, "0")}` }));
    const s = statement({ lines });
    const pdf = payoutPdf(s, lineRows(s, new Map(), []), "Demo Contractor A");
    const streams = pdf.matchAll(/stream\n([\s\S]*?)\nendstream/g).map((m) => must(m[1])).toArray();
    expect(streams).toHaveLength(3); // 132 text lines, 64 to a page
    expect(pdf).toContain("/Count 3");
    for (const st of streams) expect((st.match(/\) Tj/g) ?? []).length).toBeLessThanOrEqual(64);
    expect(streams.join("").match(/\) Tj/g) ?? []).toHaveLength(132); // 120 statement lines + the 12 of the header, rules, totals and notes
    expect(must(streams.at(-1))).toContain("(Net payable");
    for (const m of pdf.matchAll(/\/Length (\d+) >>\nstream\n([\s\S]*?)\nendstream/g)) expect(must(m[2])).toHaveLength(Number(m[1]));
    const xref = Number(must(/startxref\n(\d+)/.exec(pdf))[1]);
    expect(pdf.slice(xref, xref + 4)).toBe("xref");
    const offsets = pdf.matchAll(/(\d{10}) 00000 n/g).map((m) => Number(m[1])).toArray();
    for (const [i, o] of offsets.entries()) expect(pdf.slice(o, o + `${String(i + 1)} 0 obj`.length)).toBe(`${String(i + 1)} 0 obj`);
  });
});

describe("payouts in Malay (IR279)", () => {
  it("words the statements, KPIs, lines, questions and the PDF in Malay", () => {
    expect(statementRows([statement()], MS)[0]).toMatchObject({ label: "Ogo 2026 · stmt-202", sub: "MYR 1,160.00 · dibayar pada 15 Sep 2026", badge: { label: "Diluluskan" } });
    expect(kpis(statement(), "e8c270a5 v3", MS).map((k) => [k.label, k.sub])).toStrictEqual([
      ["Kerja dibayar", "diterima dalam semakan pada Ogos"], ["Kasar", "kad kadar e8c270a5 v3"], ["Potongan", "1 potongan kerja semula"], ["Bersih perlu dibayar", ""],
    ]);
    const rows = lineRows(statement(), new Map(), [{ id: JOB_EEEEEEEE, type: "periodic", unitName: null }], MS);
    expect(rows.map((r) => [r.work, r.accepted, r.statusText])).toStrictEqual([
      ["Pembaikan", "5 Ogo", "Diterima"], ["Pemeriksaan berkala", "12 Ogo", "Diterima"], ["Potongan kerja semula", "12 Ogo", "Potongan"], ["Pembaikan", "20 Ogo", "Diterima"], ["Pemeriksaan berkala", "—", "Tidak termasuk"],
    ]);
    const q = { id: "q1", lineId: "l3", topic: "deduction" as const, message: "Why?", state: "open" as const, reply: null, adjustmentMinor: null };
    expect(questionRows(statement({ queries: [q] }), rows, ms)[0]).toMatchObject({ title: "Potongan · job-bbbb · Potongan kerja semula", state: "Menunggu HQ" });
    const pdf = payoutPdf(statement(), rows, "Demo Contractor A", ms, "ms");
    expect(pdf).toContain("(PENYATA BAYARAN stmt-202  Ogo 2026)");
    expect(pdf).toContain("(Bersih perlu dibayar");
    expect(pdf).not.toMatch(/\P{ASCII}/u);
  });

  // IR639: the PDF's font has ASCII only, so a Japanese user's statement is the English one, not a page of “-”
  it("writes a Japanese user's statement in English", () => {
    const rows = lineRows(statement(), new Map(), [{ id: JOB_EEEEEEEE, type: "periodic", unitName: null }]);
    expect(payoutPdf(statement(), rows, "Demo Contractor A", translator("ja"), "ja")).toBe(payoutPdf(statement(), rows, "Demo Contractor A"));
  });
});
