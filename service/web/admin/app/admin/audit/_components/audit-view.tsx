"use client";

import { useRef, useState, type ReactNode } from "react";
import { Badge, Banner, Btn, Card, Choice, DataTable, EmptyState, Field, Input, ListRow, Page, Select, SummaryList, Tabs, TextLink, Timeline } from "@ac/web/components/ui";
import { useI18n } from "@ac/web/components/I18n";
import { useUrlPatch } from "@ac/web/lib/useUrlPatch";
import { useUrlTab } from "@ac/web/lib/useUrlTab";
import { callOp, OpError } from "@ac/web/lib/ops";
import { historyRow, type ApiCommand } from "@ac/web/lib/units";
import { periodError, resultTone, resultWord, type AuditDeviceEvent, type AuditFilters, type AuditRow, type Related, type RelatedRef } from "@ac/web/lib/audit";

const seed: AuditRow[] = [
  { id: "1", op: "restriction.override", target: "restriction · restriction-a41", targetKind: "restriction", targetId: "restriction-a41", versions: "version 3 → 4", actor: "hq-operator", actorId: "hq-operator", role: "admin", at: "09-22 09:14", occurred: "2026-09-22 09:14 (Asia/Kuala_Lumpur)", corr: "corr-7f21a9", res: "Success", reason: "Customer requested a short exception while the invoice is under review.",
    changes: [{ field: "exception.until", before: "null", after: "2026-09-29 00:00", changed: true }, { field: "exception.reason", before: "null", after: "Invoice under review", changed: true }, { field: "contactPhone", before: "***masked***", after: "***masked***", changed: false }, { field: "state", before: "applied", after: "applied", changed: false }] },
  { id: "2", op: "access.grant", target: "membership · membership-b09", targetKind: "membership", targetId: "membership-b09", versions: null, actor: "contractor-lead-b", actorId: "contractor-lead-b", role: "contractor", at: "09-22 08:51", occurred: "2026-09-22 08:51 (Asia/Kuala_Lumpur)", corr: "corr-2c88e0", res: "Denied", reason: null, changes: [] },
  { id: "3", op: "devices.calibrate", target: "device · device-online-rto", targetKind: "device", targetId: "device-online-rto", versions: "version 4 → 5", actor: "tech-ali", actorId: "tech-ali", role: "technician", at: "09-21 16:30", occurred: "2026-09-21 16:30 (Asia/Kuala_Lumpur)", corr: "corr-51d0aa", res: "Failed", reason: null, changes: [] },
  { id: "4", op: "commands.create", target: "command · cmd-3310", targetKind: "command", targetId: "cmd-3310", versions: "version 1", actor: "hq-operator", actorId: "hq-operator", role: "admin", at: "09-20 10:14", occurred: "2026-09-20 10:14 (Asia/Kuala_Lumpur)", corr: "corr-115e6b", res: "Pending", reason: null, changes: [] },
  { id: "5", op: "offsets.simulate · retire", target: "offset · offset-0231", targetKind: "offset_record", targetId: "offset-0231", versions: null, actor: "hq-operator", actorId: "hq-operator", role: "admin", at: "09-12 14:20", occurred: "2026-09-12 14:20 (Asia/Kuala_Lumpur)", corr: "corr-9a10f4", res: "Success", reason: null, changes: [] },
];
const seedRelated: Related = {
  restriction: { id: "restriction-a41", via: "target", note: "applied · exception until 09-29 — as this entry recorded it" }, command: { id: "cmd-3302", via: "correlation", note: "same correlation ID" }, job: null, unit: null,
};
const seedEvents: AuditDeviceEvent[] = [
  { id: "ev-42", title: "Tamper · removal suspected", type: "tamper", tone: "warn", meta: "tamper_signal · seq 42 · 09-22 02:41", state: "open · unacknowledged", badge: { text: "Not restored", tone: "crit" },
    facts: [["Event type", "tamper"], ["Evidence", "tamper_signal — Cover opened while powered"], ["Sequence", "42 (previous 41 — no gap)"], ["Unit at the time", "unit-online-rto"], ["Occurred", "2026-09-22 02:41 (Asia/Kuala_Lumpur)"], ["Restored at", "— not yet"], ["Recovery", "open · unacknowledged"]],
    alerts: [{ id: "alert-tamper-17", text: "tamper · open", href: "/admin/alerts" }], notes: [{ text: "Customer confirms nobody touched the unit. Dispatching tech-ali.", by: "hq-operator · 09-22 08:03" }] },
  { id: "ev-41", title: "Restored", type: "restored", tone: "ok", meta: "heartbeat · seq 41 · 09-21 07:05", state: "recovers communication_lost 09-21 06:27", badge: null,
    facts: [["Event type", "restored"], ["Evidence", "heartbeat — Connection restored"], ["Sequence", "41 (previous 40 — no gap)"], ["Unit at the time", "unit-online-rto"], ["Occurred", "2026-09-21 07:05 (Asia/Kuala_Lumpur)"], ["Recovery", "recovers communication_lost 09-21 06:27"]], alerts: [], notes: [] },
];
const demoFilters: AuditFilters = { from: "2026-09-01", to: "2026-09-30", actorId: "", targetKind: "", targetId: "", correlationId: "", result: "All", limit: 25 };

/** What the Server Component read for the URL (SCR-A16): the log page with its filters and options, the selected entry
 * with its correlation trace and related records, and on the device tab the devices and the chosen device's events. */
export type AuditLive = {
  tab: "log" | "devices"; rows: AuditRow[]; total: number; filters: AuditFilters; searched: boolean; actors: { id: string; name: string }[]; kinds: string[];
  selected: string | null; entryMissing: boolean; trace: { id: string; time: string; title: string; detail: string }[]; related: Related | null;
  device?: { options: { id: string; label: string }[]; deviceId: string | null; missing: boolean; label: string | null; total: number; events: AuditDeviceEvent[]; selected: string | null };
};

/** The audit explorer (FR-A16, Figma Admin 337:2 / 338:x / 340:2). `live` comes from the Server Component in API mode,
 * where every filter and selection is in the URL and the server reads again; the demo filters its seed rows locally.
 * Read only: nothing here creates, edits, deletes or exports an entry. Texts in the display language; the period's days
 * are the display time zone's (IR304, IR320). */
export function AuditView({ live }: { live?: AuditLive }) {
  const i = useI18n(), { t } = i, zone = i.display.timeZone;
  const patch = useUrlPatch();
  const [urlTab, setUrlTab] = useUrlTab<"log" | "devices">({ log: "log", devices: "devices" }, "log");
  const tab = live ? live.tab : urlTab; // API mode: the server read the tab from the URL (and loaded its data)
  const setTab = (id: "log" | "devices") => (live ? patch({ tab: id === "devices" ? "devices" : null }) : setUrlTab(id));
  const initial = live?.filters ?? demoFilters;
  const [f, setF] = useState(initial);
  // the filters restart from each new server result: adjust state during render, not in an effect
  const [source, setSource] = useState(live?.filters);
  if (live && source !== live.filters) {
    setSource(live.filters);
    setF(live.filters);
  }
  const [shown, setShown] = useState(5);
  const [selId, setSelId] = useState<string | null>(null);
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const periodErr = periodError(f, t);
  const all = live ? live.rows : seed;
  const rows = live ? all : all.filter((l) => (f.result === "All" || l.res === f.result) && (!f.correlationId || l.corr.includes(f.correlationId)) && (!f.actorId || l.actorId === f.actorId)
    && (!f.targetKind || l.targetKind === f.targetKind) && (!f.targetId || l.targetId === f.targetId)).slice(0, shown);
  const sel = live ? rows.find((r) => r.id === live.selected) ?? null : rows.find((r) => r.id === selId) ?? rows[0] ?? null;
  const total = live ? live.total : rows.length;
  const more = live ? rows.length < live.total && live.filters.limit < 100 : rows.length < seed.length && shown < seed.length;

  // a new search starts from its newest entry; "Filter by …" of the open entry keeps it open (it matches)
  const search = (next: Partial<AuditFilters>, keepEntry = false) => {
    const merged = { ...f, ...next };
    setF(merged);
    if (!live) return;
    patch({ ...Object.fromEntries(Object.entries(next).map(([k, v]) => [k, k === "result" && v === "All" ? null : String(v ?? "")])), limit: null, ...(keepEntry ? {} : { entryId: null, eventId: null }) });
  };
  const typeCorr = (v: string) => {
    setF((x) => ({ ...x, correlationId: v }));
    if (!live) return;
    if (timer.current) clearTimeout(timer.current);
    timer.current = setTimeout(() => patch({ correlationId: v, limit: null, entryId: null }), 300);
  };
  const changeDate = (which: "from" | "to", v: string) => {
    const next = { ...f, [which]: v };
    setF(next);
    if (live && !periodError(next)) patch({ from: next.from, to: next.to, limit: null, entryId: null, eventId: null });
  };
  const reset = () => {
    if (live) {
      patch({ correlationId: null, result: null, from: null, to: null, actorId: null, targetKind: null, targetId: null, limit: null, entryId: null, eventId: null });
      return;
    }
    setF(demoFilters);
  };
  const open = (id: string) => (live ? patch({ entryId: id }) : setSelId(id));
  const actors = live ? live.actors : [...new Map(seed.map((r) => [r.actorId, r.actor]))].map(([id, name]) => ({ id, name }));
  const kinds = live ? live.kinds : [...new Set(seed.map((r) => r.targetKind))].sort();
  const related = live ? live.related : sel?.id === "1" ? seedRelated : { restriction: null, command: null, job: null, unit: null };
  const trace = live ? live.trace : [];
  const corr = live ? live.filters.correlationId : f.correlationId; // the searched correlation ID (typing waits 300 ms)
  const device = live ? live.device : { options: [{ id: "device-online-rto", label: "device-online-rto · unit-online-rto" }], deviceId: "device-online-rto", missing: false, label: "device-online-rto · unit-online-rto", total: seedEvents.length, events: seedEvents, selected: null };

  return (
    <Page>
      <Banner>
        <b>{t("Read-only · entries are appended only by business events")}</b>
        <div className="text-xs">{t(live ? "No create, edit, or delete here — the log is append-only. Secrets and contacts in before/after are masked. Filters are kept in the URL; reasons and values never are."
          : "No create, edit, or delete here. Secrets and contacts in before/after are masked. This browser demo does not guarantee tamper-proof storage.")}</div>
      </Banner>
      <Tabs value={tab} onChange={setTab} tabs={[{ id: "log", label: t("Audit log"), count: total }, { id: "devices", label: t("Device events"), ...(device?.deviceId ? { count: device.total } : {}) }]} />
      {tab === "log" ? (
        <>
          <Card title={t("Filter")}>
            <div className="grid-fluid" style={{ ["--min" as string]: "170px" }}>
              <Field label={t("From")} error={periodErr} hint={periodErr ? undefined : t("Days in {zone}", { zone })}><Input type="date" value={f.from} onChange={(e) => changeDate("from", e.target.value)} /></Field>
              <Field label={t("To")}><Input type="date" value={f.to} onChange={(e) => changeDate("to", e.target.value)} /></Field>
              <Field label={t("Actor")}><Select value={f.actorId} onChange={(e) => search({ actorId: e.target.value })}><option value="">{t("Actor: All")}</option>{actors.map((a) => <option key={a.id} value={a.id}>{a.name}</option>)}</Select></Field>
              <Field label={t("Target")}><Select value={f.targetKind} onChange={(e) => search({ targetKind: e.target.value, targetId: "" })}><option value="">{t("Target: All")}</option>{kinds.map((k) => <option key={k} value={k}>{k}</option>)}</Select></Field>
              <Field label={t("Correlation ID")}><Input value={f.correlationId} onChange={(e) => typeCorr(e.target.value)} placeholder="corr-…" /></Field>
            </div>
            {f.targetId && <div className="mt-2"><span className="inline-flex items-center gap-1 rounded-full bg-surface2 px-2.5 py-1 text-xs">{t("Target ID")} · <span className="font-mono">{f.targetKind ? `${f.targetKind} · ` : ""}{f.targetId}</span><button aria-label={t("Clear the target ID")} onClick={() => search({ targetId: "" })}>✕</button></span></div>}
            <div className="mt-3 flex flex-wrap items-center justify-between gap-2">
              <Choice value={f.result} onChange={(v) => search({ result: v })} options={(["All", "Success", "Denied", "Failed", "Pending"] as const).map((k) => ({ id: k, label: k === "All" ? t("All") : resultWord(k, t) }))} />
              <span className="flex flex-wrap items-center gap-3">
                {(!live || live.searched) && <span className="text-xs text-muted">{corr ? t("{n} entries with this correlation ID", { n: total }) : t("{n} entries", { n: total })}</span>}
                <Btn size="sm" onClick={reset}>{t("Reset")}</Btn>
              </span>
            </div>
          </Card>
          <div className="split-rev">
            <Card title={corr ? t("Correlation {id}", { id: corr }) : t("Audit log · newest first")} className="self-start">
              {rows.length === 0 ? <NoEntries live={live} periodErr={periodErr} corr={corr} /> : (
                <div className="flex flex-col gap-2">{rows.map((l) => (
                  <ListRow key={l.id} selected={sel?.id === l.id} onClick={() => open(l.id)}>
                    <div className="min-w-0 flex-1"><div className="flex items-center justify-between gap-2"><b className="truncate text-[13px]">{l.op}</b><Badge tone={resultTone(l.res)}>{resultWord(l.res, t)}</Badge></div><div className="truncate text-[11px] text-muted">{l.target}</div><div className="truncate text-[11px] text-muted">{l.actor} ({l.role}) · {l.at} · {l.corr}</div></div>
                  </ListRow>
                ))}</div>
              )}
              <div className="mt-3 flex items-center justify-between gap-2"><p className="text-[11px] text-muted">{t("{n} of {total} · 25 per page", { n: rows.length, total })}</p>{more && <Btn size="sm" onClick={() => (live ? patch({ limit: String(Math.min(100, live.filters.limit + 25)) }) : setShown((s) => s + 25))}>{t("Load more")}</Btn>}</div>
            </Card>
            {live?.entryMissing ? <EmptyState title={t("That entry is not in these results")}>{t("Change the filters or load more to open it.")}</EmptyState>
              : sel && (
                <Card title={<span className="flex flex-wrap items-center gap-2">{sel.op}<Badge tone={resultTone(sel.res)}>{resultWord(sel.res, t)}</Badge></span>} sub={`${sel.target}${sel.versions ? ` · ${sel.versions}` : ""}`} className="self-start">
                  <SummaryList items={[
                    [t("Actor"), <span key="a" className="flex flex-wrap items-center justify-between gap-2"><span>{sel.actor}{sel.actorId !== sel.actor && <span className="ml-2 font-mono text-xs text-muted">{sel.actorId}</span>}</span><FilterLink onClick={() => search({ actorId: sel.actorId }, true)}>{t("Filter by actor →")}</FilterLink></span>],
                    [t("Role at the time"), t("{role} — kept as recorded, not rewritten by the current membership", { role: sel.role })],
                    [t("Occurred"), sel.occurred],
                    [t("Correlation ID"), <span key="c" className="flex flex-wrap items-center justify-between gap-2"><span className="font-mono text-xs">{sel.corr}</span><FilterLink onClick={() => search({ correlationId: sel.corr }, true)}>{t("Filter by ID →")}</FilterLink></span>],
                    [t("Target"), <span key="t" className="flex flex-wrap items-center justify-between gap-2"><span className="font-mono text-xs">{sel.target}</span><FilterLink onClick={() => search({ targetKind: sel.targetKind, targetId: sel.targetId }, true)}>{t("Filter by target →")}</FilterLink></span>],
                    [t("Reason"), sel.reason ?? "—"],
                  ]} />
                  {trace.length > 0 && (
                    <>
                      <h3 className="mt-4 mb-2 text-[13px] font-bold">{t("Correlation trace")}</h3>
                      <Timeline items={trace.map((x) => ({ time: x.time, title: x.title, detail: x.detail, tone: x.id === sel.id ? "primary" : undefined }))} />
                      <p className="mt-2 text-[11px] text-muted">{t("Pending is shown on its own; the final result is a new entry with the same correlation ID. Earlier entries are never edited.")}</p>
                    </>
                  )}
                  <h3 className="mt-4 mb-2 text-[13px] font-bold">{t("Before / after (masked)")}</h3>
                  {sel.changes.length === 0 ? <p className="text-xs text-muted">{t("No field values were recorded for this entry.")}</p> : <DataTable rows={sel.changes} rowKey={(r) => r.field} cols={[{ key: "f", label: t("Field"), render: (r) => <span className="font-mono text-xs">{r.field}</span> }, { key: "a", label: t("Before"), render: (r) => r.before }, { key: "b", label: t("After"), render: (r) => <span className={r.changed ? "font-bold text-warn" : ""}>{r.after}</span> }]} />}
                  <p className="mt-2 text-[11px] text-muted">{t("Changed fields are highlighted. Secrets and contact values stay masked in both columns; the original values are never shown.")}</p>
                  {related && <RelatedRecords related={related} demo={!live} />}
                </Card>
              )}
          </div>
        </>
      ) : device && (
        <DeviceEvents device={device} f={f} periodErr={periodErr} zone={zone} live={!!live} onDate={changeDate} onDevice={(id) => (live ? patch({ deviceId: id || null, eventId: null }) : undefined)} onEvent={(id) => live && patch({ eventId: id })} onReset={reset} />
      )}
    </Page>
  );
}

function FilterLink({ onClick, children }: { onClick: () => void; children: ReactNode }) {
  return <button className="text-xs font-semibold text-primary hover:underline" onClick={onClick}>{children}</button>;
}

/** Why the list is empty: nothing was searched (an invalid period, Figma Admin 338:602), an unknown or out-of-scope
 * correlation ID — they look the same (338:332) — or no match. */
function NoEntries({ live, periodErr, corr }: { live?: AuditLive; periodErr?: string; corr: string }) {
  const { t } = useI18n();
  if (periodErr || (live && !live.searched)) return <EmptyState title={t("Nothing was searched")}>{t("The period must start before it ends and cover at most 366 days. Earlier results are cleared so they are not mistaken for this search.")}</EmptyState>;
  if (corr) return <EmptyState title={t("No entries with this correlation ID")}>{t("This looks the same for an ID that does not exist and for one outside your managed scope. Clear the correlation ID or widen the period.")}</EmptyState>;
  return <EmptyState title={t("No audit entries match these filters")}>{t("No audit entries match this period, actor, target, correlation ID or result.")}</EmptyState>;
}

/** The restriction, command, job and unit of the entry and of its correlation (DD-A16 step 1). Each link opens the
 * record's own screen, which checks its own permission; the command is read (commands.get) only when asked (SCR-A16). */
function RelatedRecords({ related, demo }: { related: Related; demo: boolean }) {
  const { t } = useI18n();
  const line = (r: RelatedRef) => <span><span className="font-mono text-xs">{r.id.slice(0, 8)}</span> · {r.note}</span>;
  const row = (label: string, r: RelatedRef | null, none: string, link?: (r: RelatedRef) => ReactNode): [string, ReactNode] =>
    [label, r ? <span key={label} className="flex flex-wrap items-center justify-between gap-2">{line(r)}{link?.(r)}</span> : <span key={label} className="text-muted">{none}</span>];
  return (
    <>
      <h3 className="mt-4 mb-2 text-[13px] font-bold">{t("Related records")}</h3>
      <SummaryList items={[
        row(t("Restriction"), related.restriction, t("No related restriction"), (r) => <TextLink href={`/admin/restrictions?restrictionId=${r.id}`}>{t("Open restriction →")}</TextLink>),
        [t("Command"), related.command ? <CommandLine key="cmd" r={related.command} demo={demo} line={line(related.command)} /> : <span key="cmd" className="text-muted">{t("No related command")}</span>],
        row(t("Job"), related.job, t("No related job"), (r) => <TextLink href={`/admin/jobs?jobId=${r.id}`}>{t("Open job →")}</TextLink>),
        ...(related.unit ? [row(t("Unit"), related.unit, "", (r) => <TextLink href={`/admin/units?unitId=${r.id}`}>{t("Open unit →")}</TextLink>)] : []),
      ]} />
      <p className="mt-2 rounded-lg bg-primary-soft/50 px-3 py-2 text-[11px] text-muted">{t("Links open the existing detail screens, and each checks your permission there. “Load command” reads the command only when you click it. Audit entries cannot be edited, deleted or exported.")}</p>
    </>
  );
}

type Loaded = { state: "idle" | "loading" } | { state: "done"; text: string; when: string; unitId: string | null } | { state: "error"; text: string };
function CommandLine({ r, demo, line }: { r: RelatedRef; demo: boolean; line: ReactNode }) {
  const { t, display } = useI18n();
  const [c, setC] = useState<Loaded>({ state: "idle" });
  const load = async () => {
    if (demo) return setC({ state: "done", text: t("Set temperature {celsius}°C", { celsius: 24 }), when: "09-20 10:14", unitId: null });
    setC({ state: "loading" });
    try {
      const cmd = await callOp<ApiCommand & { unitId: string }>("commands.get", { id: r.id });
      const h = historyRow(cmd, t, display);
      setC({ state: "done", text: h.text, when: h.when, unitId: cmd.unitId });
    } catch (e) {
      const code = e instanceof OpError ? e.error.code : "";
      setC({ state: "error", text: t(code === "FORBIDDEN" ? "You may not read this command — it needs control.execute, restriction.read, restriction.override or automation.policy.read."
        : code === "NOT_FOUND" ? "This command is outside your scope or no longer exists." : "The command could not be loaded — try again.") });
    }
  };
  return (
    <span className="flex flex-col gap-1">
      <span className="flex flex-wrap items-center justify-between gap-2">{line}{c.state !== "done" && <Btn size="sm" disabled={c.state === "loading"} onClick={load}>{t("Load command")}</Btn>}</span>
      {c.state === "done" && <span className="flex flex-wrap items-center justify-between gap-2 text-xs"><span>{c.text} · {c.when}</span>{c.unitId && <TextLink href={`/admin/units?unitId=${c.unitId}`}>{t("Open unit →")}</TextLink>}</span>}
      {c.state === "error" && <span className="text-xs text-crit">{c.text}</span>}
    </span>
  );
}

/** The device events tab (Figma Admin 340:2): the devices the session may see, and only after one is picked its
 * events in the period, newest first — no fallback to a first or every device (IR33). */
function DeviceEvents({ device, f, periodErr, zone, live, onDate, onDevice, onEvent, onReset }: {
  device: NonNullable<AuditLive["device"]>; f: AuditFilters; periodErr?: string; zone: string; live: boolean;
  onDate: (which: "from" | "to", v: string) => void; onDevice: (id: string) => void; onEvent: (id: string) => void; onReset: () => void;
}) {
  const { t } = useI18n();
  const [pick, setPick] = useState<string | null>(null);
  const ev = device.events.find((e) => e.id === (live ? device.selected : pick)) ?? device.events[0] ?? null;
  return (
    <>
      <Card title={t("Filter")}>
        <div className="grid-fluid" style={{ ["--min" as string]: "200px" }}>
          <Field label={t("Device")}><Select value={device.deviceId ?? ""} onChange={(e) => onDevice(e.target.value)}><option value="">{t("Select a device…")}</option>{device.options.map((o) => <option key={o.id} value={o.id}>{o.label}</option>)}</Select></Field>
          <Field label={t("From")} error={periodErr} hint={periodErr ? undefined : t("Days in {zone}", { zone })}><Input type="date" value={f.from} onChange={(e) => onDate("from", e.target.value)} /></Field>
          <Field label={t("To")}><Input type="date" value={f.to} onChange={(e) => onDate("to", e.target.value)} /></Field>
        </div>
        <div className="mt-3 flex flex-wrap items-center justify-between gap-2">
          <p className="text-[11px] text-muted">{t("The devices you may see. Events load only after you pick a device — there is no “all devices” view.")}</p>
          <span className="flex items-center gap-3">{device.deviceId && <span className="text-xs text-muted">{t("{n} events", { n: device.total })}</span>}<Btn size="sm" onClick={onReset}>{t("Reset")}</Btn></span>
        </div>
      </Card>
      {!device.deviceId ? <EmptyState title={device.missing ? t("That device is not in your list") : t("Pick a device")}>{t(device.missing ? "It does not exist or is outside your scope. Pick one of your devices." : "Choose a device above to see its connection, power and tamper events.")}</EmptyState> : (
        <div className="split-rev">
          <Card title={t("Device events · newest first")} className="self-start">
            {device.events.length === 0 ? <EmptyState title={t("No device events in this period")}>{periodErr ? t("The period must start before it ends and cover at most 366 days.") : t("This device recorded no events in the period.")}</EmptyState> : (
              <div className="flex flex-col gap-2">{device.events.map((e) => (
                <ListRow key={e.id} selected={ev?.id === e.id} onClick={() => (live ? onEvent(e.id) : setPick(e.id))}>
                  <div className="min-w-0 flex-1"><div className="flex items-center justify-between gap-2"><b className="truncate text-[13px]">{e.title}</b><Badge tone={e.tone}>{e.type}</Badge></div><div className="truncate text-[11px] text-muted">{e.meta}</div><div className="truncate text-[11px] text-muted">{e.state}</div></div>
                </ListRow>
              ))}</div>
            )}
          </Card>
          {ev && (
            <Card title={<span className="flex flex-wrap items-center gap-2">{ev.title}{ev.badge && <Badge tone={ev.badge.tone}>{ev.badge.text}</Badge>}</span>} sub={device.label ?? undefined} className="self-start">
              <SummaryList items={ev.facts} />
              <h3 className="mt-4 mb-2 text-[13px] font-bold">{t("Linked alerts and audit")}</h3>
              <SummaryList items={[
                ...ev.alerts.map((a): [string, ReactNode] => [t("Alert"), <span key={a.id} className="flex flex-wrap items-center justify-between gap-2"><span>{a.text}</span><TextLink href={a.href}>{t("Open alert →")}</TextLink></span>]),
                [t("Audit"), <span key="audit" className="flex flex-wrap items-center justify-between gap-2"><span className="font-mono text-xs">device_event · {ev.id.slice(0, 8)}</span><TextLink href={`/admin/audit?targetKind=device_event&targetId=${ev.id}`}>{t("Filter log →")}</TextLink></span>],
              ]} />
              <h3 className="mt-4 mb-2 text-[13px] font-bold">{t("Response notes (read-only here)")}</h3>
              {ev.notes.length === 0 ? <p className="text-xs text-muted">{t("No response notes.")}</p> : <ul className="flex flex-col gap-2">{ev.notes.map((n, k) => <li key={k} className="rounded-lg border border-line px-3 py-2 text-[13px]">“{n.text}”<div className="text-[11px] text-muted">{n.by}</div></li>)}</ul>}
              <p className="mt-2 rounded-lg bg-primary-soft/50 px-3 py-2 text-[11px] text-muted">{t("Notes are added from Devices & models (device.write), not here.")}</p>
            </Card>
          )}
        </div>
      )}
    </>
  );
}
