"use client";

import { useState } from "react";
import { Btn, Card, Choice, Field, Page, Select, Tabs, Textarea, Timeline, useToast } from "@ac/web/components/ui";

interface TimelineEvent { t: string; title: string; d: string; c: boolean }
/** A demo event's tone: rework warns, an accepted report is ok, anything else is plain. */
function eventTone(title: string): "warn" | "ok" | undefined {
  if (title.startsWith("Rework")) return "warn";
  return title.includes("accepted") && title.startsWith("Report") ? "ok" : undefined;
}

const events: TimelineEvent[] = [
  { t: "2026-09-21 17:20", title: "Report accepted", d: "Quality review passed · visible to customer", c: true },
  { t: "2026-09-21 16:50", title: "Report v2 submitted", d: "tech-external-a · missing filter photo added", c: false },
  { t: "2026-09-21 14:05", title: "Rework requested — missing evidence", d: "Note: “Missing evidence photo for indoor unit filter” · internal", c: false },
  { t: "2026-09-21 12:30", title: "Report v1 submitted", d: "tech-external-a · 4 inspection items, 2 photos", c: false },
  { t: "2026-09-21 09:05", title: "Work started (checked in)", d: "tech-external-a on site", c: true },
  { t: "2026-09-20 10:00", title: "Schedule changed", d: "New slot 2026-09-21 09:00–11:00 (was 09-20) · customer", c: true },
  { t: "2026-09-18 09:10", title: "Technician assigned", d: "tech-external-a · 2026-09-20 09:00–11:00 · by contractor-a", c: false },
  { t: "2026-09-18 08:30", title: "Job accepted", d: "contractor-a accepted HQ offer", c: false },
  { t: "2026-09-17 16:00", title: "Offer received from HQ", d: "offer-c04-1 · terms v3 · by hq-operator", c: false },
];

/** The Phase 1A demo of the job history (fixture events only; API mode renders HistoryView from the server page). */
export function HistoryDemo() {
  type F = "all" | "customer" | "internal";
  const toast = useToast();
  const [job, setJob] = useState("job-p03");
  const [f, setF] = useState<F>("all");
  const [notice, setNotice] = useState("Schedule moved to 2026-09-22 09:00–11:00 due to technician availability.");
  const [tried, setTried] = useState(false);
  const [role, setRole] = useState("hq");
  const [vis, setVis] = useState<"internal" | "customer">("internal");
  const [previews, setPreviews] = useState([{ k: "quality_rework → technician", d: "in-app · preview · internal · 09-21 14:05" }, { k: "schedule_change → customer", d: "email · preview · customer · 09-20 10:02" }]);
  const shown = events.filter((e) => f === "all" || (f === "customer" ? e.c : !e.c));
  return (
    <Page>
      <div className="flex flex-wrap items-center gap-2"><Select aria-label="Job" className="max-w-xs" value={job} onChange={(e) => { setJob(e.target.value); }}><option value="">Select a job…</option><option value="job-p03">job-p03 · Office AC</option><option value="job-p04">job-p04 · Meeting room AC</option></Select></div>
      {job === "" ? <Card title="Select a job" sub="Choose a job above to see its history." /> : (
        <>
          <Card><div className="grid-fluid text-[13px]" style={{ ["--min"as string]: "150px" }}>{[["UNIT", "Office AC · customer-a"], ["TECHNICIAN", "tech-external-a"], ["STATUS", "Completed"], ["DELEGATION", "ended 09-22 00:00"]].map(([a, b]) => <div key={a}><div className="text-[10px] font-bold tracking-wide text-muted-foreground">{a}</div><b>{b}</b></div>)}</div></Card>
          <div className="split">
            <Card title={`Job history — ${job}`} action={<Tabs value={f} onChange={setF} tabs={[{ id: "all", label: "All events", count: events.length }, { id: "customer", label: "Customer-visible", count: events.filter((e) => e.c).length }, { id: "internal", label: "Internal", count: events.filter((e) => !e.c).length }]} />}>
              <Timeline items={shown.map((e) => ({ time: e.t.slice(5), title: e.title, detail: e.d, tone: eventTone(e.title) }))} />
            </Card>
            <div className="flex min-w-0 flex-col gap-4">
              <Card title="New communication preview">
                <div className="flex flex-col gap-3">
                  <Field label="Template"><Select><option>schedule_change</option><option>quality_rework</option></Select></Field>
                  <Field label="Visibility"><Choice value={vis} onChange={setVis} options={[{ id: "internal", label: "Internal (default)" }, { id: "customer", label: "Customer-visible" }]} /></Field>
                  <Field label="Recipient role"><Select value={role} onChange={(e) => { setRole(e.target.value); }}><option value="hq">hq</option><option value="technician">technician</option><option value="customer">customer</option></Select></Field>
                  <Field label="Message (1–2000 chars)" error={tried && !notice.trim() ? "Message is required" : undefined}><Textarea value={notice} onChange={(e) => { setNotice(e.target.value); }} /></Field>
                  <Btn variant="primary" onClick={() => { setTried(true); if (!notice.trim()) { return; } setPreviews((p) => [{ k: `schedule_change → ${role}`, d: `in-app · preview · ${vis} · just now` }, ...p]); toast("Note saved + preview created"); }}>Save note & create preview</Btn>
                  <p className="text-[11px] text-muted-foreground">No real sending. Free-form external recipients and other-job recipients are rejected. Internal notes stay hidden from customers.</p>
                </div>
              </Card>
              <Card title={`Previews on ${job} (${String(previews.length)})`}>{previews.map((p, i) => <div key={i} className="border-t border-line py-2 text-[13px] first:border-0"><b>{p.k}</b><div className="text-xs text-muted-foreground">{p.d}</div></div>)}<p className="mt-1 text-[11px] text-muted-foreground">deliveryState stays “preview” — nothing is sent in phase 1A.</p></Card>
            </div>
          </div>
        </>
      )}
    </Page>
  );
}
