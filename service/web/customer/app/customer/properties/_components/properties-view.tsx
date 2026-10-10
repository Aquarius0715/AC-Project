"use client";

import Link from "next/link";
import { useState } from "react";
import { useRouter } from "next/navigation";
import { Badge, Banner, Btn, Card, Choice, ConnBadge, cx, DataTable, EmptyState, Field, Input, Modal, Page, PowerBadge, SummaryList, useToast } from "@ac/web/components/ui";
import { useAction } from "@ac/web/lib/useAction";
import { useUrlPatch } from "@ac/web/lib/useUrlPatch";
import { nameError, type Connection, type PowerState, type Selection, type TreeProperty, type TreeSpace } from "@ac/web/lib/assets";
import { changeText, groupPlan, type GroupChange, type PlanRow, type RoomUnit } from "@ac/web/lib/clientProperties";
import { historyRow, type ApiCommand, type ApiUnitDetail } from "@ac/web/lib/units";
import { waitForCommand } from "@ac/web/lib/unitCommands";
import { useI18n, useT } from "@ac/web/components/I18n";
import type { T } from "@ac/web/lib/i18n";
import { renameLocation, sendCommand } from "../actions";

export type PropertiesLive = {
  tree: TreeProperty[]; selection: Selection | null; owner: boolean;
  filter: { powerState: PowerState | null; connections: Connection[]; total: number } | null;
  units: (RoomUnit & { place: string })[]; details: ApiUnitDetail[];
  summary: { kind: "home" | "office"; floors: string[]; rooms: number; inRooms: number; unassigned: number; edited: string } | null;
  mode: "single" | "group"; selectedUnits: string[];
};
const kindLabel: Record<string, string> = { home: "Home", office: "Office", floor: "Floor", area: "Area", room: "Room", space: "Space" };
const powerLabel: Record<PowerState, string> = { on: "Running", off: "Stopped", unknown: "Unknown" };
// “2 units”, “1 AC” in the display language
const units_ = (t: T, n: number) => t(n === 1 ? "{n} unit" : "{n} units", { n });
const acs_ = (t: T, n: number) => t(n === 1 ? "{n} AC" : "{n} ACs", { n });
const kind = (t: T, k: string) => t(kindLabel[k] ?? k);
type Renaming = { kind: "property" | "space"; id: string; version: number; name: string; where: string };

/** Units & locations (FR-C02) in API mode: the structure is read only (HQ manages it) except renaming; a room lists its
 * air conditioners, and owners can send one change to several of them (Group control, FR-C14). */
export function PropertiesView({ live }: { live: PropertiesLive }) {
  const t = useT();
  const nav = useUrlPatch();
  const [renaming, setRenaming] = useState<Renaming | null>(null);
  const sel = live.selection;
  const selId = !sel ? "" : sel.kind === "property" ? sel.property.id : sel.kind === "space" ? sel.space.id : `unassigned:${sel.property.id}`;
  const go = (patch: Record<string, string | null>) => nav({ propertyId: null, spaceId: null, unassignedOnly: null, mode: null, unitIds: null, powerState: null, connections: null, ...patch });
  if (live.tree.length === 0) return <Page><EmptyState title={t("No properties yet")}>{t("HQ registers your homes and offices with their rooms and air conditioners; they appear here once set up.")}</EmptyState></Page>;
  return (
    <Page>
      <div className="split-rev">
        <Card title={t("My properties")} action={<Badge tone="muted">{live.tree.length}</Badge>} className="self-start">
          <div className="flex flex-col text-[13px]">
            {live.tree.map((p) => (
              <div key={p.id}>
                <Row depth={0} selected={!live.filter && selId === p.id} onClick={() => go({ propertyId: p.id })} icon="⌂" name={p.name} tag={kind(t, p.kind)}
                  onRename={() => setRenaming({ kind: "property", id: p.id, version: p.version, name: p.name, where: t("{name} · {kind} (property)", { name: p.name, kind: kind(t, p.kind) }) })} />
                {p.spaces.map((s) => <SpaceRow key={s.id} s={s} p={p} depth={1} selId={live.filter ? "" : selId} go={go} rename={setRenaming} />)}
                {p.unassigned > 0 && <Row depth={1} selected={!live.filter && selId === `unassigned:${p.id}`} onClick={() => go({ propertyId: p.id, unassignedOnly: "true" })} icon="?" name={t("Unassigned units")} meta={t("· {units} · placed by HQ", { units: units_(t, p.unassigned) })} />}
              </div>
            ))}
          </div>
          <p className="mt-3 text-[11px] text-muted">{t("Read-only — HQ sets up properties, floors, rooms and units. You can rename any location with ✎ or Rename; nothing else here changes the structure.")}</p>
        </Card>
        <div className="flex min-w-0 flex-col gap-4">
          {live.filter ? <FilteredUnits live={live} clear={() => go({})} /> : sel && <Panel live={live} sel={sel} rename={setRenaming} go={go} />}
        </div>
      </div>
      {renaming && <RenameModal r={renaming} onClose={() => setRenaming(null)} />}
    </Page>
  );
}

function Row({ depth, selected, onClick, icon, name, tag, meta, onRename }: { depth: number; selected: boolean; onClick: () => void; icon: string; name: string; tag?: string; meta?: string; onRename?: () => void }) {
  const t = useT();
  return (
    <div className={cx("flex items-center gap-2 rounded-lg px-2 py-1.5", selected ? "bg-primary-soft" : "hover:bg-surface2")} style={{ paddingLeft: 8 + depth * 16 }}>
      <button type="button" onClick={onClick} aria-pressed={selected} className="flex min-w-0 flex-1 items-center gap-2 text-left">
        <span aria-hidden className="text-muted">{icon}</span><span className="truncate font-semibold">{name}</span>
        {tag && <span className="rounded bg-surface2 px-1.5 text-[10px] text-muted">{tag}</span>}
        {meta && <span className="truncate text-[11px] text-muted">{meta}</span>}
      </button>
      {onRename && <button type="button" aria-label={t("Rename {name}", { name })} onClick={onRename} className="text-muted hover:text-primary">✎</button>}
    </div>
  );
}
function SpaceRow({ s, p, depth, selId, go, rename }: { s: TreeSpace; p: TreeProperty; depth: number; selId: string; go: (x: Record<string, string | null>) => void; rename: (r: Renaming) => void }) {
  const t = useT();
  return (
    <>
      <Row depth={depth} selected={selId === s.id} onClick={() => go({ spaceId: s.id })} icon={s.children.length ? "▾" : "›"} name={s.name} tag={kind(t, s.kind)}
        meta={s.direct ? `· ${units_(t, s.direct)}` : undefined} onRename={() => rename({ kind: "space", id: s.id, version: s.version, name: s.name, where: t("{name} · {kind} in {property}", { name: s.name, kind: kind(t, s.kind), property: p.name }) })} />
      {s.children.map((c) => <SpaceRow key={c.id} s={c} p={p} depth={depth + 1} selId={selId} go={go} rename={rename} />)}
    </>
  );
}

function UnitRows({ units }: { units: (RoomUnit & { place: string })[] }) {
  const t = useT();
  return (
    <div className="flex flex-col gap-2">
      {units.map((u) => (
        <Link key={u.id} href={`/customer/units/${u.id}`} className="flex flex-wrap items-center justify-between gap-2 rounded-xl border border-line p-3 hover:bg-surface2/60">
          <span className="min-w-0"><span className="flex items-center gap-2 font-bold"><span aria-hidden className="text-primary">❄</span>{u.name}</span><span className="block text-[11px] text-muted">{u.set} · {u.place}</span></span>
          <span className="flex flex-wrap items-center gap-3 text-xs"><span>{u.temp}</span><span>{u.watts}</span><PowerBadge s={u.power} /><ConnBadge s={u.conn} />{!u.online && u.seen && <span className="text-muted">{t("seen {time}", { time: u.seen })}</span>}<span className="font-semibold text-primary">{t("Open control →")}</span></span>
        </Link>
      ))}
    </div>
  );
}

const connLabel: Record<string, string> = { online: "Online", offline: "Offline", unknown: "Unknown", connecting: "Connecting", error: "Error" };
function FilteredUnits({ live, clear }: { live: PropertiesLive; clear: () => void }) {
  const t = useT();
  const f = live.filter!;
  const label = [f.powerState && t("Power: {state}", { state: t(powerLabel[f.powerState]) }), f.connections.length > 0 && t("Connection: {states}", { states: f.connections.map((c) => t(connLabel[c] ?? c)).join(", ") })].filter(Boolean).join(" · ");
  return (
    <Card title={t("Air conditioners · {filter}", { filter: label })} sub={t("{n} of {total}", { n: live.units.length, total: f.total })} action={<Btn size="sm" onClick={clear}>{t("Clear filter")}</Btn>}>
      {live.units.length ? <UnitRows units={live.units} /> : <EmptyState title={t("No air conditioner matches")}>{t("Clear the filter to see every unit.")}</EmptyState>}
    </Card>
  );
}

function Panel({ live, sel, rename, go }: { live: PropertiesLive; sel: Selection; rename: (r: Renaming) => void; go: (x: Record<string, string | null>) => void }) {
  const t = useT();
  const p = sel.property;
  if (sel.kind === "property") {
    const s = live.summary!;
    return (
      <>
        <div className="text-[13px] text-muted">{t("My properties")} › <b className="text-ink">{p.name}</b></div>
        <Card title={<span className="flex items-center gap-2">{p.name}<Badge tone="muted">⌂ {kind(t, p.kind)}</Badge></span>}
          action={<Btn size="sm" onClick={() => rename({ kind: "property", id: p.id, version: p.version, name: p.name, where: t("{name} · {kind} (property)", { name: p.name, kind: kind(t, p.kind) }) })}>{t("✎ Rename")}</Btn>}>
          <SummaryList cols={2} items={[[t("Type"), kind(t, s.kind)], [t("Floors"), s.floors.length ? `${s.floors.length} (${s.floors.join(", ")})` : t("none")], [t("Rooms"), s.rooms], [t("Air conditioners"), t("{inRooms} in rooms · {unassigned} unassigned", { inRooms: s.inRooms, unassigned: s.unassigned })], [t("Last edited"), s.edited]]} />
          <div className="my-4 rounded-xl bg-surface2 p-4 text-center"><div className="font-bold">{t("Select a room to see its air conditioners")}</div><div className="text-xs text-muted">{t("Floor → Room → Air conditioner. Each room lists every AC inside it; open one to control it.")}</div></div>
          {live.units.length > 0 && <>
            <div className="mb-2 text-[13px] font-bold">{t("? Unassigned units ({n})", { n: live.units.length })} <span className="text-xs font-normal text-muted">{t("Not in any room")}</span></div>
            <UnitRows units={live.units} />
            <p className="mt-2 text-[11px] text-muted">{t("Moving a unit into a room is done in HQ’s unit register — contact HQ.")}</p>
          </>}
        </Card>
      </>
    );
  }
  if (sel.kind === "unassigned") {
    return (
      <Card title={t("Unassigned units ({n})", { n: live.units.length })} sub={t("{property} · not in any room — placed by HQ", { property: p.name })}>
        <UnitRows units={live.units} />
        <p className="mt-2 text-[11px] text-muted">{t("Moving a unit into a room is done in HQ’s unit register — contact HQ.")}</p>
      </Card>
    );
  }
  const s = sel.space;
  // group control acts on one space only (DD-C14): the ACs directly in this room or area, never those of child rooms
  const direct = live.units.filter((u) => live.details.find((d) => d.id === u.id)?.spaceId === s.id);
  const canGroup = live.owner && direct.length >= 2;
  const mode = canGroup ? live.mode : "single";
  return (
    <>
      <div className="text-[13px] text-muted">{t("My properties")} › {sel.path.split(" › ").map((x, i, a) => <span key={i}>{i === a.length - 1 ? <b className="text-ink">{x}</b> : `${x} › `}</span>)}</div>
      <Card title={<span className="flex items-center gap-2">{s.name}<Badge tone="muted">{kind(t, s.kind)}</Badge></span>}
        action={<Btn size="sm" onClick={() => rename({ kind: "space", id: s.id, version: s.version, name: s.name, where: t("{name} · {kind} in {property}", { name: s.name, kind: kind(t, s.kind), property: p.name }) })}>{t("✎ Rename")}</Btn>}>
        {canGroup && (
          <div className="mb-3 flex flex-wrap items-center gap-2">
            <span className="text-xs text-muted">{t("{acs} in {space} ·", { acs: acs_(t, direct.length), space: s.name })}</span>
            {(["single", "group"] as const).map((m) => <button key={m} type="button" aria-pressed={mode === m} onClick={() => go({ spaceId: s.id, mode: m === "group" ? "group" : null })}
              className={cx("rounded-control border px-3 py-1 text-xs font-semibold", mode === m ? "border-primary bg-primary-soft text-primary" : "border-line")}>{mode === m ? "✓ " : ""}{t(m === "single" ? "Single AC" : "Group control")}</button>)}
          </div>
        )}
        {mode === "group" ? <GroupPanel key={s.id} live={{ ...live, units: direct, details: live.details.filter((d) => d.spaceId === s.id) }} space={s.name} /> : live.units.length === 0 ? <p className="text-[13px] text-muted">{t("No air conditioner in this {kind}.", { kind: kind(t, s.kind).toLowerCase() })}</p> : <>
          <div className="mb-2 flex items-center justify-between text-[13px]"><b>{t("Air conditioners in this {kind} ({n})", { kind: kind(t, s.kind).toLowerCase(), n: live.units.length })}</b></div>
          <UnitRows units={live.units} />
          <div className="mt-3"><Banner>{t(live.owner ? "Each AC has its own control screen; Group control sends one setting to several ACs in a space. Unit membership is managed in HQ’s unit register." : "Each AC has its own control screen. Unit membership is managed in HQ’s unit register.")}</Banner></div>
        </>}
      </Card>
    </>
  );
}

function RenameModal({ r, onClose }: { r: Renaming; onClose: () => void }) {
  const t = useT();
  const [pending, run] = useAction();
  const [name, setName] = useState(r.name);
  const [err, setErr] = useState<string | undefined>();
  const save = () => {
    const e = nameError(name, t);
    if (e) return setErr(e);
    run(() => renameLocation(r.kind, r.id, r.version, name), t("Renamed to “{name}”", { name: name.trim() }), onClose, (f) => {
      if (f.messageKey === "error.duplicateSiblingName") setErr(t("“{name}” already exists there — choose another name. Your input is kept.", { name: name.trim() }));
      else if (f.messageKey === "error.versionConflict") setErr(t("Someone renamed it in the meantime — the tree now shows the current name. Your input is kept."));
      else setErr(undefined);
    });
  };
  return (
    <Modal open onClose={onClose} title={t("Rename {name}", { name: r.name })} footer={<><Btn onClick={onClose}>{t("Cancel")}</Btn><Btn variant="primary" disabled={pending || name.trim() === r.name} onClick={save}>{t("Save name")}</Btn></>}>
      <div className="flex flex-col gap-3">
        <p className="text-[13px] text-muted">{t("Only the name changes. Floors, rooms and units stay as they are — HQ manages the structure.")}</p>
        <SummaryList items={[[t("Location"), r.where]]} />
        <Field label={t("New name")} hint={t("1–120 characters, unique among its siblings. HQ and technicians see the new name too.")} error={err}><Input value={name} onChange={(e) => { setName(e.target.value); setErr(undefined); }} autoFocus /></Field>
      </div>
    </Modal>
  );
}

// ---- group control (FR-C14) ----

type Track = { status: "Sending" | "Confirmed" | "Failed"; note: string };
function GroupPanel({ live, space }: { live: PropertiesLive; space: string }) {
  const i = useI18n();
  const { t } = i;
  const toast = useToast();
  const router = useRouter();
  const online = live.units.filter((u) => u.online).map((u) => u.id);
  const [sel, setSel] = useState<string[]>(live.selectedUnits.length ? live.selectedUnits.filter((id) => online.includes(id)) : online);
  const [ch, setCh] = useState<GroupChange>({ power: true, celsius: 24, mode: "cool", fan: null });
  const [review, setReview] = useState<PlanRow[] | null>(null);
  const [track, setTrack] = useState<Record<string, Track>>({});
  const [sending, setSending] = useState(false);
  const caps = live.details.filter((d) => sel.includes(d.id)).map((d) => d.capabilities.temperature).filter((t): t is NonNullable<typeof t> => !!t);
  const lo = caps.length ? Math.min(...caps.map((t) => t.min)) : 16;
  const hi = caps.length ? Math.max(...caps.map((t) => t.max)) : 30;
  const toggle = (id: string) => setSel((s) => (s.includes(id) ? s.filter((x) => x !== id) : [...s, id]));
  const send = async (rows: PlanRow[]) => {
    setSending(true);
    setReview(null);
    setTrack((t) => ({ ...t, ...Object.fromEntries(rows.map((r) => [r.id, { status: "Sending" as const, note: "" }])) }));
    // each AC on its own (no rollback across ACs); its settings one after the other, stopping at the first failure
    await Promise.all(rows.map(async (r) => {
      for (const a of r.actions) {
        const created = await sendCommand(r.id, a);
        if (!created.ok) return setTrack((x) => ({ ...x, [r.id]: { status: "Failed", note: created.messageKey.replace(/^errors?\./, "").replace(/_/g, " ") } }));
        const c: ApiCommand = await waitForCommand(created.value, () => {});
        if (c.status !== "acknowledged") return setTrack((x) => ({ ...x, [r.id]: { status: "Failed", note: historyRow(c, t, i.display).text } }));
      }
      setTrack((x) => ({ ...x, [r.id]: { status: "Confirmed", note: r.change } }));
    }));
    setSending(false);
    toast(t("Group change finished — results are shown per AC"));
    router.refresh(); // re-read the units (observed settings)
  };
  const plan = (ids: string[]) => groupPlan(live.details.filter((d) => ids.includes(d.id)), ch, i);
  return (
    <div className="flex flex-col gap-3">
      <div className="flex flex-wrap items-center justify-between gap-2 rounded-xl bg-surface2 px-3 py-2 text-[13px]"><b>{t("{n} selected", { n: sel.length })}</b>
        <span className="flex gap-2"><Btn size="sm" onClick={() => setSel(online)}>{t("Select all online ({n})", { n: online.length })}</Btn><Btn size="sm" variant="ghost" onClick={() => setSel([])}>{t("Clear")}</Btn></span></div>
      <div className="grid-fluid" style={{ ["--min" as string]: "170px" }}>
        {live.units.map((u) => {
          const tr = track[u.id];
          return (
            <button key={u.id} type="button" disabled={sending} onClick={() => toggle(u.id)} aria-pressed={sel.includes(u.id)}
              className={cx("rounded-xl border p-3 text-left disabled:opacity-60", sel.includes(u.id) ? "border-2 border-primary bg-primary-soft/50" : "border-line bg-surface")}>
              <div className="flex items-center justify-between"><b className="text-[13px]">{u.name}</b><span aria-hidden className="text-xs">{sel.includes(u.id) ? "☑" : "☐"}</span></div>
              <div className="text-[11px] text-muted">{!u.online ? (u.seen ? t("last seen {when}", { when: u.seen }) : t("no connection yet")) : u.temp === "—" && u.watts === "—" ? t("no recent reading") : `${u.temp} · ${u.watts}`}</div>
              <div className="mt-1 flex flex-wrap gap-1"><PowerBadge s={u.power} /><ConnBadge s={u.conn} />{tr && <Badge tone={tr.status === "Confirmed" ? "ok" : tr.status === "Failed" ? "crit" : "primary"}>{t(tr.status)}</Badge>}</div>
              {tr?.status === "Failed" && <div className="mt-1 text-[11px] text-crit">{tr.note}</div>}
            </button>
          );
        })}
      </div>
      <Card title={t(sel.length === 1 ? "Apply to {n} selected AC" : "Apply to {n} selected ACs", { n: sel.length })}>
        <div className="flex flex-wrap items-center gap-4">
          <Field label={t("Power")}><Choice value={ch.power ? "on" : "off"} onChange={(v: "on" | "off") => setCh({ ...ch, power: v === "on" })} options={[{ id: "on", label: t("On") }, { id: "off", label: t("Off") }]} /></Field>
          {ch.power && <>
            <Field label={t("Set temperature")}><div className="flex items-center gap-2"><Btn size="sm" onClick={() => setCh({ ...ch, celsius: Math.max(lo, ch.celsius - 1) })}>−</Btn><b className="w-14 text-center">{ch.celsius} °C</b><Btn size="sm" onClick={() => setCh({ ...ch, celsius: Math.min(hi, ch.celsius + 1) })}>+</Btn></div></Field>
            <Field label={t("Mode")}><Choice value={ch.mode} onChange={(v: GroupChange["mode"]) => setCh({ ...ch, mode: v })} options={[{ id: "cool", label: t("Cool") }, { id: "dry", label: t("Dry") }, { id: "fan", label: t("Fan") }]} /></Field>
            <Field label={t("Fan")}><Choice value={ch.fan ?? "keep"} onChange={(v: string) => setCh({ ...ch, fan: v === "keep" ? null : (v as GroupChange["fan"]) })} options={[{ id: "keep", label: t("Keep") }, { id: "low", label: t("Low") }, { id: "mid", label: t("Mid") }, { id: "high", label: t("High") }]} /></Field>
          </>}
          <Btn variant="primary" disabled={!sel.length || sending} onClick={() => setReview(plan(sel))}>{t("Review & send")}</Btn>
        </div>
        <p className="mt-2 text-[11px] text-muted">{t("Each AC gets its own command and confirmation (same as single control). Offline ACs are skipped; restrictions such as a minimum temperature still apply per AC. One space only · owner role only.")}</p>
        {Object.values(track).some((x) => x.status === "Failed") && !sending && (
          <div className="mt-2"><Btn size="sm" onClick={() => { const ids = Object.entries(track).filter(([, x]) => x.status === "Failed").map(([id]) => id); setReview(plan(ids)); }}>{t("Retry failed ACs")}</Btn></div>
        )}
      </Card>
      {review && (
        <Modal open wide onClose={() => setReview(null)} title={t(review.length === 1 ? "Send to {n} selected AC in {space}?" : "Send to {n} selected ACs in {space}?", { n: review.length, space })}
          footer={<><Btn onClick={() => setReview(null)}>{t("Cancel")}</Btn><Btn variant="primary" disabled={!review.some((r) => r.actions.length)} onClick={() => send(review.filter((r) => r.actions.length))}>{t(review.filter((r) => r.actions.length).length === 1 ? "Send to {n} AC" : "Send to {n} ACs", { n: review.filter((r) => r.actions.length).length })}</Btn></>}>
          <p className="mb-2 text-[13px] font-semibold">{changeText(ch, t)}</p>
          <DataTable rowKey={(r) => r.id} rows={review} cols={[
            { key: "n", label: "AC", render: (r) => <b>{r.name}</b> },
            { key: "c", label: t("Change"), render: (r) => <>{r.change}{r.note && <div className="text-[11px] text-warn">{r.note}</div>}</> },
            { key: "r", label: t("Result"), render: (r) => <Badge tone={r.result === "Skipped" ? "unknown" : r.result === "Clamped" ? "warn" : r.result === "No change" ? "muted" : "primary"}>{t(r.result)}</Badge> },
          ]} />
          <div className="mt-2"><Banner>{t("After sending, each card shows Sending → Confirmed / Failed. Failed ACs can be retried one by one; there is no rollback across ACs.")}</Banner></div>
        </Modal>
      )}
    </div>
  );
}
