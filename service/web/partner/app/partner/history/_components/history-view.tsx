"use client";

import Link from "next/link";
import { useState } from "react";
import { Badge, Banner, Btn, Card, Choice, EmptyState, Field, Page, Select, Textarea, cx } from "@ac/web/components/ui";
import { OriginBadge } from "@ac/web/components/JobBits";
import { useT } from "@ac/web/components/I18n";
import { useAction } from "@ac/web/lib/useAction";
import { useUrlPatch } from "@ac/web/lib/useUrlPatch";
import { CHANNELS, channelText, communicationRefusal, ROLES, TEMPLATES, type TimelineItem } from "@ac/web/lib/partnerHistory";
import { communicate, type Communication, type CommunicationFailure } from "../actions";
import type { HistoryLive } from "../_lib/load";

const dot: Record<string, string> = { ok: "bg-ok/15 text-ok", warn: "bg-warn/15 text-warn", none: "bg-primary/10 text-primary" };

/** The contractor's job history (FR-P07, FR-P08, Figma Contractor 05-1…05-6) from the Core API. Texts in the user's
 * display language; the times come formatted from the loader and the Server Action (IR278). */
export function HistoryView({ live }: { live: HistoryLive }) {
  if (live.detail) return <Detail key={live.detail.missing ? live.detail.id : live.detail.row.id} live={live} detail={live.detail} />;
  return <List live={live} />;
}

const keep = (live: HistoryLive) => [live.tab !== "all" ? `tab=${live.tab}` : "", live.sort !== "latest" ? `sort=${live.sort}` : "", live.period !== "90d" ? `period=${live.period}` : ""].filter(Boolean).join("&");
const hrefOf = (live: HistoryLive, jobId?: string) => { const q = [jobId ? `jobId=${jobId}` : "", keep(live)].filter(Boolean).join("&"); return `/partner/history${q ? `?${q}` : ""}`; };

function List({ live }: { live: HistoryLive }) {
  const t = useT();
  const patch = useUrlPatch();
  const all = live.tabs.find((x) => x.id === "all")!.count;
  return (
    <Page>
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div role="tablist" className="flex flex-wrap gap-2">
          {live.tabs.map((x) => <button key={x.id} role="tab" aria-selected={live.tab === x.id} onClick={() => patch({ tab: x.id === "all" ? null : x.id })} className={cx("rounded-control border px-3 py-1.5 text-[13px] font-semibold", live.tab === x.id ? "border-primary bg-primary text-white" : "border-line bg-surface text-ink hover:bg-surface2")}>{x.label} ({x.count})</button>)}
        </div>
        <div className="flex flex-wrap gap-2">
          <Select aria-label={t("Sort")} className="w-auto" value={live.sort} onChange={(e) => patch({ sort: e.target.value === "latest" ? null : e.target.value })}>{live.sorts.map((s) => <option key={s.id} value={s.id}>{s.text}</option>)}</Select>
          <Select aria-label={t("Period")} className="w-auto" value={live.period} onChange={(e) => patch({ period: e.target.value === "90d" ? null : e.target.value })}>{live.periods.map((p) => <option key={p.id} value={p.id}>{p.text}</option>)}</Select>
        </div>
      </div>
      <Card title={t("Your jobs — {company} only · choose a job to open its history", { company: live.company })}>
        {live.rows.length === 0 ? <EmptyState title={t(all ? "No jobs in this view" : "No jobs yet")}>{t("Jobs appear here once your company accepts an offer.")}</EmptyState> : (
          <div className="scroll-x">
            <table className="w-full min-w-[880px] text-[13px]">
              <thead><tr className="text-left text-[11px] uppercase tracking-wide text-muted"><th className="py-2 font-semibold">{t("Job")}</th><th className="py-2 font-semibold">{t("Technician")}</th><th className="py-2 font-semibold">{t("Last event")}</th><th className="py-2 font-semibold">{t("Status")}</th><th /></tr></thead>
              <tbody>
                {live.rows.map((r) => (
                  <tr key={r.id} className="border-t border-line align-top hover:bg-surface2/60">
                    <td className="py-2.5 pr-3"><Link href={hrefOf(live, r.id)} className="font-semibold text-ink hover:text-primary">{r.short} · {r.title}</Link>{r.origin && <span className="ml-2 align-middle"><OriginBadge origin={r.origin === "periodic_plan" ? "plan" : "request"} /></span>}<span className="block text-[11px] text-muted">{r.sub}</span></td>
                    <td className="py-2.5 pr-3"><b className={cx("font-semibold", r.tech.tone === "crit" && "text-crit")}>{r.tech.name}</b><span className="block text-[11px] text-muted">{r.tech.sub}</span></td>
                    <td className="py-2.5 pr-3">{r.last ? <><b className={cx("font-semibold", r.overdue && "text-crit")}>{r.overdue ? t("Work window ended") : r.last.title}</b><span className="block text-[11px] text-muted">{r.last.when}</span></> : <span className="text-muted">—</span>}</td>
                    <td className="py-2.5 pr-3"><Badge tone={r.badge.tone}>{r.badge.text}</Badge></td>
                    <td className="py-2.5 text-right"><Link href={hrefOf(live, r.id)} aria-label={t("Open the history of {id}", { id: r.short })} className="text-muted hover:text-primary">›</Link></td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
        <p className="mt-3 text-[11px] text-muted">{t(live.total > 100 ? "Showing {shown} of {total} (the latest 100) · own company only (FR-P08) · offers appear here after acceptance." : "Showing {shown} of {total} · own company only (FR-P08) · offers appear here after acceptance.", { shown: live.rows.length, total: all })}</p>
      </Card>
    </Page>
  );
}

type Detail = NonNullable<HistoryLive["detail"]>;
type SessionPreview = { id: string; title: string; sub: string };

function Detail({ live, detail }: { live: HistoryLive; detail: Detail }) {
  const t = useT();
  const patch = useUrlPatch();
  const [view, setView] = useState<"all" | "customer" | "internal">("all");
  const [previews, setPreviews] = useState<SessionPreview[]>([]);
  if (detail.missing) {
    return <Page><Crumb live={live} text={detail.id.slice(0, 8)} /><EmptyState title={t("This job isn’t in your company’s history")} action={<Link className="text-sm font-semibold text-primary" href={hrefOf(live)}>{t("Back to the job history")}</Link>}>{t("It belongs to another company, was never accepted by yours, or does not exist.")}</EmptyState></Page>;
  }
  const r = detail.row;
  const items = detail.items.filter((e) => view === "all" || e.badge === view);
  const count = (b: TimelineItem["badge"]) => detail.items.filter((e) => e.badge === b).length;
  return (
    <Page>
      <Crumb live={live} text={r.short} />
      {r.ended && <Banner>{t("Delegation ended — only your company’s own decisions remain visible; unit data, notes and reports are not shown any more (FR-P08).")}</Banner>}
      <div className="split">
        <Card title={t("Job history — {id}", { id: r.short })} action={<Select aria-label={t("Job")} className="w-auto" value={r.id} onChange={(e) => patch({ jobId: e.target.value })}>{live.options.map((o) => <option key={o.id} value={o.id}>{o.label}</option>)}</Select>}>
          <div className="grid-fluid rounded-xl border border-line bg-surface2/60 p-3 text-[13px]" style={{ ["--min" as string]: "150px" }}>
            {([["unit", t("Unit"), r.unit ?? r.title], ["tech", t("Technician"), r.tech.name], ["status", t("Status"), r.badge.text], ["delegation", t("Delegation"), r.delegation]] as const).map(([k, label, v]) => <div key={k}><div className="text-[10px] font-bold uppercase tracking-wide text-muted">{label}</div><b className={cx(k === "status" && r.badge.tone === "ok" && "text-ok", k === "status" && r.badge.tone === "crit" && "text-crit")}>{v}</b></div>)}
          </div>
          <div className="mt-3 flex flex-wrap gap-2">
            {([["all", t("All events"), detail.items.length], ["customer", t("Customer-visible"), count("customer")], ["internal", t("Internal"), count("internal")]] as const).map(([id, label, n]) => (
              <button key={id} type="button" aria-pressed={view === id} onClick={() => setView(id)} className={cx("rounded-full border px-3 py-1 text-xs font-semibold", view === id ? "border-primary bg-primary-soft text-primary" : "border-line bg-surface")}>{label} ({n})</button>
            ))}
          </div>
          <ol className="mt-3 flex flex-col">
            {items.length === 0 && <li className="py-3 text-[13px] text-muted">{t("No events in this view.")}</li>}
            {items.map((e) => (
              <li key={e.id} className="flex items-start gap-3 border-t border-line py-2.5 first:border-0">
                <span aria-hidden className={cx("grid h-7 w-7 shrink-0 place-items-center rounded-lg text-xs", dot[e.tone ?? "none"])}>{e.tone === "warn" ? "!" : e.tone === "ok" ? "✓" : "•"}</span>
                <span className="min-w-0 flex-1"><b className="block text-[13px]">{e.title}</b><span className="block text-[11px] text-muted">{[e.detail, e.at].filter(Boolean).join(" · ")}</span></span>
                <Badge tone={e.badge === "customer" ? "primary" : "muted"}>{t(e.badge)}</Badge>
              </li>
            ))}
          </ol>
          <p className="mt-2 text-[11px] text-muted">{t("The customer sees every event except internal notes.")}</p>
        </Card>
        <div className="flex min-w-0 flex-col gap-4">
          <Compose detail={detail} onPreview={(p) => setPreviews((list) => [p, ...list])} />
          <Card title={t("Previews on {id} ({n})", { id: r.short, n: previews.length })}>
            {previews.length === 0 && <p className="text-[13px] text-muted">{t("No previews yet in this session.")}</p>}
            {previews.map((p) => <div key={p.id} className="flex items-start justify-between gap-2 border-t border-line py-2 first:border-0"><span><b className="block text-[13px]">{p.title}</b><span className="text-[11px] text-muted">{p.sub}</span></span><Badge tone="primary">{t("preview")}</Badge></div>)}
            <p className="mt-1 text-[11px] text-muted">{t("deliveryState stays “preview” — nothing is sent in phase 1A, and previews are not stored.")}</p>
          </Card>
        </div>
      </div>
    </Page>
  );
}

function Crumb({ live, text }: { live: HistoryLive; text: string }) {
  const t = useT();
  return <div className="flex flex-wrap items-center gap-2 text-[13px] text-muted"><Link href={hrefOf(live)} className="font-semibold text-primary">{t("← Job history")}</Link> › <b className="text-ink">{text}</b></div>;
}

function Compose({ detail, onPreview }: { detail: Extract<Detail, { missing: false }>; onPreview: (p: SessionPreview) => void }) {
  const t = useT();
  const r = detail.row, mode = detail.mode;
  const [pending, run] = useAction();
  const [template, setTemplate] = useState<string>(TEMPLATES[0].id);
  const [visibility, setVisibility] = useState<"internal" | "customer">("internal");
  const [role, setRole] = useState<string>(ROLES.find((x) => detail.recipients.some((p) => p.role === x.id))?.id ?? ROLES[0].id);
  const ofRole = detail.recipients.filter((p) => p.role === role);
  const [recipient, setRecipient] = useState<string>(ofRole[0]?.id ?? "");
  const chosen = ofRole.find((p) => p.id === recipient) ?? ofRole[0];
  const [channel, setChannel] = useState<string>("inApp");
  const [message, setMessage] = useState("");
  const [tried, setTried] = useState(false);
  const [result, setResult] = useState<{ tone: "ok" | "crit" | "warn"; text: string } | null>(null);
  const channels = CHANNELS.filter((c) => !chosen || chosen.allowedChannels.includes(c.id));
  const tooLong = message.trim().length > 2000;
  const closed = mode.kind === "closed";
  const send = () => {
    setTried(true);
    if (!message.trim() || tooLong || !chosen) return;
    const ch = channels.some((c) => c.id === channel) ? channel : "inApp";
    run(() => communicate({ jobId: r.id, version: r.version, saveNote: mode.kind === "note", message, visibility, templateKey: template as Communication["templateKey"], channel: ch as Communication["channel"], recipientMembershipId: chosen.id }),
      t(mode.kind === "note" ? "Note saved + preview created" : "Preview created"),
      (v) => {
        const label = ROLES.find((x) => x.id === role)?.label ?? role;
        onPreview({ id: v.preview.id, title: `${template} → ${label} (${chosen.displayLabel})`, sub: [ch, t(v.preview.deliveryState), v.noteSaved ? t("note {visibility}", { visibility: t(visibility) }) : "", v.preview.at].filter(Boolean).join(" · ") });
        setResult({
          tone: "ok",
          text: v.noteSaved ? t(visibility === "internal" ? "Saved the note (internal) and created the {channel} preview for {name}. Nothing was sent; internal notes stay hidden from the customer." : "Saved the note (customer-visible) and created the {channel} preview for {name}. Nothing was sent.", { channel: channelText(ch, t), name: chosen.displayLabel })
            : t("Created the {channel} preview for {name}. Nothing was sent.", { channel: channelText(ch, t), name: chosen.displayLabel }),
        });
        if (v.noteSaved) { setMessage(""); setTried(false); }
      },
      (f) => {
        const c = f as CommunicationFailure;
        setResult({ tone: c.noteSaved ? "warn" : "crit", text: c.noteSaved ? t("The note was saved, but the preview failed: {reason}", { reason: communicationRefusal(f, t) }) : t("Not saved: {reason} Your message is kept.", { reason: communicationRefusal(f, t) }) });
      });
  };
  return (
    <Card title={t("New communication preview")}>
      <div className="flex flex-col gap-3">
        {mode.text && <Banner tone={closed ? undefined : "warn"}>{mode.text}</Banner>}
        {result && <Banner tone={result.tone}>{result.text}</Banner>}
        <Field label={t("Template")}><Select value={template} disabled={closed} onChange={(e) => setTemplate(e.target.value)}>{TEMPLATES.map((x) => <option key={x.id} value={x.id}>{x.label}</option>)}</Select></Field>
        {mode.kind === "note" && <Field label={t("Visibility")}><Choice value={visibility} onChange={setVisibility} options={[{ id: "internal", label: t("Internal (default)") }, { id: "customer", label: t("Customer-visible") }]} /></Field>}
        <Field label={t("Recipient role")} hint={chosen ? `${t(ROLES.find((x) => x.id === role)?.sub ?? "")} · ${chosen.displayLabel}` : t("Nobody of this role is part of the job.")}>
          <Select value={role} disabled={closed} onChange={(e) => { setRole(e.target.value); setRecipient(detail.recipients.find((p) => p.role === e.target.value)?.id ?? ""); }}>
            {ROLES.map((x) => <option key={x.id} value={x.id} disabled={!detail.recipients.some((p) => p.role === x.id)}>{x.label}</option>)}
          </Select>
        </Field>
        {ofRole.length > 1 && <Field label={t("Recipient")}><Select value={chosen?.id ?? ""} onChange={(e) => setRecipient(e.target.value)}>{ofRole.map((p) => <option key={p.id} value={p.id}>{p.displayLabel}</option>)}</Select></Field>}
        <Field label={t("Channel")}><Select value={channels.some((c) => c.id === channel) ? channel : "inApp"} disabled={closed} onChange={(e) => setChannel(e.target.value)}>{channels.map((c) => <option key={c.id} value={c.id}>{t("{channel} · preview only", { channel: c.id })}</option>)}</Select></Field>
        <Field label={t("Message (1–2000 chars)")} error={tried && !message.trim() ? t("A message is required (VALIDATION) — nothing was saved") : tooLong ? t("At most 2000 characters") : undefined}>
          <Textarea value={message} disabled={closed} onChange={(e) => setMessage(e.target.value)} placeholder={t("Schedule moved to 2026-09-22 09:00–11:00 due to technician availability.")} />
        </Field>
        <div className="flex gap-2"><Btn disabled={pending || closed} onClick={() => { setMessage(""); setTried(false); setResult(null); }}>{t("Clear")}</Btn><Btn variant="primary" disabled={pending || closed || !chosen} onClick={send}>{mode.button}</Btn></div>
        <p className="text-[11px] text-muted">{t("No real sending. Only people involved in this job are offered; free-form external recipients and other jobs’ recipients are rejected. Internal notes stay hidden from customers.")}</p>
      </div>
    </Card>
  );
}
