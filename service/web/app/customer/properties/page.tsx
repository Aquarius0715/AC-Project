"use client";

import Link from "next/link";
import { useState } from "react";
import { Banner, Btn, Card, Field, Input, Modal, Page, PowerBadge, SummaryList, Badge, cx, useToast } from "@/components/ui";
import { GroupControl, GUnit } from "@/components/Features";

type Node = { id: string; name: string; type: string; units?: number; children?: Node[]; unassigned?: boolean };
const tree: Node[] = [
  { id: "home-a", name: "Home A", type: "Home", children: [
    { id: "1f", name: "1F", type: "Floor", children: [
      { id: "bedroom", name: "Bedroom", type: "Room", units: 2 },
      { id: "living", name: "Living room", type: "Room", units: 1 },
      { id: "kitchen", name: "Kitchen", type: "Room", units: 1 } ] },
    { id: "2f", name: "2F", type: "Floor", children: [{ id: "study", name: "Study", type: "Room", units: 1 }] },
    { id: "unassigned", name: "Unassigned units", type: "", units: 1, unassigned: true },
  ] },
  { id: "office-a", name: "Office A", type: "Office", children: [
    { id: "open", name: "Open office", type: "Area", children: [{ id: "ws", name: "Workstations", type: "Space", units: 8 }] },
    { id: "3f", name: "3F", type: "Floor", children: [{ id: "meeting", name: "Meeting room", type: "Room", units: 2 }] },
  ] },
];
const roomUnits: Record<string, { id: string; name: string; temp: number; w: number; state: "running" | "stopped" }[]> = {
  bedroom: [{ id: "unit-online-rto", name: "Bedroom AC", temp: 28, w: 680, state: "running" }, { id: "unit-bedroom-2", name: "Bedroom AC #2", temp: 27, w: 0, state: "stopped" }],
  living: [{ id: "unit-non-rto", name: "Living room AC", temp: 26.5, w: 510, state: "running" }],
  kitchen: [{ id: "unit-kitchen-a", name: "Kitchen AC", temp: 27.2, w: 545, state: "running" }],
};
const groupUnits: Record<string, GUnit[]> = {
  ws: Array.from({ length: 8 }, (_, i) => ({ id: `ws-ac-0${i + 1}`, name: `WS-AC 0${i + 1}`, temp: i === 2 || i === 7 ? null : 24 + (i % 3) * 0.5, w: i === 2 || i === 7 ? null : 580 + i * 10, state: (i === 5 ? "unknown" : i === 2 ? "stopped" : "running") as GUnit["state"], online: i !== 5 })),
  bedroom: [{ id: "unit-online-rto", name: "Bedroom AC", temp: 28, w: 680, state: "running", online: true }, { id: "unit-bedroom-2", name: "Bedroom AC #2", temp: 27, w: 0, state: "stopped", online: true }],
  meeting: [{ id: "unit-meeting", name: "Meeting room AC", temp: 25, w: 600, state: "running", online: true, min: 24 }, { id: "unit-meeting-2", name: "Meeting AC 2", temp: 25.5, w: 590, state: "running", online: true }],
};

function TreeRow({ n, depth, sel, onSel, onRename }: { n: Node; depth: number; sel: string; onSel: (id: string) => void; onRename: (n: Node) => void }) {
  return (
    <>
      <div className={cx("flex items-center gap-2 rounded-lg px-2 py-1.5", sel === n.id ? "bg-primary-soft" : "hover:bg-surface2")} style={{ paddingLeft: 8 + depth * 16 }}>
        <button onClick={() => onSel(n.id)} className="flex min-w-0 flex-1 items-center gap-2 text-left">
          <span className="text-muted">{n.children ? "▾" : n.unassigned ? "?" : "›"}</span>
          <span className="truncate font-semibold">{n.name}</span>
          {n.type && <span className="rounded bg-surface2 px-1.5 text-[10px] text-muted">{n.type}</span>}
          {n.units !== undefined && <span className="text-[11px] text-muted">· {n.units} unit{n.units > 1 ? "s" : ""}</span>}
        </button>
        {!n.unassigned && <button aria-label={`Rename ${n.name}`} onClick={() => onRename(n)} className="text-muted hover:text-primary">✎</button>}
      </div>
      {n.children?.map((c) => <TreeRow key={c.id} n={c} depth={depth + 1} sel={sel} onSel={onSel} onRename={onRename} />)}
    </>
  );
}

export default function Properties() {
  const toast = useToast();
  const [sel, setSel] = useState("home-a");
  const [renaming, setRenaming] = useState<Node | null>(null);
  const [name, setName] = useState("");
  const [err, setErr] = useState<string>();
  const [mode, setMode] = useState<"single" | "group">("single");
  const startRename = (n: Node) => { setRenaming(n); setName(n.name); setErr(undefined); };
  const save = () => {
    if (!name.trim()) return setErr("Name is required (1–120 characters)");
    if (name.trim().toLowerCase() === "office a" && renaming?.id !== "office-a") return setErr("A location with this name already exists");
    toast(`Renamed to “${name}”`); setRenaming(null);
  };
  const us = roomUnits[sel];
  return (
    <Page>
      <div className="split-rev">
        <Card title="My properties" action={<Badge tone="muted">2</Badge>} className="self-start">
          <div className="flex flex-col text-[13px]">{tree.map((n) => <TreeRow key={n.id} n={n} depth={0} sel={sel} onSel={(id) => { setSel(id); setMode("single"); }} onRename={startRename} />)}</div>
          <p className="mt-3 text-[11px] text-muted">Read-only — HQ sets up properties, floors, rooms and units. You can rename any location with ✎ or Rename; nothing else here changes the structure.</p>
        </Card>
        <Card title={sel === "ws" ? "Office A › Open office › Workstations" : sel === "meeting" ? "Office A › 3F › Meeting room" : sel === "home-a" || !us ? (sel === "unassigned" ? "Unassigned units (1)" : "Home A") : "Home A › 1F › " + sel[0].toUpperCase() + sel.slice(1)} sub={sel === "home-a" ? "Home A · ⌂ Home" : undefined} action={sel !== "unassigned" && <Btn size="sm" onClick={() => startRename(tree[0])}>✎ Rename</Btn>}>
          {sel === "home-a" && (
            <>
              <SummaryList cols={2} items={[["Type", "Home"], ["Floors", "2 (1F, 2F)"], ["Rooms", "4"], ["Air conditioners", "5 in rooms · 1 unassigned"], ["Last edited", "Sep 12 · version 3"]]} />
              <div className="my-4 rounded-xl bg-surface2 p-4 text-center"><div className="font-bold">Select a room to see its air conditioners</div><div className="text-xs text-muted">Floor → Room → Air conditioner. Each room lists every AC inside it; open one to control it.</div></div>
            </>
          )}
          {groupUnits[sel] && (
            <div className="mb-3 flex flex-wrap items-center gap-2"><span className="text-xs text-muted">{groupUnits[sel].length} ACs ·</span>{(["single", "group"] as const).map((m) => <button key={m} aria-pressed={mode === m} onClick={() => setMode(m)} className={cx("rounded-control border px-3 py-1 text-xs font-semibold", mode === m ? "border-primary bg-primary-soft text-primary" : "border-line")}>{mode === m ? "✓ " : ""}{m === "single" ? "Single AC" : "Group control"}</button>)}</div>
          )}
          {groupUnits[sel] && mode === "group" && <GroupControl space={sel === "ws" ? "Workstations" : sel === "meeting" ? "Meeting room" : "Bedroom"} units={groupUnits[sel]} />}
          {sel === "ws" && mode === "single" && <div className="flex flex-col gap-2">{groupUnits.ws.map((u) => <Link key={u.id} href="/customer/units/unit-online-rto" className="flex items-center justify-between rounded-xl border border-line p-3 hover:bg-surface2/60"><b>❄ {u.name}</b><span className="flex items-center gap-3 text-xs"><PowerBadge s={u.state} />{u.temp ?? "—"} °C ›</span></Link>)}</div>}
          {us && mode === "single" && (
            <div className="flex flex-col gap-2">
              {us.map((u) => (
                <Link key={u.id} href={`/customer/units/${u.id}`} className="flex flex-wrap items-center justify-between gap-2 rounded-xl border border-line p-3 hover:bg-surface2/60"><span className="flex items-center gap-2 font-bold"><span className="text-primary">❄</span>{u.name}</span><span className="flex items-center gap-3 text-xs"><PowerBadge s={u.state} /><span>{u.temp} °C</span><span>{u.w} W</span><span className="text-muted">›</span></span></Link>
              ))}
            </div>
          )}
          {sel !== "home-a" && !us && !groupUnits[sel] && sel !== "unassigned" && <p className="text-[13px] text-muted">{sel === "study" ? "Study AC is offline (unknown)." : "Select a room with air conditioners to see them here."}</p>}
          {(sel === "home-a" || sel === "unassigned") && (
            <div className="mt-2">
              <div className="mb-2 text-[13px] font-bold">? Unassigned units (1) <span className="text-xs font-normal text-muted">Not in any room</span></div>
              <Link href="/customer/units/unit-guest" className="flex items-center justify-between rounded-xl border border-line p-3 hover:bg-surface2/60"><span><b>❄ Guest AC</b><span className="block text-[11px] text-muted">Home A · no room assigned</span></span><span className="text-xs">25.5 °C · 0 W ›</span></Link>
              <p className="mt-2 text-[11px] text-muted">Moving a unit into a room is done in HQ’s unit register — contact HQ.</p>
            </div>
          )}
        </Card>
      </div>
      <Modal open={!!renaming} onClose={() => setRenaming(null)} title={`Rename ${renaming?.name ?? "location"}`} footer={<><Btn onClick={() => setRenaming(null)}>Cancel</Btn><Btn variant="primary" onClick={save}>Save</Btn></>}>
        <Field label="Name" error={err} hint="Try “Office A” to see the duplicate-name validation."><Input value={name} onChange={(e) => setName(e.target.value)} autoFocus /></Field>
      </Modal>
    </Page>
  );
}
