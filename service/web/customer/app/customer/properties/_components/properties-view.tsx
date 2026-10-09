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
const plural = (n: number, w: string) => `${n} ${w}${n === 1 ? "" : "s"}`;
type Renaming = { kind: "property" | "space"; id: string; version: number; name: string; where: string };

/** Units & locations (FR-C02) in API mode: the structure is read only (HQ manages it) except renaming; a room lists its
 * air conditioners, and owners can send one change to several of them (Group control, FR-C14). */
export function PropertiesView({ live }: { live: PropertiesLive }) {
  const nav = useUrlPatch();
  const [renaming, setRenaming] = useState<Renaming | null>(null);
  const sel = live.selection;
  const selId = !sel ? "" : sel.kind === "property" ? sel.property.id : sel.kind === "space" ? sel.space.id : `unassigned:${sel.property.id}`;
  const go = (patch: Record<string, string | null>) => nav({ propertyId: null, spaceId: null, unassignedOnly: null, mode: null, unitIds: null, powerState: null, connections: null, ...patch });
  if (live.tree.length === 0) return <Page><EmptyState title="No properties yet">HQ registers your homes and offices with their rooms and air conditioners; they appear here once set up.</EmptyState></Page>;
  return (
    <Page>
      <div className="split-rev">
        <Card title="My properties" action={<Badge tone="muted">{live.tree.length}</Badge>} className="self-start">
          <div className="flex flex-col text-[13px]">
            {live.tree.map((p) => (
              <div key={p.id}>
                <Row depth={0} selected={!live.filter && selId === p.id} onClick={() => go({ propertyId: p.id })} icon="⌂" name={p.name} tag={kindLabel[p.kind]}
                  onRename={() => setRenaming({ kind: "property", id: p.id, version: p.version, name: p.name, where: `${p.name} · ${kindLabel[p.kind]} (property)` })} />
                {p.spaces.map((s) => <SpaceRow key={s.id} s={s} p={p} depth={1} selId={live.filter ? "" : selId} go={go} rename={setRenaming} />)}
                {p.unassigned > 0 && <Row depth={1} selected={!live.filter && selId === `unassigned:${p.id}`} onClick={() => go({ propertyId: p.id, unassignedOnly: "true" })} icon="?" name="Unassigned units" meta={`· ${plural(p.unassigned, "unit")} · placed by HQ`} />}
              </div>
            ))}
          </div>
          <p className="mt-3 text-[11px] text-muted">Read-only — HQ sets up properties, floors, rooms and units. You can rename any location with ✎ or Rename; nothing else here changes the structure.</p>
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
  return (
    <div className={cx("flex items-center gap-2 rounded-lg px-2 py-1.5", selected ? "bg-primary-soft" : "hover:bg-surface2")} style={{ paddingLeft: 8 + depth * 16 }}>
      <button type="button" onClick={onClick} aria-pressed={selected} className="flex min-w-0 flex-1 items-center gap-2 text-left">
        <span aria-hidden className="text-muted">{icon}</span><span className="truncate font-semibold">{name}</span>
        {tag && <span className="rounded bg-surface2 px-1.5 text-[10px] text-muted">{tag}</span>}
        {meta && <span className="truncate text-[11px] text-muted">{meta}</span>}
      </button>
      {onRename && <button type="button" aria-label={`Rename ${name}`} onClick={onRename} className="text-muted hover:text-primary">✎</button>}
    </div>
  );
}
function SpaceRow({ s, p, depth, selId, go, rename }: { s: TreeSpace; p: TreeProperty; depth: number; selId: string; go: (x: Record<string, string | null>) => void; rename: (r: Renaming) => void }) {
  return (
    <>
      <Row depth={depth} selected={selId === s.id} onClick={() => go({ spaceId: s.id })} icon={s.children.length ? "▾" : "›"} name={s.name} tag={kindLabel[s.kind]}
        meta={s.direct ? `· ${plural(s.direct, "unit")}` : undefined} onRename={() => rename({ kind: "space", id: s.id, version: s.version, name: s.name, where: `${s.name} · ${kindLabel[s.kind]} in ${p.name}` })} />
      {s.children.map((c) => <SpaceRow key={c.id} s={c} p={p} depth={depth + 1} selId={selId} go={go} rename={rename} />)}
    </>
  );
}

function UnitRows({ units }: { units: (RoomUnit & { place: string })[] }) {
  return (
    <div className="flex flex-col gap-2">
      {units.map((u) => (
        <Link key={u.id} href={`/customer/units/${u.id}`} className="flex flex-wrap items-center justify-between gap-2 rounded-xl border border-line p-3 hover:bg-surface2/60">
          <span className="min-w-0"><span className="flex items-center gap-2 font-bold"><span aria-hidden className="text-primary">❄</span>{u.name}</span><span className="block text-[11px] text-muted">{u.set} · {u.place}</span></span>
          <span className="flex flex-wrap items-center gap-3 text-xs"><span>{u.temp}</span><span>{u.watts}</span><PowerBadge s={u.power} /><ConnBadge s={u.conn} />{!u.online && u.seen && <span className="text-muted">seen {u.seen}</span>}<span className="font-semibold text-primary">Open control →</span></span>
        </Link>
      ))}
    </div>
  );
}

function FilteredUnits({ live, clear }: { live: PropertiesLive; clear: () => void }) {
  const f = live.filter!;
  const label = [f.powerState && `Power: ${powerLabel[f.powerState]}`, f.connections.length > 0 && `Connection: ${f.connections.join(", ")}`].filter(Boolean).join(" · ");
  return (
    <Card title={`Air conditioners · ${label}`} sub={`${live.units.length} of ${f.total}`} action={<Btn size="sm" onClick={clear}>Clear filter</Btn>}>
      {live.units.length ? <UnitRows units={live.units} /> : <EmptyState title="No air conditioner matches">Clear the filter to see every unit.</EmptyState>}
    </Card>
  );
}

function Panel({ live, sel, rename, go }: { live: PropertiesLive; sel: Selection; rename: (r: Renaming) => void; go: (x: Record<string, string | null>) => void }) {
  const p = sel.property;
  if (sel.kind === "property") {
    const s = live.summary!;
    return (
      <>
        <div className="text-[13px] text-muted">My properties › <b className="text-ink">{p.name}</b></div>
        <Card title={<span className="flex items-center gap-2">{p.name}<Badge tone="muted">⌂ {kindLabel[p.kind]}</Badge></span>}
          action={<Btn size="sm" onClick={() => rename({ kind: "property", id: p.id, version: p.version, name: p.name, where: `${p.name} · ${kindLabel[p.kind]} (property)` })}>✎ Rename</Btn>}>
          <SummaryList cols={2} items={[["Type", kindLabel[s.kind]], ["Floors", s.floors.length ? `${s.floors.length} (${s.floors.join(", ")})` : "none"], ["Rooms", s.rooms], ["Air conditioners", `${s.inRooms} in rooms · ${s.unassigned} unassigned`], ["Last edited", s.edited]]} />
          <div className="my-4 rounded-xl bg-surface2 p-4 text-center"><div className="font-bold">Select a room to see its air conditioners</div><div className="text-xs text-muted">Floor → Room → Air conditioner. Each room lists every AC inside it; open one to control it.</div></div>
          {live.units.length > 0 && <>
            <div className="mb-2 text-[13px] font-bold">? Unassigned units ({live.units.length}) <span className="text-xs font-normal text-muted">Not in any room</span></div>
            <UnitRows units={live.units} />
            <p className="mt-2 text-[11px] text-muted">Moving a unit into a room is done in HQ’s unit register — contact HQ.</p>
          </>}
        </Card>
      </>
    );
  }
  if (sel.kind === "unassigned") {
    return (
      <Card title={`Unassigned units (${live.units.length})`} sub={`${p.name} · not in any room — placed by HQ`}>
        <UnitRows units={live.units} />
        <p className="mt-2 text-[11px] text-muted">Moving a unit into a room is done in HQ’s unit register — contact HQ.</p>
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
      <div className="text-[13px] text-muted">My properties › {sel.path.split(" › ").map((x, i, a) => <span key={i}>{i === a.length - 1 ? <b className="text-ink">{x}</b> : `${x} › `}</span>)}</div>
      <Card title={<span className="flex items-center gap-2">{s.name}<Badge tone="muted">{kindLabel[s.kind]}</Badge></span>}
        action={<Btn size="sm" onClick={() => rename({ kind: "space", id: s.id, version: s.version, name: s.name, where: `${s.name} · ${kindLabel[s.kind]} in ${p.name}` })}>✎ Rename</Btn>}>
        {canGroup && (
          <div className="mb-3 flex flex-wrap items-center gap-2">
            <span className="text-xs text-muted">{plural(direct.length, "AC")} in {s.name} ·</span>
            {(["single", "group"] as const).map((m) => <button key={m} type="button" aria-pressed={mode === m} onClick={() => go({ spaceId: s.id, mode: m === "group" ? "group" : null })}
              className={cx("rounded-control border px-3 py-1 text-xs font-semibold", mode === m ? "border-primary bg-primary-soft text-primary" : "border-line")}>{mode === m ? "✓ " : ""}{m === "single" ? "Single AC" : "Group control"}</button>)}
          </div>
        )}
        {mode === "group" ? <GroupPanel key={s.id} live={{ ...live, units: direct, details: live.details.filter((d) => d.spaceId === s.id) }} space={s.name} /> : live.units.length === 0 ? <p className="text-[13px] text-muted">No air conditioner in this {kindLabel[s.kind].toLowerCase()}.</p> : <>
          <div className="mb-2 flex items-center justify-between text-[13px]"><b>Air conditioners in this {kindLabel[s.kind].toLowerCase()} ({live.units.length})</b></div>
          <UnitRows units={live.units} />
          <div className="mt-3"><Banner>Each AC has its own control screen{live.owner ? "; Group control sends one setting to several ACs in a space" : ""}. Unit membership is managed in HQ’s unit register.</Banner></div>
        </>}
      </Card>
    </>
  );
}

function RenameModal({ r, onClose }: { r: Renaming; onClose: () => void }) {
  const [pending, run] = useAction();
  const [name, setName] = useState(r.name);
  const [err, setErr] = useState<string | undefined>();
  const save = () => {
    const e = nameError(name);
    if (e) return setErr(e);
    run(() => renameLocation(r.kind, r.id, r.version, name), `Renamed to “${name.trim()}”`, onClose, (f) => {
      if (f.messageKey === "error.duplicateSiblingName") setErr(`“${name.trim()}” already exists there — choose another name. Your input is kept.`);
      else if (f.messageKey === "error.versionConflict") setErr("Someone renamed it in the meantime — the tree now shows the current name. Your input is kept.");
      else setErr(undefined);
    });
  };
  return (
    <Modal open onClose={onClose} title={`Rename ${r.name}`} footer={<><Btn onClick={onClose}>Cancel</Btn><Btn variant="primary" disabled={pending || name.trim() === r.name} onClick={save}>Save name</Btn></>}>
      <div className="flex flex-col gap-3">
        <p className="text-[13px] text-muted">Only the name changes. Floors, rooms and units stay as they are — HQ manages the structure.</p>
        <SummaryList items={[["Location", r.where]]} />
        <Field label="New name" hint="1–120 characters, unique among its siblings. HQ and technicians see the new name too." error={err}><Input value={name} onChange={(e) => { setName(e.target.value); setErr(undefined); }} autoFocus /></Field>
      </div>
    </Modal>
  );
}

// ---- group control (FR-C14) ----

type Track = { status: "Sending" | "Confirmed" | "Failed"; note: string };
function GroupPanel({ live, space }: { live: PropertiesLive; space: string }) {
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
        if (!created.ok) return setTrack((t) => ({ ...t, [r.id]: { status: "Failed", note: created.messageKey.replace(/^errors?\./, "").replace(/_/g, " ") } }));
        const c: ApiCommand = await waitForCommand(created.value, () => {});
        if (c.status !== "acknowledged") return setTrack((t) => ({ ...t, [r.id]: { status: "Failed", note: historyRow(c).text } }));
      }
      setTrack((t) => ({ ...t, [r.id]: { status: "Confirmed", note: r.change } }));
    }));
    setSending(false);
    toast("Group change finished — results are shown per AC");
    router.refresh(); // re-read the units (observed settings)
  };
  const plan = (ids: string[]) => groupPlan(live.details.filter((d) => ids.includes(d.id)), ch);
  return (
    <div className="flex flex-col gap-3">
      <div className="flex flex-wrap items-center justify-between gap-2 rounded-xl bg-surface2 px-3 py-2 text-[13px]"><b>{sel.length} selected</b>
        <span className="flex gap-2"><Btn size="sm" onClick={() => setSel(online)}>Select all online ({online.length})</Btn><Btn size="sm" variant="ghost" onClick={() => setSel([])}>Clear</Btn></span></div>
      <div className="grid-fluid" style={{ ["--min" as string]: "170px" }}>
        {live.units.map((u) => {
          const t = track[u.id];
          return (
            <button key={u.id} type="button" disabled={sending} onClick={() => toggle(u.id)} aria-pressed={sel.includes(u.id)}
              className={cx("rounded-xl border p-3 text-left disabled:opacity-60", sel.includes(u.id) ? "border-2 border-primary bg-primary-soft/50" : "border-line bg-surface")}>
              <div className="flex items-center justify-between"><b className="text-[13px]">{u.name}</b><span aria-hidden className="text-xs">{sel.includes(u.id) ? "☑" : "☐"}</span></div>
              <div className="text-[11px] text-muted">{!u.online ? (u.seen ? `last seen ${u.seen}` : "no connection yet") : u.temp === "—" && u.watts === "—" ? "no recent reading" : `${u.temp} · ${u.watts}`}</div>
              <div className="mt-1 flex flex-wrap gap-1"><PowerBadge s={u.power} /><ConnBadge s={u.conn} />{t && <Badge tone={t.status === "Confirmed" ? "ok" : t.status === "Failed" ? "crit" : "primary"}>{t.status}</Badge>}</div>
              {t?.status === "Failed" && <div className="mt-1 text-[11px] text-crit">{t.note}</div>}
            </button>
          );
        })}
      </div>
      <Card title={`Apply to ${sel.length} selected AC${sel.length === 1 ? "" : "s"}`}>
        <div className="flex flex-wrap items-center gap-4">
          <Field label="Power"><Choice value={ch.power ? "on" : "off"} onChange={(v: "on" | "off") => setCh({ ...ch, power: v === "on" })} options={[{ id: "on", label: "On" }, { id: "off", label: "Off" }]} /></Field>
          {ch.power && <>
            <Field label="Set temperature"><div className="flex items-center gap-2"><Btn size="sm" onClick={() => setCh({ ...ch, celsius: Math.max(lo, ch.celsius - 1) })}>−</Btn><b className="w-14 text-center">{ch.celsius} °C</b><Btn size="sm" onClick={() => setCh({ ...ch, celsius: Math.min(hi, ch.celsius + 1) })}>+</Btn></div></Field>
            <Field label="Mode"><Choice value={ch.mode} onChange={(v: GroupChange["mode"]) => setCh({ ...ch, mode: v })} options={[{ id: "cool", label: "Cool" }, { id: "dry", label: "Dry" }, { id: "fan", label: "Fan" }]} /></Field>
            <Field label="Fan"><Choice value={ch.fan ?? "keep"} onChange={(v: string) => setCh({ ...ch, fan: v === "keep" ? null : (v as GroupChange["fan"]) })} options={[{ id: "keep", label: "Keep" }, { id: "low", label: "Low" }, { id: "mid", label: "Mid" }, { id: "high", label: "High" }]} /></Field>
          </>}
          <Btn variant="primary" disabled={!sel.length || sending} onClick={() => setReview(plan(sel))}>Review & send</Btn>
        </div>
        <p className="mt-2 text-[11px] text-muted">Each AC gets its own command and confirmation (same as single control). Offline ACs are skipped; restrictions such as a minimum temperature still apply per AC. One space only · owner role only.</p>
        {Object.values(track).some((t) => t.status === "Failed") && !sending && (
          <div className="mt-2"><Btn size="sm" onClick={() => { const ids = Object.entries(track).filter(([, t]) => t.status === "Failed").map(([id]) => id); setReview(plan(ids)); }}>Retry failed ACs</Btn></div>
        )}
      </Card>
      {review && (
        <Modal open wide onClose={() => setReview(null)} title={`Send to ${review.length} selected AC${review.length === 1 ? "" : "s"} in ${space}?`}
          footer={<><Btn onClick={() => setReview(null)}>Cancel</Btn><Btn variant="primary" disabled={!review.some((r) => r.actions.length)} onClick={() => send(review.filter((r) => r.actions.length))}>Send to {review.filter((r) => r.actions.length).length} AC{review.filter((r) => r.actions.length).length === 1 ? "" : "s"}</Btn></>}>
          <p className="mb-2 text-[13px] font-semibold">{changeText(ch)}</p>
          <DataTable rowKey={(r) => r.id} rows={review} cols={[
            { key: "n", label: "AC", render: (r) => <b>{r.name}</b> },
            { key: "c", label: "Change", render: (r) => <>{r.change}{r.note && <div className="text-[11px] text-warn">{r.note}</div>}</> },
            { key: "r", label: "Result", render: (r) => <Badge tone={r.result === "Skipped" ? "unknown" : r.result === "Clamped" ? "warn" : r.result === "No change" ? "muted" : "primary"}>{r.result}</Badge> },
          ]} />
          <div className="mt-2"><Banner>After sending, each card shows Sending → Confirmed / Failed. Failed ACs can be retried one by one; there is no rollback across ACs.</Banner></div>
        </Modal>
      )}
    </div>
  );
}
