"use client";
const OPEN_SCHEDULE = "Open schedule";

import { useRef, useState } from "react";
import { Badge, Banner, Btn, Card, cx, EmptyState, Field, Input, LinkButton, Modal, Page, Select, Tabs, TextLink, UtilizationBar, ScrollX } from "@ac/web/components/ui";
import { useLocale, useT } from "@ac/web/components/I18n";
import { useAction } from "@ac/web/lib/useAction";
import { useUrlPatch } from "@ac/web/lib/useUrlPatch";
import { useUrlTab } from "@ac/web/lib/useUrlTab";
import { clearEffect, clearError, dayRange, UNAVAILABILITY_TYPES, unavailabilityConflicts, unavailabilityErrors, unavailabilityRefusal, type UnavailabilityForm, type UnavailabilityRow, type WeekCell } from "@ac/web/lib/partnerTeam";
import { clearUnavailability, setUnavailability } from "../actions";
import { Certifications } from "./certifications";
import type { TeamLive } from "../_lib/load";
import { Icon } from "@ac/web/components/Icon";
import { oneOf } from "@ac/web/lib/oneOf";
import { byCount } from "@ac/web/lib/i18n";

type Live = Extract<TeamLive, { notFound: false }>;
const DAY_TONE = new Map([["crit", "text-crit"], ["warn", "text-warn"]]);
const cellTone: Record<WeekCell["kind"], string> = {
  busy: "bg-primary text-white", free: "border border-line bg-surface text-muted-foreground", off: "bg-warn-soft text-warn", none: "bg-surface2 text-muted-foreground",
};

/** Team & capacity (FR-P06, Figma Contractor 04-1…04-5, 04-8) from the Core API: the URL's date, qualification and
activeOnly, the technicians with the chosen day, the team's week, the qualification grants and unavailable days —
added in a dialog, listed while upcoming and removed from a chosen day on (IR612).
Texts in the user's display language; every date and time comes formatted from the loader (IR276). */
export function TeamView({ live }: Readonly<{ live: TeamLive }>) {
  const t = useT();
  const [tab, setTab] = useUrlTab<"members" | "certs">({ members: "overview", certs: "certifications" }, "members");
  const tabs = <div><Tabs value={tab} onChange={setTab} tabs={[{ id: "members", label: t("Members") }, { id: "certs", label: t("Certifications") }]} /></div>;
  if (live.notFound) {
    return (
      <Page narrow>
        <div className="flex justify-center">{tabs}</div>
        <Card title={t("Roster not found")}>
          <p className="text-[13px] text-muted-foreground">{t("You can only see {company}’s own technicians. Changing the company ID in the URL does not reveal another company’s roster.", { company: live.company })}</p>
          <div className="mt-3"><LinkButton href="/partner/team" variant="primary" size="sm">{t("Back to my team")}</LinkButton></div>
        </Card>
      </Page>
    );
  }
  return <Team live={live} tab={tab} tabs={tabs} />;
}

function Team({ live, tab, tabs }: Readonly<{ live: Live; tab: "members" | "certs"; tabs: React.ReactNode }>) {
  const t = useT();
  const patch = useUrlPatch();
  const [open, setOpen] = useState(false);
  // the remove dialog stays mounted while it closes, so its focus handling runs (n renews its state for each opening)
  const [removing, setRemoving] = useState<{ row: UnavailabilityRow; open: boolean; n: number } | null>(null);
  if (live.total === 0) {
    return <Page>{tabs}<EmptyState title={t("No technicians")}>{t("No technicians in your company yet. Add them in Users.")}</EmptyState></Page>;
  }
  if (tab === "certs") return <Page>{tabs}<Certifications live={live} /></Page>;
  const q = live.query;
  const isNone = live.rows.length === 0;
  return (
    <Page>
      {tabs}
      <div className="flex flex-wrap items-center gap-2">
        <Input type="date" aria-label={t("Date")} className="w-auto" value={q.date} onChange={(e) => { if (e.target.value) patch({ date: e.target.value === q.today ? null : e.target.value }); }} />
        <Select aria-label={t("Qualification")} className="w-auto" value={q.qualification ?? ""} onChange={(e) => { patch({ qualification: e.target.value || null }); }}>
          <option value="">{t("All qualifications")}</option>
          {live.qualifications.map((x) => <option key={x.code} value={x.code}>{x.text}</option>)}
        </Select>
        <Btn size="sm" aria-pressed={q.activeOnly} className={cx(q.activeOnly && "border-primary bg-primary-soft text-primary")} onClick={() => { patch({ activeOnly: q.activeOnly ? "false" : null }); }}>
          {q.activeOnly ? <><Icon name="check" />{t("Active only")}</> : t("Include expired")}
        </Btn>
      </div>
      <Card title={`${t("Technicians — {company}", { company: live.company })}${q.activeOnly ? ` · ${t("{n} active", { n: live.activeCount })}` : ""}`}
        action={<div className="flex flex-wrap gap-2"><LinkButton href="/partner/schedule" size="sm">{t(OPEN_SCHEDULE)} <Icon name="arrowRight" /></LinkButton><Btn size="sm" onClick={() => { setOpen(true); }} disabled={live.technicians.length === 0}><Icon name="add" /> {t("Unavailable days")}</Btn></div>}>
        {isNone ? (
          <div className="py-6 text-center">
            <b className="text-[13px]">{t("No {company} technicians hold this qualification", { company: live.company })}</b>
            <p className="mt-1 text-xs text-muted-foreground">{t("Qualifications are granted by HQ: upload a renewed certificate under Certifications. Accounts are managed in Users.")}</p>
            <Btn size="sm" className="mt-3" onClick={() => { patch({ qualification: null }); }}>{t("Clear filter")}</Btn>
          </div>
        ) : (
          <ul className="divide-y divide-line">
            {live.rows.map((r) => (
              <li key={r.id} className="flex flex-wrap items-center gap-3 py-3">
                <span aria-hidden className={cx("grid h-9 w-9 shrink-0 place-items-center rounded-xl bg-primary-soft text-primary", !r.active && "opacity-50")}><Icon name="account" /></span>
                <div className="min-w-[200px] flex-1"><b className={cx("block text-[13px]", !r.active && "text-muted-foreground")}>{r.name}</b><span className="block text-[11px] text-muted-foreground">{r.sub}</span></div>
                {r.active && (
                  <div className="w-44">
                    <div className="mb-1 flex justify-between gap-2 text-[11px]"><span className="text-muted-foreground">{live.week.short}</span><b>{r.week.text}</b></div>
                    <UtilizationBar pct={r.week.pct} label={`${r.name}: ${r.week.text}`} />
                  </div>
                )}
                <div className="min-w-[180px] text-right"><span className={cx("block text-xs", DAY_TONE.get(r.day.tone ?? "") ?? "text-ink")}>{r.day.main}</span><span className="block text-[11px] text-muted-foreground">{r.day.sub}</span></div>
                <div className="w-16 text-right"><b className="block text-lg">{r.util}</b><span className="block text-[10px] text-muted-foreground">{t("utilization")}</span></div>
              </li>
            ))}
          </ul>
        )}
      </Card>
      <p className="text-[11px] text-muted-foreground">{t("Utilization = assigned hours ÷ configured available hours (4 h / 8 h = 50%). Undefined available hours show “—”, not 0%. Accounts are managed in Users.")}</p>
      {!isNone && (
        <div className="split">
          <WeekCard live={live} />
          <Card title={t("Qualifications")}>
            <ul className="divide-y divide-line">
              {live.grants.map((g) => (
                <li key={g.key} className="flex items-center justify-between gap-2 py-2">
                  <span className="min-w-0"><b className="block text-[13px]">{g.name}</b><span className="block text-[11px] text-muted-foreground">{g.sub}</span></span>
                  <Badge tone={g.badge.tone}>{g.badge.text}</Badge>
                </li>
              ))}
            </ul>
            <p className="mt-2 text-[11px] text-muted-foreground">
              {live.hidden > 0 && `${t(live.hidden === 1 ? "1 expired member hidden (Active only)." : "{n} expired members hidden (Active only).", { n: live.hidden })} `}
              {live.expired.length > 0 ? t("Expired memberships ({names}) are listed but cannot be selected for assignment. A new access end is set in Users.", { names: live.expired.join(", ") }) : t("Qualifications are granted by HQ from your technicians’ certificates; accounts are managed in Users.")}
            </p>
          </Card>
        </div>
      )}
      <Card title={t("Upcoming unavailable days")}>
        <div id="unavailable-days" tabIndex={-1} role="region" aria-label={t("Upcoming unavailable days")} className="rounded-lg outline-none focus-visible:ring-2 focus-visible:ring-ring/50">
        {live.unavailable.length === 0 ? <p className="py-2 text-[13px] text-muted-foreground">{t("No upcoming unavailable days. Add them with Unavailable days above.")}</p> : (
          <ul className="divide-y divide-line">
            {live.unavailable.map((u) => (
              <li key={u.id} className="flex flex-wrap items-center gap-3 py-2.5">
                <div className="min-w-[220px] flex-1"><b className="block break-words text-[13px]">{u.who}</b><span className="block text-[11px] text-muted-foreground">{u.note ? `${u.type} · ${u.note}` : u.type}</span></div>
                <span className="min-w-[160px] text-[13px]">{u.days}</span>
                <Btn size="sm" variant="ghost" aria-label={t("Remove unavailable days of {who}, {days}", { who: u.who, days: u.days })} onClick={() => { setRemoving((r) => ({ row: u, open: true, n: (r?.n ?? 0) + 1 })); }}><Icon name="delete" />{t("Remove")}</Btn>
              </li>
            ))}
          </ul>
        )}
        </div>
        <p className="mt-2 text-[11px] text-muted-foreground">{t("Days that have passed stay recorded. Removing days makes the technician available for visits on them again.")}</p>
      </Card>
      {open && <UnavailableDays live={live} onClose={() => { setOpen(false); }} />}
      {removing && <RemoveDays key={removing.n} row={removing.row} open={removing.open} onClose={() => { setRemoving((r) => r && { ...r, open: false }); }} />}
    </Page>
  );
}

/** Remove unavailable days (IR612): from the row's first day — or today, once it has started — all of it goes; a later
day keeps the days before it. The effect is said before the confirm; members.clearUnavailability sends the row's
version, so a row someone changed meanwhile is refused and the list reloads. */
function RemoveDays({ row, open, onClose }: Readonly<{ row: UnavailabilityRow; open: boolean; onClose: () => void }>) {
  const t = useT();
  const locale = useLocale();
  const [pending, run] = useAction();
  const [from, setFrom] = useState(row.first);
  const [tried, setTried] = useState(false);
  const gone = useRef(false); // the whole row went, and its Remove button with it: focus goes to the list
  const error = tried ? clearError(row, from, locale, t) : null;
  const remove = () => {
    setTried(true);
    if (clearError(row, from, locale, t)) return;
    run(() => clearUnavailability(row.id, row.version, from),
      (v) => (v.kept ? t("Unavailable days shortened — {who} can take visits again from {day}.", { who: row.who, day: dayRange(from, from, locale) }) : t("Unavailable days removed — {who} can take visits on those days again.", { who: row.who })),
      (v) => { gone.current = !v.kept; onClose(); },
      (f) => { if (f.code !== "VALIDATION") onClose(); });
  };
  return (
    <Modal open={open} onClose={onClose} title={t("Remove unavailable days")} finalFocus={() => (gone.current ? document.querySelector("#unavailable-days") : true)} footer={<>
      <Btn onClick={onClose}>{t("Cancel")}</Btn>
      <Btn variant="danger" disabled={pending} onClick={remove}>{t("Remove")}</Btn>
    </>}>
      <p className="text-[13px]"><b className="break-words">{row.who}</b> · {row.type} · {row.days}</p>
      <Field label={t("Remove from")} error={error ?? undefined}>
        <Input type="date" value={from} min={row.first} max={row.to} onChange={(e) => { setFrom(e.target.value); setTried(false); }} />
      </Field>
      {!clearError(row, from, locale, t) && <Banner tone="warn">{clearEffect(row, from, locale, t)}</Banner>}
    </Modal>
  );
}

function WeekCard({ live }: Readonly<{ live: Live }>) {
  const t = useT();
  const w = live.week;
  return (
    <Card title={w.title} action={<TextLink href="/partner/schedule">{t(OPEN_SCHEDULE)} <Icon name="arrowRight" /></TextLink>}>
      <ScrollX>
        <table className="w-full min-w-[560px] text-xs">
          <thead><tr><th />{w.days.map((d, k) => <th key={w.dates[k]} className={cx("p-1 text-center font-semibold", k === w.dayIndex ? "text-primary" : "text-muted-foreground")}>{d}</th>)}</tr></thead>
          <tbody>{w.rows.map((r) => (
            <tr key={r.id} className="border-t border-line">
              <td className="py-1.5 pr-2 font-semibold">{r.name}</td>
              {r.cells.map((c, k) => <td key={w.dates[k]} className="p-1"><div className={cx("rounded-lg px-1.5 py-2 text-center font-semibold", cellTone[c.kind], k === w.dayIndex && "ring-2 ring-primary/40")}>{c.text}</div></td>)}
            </tr>
          ))}</tbody>
        </table>
      </ScrollX>
      <dl className="mt-3 flex flex-wrap gap-6 text-xs">
        {([[t("Assigned"), w.stats.assigned], [t("Available"), w.stats.available], [t("Team utilization"), w.stats.utilization], [w.freeLabel, w.stats.free]] as const).map(([k, v]) => {
          return (
            <div key={k}><dt className="text-[11px] text-muted-foreground">{k}</dt><dd className="text-[15px] font-bold">{v}</dd></div>
          );
        })}
      </dl>
      <p className="mt-2 text-[11px] text-muted-foreground">
        {t("Sat/Sun have no configured available hours, so utilization shows “—”.")}
        {live.otherZone && ` ${t("Days and hours are in Kuala Lumpur time (Asia/Kuala_Lumpur).")}`}
      </p>
    </Card>
  );
}

/** Add unavailable days (Figma 04-8): technician or the whole company, Kuala Lumpur days, type and note; the confirmed
assignments in the days are listed before saving and kept by the save (DD-P06). */
function UnavailableDays({ live, onClose }: Readonly<{ live: Live; onClose: () => void }>) {
  const t = useT();
  const [pending, run] = useAction();
  const [form, setForm] = useState<UnavailabilityForm>({ membershipId: live.technicians[0]?.id ?? "", from: live.query.date, to: live.query.date, type: "annual_leave", note: "" });
  const [tried, setTried] = useState(false);
  const [server, setServer] = useState<Record<string, string>>({});
  const [refused, setRefused] = useState<string | null>(null);
  const local = tried ? unavailabilityErrors(form, t) : {};
  const fieldError = (k: "from" | "to" | "type" | "note") => (local as Record<string, string | undefined>)[k] ?? server[k];
  const set = (p: Partial<UnavailabilityForm>) => { setForm({ ...form, ...p }); setServer({}); setRefused(null); };
  const conflicts = unavailabilityConflicts(live.jobs, form);
  const names = new Map(live.technicians.map((x) => [x.id, x.name]));
  const list = conflicts.map((c) => `${c.short} · ${form.membershipId ? "" : `${names.get(c.technicianId) ?? ""} · `}${c.text}`).join("; ");
  const save = () => {
    setTried(true);
    if (Object.keys(unavailabilityErrors(form, t)).length > 0) return;
    run(() => setUnavailability({ membershipId: form.membershipId || null, from: form.from, to: form.to, type: form.type, note: form.note.trim() || undefined }),
      (v) => {
        return byCount(v.conflicts, {
          none: t("Unavailable days saved — the available hours of those days are 0."),
          one: t("Unavailable days saved — 1 confirmed assignment overlaps them; reassign it in Schedule & assignments."),
          many: t("Unavailable days saved — {n} confirmed assignments overlap them; reassign them in Schedule & assignments.", { n: v.conflicts }),
        });
      },
      () => { onClose(); },
      (f) => { const r = unavailabilityRefusal(f, t); setServer(r.fields); setRefused(r.text); });
  };
  return (
    <Modal open onClose={onClose} title={t("Add unavailable days")} footer={<>
      <Btn onClick={onClose}>{t("Cancel")}</Btn>
      <LinkButton href={conflicts[0] ? `/partner/schedule?jobId=${conflicts[0].jobId}` : "/partner/schedule"}>{t(OPEN_SCHEDULE)} <Icon name="arrowRight" /></LinkButton>
      <Btn variant="primary" disabled={pending} onClick={save}>{t("Save")}</Btn>
    </>}>
      <Field label={t("Technician")}>
        <Select value={form.membershipId} onChange={(e) => { set({ membershipId: e.target.value }); }}>
          {live.technicians.map((x) => <option key={x.id} value={x.id}>{x.name}</option>)}
          <option value="">{t("All technicians (public holiday)")}</option>
        </Select>
      </Field>
      <div className="grid-fluid" style={{ ["--min" as string]: "150px" }}>
        <Field label={t("From")} error={fieldError("from")}><Input type="date" value={form.from} onChange={(e) => { set({ from: e.target.value }); }} /></Field>
        <Field label={t("To")} error={fieldError("to")}><Input type="date" value={form.to} min={form.from} onChange={(e) => { set({ to: e.target.value }); }} /></Field>
        <Field label={t("Type")} error={fieldError("type")}>
          <Select value={form.type} onChange={(e) => { set({ type: oneOf(UNAVAILABILITY_TYPES.map((x) => x.id), e.target.value, form.type) }); }}>{live.types.map((x) => <option key={x.id} value={x.id}>{x.text}</option>)}</Select>
        </Field>
      </div>
      <Field label={t("Note (optional)")} error={fieldError("note")}><Input value={form.note} maxLength={500} onChange={(e) => { set({ note: e.target.value }); }} /></Field>
      {conflicts.length > 0 && <Banner tone="warn">{t(conflicts.length === 1 ? "Conflicts with 1 confirmed assignment: {list}. Saving keeps the assignment — reassign it in Schedule & assignments." : "Conflicts with {n} confirmed assignments: {list}. Saving keeps the assignments — reassign them in Schedule & assignments.", { n: conflicts.length, list })}</Banner>}
      {refused && <Banner tone="crit">{refused}</Banner>}
      <p className="text-[11px] text-muted-foreground">
        {t("Unavailable days set the technician’s available hours to 0 for those dates, so utilization and the lists of who can take a visit skip them. For a public holiday of the whole team choose Public holiday and All technicians.")}
        {live.otherZone && ` ${t("The days are Kuala Lumpur days.")}`}
      </p>
    </Modal>
  );
}
