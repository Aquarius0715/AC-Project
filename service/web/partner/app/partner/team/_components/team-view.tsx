"use client";

import { useState } from "react";
import { Badge, Banner, Btn, Card, cx, EmptyState, Field, Input, LinkBtn, Modal, Page, Select, Tabs, TextLink, UtilBar } from "@ac/web/components/ui";
import { useT } from "@ac/web/components/I18n";
import { useAction } from "@ac/web/lib/useAction";
import { useUrlPatch } from "@ac/web/lib/useUrlPatch";
import { useUrlTab } from "@ac/web/lib/useUrlTab";
import { unavailabilityConflicts, unavailabilityErrors, unavailabilityRefusal, type UnavailabilityForm, type WeekCell } from "@ac/web/lib/partnerTeam";
import { setUnavailability } from "../actions";
import { Certifications } from "./certifications";
import type { TeamLive } from "../_lib/load";

type Live = Extract<TeamLive, { notFound: false }>;
const cellTone: Record<WeekCell["kind"], string> = {
  busy: "bg-primary/70 text-white", free: "border border-line bg-surface text-muted", off: "bg-warn-soft text-warn", none: "bg-surface2 text-muted",
};

/** Team & capacity (FR-P06, Figma Contractor 04-1…04-5, 04-8) from the Core API: the URL's date, qualification and
 * activeOnly, the technicians with the chosen day, the team's week, the qualification grants and unavailable days.
 * Texts in the user's display language; every date and time comes formatted from the loader (IR276). */
export function TeamView({ live }: { live: TeamLive }) {
  const t = useT();
  const [tab, setTab] = useUrlTab<"members" | "certs">({ members: "overview", certs: "certifications" }, "members");
  const tabs = <div><Tabs value={tab} onChange={setTab} tabs={[{ id: "members", label: t("Members") }, { id: "certs", label: t("Certifications") }]} /></div>;
  if (live.notFound) {
    return (
      <Page narrow>
        <div className="flex justify-center">{tabs}</div>
        <Card title={t("Roster not found")}>
          <p className="text-[13px] text-muted">{t("You can only see {company}’s own technicians. Changing the company ID in the URL does not reveal another company’s roster.", { company: live.company })}</p>
          <div className="mt-3"><LinkBtn href="/partner/team" variant="primary" size="sm">{t("Back to my team")}</LinkBtn></div>
        </Card>
      </Page>
    );
  }
  return <Team live={live} tab={tab} tabs={tabs} />;
}

function Team({ live, tab, tabs }: { live: Live; tab: "members" | "certs"; tabs: React.ReactNode }) {
  const t = useT();
  const patch = useUrlPatch();
  const [open, setOpen] = useState(false);
  const q = live.query;
  if (live.total === 0) {
    return <Page>{tabs}<EmptyState title={t("No technicians")}>{t("No technician membership in your company. New technicians are registered by HQ.")}</EmptyState></Page>;
  }
  if (tab === "certs") return <Page>{tabs}<Certifications live={live} /></Page>;
  const none = live.rows.length === 0;
  return (
    <Page>
      {tabs}
      <div className="flex flex-wrap items-center gap-2">
        <Input type="date" aria-label={t("Date")} className="w-auto" value={q.date} onChange={(e) => e.target.value && patch({ date: e.target.value === q.today ? null : e.target.value })} />
        <Select aria-label={t("Qualification")} className="w-auto" value={q.qualification ?? ""} onChange={(e) => patch({ qualification: e.target.value || null })}>
          <option value="">{t("All qualifications")}</option>
          {live.qualifications.map((x) => <option key={x.code} value={x.code}>{x.text}</option>)}
        </Select>
        <Btn size="sm" aria-pressed={q.activeOnly} className={cx(q.activeOnly && "border-primary bg-primary-soft text-primary")} onClick={() => patch({ activeOnly: q.activeOnly ? "false" : null })}>
          {q.activeOnly ? `✓ ${t("Active only")}` : t("Include expired")}
        </Btn>
      </div>
      <Card title={`${t("Technicians — {company}", { company: live.company })}${q.activeOnly ? ` · ${t("{n} active", { n: live.activeCount })}` : ""}`}
        action={<div className="flex flex-wrap gap-2"><LinkBtn href="/partner/schedule" size="sm">{t("Open schedule →")}</LinkBtn><Btn size="sm" onClick={() => setOpen(true)} disabled={!live.technicians.length}>{t("+ Unavailable days")}</Btn></div>}>
        {none ? (
          <div className="py-6 text-center">
            <b className="text-[13px]">{t("No {company} technicians hold this qualification", { company: live.company })}</b>
            <p className="mt-1 text-xs text-muted">{t("Ask HQ to update qualifications — this screen cannot change memberships.")}</p>
            <Btn size="sm" className="mt-3" onClick={() => patch({ qualification: null })}>{t("Clear filter")}</Btn>
          </div>
        ) : (
          <ul className="divide-y divide-line">
            {live.rows.map((r) => (
              <li key={r.id} className={cx("flex flex-wrap items-center gap-3 py-3", !r.active && "opacity-70")}>
                <span aria-hidden className="grid h-9 w-9 shrink-0 place-items-center rounded-xl bg-primary-soft text-primary">☻</span>
                <div className="min-w-[200px] flex-1"><b className="block text-[13px]">{r.name}</b><span className="block text-[11px] text-muted">{r.sub}</span></div>
                {r.active && (
                  <div className="w-44">
                    <div className="mb-1 flex justify-between gap-2 text-[11px]"><span className="text-muted">{live.week.short}</span><b>{r.week.text}</b></div>
                    <UtilBar pct={r.week.pct} />
                  </div>
                )}
                <div className="min-w-[180px] text-right"><span className={cx("block text-xs", r.day.tone === "crit" ? "text-crit" : r.day.tone === "warn" ? "text-warn" : "text-ink")}>{r.day.main}</span><span className="block text-[11px] text-muted">{r.day.sub}</span></div>
                <div className="w-16 text-right"><b className="block text-lg">{r.util}</b><span className="block text-[10px] text-muted">{t("utilization")}</span></div>
              </li>
            ))}
          </ul>
        )}
      </Card>
      <p className="text-[11px] text-muted">{t("Utilization = assigned hours ÷ configured available hours (4 h / 8 h = 50%). Undefined available hours show “—”, not 0%. Read-only — membership changes are requested from HQ.")}</p>
      {!none && (
        <div className="split">
          <WeekCard live={live} />
          <Card title={t("Qualifications")}>
            <ul className="divide-y divide-line">
              {live.grants.map((g) => (
                <li key={g.key} className="flex items-center justify-between gap-2 py-2">
                  <span className="min-w-0"><b className="block text-[13px]">{g.name}</b><span className="block text-[11px] text-muted">{g.sub}</span></span>
                  <Badge tone={g.badge.tone}>{g.badge.text}</Badge>
                </li>
              ))}
            </ul>
            <p className="mt-2 text-[11px] text-muted">
              {live.hidden > 0 && `${t(live.hidden === 1 ? "1 expired member hidden (Active only)." : "{n} expired members hidden (Active only).", { n: live.hidden })} `}
              {live.expired.length > 0 ? t("Expired memberships ({names}) are listed but cannot be selected for assignment. Changes are requested from HQ.", { names: live.expired.join(", ") }) : t("Qualification or membership changes are requested from HQ — contractors cannot grant permissions.")}
            </p>
          </Card>
        </div>
      )}
      {open && <UnavailableDays live={live} onClose={() => setOpen(false)} />}
    </Page>
  );
}

function WeekCard({ live }: { live: Live }) {
  const t = useT();
  const w = live.week;
  return (
    <Card title={w.title} action={<TextLink href="/partner/schedule">{t("Open schedule →")}</TextLink>}>
      <div className="scroll-x">
        <table className="w-full min-w-[560px] text-xs">
          <thead><tr><th />{w.days.map((d, k) => <th key={w.dates[k]} className={cx("p-1 text-center font-semibold", k === w.dayIndex ? "text-primary" : "text-muted")}>{d}</th>)}</tr></thead>
          <tbody>{w.rows.map((r) => (
            <tr key={r.id} className="border-t border-line">
              <td className="py-1.5 pr-2 font-semibold">{r.name}</td>
              {r.cells.map((c, k) => <td key={w.dates[k]} className="p-1"><div className={cx("rounded-lg px-1.5 py-2 text-center font-semibold", cellTone[c.kind], k === w.dayIndex && "ring-2 ring-primary/40")}>{c.text}</div></td>)}
            </tr>
          ))}</tbody>
        </table>
      </div>
      <dl className="mt-3 flex flex-wrap gap-6 text-xs">
        {([[t("Assigned"), w.stats.assigned], [t("Available"), w.stats.available], [t("Team utilization"), w.stats.utilization], [w.freeLabel, w.stats.free]] as const).map(([k, v]) => (
          <div key={k}><dt className="text-[11px] text-muted">{k}</dt><dd className="text-[15px] font-bold">{v}</dd></div>
        ))}
      </dl>
      <p className="mt-2 text-[11px] text-muted">
        {t("Sat/Sun have no configured available hours, so utilization shows “—”.")}
        {live.otherZone && ` ${t("Days and hours are in Kuala Lumpur time (Asia/Kuala_Lumpur).")}`}
      </p>
    </Card>
  );
}

/** Add unavailable days (Figma 04-8): technician or the whole company, Kuala Lumpur days, type and note; the confirmed
 * assignments in the days are listed before saving and kept by the save (DD-P06). */
function UnavailableDays({ live, onClose }: { live: Live; onClose: () => void }) {
  const t = useT();
  const [pending, run] = useAction();
  const [form, setForm] = useState<UnavailabilityForm>({ membershipId: live.technicians[0]?.id ?? "", from: live.query.date, to: live.query.date, type: "annual_leave", note: "" });
  const [tried, setTried] = useState(false);
  const [server, setServer] = useState<Record<string, string>>({});
  const [refused, setRefused] = useState<string | null>(null);
  const local = tried ? unavailabilityErrors(form, t) : {};
  const err = (k: "from" | "to" | "type" | "note") => (local as Record<string, string | undefined>)[k] ?? server[k];
  const set = (p: Partial<UnavailabilityForm>) => { setForm({ ...form, ...p }); setServer({}); setRefused(null); };
  const conflicts = unavailabilityConflicts(live.jobs, form);
  const names = new Map(live.technicians.map((x) => [x.id, x.name]));
  const list = conflicts.map((c) => `${c.short} · ${form.membershipId ? "" : `${names.get(c.technicianId) ?? ""} · `}${c.text}`).join("; ");
  const save = () => {
    setTried(true);
    if (Object.keys(unavailabilityErrors(form, t)).length) return;
    run(() => setUnavailability({ membershipId: form.membershipId || null, from: form.from, to: form.to, type: form.type as "annual_leave" | "sick" | "training" | "public_holiday" | "other", note: form.note.trim() || undefined }),
      (v) => (v.conflicts === 0 ? t("Unavailable days saved — the available hours of those days are 0.")
        : t(v.conflicts === 1 ? "Unavailable days saved — 1 confirmed assignment overlaps them; reassign it in Schedule & assignments." : "Unavailable days saved — {n} confirmed assignments overlap them; reassign them in Schedule & assignments.", { n: v.conflicts })),
      () => onClose(),
      (f) => { const r = unavailabilityRefusal(f, t); setServer(r.fields); setRefused(r.text); });
  };
  return (
    <Modal open onClose={onClose} title={t("Add unavailable days")} footer={<>
      <Btn onClick={onClose}>{t("Cancel")}</Btn>
      <LinkBtn href={conflicts[0] ? `/partner/schedule?jobId=${conflicts[0].jobId}` : "/partner/schedule"}>{t("Open schedule →")}</LinkBtn>
      <Btn variant="primary" disabled={pending} onClick={save}>{t("Save")}</Btn>
    </>}>
      <Field label={t("Technician")}>
        <Select value={form.membershipId} onChange={(e) => set({ membershipId: e.target.value })}>
          {live.technicians.map((x) => <option key={x.id} value={x.id}>{x.name}</option>)}
          <option value="">{t("All technicians (public holiday)")}</option>
        </Select>
      </Field>
      <div className="grid-fluid" style={{ ["--min" as string]: "150px" }}>
        <Field label={t("From")} error={err("from")}><Input type="date" value={form.from} onChange={(e) => set({ from: e.target.value })} /></Field>
        <Field label={t("To")} error={err("to")}><Input type="date" value={form.to} min={form.from} onChange={(e) => set({ to: e.target.value })} /></Field>
        <Field label={t("Type")} error={err("type")}>
          <Select value={form.type} onChange={(e) => set({ type: e.target.value })}>{live.types.map((x) => <option key={x.id} value={x.id}>{x.text}</option>)}</Select>
        </Field>
      </div>
      <Field label={t("Note (optional)")} error={err("note")}><Input value={form.note} maxLength={500} onChange={(e) => set({ note: e.target.value })} /></Field>
      {conflicts.length > 0 && <Banner tone="warn">{t(conflicts.length === 1 ? "Conflicts with 1 confirmed assignment: {list}. Saving keeps the assignment — reassign it in Schedule & assignments." : "Conflicts with {n} confirmed assignments: {list}. Saving keeps the assignments — reassign them in Schedule & assignments.", { n: conflicts.length, list })}</Banner>}
      {refused && <Banner tone="crit">{refused}</Banner>}
      <p className="text-[11px] text-muted">
        {t("Unavailable days set the technician’s available hours to 0 for those dates, so utilization and candidate lists (members.eligible) skip them. For a public holiday of the whole team choose Public holiday and All technicians.")}
        {live.otherZone && ` ${t("The days are Kuala Lumpur days.")}`}
      </p>
    </Modal>
  );
}
