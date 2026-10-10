"use client";

import Link from "next/link";
import { useState } from "react";
import { Badge, Banner, Btn, Card, EmptyState, Field, Page, SeverityBadge, SummaryList, Textarea, cx, useToast } from "@ac/web/components/ui";
import { useT } from "@ac/web/components/I18n";
import { EvidencePicker } from "@ac/web/components/EvidencePicker";
import { evidenceProblem, evidenceRefusal } from "@ac/web/lib/alertEvidence";
import { useUrlPatch } from "@ac/web/lib/useUrlPatch";
import { refusal } from "@ac/web/lib/techAlerts";
import { acknowledgeAlert, resolveAlert } from "../actions";
import type { AlertsLive } from "../_lib/load";

const BANNER = { crit: "border-crit/40 bg-crit-soft text-crit", warn: "border-[#fdba74] bg-warn-soft text-warn", ok: "border-ok/40 bg-ok-soft text-ok", primary: "border-line bg-surface2 text-ink", muted: "border-line bg-surface2 text-ink" } as const;

/** The technician's alert evidence (FR-T07, Figma Technician 02-14…02-17): the alert with its policy's condition, the
 * evidence, resolution — acknowledge, and resolve with a reason and the evidence it cites (alert.resolve, IR327) — its
 * history and what it relates to;
 * the unit's other alerts to switch to (?alertId=). Texts in the user's display language; the times come formatted from
 * the loader (IR284). Acknowledge and resolve are Server Actions with the alert version (IR87, IR94). */
export function AlertEvidenceView({ live }: { live: AlertsLive }) {
  const t = useT();
  const toast = useToast();
  const patch = useUrlPatch();
  const [reason, setReason] = useState("");
  const [picked, setPicked] = useState<string[]>([]);
  const [msg, setMsg] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const a = live.alert;
  const act = async (op: "acknowledge" | "resolve") => {
    if (!a) return;
    const ids = picked.filter((id) => live.evidence?.some((r) => r.id === id)); // a re-read list drops what is no longer a candidate
    if (op === "resolve" && !reason.trim()) return setMsg(t("A resolution reason is required (1–1000 characters)."));
    const problem = op === "resolve" && live.evidence ? evidenceProblem(a.policyless, ids, t) : null;
    if (problem) return setMsg(problem);
    setBusy(true);
    setMsg(null);
    const res = op === "resolve" ? await resolveAlert(a.id, a.version, reason.trim(), ids) : await acknowledgeAlert(a.id, a.version);
    setBusy(false);
    if (!res.ok) return setMsg(evidenceRefusal(res.fieldErrors, t) ?? refusal(res, a.resolution.forbidden, t));
    toast(t(op === "resolve" ? "Alert resolved" : "Alert acknowledged"));
    setReason("");
    setPicked([]);
  };
  return (
    <Page>
      <div className="flex flex-wrap items-center gap-2 text-[13px]"><Link href={live.unitHref} className="font-semibold text-primary" aria-label={t("Back")}>‹</Link><h1 className="text-base font-bold">{t("Alert evidence")}</h1><span className="text-muted">· {live.unitName}</span></div>
      {!a ? <Card><EmptyState title={t("No alerts on this unit")}>{t("Alerts raised on this unit appear here with their evidence.")}</EmptyState></Card> : (
        <>
          <div className={cx("flex flex-wrap items-center justify-between gap-3 rounded-2xl border px-4 py-3", BANNER[a.banner.tone])}>
            <div className="min-w-0"><b className="block text-[15px]">{a.banner.title}</b><span className="text-xs">{a.banner.sub}</span></div>
            <div className="flex items-center gap-2"><SeverityBadge s={a.severity} />{a.status === "open" ? <Btn size="sm" disabled={busy} onClick={() => act("acknowledge")}>{t("Acknowledge")}</Btn> : <b className="text-xs">{a.banner.state}</b>}</div>
          </div>
          <div className="grid-fluid" style={{ ["--min" as string]: "260px" }}>
            {a.cards.map((c) => (
              <div key={c.title} className="rounded-2xl border border-line bg-surface p-4">
                <div className="text-[11px] font-semibold uppercase tracking-wide text-muted">{c.kind}</div>
                <div className="text-[13px] font-semibold">{c.title}</div>
                <div className="mt-1 text-[15px] font-bold">{c.value}</div>
                <div className="mt-1 text-[11px] text-muted">{c.sub}</div>
              </div>
            ))}
          </div>
          <Card title={t("Resolution")}>
            {msg && <div className="mb-3"><Banner tone="crit">{msg}</Banner></div>}
            {a.resolution.done ? <Banner tone="ok">{a.resolution.done}</Banner> : (
              <>
                {a.resolution.info && <Banner>{a.resolution.info}</Banner>}
                {live.evidence && <div className="mt-3"><EvidencePicker rows={live.evidence} policyless={a.policyless} picked={picked.filter((id) => live.evidence?.some((r) => r.id === id))} onChange={setPicked} /></div>}
                <div className="mt-3"><Field label={t("Resolution reason (required, 1–1000 characters)")}><Textarea value={reason} maxLength={1000} placeholder={t("e.g. Confirmed on site that airflow is restored after filter cleaning.")} onChange={(e) => setReason(e.target.value)} /></Field></div>
                <div className="mt-3"><Btn variant="primary" className="w-full" disabled={busy} onClick={() => act("resolve")}>{t("Resolve alert")}</Btn></div>
              </>
            )}
            <p className="mt-2 text-[11px] text-muted">{a.resolution.note}</p>
          </Card>
          <div className="split">
            <Card title={t("Alert history")}>
              {a.history.map((h) => (
                <div key={h.id} className="flex items-start justify-between gap-2 border-t border-line py-2 first:border-0">
                  <span className="min-w-0"><b className="block text-[13px]">{h.time} · {h.title}</b>{h.sub && <span className="text-xs text-muted">{h.sub}</span>}</span><Badge tone={h.badge.tone}>{h.badge.text}</Badge>
                </div>
              ))}
            </Card>
            <div className="flex min-w-0 flex-col gap-4">
              <Card title={t("Related")}><SummaryList items={a.related} /></Card>
              {live.count > 1 && (
                <Card title={t("Alerts on this unit ({n})", { n: live.count })}>
                  {live.choices.map((x) => (
                    <button key={x.id} type="button" onClick={() => { setMsg(null); setPicked([]); patch({ alertId: x.id }); }} className={cx("flex w-full items-center justify-between gap-2 border-t border-line py-2 text-left first:border-0", x.id === a.id && "font-bold")}>
                      <span className="min-w-0"><span className="block text-[13px]">{x.title}</span><span className="text-xs font-normal text-muted">{x.sub}</span></span><SeverityBadge s={x.severity} />
                    </button>
                  ))}
                </Card>
              )}
            </div>
          </div>
        </>
      )}
    </Page>
  );
}
