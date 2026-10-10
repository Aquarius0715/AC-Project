"use client";

import { useState } from "react";
import { Badge, Banner, Btn, Card, DataTable, EmptyState, Field, Kpi, ListRow, Modal, Page, Select, SummaryList, Textarea } from "@ac/web/components/ui";
import { useLocale, useT } from "@ac/web/components/I18n";
import { useAction } from "@ac/web/lib/useAction";
import { useUrlPatch } from "@ac/web/lib/useUrlPatch";
import type { T } from "@ac/web/lib/i18n";
import { payoutPdf, type ApiPayoutQuestion, type ApiPayoutStatement, type Kpi as KpiRow, type LineRow, type StatementRow } from "@ac/web/lib/partnerPayouts";
import type { ActionFailure } from "@ac/web/lib/actionMessage";
import { askHq } from "../actions";

export type PayoutsLive = {
  periods: { value: string; text: string }[]; period: string; status: string; statements: StatementRow[]; company: string; topics: { id: ApiPayoutQuestion["topic"]; text: string }[];
  selected: null | {
    statement: ApiPayoutStatement; kpis: KpiRow[]; rows: LineRow[]; questions: { id: string; title: string; message: string; state: string; reply: string | null; tone: "warn" | "ok" }[];
    title: string; sub: string; totals: [string, string][];
  };
};

const refusal = (f: ActionFailure, t: T): string | null =>
  f.code === "CONFLICT" ? t("CONFLICT — the statement changed (or is no longer approved); the page shows the latest state.")
    : f.code === "NOT_FOUND" ? t("NOT_FOUND — this statement or line is not available to your company.") : null;

/** Payout statements of the company (FR-P10): approved and paid statements only, lines priced by the rate card,
 * questions to HQ on a line; corrections come back as adjustments on the next statement. Texts in the user's display
 * language; the dates come formatted from the loader (IR279). */
export function PayoutsView({ live }: { live: PayoutsLive }) {
  const t = useT();
  const locale = useLocale();
  const nav = useUrlPatch();
  const [pending, run] = useAction();
  const [ask, setAsk] = useState<null | { lineId: string }>(null);
  const [topic, setTopic] = useState<ApiPayoutQuestion["topic"]>("amount");
  const [msg, setMsg] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [refused, setRefused] = useState<string | null>(null);
  const sel = live.selected;
  const askable = sel?.rows.filter((r) => r.askable && r.lineId && !r.question) ?? [];
  const questionError = t("Enter your question (1–2000 characters)");
  const send = () => {
    const m = msg.trim();
    if (!sel || !ask) return;
    if (m.length < 1 || m.length > 2000) return setError(questionError);
    setError(null);
    run(() => askHq(sel.statement.id, sel.statement.version, ask.lineId, topic, m), t("Question sent — HQ answers on the statement"), () => { setAsk(null); setMsg(""); setRefused(null); },
      (f) => { setRefused(refusal(f, t)); if (f.fieldErrors.message) setError(questionError); });
  };
  const download = () => {
    if (!sel) return;
    const url = URL.createObjectURL(new Blob([payoutPdf(sel.statement, sel.rows, live.company, t, locale)], { type: "application/pdf" }));
    const a = document.createElement("a");
    a.href = url; a.download = `statement-${sel.statement.period}-${sel.statement.id.slice(0, 8)}.pdf`; a.click();
    URL.revokeObjectURL(url);
  };
  return (
    <Page>
      <div className="flex flex-wrap items-center gap-2">
        <div className="w-48"><Select value={live.period} onChange={(e) => nav({ period: e.target.value || null, statementId: null })} aria-label={t("Period")}><option value="">{t("Period: {period}", { period: t("All") })}</option>{live.periods.map((p) => <option key={p.value} value={p.value}>{t("Period: {period}", { period: p.text })}</option>)}</Select></div>
        <div className="w-48"><Select value={live.status} onChange={(e) => nav({ status: e.target.value || null, statementId: null })} aria-label={t("Status")}><option value="">{t("Status: All")}</option><option value="approved">{t("Status: {status}", { status: t("Approved") })}</option><option value="paid">{t("Status: {status}", { status: t("Paid") })}</option></Select></div>
      </div>
      {refused && <Banner tone="crit">{refused}</Banner>}
      {sel && <div className="grid-fluid" style={{ ["--min" as string]: "200px" }}>{sel.kpis.map((k) => <Kpi key={k.label} label={k.label} value={k.value} sub={k.badge ? <Badge tone={k.badge.tone}>{k.badge.label}</Badge> : k.sub} />)}</div>}
      <div className="split-rev">
        <Card title={t("Statements")} className="self-start">
          {live.statements.length === 0 ? <EmptyState title={t("No statements yet")}>{t(live.period || live.status ? "No statement matches the filter." : "A statement appears here once HQ approves your first one.")}</EmptyState> : (
            <div className="flex flex-col gap-2">{live.statements.map((s) => (
              <ListRow key={s.id} selected={sel?.statement.id === s.id} onClick={() => nav({ statementId: s.id })}><div className="min-w-0 flex-1"><b className="text-[13px]">{s.label}</b><div className="text-[11px] text-muted">{s.sub}</div></div><Badge tone={s.badge.tone}>{s.badge.label}</Badge></ListRow>
            ))}</div>
          )}
          <p className="mt-2 text-[11px] text-muted">{t("A statement is issued on the 1st for jobs whose report was accepted in the previous month. HQ approves it; payment follows on the 15th. Drafts stay with HQ.")}</p>
        </Card>
        {!sel ? <Card title={t("Statement")}><EmptyState title={t("No statement selected")}>{t("Only approved and paid statements are shown; jobs still in review move to the next statement.")}</EmptyState></Card> : (
          <Card title={sel.title} sub={sel.sub}
            action={<div className="flex items-center gap-2"><Badge tone={sel.statement.status === "paid" ? "ok" : "primary"}>{t(sel.statement.status === "paid" ? "Paid" : "Approved")}</Badge><Btn size="sm" onClick={download}>{t("Download PDF")}</Btn></div>}>
            <DataTable rowKey={(r) => r.key} rows={sel.rows} cols={[
              { key: "job", label: t("Job"), render: (r) => <b>{r.job}</b> },
              { key: "work", label: t("Work"), render: (r) => <><div>{r.work}</div>{r.sub && <div className="text-[11px] text-muted">{r.sub}</div>}</> },
              { key: "accepted", label: t("Accepted"), render: (r) => <span className="whitespace-nowrap">{r.accepted}</span> },
              { key: "status", label: t("Status"), render: (r) => <Badge tone={r.tone}>{r.statusText}</Badge> },
              { key: "amount", label: sel.statement.currency, render: (r) => <span className="font-semibold">{r.amount}</span>, className: "text-right" },
              { key: "action", label: "", render: (r) => r.question ? <span className="text-[11px] text-muted">{t(r.question.state === "open" ? "Question open" : r.question.state === "adjusted" ? "Adjusted" : "Answered")}</span>
                : r.askable ? <button type="button" disabled={pending} className="text-xs font-semibold text-primary disabled:opacity-50" onClick={() => { setAsk({ lineId: r.lineId! }); setTopic(r.status === "deduction" ? "deduction" : "amount"); setError(null); }}>{t("Ask HQ")}</button>
                : <span className="text-[11px] text-muted">{r.status === "not_included" ? t("next statement") : ""}</span> },
            ]} />
            <div className="mt-3"><SummaryList items={[...sel.totals.map(([k, v]): [string, React.ReactNode] => [k, <b key={k}>{v}</b>]), [t("Bank account"), t("set by HQ — not shown in this demo")]]} /></div>
            <p className="mt-2 text-[11px] text-muted">{t("Rates come from your rate card with HQ. Only jobs whose report your review accepted (FR-P05) are paid; jobs still in review move to the next statement. Amounts are shown in {currency}.", { currency: sel.statement.currency })}</p>
            {sel.questions.length > 0 && <div className="mt-4"><b className="text-[13px]">{t("Questions to HQ")}</b><ul className="mt-1 divide-y divide-line">{sel.questions.map((q) => (
              <li key={q.id} className="py-2 text-[13px]"><div className="flex flex-wrap items-center justify-between gap-2"><span>{q.title}</span><Badge tone={q.tone}>{q.state}</Badge></div><div className="text-xs text-muted">“{q.message}”{q.reply ? ` — ${t("HQ: {reply}", { reply: q.reply })}` : ""}</div></li>
            ))}</ul></div>}
          </Card>
        )}
      </div>
      <Modal open={!!ask} onClose={() => setAsk(null)} title={t("Ask HQ about this line")} footer={<><Btn onClick={() => setAsk(null)}>{t("Cancel")}</Btn><Btn variant="primary" disabled={pending} onClick={send}>{t("Send")}</Btn></>}>
        <Field label={t("Line")}><Select value={ask?.lineId ?? ""} onChange={(e) => setAsk({ lineId: e.target.value })}>{askable.map((r) => <option key={r.lineId!} value={r.lineId!}>{r.job} · {r.work} · {r.amount} {sel?.statement.currency}</option>)}</Select></Field>
        <Field label={t("Topic")}><Select value={topic} onChange={(e) => setTopic(e.target.value as ApiPayoutQuestion["topic"])}>{live.topics.map((x) => <option key={x.id} value={x.id}>{x.text}</option>)}</Select></Field>
        <Field label={t("Question (required, 1–2000 characters)")} error={error ?? undefined}><Textarea value={msg} maxLength={2000} onChange={(e) => setMsg(e.target.value)} placeholder={t("The 2nd return was caused by a missing part from HQ stock.")} /></Field>
        <Banner>{t("HQ answers on the statement and in the job’s history. If HQ agrees, an adjustment line is added to your next statement; this statement stays approved.")}</Banner>
      </Modal>
    </Page>
  );
}
