"use client";

import { useState } from "react";
import { Badge, Banner, Btn, Card, Choice, EmptyState, Field, Input, Modal, OnOffBadge, Page, PageHead, Select, SummaryList, Toggle, cx, useToast } from "@ac/web/components/ui";
import { Icon, type IconName } from "@ac/web/components/Icon";
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from "@ac/web/components/ui/dropdown-menu";
import { Button } from "@ac/web/components/ui/button";

/** One “Then” row: a schedule's start and end at their times, or an event rule's action (as the live cards). */
type Step = { label: string; time: string | null; action: string };
type Auto = { id: string; name: string; kind: string; icon: IconName; when: string; steps: Step[]; unit: string; note?: string; on: boolean };
const start = (time: string, action: string): Step => ({ label: "Start", time, action });
const end = (time: string, action: string): Step => ({ label: "End", time, action });
const then = (action: string): Step[] => [{ label: "Then", time: null, action }];
const seed: Auto[] = [
  { id: "a1", name: "Weekday pre-cool", kind: "Schedule", icon: "schedule", when: "Mon, Wed at 18:00 (Asia/Kuala_Lumpur)", steps: [start("18:00", "Set temperature 25°C"), end("22:00", "Power OFF")], unit: "Bedroom AC · Home A › 1F › Bedroom · next run Mon Sep 28 18:00", on: true },
  { id: "a2", name: "Away power-save", kind: "Location", icon: "location", when: "Everyone leaves Home A (departure event)", steps: then("Set temperature 28°C"), unit: "Living room AC · Home A › 1F", on: true },
  { id: "a3", name: "Night comfort", kind: "Schedule", icon: "schedule", when: "Every day 23:00 → 06:00 next day (overnight)", steps: [start("23:00", "Set temperature 26°C, fan Low"), end("06:00 next day", "Power OFF")], unit: "Bedroom AC #2 · next run tonight 23:00", on: true },
  { id: "a4", name: "Room empty stop", kind: "Presence", icon: "presence", when: "No one detected for 30 min (demo occupancy)", steps: then("Power OFF"), unit: "Study AC · Home A › 2F", note: "Last evaluation skipped 08:30 — occupancy data missing (never counted as a match)", on: true },
  { id: "a5", name: "Hot afternoon pre-cool", kind: "Weather", icon: "weather", when: "Outdoor temperature ≥ 33°C (demo weather) · Only if: weekday", steps: then("Set temperature 24°C"), unit: "Living room AC", on: false },
  { id: "a6", name: "Vacation mode", kind: "Location", icon: "location", when: "Arrival at Home A", steps: then("Set temperature 25°C"), unit: "Bedroom AC", on: false },
];

/** The Phase 1A demo of the automations screen (fixtures; DATA_SOURCE=api uses AutomationsView). */
export function AutomationsDemo() {
  const toast = useToast();
  const [list, setList] = useState(seed);
  const [consent, setConsent] = useState(true);
  const [edit, setEdit] = useState<Auto | "new" | null>(null);
  const [del, setDel] = useState<Auto | null>(null);
  const [step, setStep] = useState<"form" | "review">("form");
  const [f, setF] = useState({ name: "", kind: "Schedule", time: "18:00", temp: "25", unit: "Bedroom AC" });
  const [err, setErr] = useState<Record<string, string>>({});

  const open = (a: Auto | "new") => { setEdit(a); setStep("form"); setErr({}); setF(a === "new" ? { name: "", kind: "Schedule", time: "18:00", temp: "25", unit: "Bedroom AC" } : { name: a.name, kind: a.kind, time: "18:00", temp: "25", unit: a.unit.split(" · ")[0] }); };
  const review = () => {
    const e: Record<string, string> = {};
    if (!f.name.trim()) e.name = "Name is required";
    const t = +f.temp; if (!(t >= 16 && t <= 30)) e.temp = "16–30°C only (device capability)";
    if (f.kind === "Location" && !consent) e.kind = "Location consent is withdrawn — grant it to use location triggers";
    setErr(e); if (!Object.keys(e).length) setStep("review");
  };
  const save = () => {
    if (edit === "new") setList((l) => [...l, { id: "n" + Date.now(), name: f.name, kind: f.kind, icon: f.kind === "Location" ? "location" : "schedule", when: `${f.kind === "Schedule" ? "Every day at " + f.time : "Arrival at Home A"}`, steps: f.kind === "Schedule" ? [start(f.time, `Set temperature ${f.temp}°C`), end("22:00", "Power OFF")] : then(`Set temperature ${f.temp}°C`), unit: f.unit, on: true }]);
    else if (edit) setList((l) => l.map((x) => (x.id === edit.id ? { ...x, name: f.name, steps: x.steps.map((st, i) => (i === 0 ? { ...st, action: `Set temperature ${f.temp}°C` } : st)), unit: f.unit } : x)));
    toast("Automation saved"); setEdit(null);
  };
  const onCount = list.filter((a) => a.on).length;
  return (
    <Page>
      <PageHead title="Your automations" sub={`${list.length} automations · ${onCount} on · each rule controls one AC · checked again when it runs`} action={<Btn variant="primary" onClick={() => open("new")}><Icon name="add" /> Create automation</Btn>} />
      <Card>
        <div className="flex flex-wrap items-center justify-between gap-3">
          <div><div className="flex items-center gap-1.5 font-bold"><Icon name="location" />Location consent — for arrival/departure automations only</div><div className="text-xs text-muted-foreground">{consent ? "Granted Sep 10, 2026 · used only by 2 location automations · demo location events (no real GPS history)." : "Withdrawn — location automations are paused. Schedule automations are unaffected."}</div></div>
          <Btn size="sm" variant={consent ? "danger" : "primary"} onClick={() => { setConsent(!consent); toast(consent ? "Location consent withdrawn" : "Location consent granted", consent ? "warn" : "ok"); }}>{consent ? "Withdraw consent" : "Grant consent"}</Btn>
        </div>
      </Card>
      {list.length === 0 ? <EmptyState title="No automations yet" action={<Btn size="sm" onClick={() => open("new")}><Icon name="add" /> Create automation</Btn>}>Create one to pre-cool before you arrive. Empty is a valid state — not an error.</EmptyState> : (
        <div className="flex flex-col gap-3">
          {list.map((a) => {
            const paused = a.kind === "Location" && !consent;
            return (
              <Card key={a.id} className={cx(!a.on && "opacity-80")}>
                <div className="flex flex-wrap items-start justify-between gap-3">
                  <div className="min-w-0 flex-1">
                    <div className="flex flex-wrap items-center gap-2"><b className="text-[15px]">{a.name}</b><Badge tone="primary" icon={a.icon}>{a.kind}</Badge>{paused && <Badge tone="warn">Paused — consent withdrawn</Badge>}</div>
                    <p className="mt-2 text-sm"><span className="mr-1.5 font-semibold text-muted-foreground">When</span>{a.when}</p>
                    <dl className="mt-2 grid grid-cols-[auto_auto_minmax(0,1fr)] items-baseline gap-x-3 gap-y-1.5 rounded-lg bg-surface2 px-3 py-2 text-sm">
                      {a.steps.map((st) => (
                        <div key={st.label} className="contents">
                          <dt className="font-semibold text-muted-foreground">{st.label}</dt>
                          <dd className="font-semibold tabular-nums">{st.time ?? ""}</dd>
                          <dd className="min-w-0 break-words font-medium text-ink">{st.action}</dd>
                        </div>
                      ))}
                    </dl>
                    <p className="mt-2 text-xs text-muted-foreground">{a.unit}</p>
                    {a.note && <p className="mt-1 flex items-center gap-1 text-xs text-warn"><Icon name="info" />{a.note}</p>}
                  </div>
                  <div className="relative flex items-center gap-2">
                    <OnOffBadge on={a.on && !paused} />
                    <Toggle on={a.on} onChange={(v) => setList((l) => l.map((x) => (x.id === a.id ? { ...x, on: v } : x)))} label={`Toggle ${a.name}`} />
                    <Btn size="sm" onClick={() => open(a)}>Edit</Btn>
                    <DropdownMenu>
                      <DropdownMenuTrigger render={<Button variant="ghost" size="icon-sm" aria-label="More" />}><Icon name="more" /></DropdownMenuTrigger>
                      <DropdownMenuContent align="end" className="min-w-40"><DropdownMenuItem variant="destructive" onClick={() => setDel(a)}><Icon name="delete" /> Delete…</DropdownMenuItem></DropdownMenuContent>
                    </DropdownMenu>
                  </div>
                </div>
              </Card>
            );
          })}
        </div>
      )}
      <p className="text-xs text-muted-foreground">Priority when rules overlap: device capabilities & active restrictions → HQ policy → your automations. List order is not run priority. Missing condition data never counts as a match.</p>

      <Modal open={!!edit} onClose={() => setEdit(null)} wide title={step === "review" ? "Review / Test result" : edit === "new" ? "Create automation" : "Edit automation"} footer={step === "form" ? <><Btn onClick={() => setEdit(null)}>Cancel</Btn><Btn variant="primary" onClick={review}>Review</Btn></> : <><Btn onClick={() => setStep("form")}><Icon name="arrowLeft" /> Back</Btn><Btn variant="primary" onClick={save}>Save automation</Btn></>}>
        {step === "form" ? (
          <>
            <Field label="Name" error={err.name}><Input value={f.name} onChange={(e) => setF({ ...f, name: e.target.value })} placeholder="e.g. Weekday pre-cool" /></Field>
            <Field label="Trigger" error={err.kind}><Choice value={f.kind as "Schedule"} onChange={(v) => setF({ ...f, kind: v })} options={["Schedule", "Location", "Presence", "Weather"].map((k) => ({ id: k as "Schedule", label: k }))} /></Field>
            <div className="grid-fluid" style={{ ["--min" as string]: "160px" }}>
              <Field label="Air conditioner"><Select value={f.unit} onChange={(e) => setF({ ...f, unit: e.target.value })}>{["Bedroom AC", "Bedroom AC #2", "Living room AC", "Kitchen AC", "Study AC"].map((u) => <option key={u}>{u}</option>)}</Select></Field>
              {f.kind === "Schedule" && <Field label="Time"><Input type="time" value={f.time} onChange={(e) => setF({ ...f, time: e.target.value })} /></Field>}
              <Field label="Set temperature (°C)" error={err.temp}><Input type="number" value={f.temp} onChange={(e) => setF({ ...f, temp: e.target.value })} /></Field>
            </div>
            {f.kind === "Location" && <Banner tone={consent ? "ok" : "warn"}>{consent ? "Location consent granted — demo location events only." : "Location consent withdrawn."}</Banner>}
          </>
        ) : (
          <>
            <Banner tone="ok">Simulation passed — no commands were sent.</Banner>
            <SummaryList items={[["Name", f.name], ["Trigger", f.kind], ["Target", f.unit], ["Action", `Set temperature ${f.temp}°C`], ["Next runs", "Mon Sep 28 18:00 · Wed Sep 30 18:00 · Mon Oct 5 18:00"]]} />
            <p className="text-xs text-muted-foreground">Conditions are checked again each time the rule runs. Active restrictions and device capabilities take priority.</p>
          </>
        )}
      </Modal>
      <Modal open={!!del} onClose={() => setDel(null)} title="Delete automation?" footer={<><Btn onClick={() => setDel(null)}>Cancel</Btn><Btn variant="danger" onClick={() => { setList((l) => l.filter((x) => x.id !== del!.id)); toast(`“${del!.name}” removed`); setDel(null); }}>Delete</Btn></>}>
        <p className="text-[13px]">“{del?.name}” will stop running. Past command history is kept.</p>
      </Modal>
    </Page>
  );
}
