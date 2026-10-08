"use client";

import Link from "next/link";
import { use, useState } from "react";
import { Badge, Banner, Btn, Card, Field, Modal, Page, SummaryList, Textarea, useToast } from "@ac/web/components/ui";

export default function Review({ params }: { params: Promise<{ id: string }> }) {
  const { id } = use(params);
  const toast = useToast();
  const [state, setState] = useState<"pending" | "accepted" | "returned">("pending");
  const [ret, setRet] = useState(false);
  const [why, setWhy] = useState("");
  const [tried, setTried] = useState(false);
  const checks: [string, string, string][] = [["Indoor unit — filter condition", "Normal", "ok"], ["Outdoor unit — refrigerant pressure", "Normal", "ok"], ["Electrical — wiring insulation", "Not applicable", "muted"], ["Drainage — condensate flow", "Normal", "ok"]];
  return (
    <Page>
      <div className="text-[13px] text-muted"><Link href="/partner/jobs" className="font-semibold text-primary">← Jobs</Link> › {id} · Quality review</div>
      {state === "accepted" && <Banner tone="ok">Accepted → completed (receipt). Report v1 is now visible to the customer. The linked alert stays open.</Banner>}
      {state === "returned" && <Banner tone="warn">Returned for rework. tech-external-a can resume as v2.</Banner>}
      <div className="split">
        <div className="flex min-w-0 flex-col gap-4">
          <Card title="Submitted report — version 1" sub="tech-external-a · submitted 2026-09-20 16:40">
            <ul className="divide-y divide-line">{checks.map(([t, r, tone]) => <li key={t} className="py-2 text-[13px]"><div className="flex flex-wrap items-center justify-between gap-2"><span>{t}</span><Badge tone={tone as "ok"}>{r}</Badge></div>{r === "Not applicable" && <div className="text-xs text-muted">Reason: unit has no exposed wiring in this installation</div>}</li>)}</ul>
            <h3 className="mt-4 mb-1 text-[13px] font-bold">Photos</h3><div className="grid-fluid" style={{ ["--min"as string]: "110px" }}>{[1, 2, 3].map((n) => <div key={n} className="grid aspect-[4/3] place-items-center rounded-xl bg-surface2 text-xs text-muted">Photo {n}</div>)}</div>
            <h3 className="mt-4 mb-1 text-[13px] font-bold">Work performed & next action</h3><p className="text-[13px]">Cleaned outdoor coil, checked refrigerant pressure (within spec), tested drainage flow. No parts replaced. Next: routine inspection in 6 months.</p>
            <h3 className="mt-4 mb-1 text-[13px] font-bold">Readings recorded</h3><SummaryList items={[["Refrigerant pressure, low side", "4.6 bar · within spec"], ["Supply air temperature", "13.9 °C · good"], ["Condensate flow test", "Pass · 2 min"], ["Time on site", "09:05 – 11:40 (2 h 35 m)"]]} />
          </Card>
        </div>
        <div className="flex min-w-0 flex-col gap-4">
          <Card title="Quality review" sub="Author: tech-external-a · Reviewer: contractor-a (must differ)">
            <p className="mb-2 text-xs text-muted">A contributor to this report version cannot approve it (IR31). Completion does not automatically resolve the linked alert.</p>
            <SummaryList items={[["Required inspection items", "4 / 4 recorded"], ["Not-applicable reasons", "1 / 1 given"], ["Photos", "3 attached"], ["Readings", "4 recorded"], ["Contributors to v1", "tech-external-a only"]]} />
            <p className="my-2 text-xs font-semibold text-ok">✓ All required evidence present — acceptable.</p>
            {state === "pending" && <div className="flex flex-wrap gap-2"><Btn variant="primary" onClick={() => { setState("accepted"); toast("Report accepted"); }}>Accept report</Btn><Btn onClick={() => setRet(true)}>Return for rework…</Btn></div>}
          </Card>
          <Card title="Versions & linked alert"><SummaryList items={[["v1", "submitted 2026-09-20 16:40 by tech-external-a · reviewing now"], ["Vibration anomaly (alert)", "09-13 22:40 · stays open after completion"], ["Job due", "work window 09-18 → 09-23 00:00"]]} /></Card>
        </div>
      </div>
      <Modal open={ret} onClose={() => setRet(false)} title="Return for rework" footer={<><Btn onClick={() => setRet(false)}>Cancel</Btn><Btn variant="primary" onClick={() => { setTried(true); if (!why.trim()) return; setRet(false); setState("returned"); toast("Returned for rework", "warn"); }}>Return report</Btn></>}>
        <Field label="Reason (required)" error={tried && !why.trim() ? "A reason is required" : undefined}><Textarea value={why} onChange={(e) => setWhy(e.target.value)} placeholder="Missing evidence photo for indoor unit filter" /></Field>
      </Modal>
    </Page>
  );
}
