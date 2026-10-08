"use client";

import Link from "next/link";
import { use, useState } from "react";
import { Banner, Btn, Card, Check, Field, Input, Page, SummaryList, Textarea, Timeline, useToast } from "@/components/ui";

type A = "defer" | "exempt" | "cancel" | "override";
const acts: [A, string, string, string][] = [
  ["defer", "Grace period", "restrictions.defer", "Pause until a date; → release_requested (source: exception)"],
  ["exempt", "Exception", "restrictions.exempt", "Exempt until a date; → release_requested (source: exception)"],
  ["cancel", "Cancel", "restrictions.cancel", "Applied → release_requested (source: cancel). Does not assume nothing was applied"],
  ["override", "Override release", "restrictions.override · needs restriction.override", "Force release; the invoice stays unpaid"],
];

export default function Exception({ params }: { params: Promise<{ id: string }> }) {
  const { id } = use(params);
  const toast = useToast();
  const [act, setAct] = useState<A>("exempt");
  const [until, setUntil] = useState("2026-10-15");
  const [reason, setReason] = useState("Customer disputes the August amount; finance review in progress. Exempt until the review closes.");
  const [hasOverride, setHasOverride] = useState(true);
  const [done, setDone] = useState(false);
  const [tried, setTried] = useState(false);
  const needsDate = act === "defer" || act === "exempt";
  const [openedAt] = useState(() => Date.now()); // reference time for the date check (render stays pure)
  const dateErr = tried && needsDate && (!until || new Date(until).getTime() < openedAt - 86400e3) ? "Date must be in the future" : undefined;
  const run = () => { setTried(true); if (!reason.trim() || dateErr) return; if (act === "override" && !hasOverride) return toast("FORBIDDEN — restriction.override required", "crit"); setDone(true); toast("Action recorded"); };
  return (
    <Page>
      <div className="flex flex-wrap items-center gap-2 text-[13px] text-muted"><Link href="/admin/restrictions" className="font-semibold text-primary">← Back</Link>/ <b className="text-ink">{id}</b></div>
      {done && <Banner tone="ok">State: applied → release_requested; a remove command was sent to unit-limited. Released only after the unit confirms. invoice-overdue-a stays unpaid.</Banner>}
      <div className="split">
        <div className="flex min-w-0 flex-col gap-4">
          <Card title="Choose an action" sub="restrictions.defer / exempt / cancel / override">
            <div className="grid-fluid" style={{ ["--min"as string]: "220px" }}>{acts.map(([k, l, op, d]) => <button key={k} disabled={k === "override" && !hasOverride} onClick={() => setAct(k)} aria-pressed={act === k} className={`rounded-xl border p-3 text-left disabled:opacity-50 ${act === k ? "border-primary bg-primary-soft/60" : "border-line hover:bg-surface2"}`}><b className="text-[13px]">{l}</b><div className="font-mono text-[10px] text-muted">{op}</div><div className="mt-1 text-xs text-muted">{d}</div></button>)}</div>
            {needsDate && <div className="mt-4 max-w-xs"><Field label="Exception until · required, future" error={dateErr} hint="Expiry does not reapply the restriction automatically — conditions are checked again"><Input type="date" value={until} onChange={(e) => setUntil(e.target.value)} /></Field></div>}
            <div className="mt-3"><Field label="Reason · required" error={tried && !reason.trim() ? "A reason is required" : undefined} hint={`${reason.length} / 1000`}><Textarea value={reason} onChange={(e) => setReason(e.target.value)} /></Field></div>
            <div className="mt-3 flex flex-wrap items-center justify-between gap-2"><Check label="demo: I hold restriction.override" checked={hasOverride} onChange={setHasOverride} /><Btn variant={act === "override" ? "danger" : "primary"} disabled={done} onClick={run}>Apply {acts.find((a) => a[0] === act)![1].toLowerCase()}</Btn></div>
          </Card>
          <Card title="What happens"><ul className="list-disc pl-5 text-[13px]"><li>State: applied → release_requested; a remove command is sent to unit-limited</li><li>Released only after the unit confirms (per-unit evidence)</li><li>invoice-overdue-a stays unpaid — billing is not settled</li><li>Before/after, expiry, reason and your name are recorded in the audit log</li></ul></Card>
        </div>
        <div className="flex min-w-0 flex-col gap-4">
          <Card title="Summary"><SummaryList items={[["Policy", "Temperature limit ≥ 24 °C"], ["Units", "unit-limited · applied"], ["Cause", "invoice-overdue-a · unpaid"], ["Grace / exception", done ? `Until ${until}` : "None"], ["Your permissions", hasOverride ? "restriction.write · restriction.override" : "restriction.write only"]]} /></Card>
          <Card title="Audit" sub="audit.read"><Timeline items={[{ time: "09-13 08:00:20", title: "Applied", detail: "unit-limited confirmed setpoint ≥ 24 °C · by system", tone: "warn" }, { time: "09-13 08:00", title: "Executed", detail: "requested · 1 command · rules demo-v1 confirmed · by hq-restriction-manager" }, { time: "09-12 08:00", title: "Scheduled", detail: "notice sent to customer-a · executeAfter 09-13 08:00 · by hq-restriction-manager" }]} /></Card>
        </div>
      </div>
    </Page>
  );
}
