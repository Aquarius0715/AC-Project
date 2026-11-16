"use client";
const MY_PROPERTIES = "My properties";

import Link from "next/link";
import { useEffect, useState } from "react";
import { useRouter, useSearchParams } from "next/navigation";
import { Badge, Banner, Btn, Card, Choice, ConnBadge, cx, DataTable, EmptyState, Field, Modal, Page, PowerBadge, Search, SummaryList, useToast, Plain } from "@ac/web/components/ui";
import { useUrlPatchPending } from "@ac/web/lib/useUrlPatch";
import { selectionId, type Connection, type PowerState, type Selection, type TreeProperty, type TreeSpace } from "@ac/web/lib/assets";
import { changeText, groupPlan, hasChoice, KEEP_ALL, TURN_OFF, type GroupChange, type PlanRow, type RoomUnit } from "@ac/web/lib/clientProperties";
import { historyRow, type ApiCommand, type ApiUnitDetail } from "@ac/web/lib/units";
import { followCommand } from "@ac/web/lib/unitCommands";
import { sendPaced } from "@ac/web/lib/writeLimit";
import { actionMessage } from "@ac/web/lib/actionMessage";
import { useI18n, useT } from "@ac/web/components/I18n";
import type { T } from "@ac/web/lib/i18n";
import { sendCommand } from "../actions";
import { RenameModal, type Renaming } from "../../_components/rename-modal";
import { oneOf } from "@ac/web/lib/oneOf";
import { Icon, type IconName } from "@ac/web/components/Icon";
import { unreachableFailure } from "@ac/web/lib/unreachable";

export interface PropertiesLive {
  tree: TreeProperty[]; selection: Selection | null; owner: boolean;
  filter: { powerState: PowerState | null; connections: Connection[]; search: string; total: number } | null;
  units: (RoomUnit & { place: string })[]; details: ApiUnitDetail[];
  /** the scenario's now: a group plan's “offline since” keeps its date on another day */
  now: string;
  summary: { kind: "home" | "office"; floors: string[]; rooms: number; inRooms: number; unassigned: number; edited: string } | null;
  mode: "single" | "group"; selectedUnits: string[];
}
const kindLabel: Record<string, string> = { home: "Home", office: "Office", floor: "Floor", area: "Area", room: "Room", space: "Space" };
const powerLabel: Record<PowerState, string> = { on: "Running", off: "Stopped", unknown: "Unknown" };
// “2 units”, “1 AC” in the display language
const unitsText = (t: T, n: number) => t(n === 1 ? "{n} unit" : "{n} units", { n });
const acsText = (t: T, n: number) => t(n === 1 ? "{n} AC" : "{n} ACs", { n });
const kind = (t: T, k: string) => t(kindLabel[k] ?? k);

/** Units & locations (FR-C02) in API mode: the structure is read only (HQ manages it) except renaming; a room lists its
air conditioners, and owners can send one change to several of them (Group control, FR-C14). */
// a command's state and a group test's result as a badge colour (primary for the others)
const TRY_TONE = new Map<string, "ok" | "crit" | "warn">([["Confirmed", "ok"], ["Failed", "crit"], ["Waiting", "warn"], ["Unknown", "warn"]]);
const RESULT_TONE = new Map<string, "unknown" | "warn" | "muted">([["Skipped", "unknown"], ["Clamped", "warn"], ["No change", "muted"]]);
/** The tree row a navigation patch selects: a space, a property's unassigned units or the property; null for none. */
function targetOf(patch: Record<string, string | null>): string | null {
  return patch.unassignedOnly && patch.propertyId ? `unassigned:${patch.propertyId}` : patch.spaceId ?? patch.propertyId ?? null;
}

export function PropertiesView({ live }: Readonly<{ live: PropertiesLive }>) {
  const t = useT();
  const [nav, isNavPending] = useUrlPatchPending();
  const [renaming, setRenaming] = useState<Renaming | null>(null);
  // a location or KPI filter the user chose: on a phone its panel comes above the tree, else a tap showed nothing new on screen
  const params = useSearchParams();
  const isChosen = !!live.filter || params.has("spaceId") || params.has("propertyId");
  // the location just clicked: the tree marks it at once while the server renders its panel (UI/UX loop R75)
  const [target, setTarget] = useState<string | null>(null);
  if (live.tree.length === 0) return <Page><EmptyState title={t("No properties yet")}>{t("HQ registers your homes and offices with their rooms and air conditioners; they appear here once set up.")}</EmptyState></Page>;
  const sel = live.selection;
  const selId = selectionId(sel);
  const isPendingTarget = isNavPending && target !== null;
  const isFilterShown = !!live.filter && !isPendingTarget;
  const shownSel = isPendingTarget ? target : selId;
  const go = (patch: Record<string, string | null>) => { setTarget(targetOf(patch)); nav({ propertyId: null, spaceId: null, unassignedOnly: null, mode: null, unitIds: null, powerState: null, connections: null, search: null, ...patch }); };
  return (
    <Page>
      <div className="split-rev">
        <Card title={t(MY_PROPERTIES)} action={<Badge tone="muted">{live.tree.length}</Badge>} className="self-start">
          <UnitSearch current={live.filter?.search ?? ""} nav={nav} />
          <div className="flex flex-col text-[13px]">
            {live.tree.map((p) => (
              <div key={p.id}>
                <Row depth={0} selected={!isFilterShown && shownSel === p.id} onClick={() => { go({ propertyId: p.id }); }} icon="home" name={p.name} tag={kind(t, p.kind)}
                  onRename={() => { setRenaming({ kind: "property", id: p.id, version: p.version, name: p.name, where: t("{name} · {kind} (property)", { name: p.name, kind: kind(t, p.kind) }) }); }} />
                {p.spaces.map((s) => <SpaceRow key={s.id} s={s} p={p} depth={1} selId={isFilterShown ? "" : shownSel} go={go} rename={setRenaming} />)}
                {p.unassigned > 0 && <Row depth={1} selected={!isFilterShown && shownSel === `unassigned:${p.id}`} onClick={() => { go({ propertyId: p.id, unassignedOnly: "true" }); }} icon="unknown" name={t("Unassigned units")} meta={t("· {units} · placed by HQ", { units: unitsText(t, p.unassigned) })} />}
              </div>
            ))}
          </div>
          <p className="mt-3 text-[11px] text-muted-foreground">{t("Read-only — HQ sets up properties, floors, rooms and units. You can rename any location with its edit button or Rename; nothing else here changes the structure.")}</p>
        </Card>
        {/* the old panel stays until the new one arrives: dimmed and busy, so it does not read as the clicked location's */}
        <div aria-busy={isPendingTarget} className={cx("flex min-w-0 flex-col gap-4 transition-opacity", isChosen && "order-first @min-[880px]:order-none", isPendingTarget && "opacity-60")}>
          {live.filter ? <FilteredUnits live={live} clear={() => { go({}); }} /> : sel && <Panel live={live} sel={sel} rename={setRenaming} go={go} isPending={isNavPending} />}
        </div>
      </div>
      {renaming && <RenameModal r={renaming} onClose={() => { setRenaming(null); }} />}
    </Page>
  );
}

/** Find a room or AC by name (IR801): a complaint names a room ("the director's office is hot"), so the tree need not be
scanned. The URL's search lists the units whose name or place matches; typing waits a moment before it asks. */
function UnitSearch({ current, nav }: Readonly<{ current: string; nav: (patch: Record<string, string | null>) => void }>) {
  const t = useT();
  const [q, setQ] = useState(current);
  // another navigation (a tree row, Clear filter) cleared or changed the search: the box follows the URL
  const [seen, setSeen] = useState(current);
  if (seen !== current) {
    setSeen(current);
    setQ(current);
  }
  useEffect(() => {
    const next = q.trim();
    const id = next === current ? null : setTimeout(() => { nav({ propertyId: null, spaceId: null, unassignedOnly: null, mode: null, unitIds: null, powerState: null, connections: null, search: next || null }); }, 300);
    return () => { if (id !== null) clearTimeout(id); };
  }, [q, current, nav]);
  return <div className="mb-3"><Search placeholder={t("Find a room or AC…")} value={q} onChange={setQ} /></div>;
}

function Row({ depth, selected, onClick, icon, name, tag, meta, onRename }: Readonly<{ depth: number; selected: boolean; onClick: () => void; icon: IconName; name: string; tag?: string; meta?: string; onRename?: () => void }>) {
  const t = useT();
  return (
    <div className={cx("flex items-center gap-2 rounded-lg px-2 py-1.5", selected ? "bg-primary-soft" : "hover:bg-surface2")} style={{ paddingLeft: 8 + depth * 16 }}>
      <Plain type="button" onClick={onClick} aria-pressed={selected} className="flex min-w-0 flex-1 items-center gap-2 text-left">
        <Icon name={icon} className="shrink-0 text-muted-foreground" />
        {/* the type and the unit count wrap below a name the row cannot fit beside them (a touch screen's 44 px edit button, IR618) */}
        <span className="flex min-w-0 flex-wrap items-center gap-x-2 gap-y-0.5">
          <span className="truncate font-semibold">{name}</span>
          {tag && <span className="rounded bg-surface2 px-1.5 text-[10px] text-muted-foreground">{tag}</span>}
          {meta && <span className="truncate text-[11px] text-muted-foreground">{meta}</span>}
        </span>
      </Plain>
      {onRename && <Plain touch type="button" aria-label={t("Rename {name}", { name })} onClick={onRename} className="text-muted-foreground hover:text-primary"><Icon name="edit" /></Plain>}
    </div>
  );
}
function SpaceRow({ s, p, depth, selId, go, rename }: Readonly<{ s: TreeSpace; p: TreeProperty; depth: number; selId: string; go: (x: Record<string, string | null>) => void; rename: (r: Renaming) => void }>) {
  const t = useT();
  return (
    <>
      <Row depth={depth} selected={selId === s.id} onClick={() => { go({ spaceId: s.id }); }} icon={s.children.length > 0 ? "chevronDown" : "chevronRight"} name={s.name} tag={kind(t, s.kind)}
        meta={s.direct ? `· ${unitsText(t, s.direct)}` : undefined} onRename={() => { rename({ kind: "space", id: s.id, version: s.version, name: s.name, where: t("{name} · {kind} in {property}", { name: s.name, kind: kind(t, s.kind), property: p.name }) }); }} />
      {s.children.map((c) => <SpaceRow key={c.id} s={c} p={p} depth={depth + 1} selId={selId} go={go} rename={rename} />)}
    </>
  );
}

function UnitRows({ units }: Readonly<{ units: (RoomUnit & { place: string })[] }>) {
  const t = useT();
  return (
    <div className="flex flex-col gap-2">
      {units.map((u) => (
        <Link key={u.id} href={`/customer/units/${u.id}`} className="flex flex-wrap items-center justify-between gap-2 rounded-xl border border-line p-3 hover:bg-surface2/60">
          <span className="min-w-0"><span className="flex items-center gap-2 font-bold"><Icon name="units" className="text-primary" />{u.name}</span><span className="block text-[11px] text-muted-foreground">{u.set} · {u.place}</span></span>
          <span className="flex flex-wrap items-center gap-3 text-xs"><Readings temp={u.temp} watts={u.watts} /><PowerBadge s={u.power} /><ConnBadge s={u.conn} />{!u.online && u.seen && <span className="text-muted-foreground">{t("seen {time}", { time: u.seen })}</span>}<span className="font-semibold text-primary">{t("Open control")} <Icon name="arrowRight" /></span></span>
        </Link>
      ))}
    </div>
  );
}

const connLabel: Record<string, string> = { online: "Online", offline: "Offline", unknown: "Unknown", connecting: "Connecting", error: "Error" };
function FilteredUnits({ live, clear }: Readonly<{ live: PropertiesLive; clear: () => void }>) {
  const t = useT();
  // “left on” check (IR798): an owner turns the running ACs off from this list, unticking those that should keep running,
  // instead of opening each AC's own screen
  const [isTurningOff, setIsTurningOff] = useState(false);
  const f = live.filter;
  if (!f) return null;
  const label = [f.search && t("“{q}”", { q: f.search }), f.powerState && t("Power: {state}", { state: t(powerLabel[f.powerState]) }), f.connections.length > 0 && t("Connection: {states}", { states: f.connections.map((c) => t(connLabel[c] ?? c)).join(", ") })].filter(Boolean).join(" · ");
  const isRunningList = live.owner && f.powerState === "on" && f.connections.length === 0 && !f.search;
  const canTurnOff = isRunningList && live.units.length >= 2;
  // the panel stays once opened: the ACs it turned off leave the re-read list, and its results must not vanish with them
  const isPanelShown = isRunningList && isTurningOff;
  const action = (
    <span className="flex flex-wrap gap-2">
      {(canTurnOff || isPanelShown) && (isTurningOff
        ? <Btn size="sm" onClick={() => { setIsTurningOff(false); }}>{t("Back to the list")}</Btn>
        : <Btn size="sm" variant="primary" onClick={() => { setIsTurningOff(true); }}><Icon name="power" />{t("Turn off running ACs…")}</Btn>)}
      <Btn size="sm" onClick={clear}>{t("Clear filter")}</Btn>
    </span>
  );
  const body = live.units.length > 0 ? <UnitRows units={live.units} /> : <EmptyState title={t("No air conditioner matches")}>{t(f.search ? "No room or AC name contains “{q}”. Check the spelling, or clear the filter to see every unit." : "Clear the filter to see every unit.", { q: f.search })}</EmptyState>;
  return (
    <Card title={t("Air conditioners · {filter}", { filter: label })} sub={t("{n} of {total}", { n: live.units.length, total: f.total })} action={action}>
      {isPanelShown ? <GroupPanel live={live} space="" offOnly /> : body}
    </Card>
  );
}

function Panel({ live, sel, rename, go, isPending }: Readonly<{ live: PropertiesLive; sel: Selection; rename: (r: Renaming) => void; go: (x: Record<string, string | null>) => void; isPending: boolean }>) {
  // the mode the user just chose, shown at once while the server renders it (the switch took a second with no sign)
  const [chosen, setChosen] = useState<"single" | "group" | null>(null);
  const t = useT();
  const p = sel.property;
  if (sel.kind === "property") {
    const s = live.summary;
    if (!s) return null;
    const isUnassigned = (id: string) => live.details.find((d) => d.id === id)?.spaceId === null;
    const unassigned = live.units.filter((u) => isUnassigned(u.id));
    const inRooms = live.units.filter((u) => !isUnassigned(u.id));
    return (
      <>
        <div className="text-[13px] text-muted-foreground">{t(MY_PROPERTIES)} › <b className="text-ink">{p.name}</b></div>
        <Card title={<span className="flex items-center gap-2">{p.name}<Badge tone="muted" icon="home">{kind(t, p.kind)}</Badge></span>}
          action={<Btn size="sm" onClick={() => { rename({ kind: "property", id: p.id, version: p.version, name: p.name, where: t("{name} · {kind} (property)", { name: p.name, kind: kind(t, p.kind) }) }); }}><Icon name="edit" />{t("Rename")}</Btn>}>
          <SummaryList cols={2} items={[[t("Type"), kind(t, s.kind)], [t("Floors"), s.floors.length > 0 ? `${String(s.floors.length)} (${s.floors.join(", ")})` : t("none")], [t("Rooms"), s.rooms], [t("Air conditioners"), t("{inRooms} in rooms · {unassigned} unassigned", { inRooms: s.inRooms, unassigned: s.unassigned })], [t("Last edited"), s.edited]]} />
          {/* the property's ACs at a glance, each with its room, instead of "select a room" first */}
          {inRooms.length > 0 ? <div className="mt-4">
            <div className="mb-2 text-[13px] font-bold">{t("Air conditioners in this property ({n})", { n: inRooms.length })}</div>
            <UnitRows units={inRooms} />
          </div> : <p className="my-4 text-[13px] text-muted-foreground">{t("No air conditioner in a room of this property yet.")}</p>}
          {unassigned.length > 0 && <>
            <div className="mb-2 mt-4 flex flex-wrap items-center gap-1.5 text-[13px] font-bold"><Icon name="unknown" className="text-muted-foreground" />{t("Unassigned units ({n})", { n: unassigned.length })} <span className="text-xs font-normal text-muted-foreground">{t("Not in any room")}</span></div>
            <UnitRows units={unassigned} />
            <p className="mt-2 text-[11px] text-muted-foreground">{t("Moving a unit into a room is done in HQ’s unit register — contact HQ.")}</p>
          </>}
        </Card>
      </>
    );
  }
  if (sel.kind === "unassigned") {
    return (
      <Card title={t("Unassigned units ({n})", { n: live.units.length })} sub={t("{property} · not in any room — placed by HQ", { property: p.name })}>
        <UnitRows units={live.units} />
        <p className="mt-2 text-[11px] text-muted-foreground">{t("Moving a unit into a room is done in HQ’s unit register — contact HQ.")}</p>
      </Card>
    );
  }
  const s = sel.space;
  // group control acts on one space only (DD-C14): the ACs directly in this room or area, never those of child rooms
  const direct = live.units.filter((u) => live.details.find((d) => d.id === u.id)?.spaceId === s.id);
  const canGroup = live.owner && direct.length >= 2;
  const mode = canGroup ? live.mode : "single";
  const shownMode = isPending && chosen ? chosen : mode;
  return (
    <>
      <div className="text-[13px] text-muted-foreground">{t(MY_PROPERTIES)} › {sel.path.split(" › ").map((x, i, a) => <span key={i}>{i === a.length - 1 ? <b className="text-ink">{x}</b> : `${x} › `}</span>)}</div>
      <Card title={<span className="flex items-center gap-2">{s.name}<Badge tone="muted">{kind(t, s.kind)}</Badge></span>}
        action={<Btn size="sm" onClick={() => { rename({ kind: "space", id: s.id, version: s.version, name: s.name, where: t("{name} · {kind} in {property}", { name: s.name, kind: kind(t, s.kind), property: p.name }) }); }}><Icon name="edit" />{t("Rename")}</Btn>}>
        {canGroup && (
          <div className="mb-3 flex flex-wrap items-center gap-2">
            <span className="text-xs text-muted-foreground">{t("{acs} in {space} ·", { acs: acsText(t, direct.length), space: s.name })}</span>
            {(["single", "group"] as const).map((m) => {
              const isLoading = isPending && chosen === m && mode !== m;
              return <Plain key={m} type="button" aria-pressed={shownMode === m} aria-busy={isLoading} onClick={() => { setChosen(m); go({ spaceId: s.id, mode: m === "group" ? "group" : null }); }}
                className={cx("inline-flex items-center gap-1 rounded-control border px-3 py-1 text-xs font-semibold", shownMode === m ? "border-primary bg-primary-soft text-primary" : "border-line")}>{isLoading ? <Icon name="refresh" className="animate-spin" /> : shownMode === m && <Icon name="check" />}{t(m === "single" ? "Single AC" : "Group control")}</Plain>;
            })}
          </div>
        )}
        {mode === "group" ? <GroupPanel key={s.id} live={{ ...live, units: direct, details: live.details.filter((d) => d.spaceId === s.id) }} space={s.name} /> : null}{mode !== "group" && live.units.length === 0 && <p className="text-[13px] text-muted-foreground">{t("No air conditioner in this {kind}.", { kind: kind(t, s.kind).toLowerCase() })}</p>}{mode !== "group" && live.units.length > 0 && <>
          <div className="mb-2 flex items-center justify-between text-[13px]"><b>{t("Air conditioners in this {kind} ({n})", { kind: kind(t, s.kind).toLowerCase(), n: live.units.length })}</b></div>
          <UnitRows units={live.units} />
          <div className="mt-3"><Banner>{t(live.owner ? "Each AC has its own control screen; Group control sends one setting to several ACs in a space. Unit membership is managed in HQ’s unit register." : "Each AC has its own control screen. Unit membership is managed in HQ’s unit register.")}</Banner></div>
        </>}
      </Card>
    </>
  );
}

/** The room temperature and power draw of a row: only what was read (a bare "—" said nothing), and "no recent reading" when neither was. */
function Readings({ temp, watts }: Readonly<{ temp: string; watts: string }>) {
  const t = useT();
  const read = [temp, watts].filter((v) => v !== "—");
  return read.length === 0 ? <span className="text-muted-foreground">{t("no recent reading")}</span> : <span>{read.join(" · ")}</span>;
}

// ---- group control (FR-C14) ----

/** The power choice of the group panel: Keep (null), On or Off. */
const powerChoice = (power: boolean | null): "keep" | "on" | "off" => {
  if (power === null) return "keep";
  return power ? "on" : "off";
};

/** The group setpoint: Keep (each AC keeps its own) until a step is pressed, which starts from 24 °C within the range. */
function TemperatureStep({ celsius, lo, hi, onChange }: Readonly<{ celsius: number | null; lo: number; hi: number; onChange: (celsius: number | null) => void }>) {
  const { t } = useI18n();
  const step = (delta: number) => { onChange(Math.min(hi, Math.max(lo, celsius === null ? 24 : celsius + delta))); };
  return (
    <Field label={t("Set temperature")}>
      <div className="flex items-center gap-2">
        <Btn size="sm" onClick={() => { step(-1); }} aria-label={t("Lower the temperature")}><Icon name="remove" /></Btn>
        <b className="w-16 text-center">{celsius === null ? t("Keep") : `${String(celsius)} °C`}</b>
        <Btn size="sm" onClick={() => { step(1); }} aria-label={t("Raise the temperature")}><Icon name="add" /></Btn>
        {celsius !== null && <Btn size="sm" variant="ghost" onClick={() => { onChange(null); }}>{t("Keep")}</Btn>}
      </div>
    </Field>
  );
}

/** Unknown: the command was sent but its answer could not be read, so it is neither failed nor retried (IR728). */
interface Track { status: "Sending" | "Waiting" | "Confirmed" | "Failed" | "Unknown"; note: string }
/** offOnly: “Turn off running ACs” (IR798) — power off is the only change, across the properties of the Running list. */
function GroupPanel({ live, space, offOnly = false }: Readonly<{ live: PropertiesLive; space: string; offOnly?: boolean }>) {
  const i = useI18n();
  const { t } = i;
  const toast = useToast();
  const router = useRouter();
  const online = live.units.filter((u) => u.online).map((u) => u.id);
  const [chosen, setChosen] = useState<string[]>(live.selectedUnits.length > 0 ? live.selectedUnits.filter((id) => online.includes(id)) : online);
  // an AC that left the list (turned off, re-read after sending) is no longer selected
  const sel = chosen.filter((id) => live.units.some((u) => u.id === id));
  const [ch, setCh] = useState<GroupChange>(offOnly ? TURN_OFF : KEEP_ALL); // every setting kept until the user chooses one
  const [review, setReview] = useState<PlanRow[] | null>(null);
  const [track, setTrack] = useState<Record<string, Track>>({});
  const [sending, setSending] = useState(false);
  const caps = live.details.filter((d) => sel.includes(d.id)).map((d) => d.capabilities.temperature).filter((cap): cap is NonNullable<typeof cap> => !!cap);
  const lo = caps.length > 0 ? Math.min(...caps.map((cap) => cap.min)) : 16;
  const hi = caps.length > 0 ? Math.max(...caps.map((cap) => cap.max)) : 30;
  const toggle = (id: string) => { setChosen((s) => (s.includes(id) ? s.filter((x) => x !== id) : [...s, id])); };
  const send = async (rows: PlanRow[]) => {
    setSending(true);
    setReview(null);
    setTrack((prev) => ({ ...prev, ...Object.fromEntries(rows.map((r) => [r.id, { status: "Sending" as const, note: "" }])) }));
    // each AC on its own (no rollback across ACs); its settings one after the other, stopping at the first failure. A
    // command over the member's write limit waits the advised seconds and is sent again (IR375, IR380)
    const wait = (id: string) => (s: number) => { setTrack((x) => ({ ...x, [id]: { status: "Waiting", note: t("Write limit reached — sending again in {s} s", { s }) } })); };
    await Promise.all(rows.map(async (r) => {
      try {
        for (const a of r.actions) {
          const created = await sendPaced(() => sendCommand(r.id, a), wait(r.id));
          if (!created.ok) { setTrack((x) => ({ ...x, [r.id]: { status: "Failed", note: actionMessage(created, t) } })); return; }
          setTrack((x) => ({ ...x, [r.id]: { status: "Sending", note: "" } }));
          const followed = await followCommand(created.value, () => { /* the row already says Sending */ });
          if (!followed.answered) { setTrack((x) => ({ ...x, [r.id]: { status: "Unknown", note: t("Sent — its answer could not be read. Refresh to see the AC's state.") } })); return; }
          const c: ApiCommand = followed.command;
          if (c.status !== "acknowledged") { setTrack((x) => ({ ...x, [r.id]: { status: "Failed", note: historyRow(c, t, i.display).text } })); return; }
        }
        setTrack((x) => ({ ...x, [r.id]: { status: "Confirmed", note: r.change } }));
      } catch {
        // the action got no answer: this AC failed, the others go on and the button comes back (IR718)
        setTrack((x) => ({ ...x, [r.id]: { status: "Failed", note: actionMessage(unreachableFailure(), t) } }));
      }
    }));
    setSending(false);
    toast(t("Group change finished — results are shown per AC"));
    router.refresh(); // re-read the units (observed settings)
  };
  // the failed ACs the list still holds: one turned off meanwhile has nothing left to retry
  const failed = Object.entries(track).filter(([id, x]) => x.status === "Failed" && live.units.some((u) => u.id === id)).map(([id]) => id);
  // confirmed ACs that the re-read list no longer holds: said in one line, as their cards are gone (IR798)
  const turnedOff = Object.entries(track).filter(([id, x]) => x.status === "Confirmed" && live.units.every((u) => u.id !== id)).length;
  const cardTitle = () => {
    const n = sel.length;
    if (offOnly) return t(n === 1 ? "Turn off {n} selected AC" : "Turn off {n} selected ACs", { n });
    return t(n === 1 ? "Apply to {n} selected AC" : "Apply to {n} selected ACs", { n });
  };
  const reviewTitle = (n: number) => {
    if (offOnly) return t(n === 1 ? "Turn off {n} selected AC?" : "Turn off {n} selected ACs?", { n });
    return t(n === 1 ? "Send to {n} selected AC in {space}?" : "Send to {n} selected ACs in {space}?", { n, space });
  };
  const plan = (ids: string[]) => groupPlan(live.details.filter((d) => ids.includes(d.id)), ch, i, Date.parse(live.now));
  return (
    <div className="flex flex-col gap-3">
      <div className="flex flex-wrap items-center justify-between gap-2 rounded-xl bg-surface2 px-3 py-2 text-[13px]"><b>{t("{n} selected", { n: sel.length })}</b>
        {/* each button is off when it would change nothing: all online already chosen, or nothing to clear (UI/UX loop R96) */}
        <span className="flex gap-2"><Btn size="sm" disabled={online.every((id) => sel.includes(id))} onClick={() => { setChosen(online); }}>{t("Select all online ({n})", { n: online.length })}</Btn><Btn size="sm" variant="ghost" disabled={sel.length === 0} onClick={() => { setChosen([]); }}>{t("Clear")}</Btn></span></div>
      <div className="grid-fluid" style={{ ["--min" as string]: "170px" }}>
        {live.units.map((u) => {
          const tr = track[u.id];
          return (
            <Plain key={u.id} type="button" disabled={sending} onClick={() => { toggle(u.id); }} aria-pressed={sel.includes(u.id)}
              className={cx("rounded-xl border p-3 text-left disabled:opacity-60", sel.includes(u.id) ? "border-2 border-primary bg-primary-soft/50" : "border-line bg-surface")}>
              <div className="flex items-center justify-between"><b className="text-[13px]">{u.name}</b><Icon name={sel.includes(u.id) ? "checked" : "unchecked"} className="text-sm" /></div>
              {offOnly && <div className="text-[11px] text-muted-foreground">{u.place}</div>}
              <div className="text-[11px] text-muted-foreground">{u.online && <Readings temp={u.temp} watts={u.watts} />}{!u.online && u.seen && t("last seen {when}", { when: u.seen })}{!u.online && !u.seen && t("no connection yet")}</div>
              <div className="mt-1 flex flex-wrap gap-1"><PowerBadge s={u.power} /><ConnBadge s={u.conn} />{tr && <Badge tone={TRY_TONE.get(tr.status) ?? "primary"}>{t(tr.status)}</Badge>}</div>
              {(tr?.status === "Failed" || tr?.status === "Waiting" || tr?.status === "Unknown") && <div className={cx("mt-1 text-[11px]", tr.status === "Failed" ? "text-crit" : "text-warn")}>{tr.note}</div>}
            </Plain>
          );
        })}
      </div>
      <Card title={cardTitle()}>
        {offOnly ? <>
          {turnedOff > 0 && <p className="mb-2 flex items-center gap-1.5 text-[13px] font-semibold text-ok"><Icon name="check" />{t(turnedOff === 1 ? "{n} AC turned off — it has left the Running list" : "{n} ACs turned off — they have left the Running list", { n: turnedOff })}</p>}
          {live.units.length === 0 && <p className="mb-2 text-[13px] text-muted-foreground">{t("No AC is running any more.")}</p>}
          <Btn variant="primary" disabled={sel.length === 0 || sending} onClick={() => { setReview(plan(sel)); }}>{t("Review & turn off")}</Btn>
          <p className="mt-2 text-[11px] text-muted-foreground">{t("Untick the ACs that should keep running. Each AC gets its own command and confirmation (same as single control); offline ACs are skipped. Owner role only.")}</p>
        </> : <>
          <div className="flex flex-wrap items-center gap-4">
            <Field label={t("Power")}><Choice value={powerChoice(ch.power)} onChange={(v: string) => { setCh({ ...ch, power: v === "keep" ? null : v === "on" }); }} options={[{ id: "keep", label: t("Keep") }, { id: "on", label: t("On") }, { id: "off", label: t("Off") }]} /></Field>
            {ch.power !== false && <>
              <TemperatureStep celsius={ch.celsius} lo={lo} hi={hi} onChange={(celsius) => { setCh({ ...ch, celsius }); }} />
              <Field label={t("Mode")}><Choice value={ch.mode ?? "keep"} onChange={(v: string) => { setCh({ ...ch, mode: oneOf(["cool", "dry", "fan"] as const, v, null) }); }} options={[{ id: "keep", label: t("Keep") }, { id: "cool", label: t("Cool") }, { id: "dry", label: t("Dry") }, { id: "fan", label: t("Fan") }]} /></Field>
              <Field label={t("Fan")}><Choice value={ch.fan ?? "keep"} onChange={(v: string) => { setCh({ ...ch, fan: oneOf(["low", "mid", "high"] as const, v, null) }); }} options={[{ id: "keep", label: t("Keep") }, { id: "low", label: t("Low") }, { id: "mid", label: t("Mid") }, { id: "high", label: t("High") }]} /></Field>
            </>}
            <Btn variant="primary" disabled={sel.length === 0 || sending || !hasChoice(ch)} onClick={() => { setReview(plan(sel)); }}>{t("Review & send")}</Btn>
          </div>
          {!hasChoice(ch) && <p className="mt-2 text-xs text-muted-foreground">{t("Every setting is kept — choose what to change.")}</p>}
          <p className="mt-2 text-[11px] text-muted-foreground">{t("Each AC gets its own command and confirmation (same as single control). Offline ACs are skipped; restrictions such as a minimum temperature still apply per AC. One space only · owner role only.")}</p>
        </>}
        {failed.length > 0 && !sending && (
          <div className="mt-2"><Btn size="sm" onClick={() => { setReview(plan(failed)); }}>{t("Retry failed ACs")}</Btn></div>
        )}
      </Card>
      {review && (
        <Modal open wide onClose={() => { setReview(null); }} title={reviewTitle(review.length)}
          footer={<><Btn onClick={() => { setReview(null); }}>{t("Cancel")}</Btn><Btn variant="primary" disabled={review.every((r) => r.actions.length === 0)} onClick={() => { void send(review.filter((r) => r.actions.length)); }}>{t(review.filter((r) => r.actions.length).length === 1 ? "Send to {n} AC" : "Send to {n} ACs", { n: review.filter((r) => r.actions.length).length })}</Btn></>}>
          <p className="mb-2 text-[13px] font-semibold">{changeText(ch, t)}</p>
          <DataTable rowKey={(r) => r.id} rows={review} cols={[
            { key: "n", label: "AC", render: (r) => <b>{r.name}</b> },
            { key: "c", label: t("Change"), render: (r) => <>{r.change}{r.note && <div className="text-[11px] text-warn">{r.note}</div>}</> },
            { key: "r", label: t("Result"), render: (r) => <Badge tone={RESULT_TONE.get(r.result) ?? "primary"}>{t(r.result)}</Badge> },
          ]} />
          <div className="mt-2"><Banner>{t("After sending, each card shows Sending → Confirmed / Failed. Failed ACs can be retried one by one; there is no rollback across ACs.")}</Banner></div>
        </Modal>
      )}
    </div>
  );
}
