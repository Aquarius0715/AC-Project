"use client";

/** Features confirmed from the Figma proposals on 2026-10-02 (FR-C14, C16, A18–A20, A23). */
import { useState } from "react";
import { Badge, Banner, Btn, Card, Check, Choice, DataTable, Field, Input, Kpi, ListRow, Modal, PowerBadge, Select, Steps, SummaryList, Toggle, UtilBar, cx, useToast } from "./ui";

/* ───── Client · Energy export (FR-C16) ───── */
export function ExportReportModal({ open, onClose }: { open: boolean; onClose: () => void }) {
  const toast = useToast();
  const [fmt, setFmt] = useState<"pdf" | "csv">("pdf");
  const [monthly, setMonthly] = useState(false);
  return (
    <Modal open={open} onClose={onClose} title="Export report" footer={<><Btn onClick={onClose}>Cancel</Btn><Btn variant="primary" onClick={() => { onClose(); toast(`${fmt === "pdf" ? "PDF" : "CSV"} downloaded (demo file)${monthly ? " · monthly email on" : ""}`); }}>Download {fmt.toUpperCase()}</Btn></>}>
      <div className="grid-fluid" style={{ ["--min" as string]: "180px" }}>
        <Field label="Report"><Select><option>Monthly summary</option><option>Selected period</option></Select></Field>
        <Field label="Month"><Select><option>September 2026</option><option>August 2026</option></Select></Field>
      </div>
      <Field label="Locations"><Select><option>All · Home A, Office A</option><option>Home A</option><option>Office A</option></Select></Field>
      <div className="flex flex-col gap-1.5"><span className="text-xs font-semibold">Include</span>{["Energy & cost by AC (kWh, MYR)", "Comparison with last month", "CO₂ and offsets", "Alerts & maintenance done"].map((l, i) => <Check key={l} label={l} checked={i < 3 ? true : undefined} />)}</div>
      <Field label="Format"><Choice value={fmt} onChange={setFmt} options={[{ id: "pdf", label: "PDF" }, { id: "csv", label: "CSV (raw data)" }]} /></Field>
      <Check label="Email this report to me on the 1st of every month" checked={monthly} onChange={setMonthly} />
      <p className="text-[11px] text-muted">Figures use the same tariff and estimation labels as the screen (estimated values stay marked).</p>
    </Modal>
  );
}

/* ───── Client · Group control (FR-C14, IR109) ───── */
export type GUnit = { id: string; name: string; temp: number | null; w: number | null; state: "running" | "stopped" | "unknown"; online: boolean; min?: number };
export function GroupControl({ space, units }: { space: string; units: GUnit[] }) {
  const toast = useToast();
  const [sel, setSel] = useState<string[]>(units.filter((u) => u.online).slice(0, 4).map((u) => u.id));
  const [power, setPower] = useState<"on" | "off">("on");
  const [temp, setTemp] = useState(24);
  const [mode, setMode] = useState<"cool" | "auto" | "fan">("cool");
  const [review, setReview] = useState(false);
  const toggle = (id: string) => setSel((s) => (s.includes(id) ? s.filter((x) => x !== id) : [...s, id]));
  const chosen = units.filter((u) => sel.includes(u.id));
  return (
    <div className="flex flex-col gap-3">
      <div className="flex flex-wrap items-center justify-between gap-2 rounded-xl bg-surface2 px-3 py-2 text-[13px]"><b>{sel.length} selected</b><span className="flex gap-2"><Btn size="sm" onClick={() => setSel(units.filter((u) => u.online).map((u) => u.id))}>Select all online ({units.filter((u) => u.online).length})</Btn><Btn size="sm" variant="ghost" onClick={() => setSel([])}>Clear</Btn></span></div>
      <div className="grid-fluid" style={{ ["--min" as string]: "170px" }}>
        {units.map((u) => (
          <button key={u.id} disabled={!u.online} onClick={() => toggle(u.id)} aria-pressed={sel.includes(u.id)} className={cx("rounded-xl border p-3 text-left disabled:opacity-60", sel.includes(u.id) ? "border-2 border-primary bg-primary-soft/50" : "border-line bg-surface")}>
            <div className="flex items-center justify-between"><b className="text-[13px]">{u.name}</b><span className="text-xs">{sel.includes(u.id) ? "☑" : "☐"}</span></div>
            <div className="text-[11px] text-muted">{u.temp === null ? "—" : `${u.temp} °C · ${u.w} W`}</div>
            <div className="mt-1 flex gap-1"><PowerBadge s={u.state} />{!u.online && <Badge tone="unknown">Offline</Badge>}{u.min && <Badge tone="warn">min {u.min} °C</Badge>}</div>
          </button>
        ))}
      </div>
      <Card title={`Apply to ${sel.length} selected ACs`}>
        <div className="flex flex-wrap items-center gap-4">
          <Choice value={power} onChange={setPower} options={[{ id: "on", label: "On" }, { id: "off", label: "Off" }]} />
          <div className="flex items-center gap-2"><Btn size="sm" onClick={() => setTemp((t) => Math.max(16, t - 1))}>−</Btn><b className="w-14 text-center">{temp} °C</b><Btn size="sm" onClick={() => setTemp((t) => Math.min(30, t + 1))}>+</Btn></div>
          <Choice value={mode} onChange={setMode} options={[{ id: "cool", label: "Cool" }, { id: "auto", label: "Auto" }, { id: "fan", label: "Fan" }]} />
          <Btn variant="primary" disabled={!sel.length} onClick={() => setReview(true)}>Review & send</Btn>
        </div>
        <p className="mt-2 text-[11px] text-muted">Each AC gets its own command and confirmation (same as single control). One space only · owner role only.</p>
      </Card>
      <Modal open={review} onClose={() => setReview(false)} title={`Send to ${chosen.length} ACs in ${space}?`} footer={<><Btn onClick={() => setReview(false)}>Back</Btn><Btn variant="primary" onClick={() => { setReview(false); toast(`${chosen.length} commands sent — results arrive per AC`); }}>Send {chosen.length} commands</Btn></>}>
        <DataTable rowKey={(r) => r.id} rows={chosen} cols={[{ key: "n", label: "AC", render: (r) => <b>{r.name}</b> }, { key: "c", label: "Change", render: (r) => (r.min && temp < r.min ? <span className="text-warn">Set to {r.min} °C (restriction)</span> : `${power === "on" ? "On" : "Off"} · ${temp} °C · ${mode}`) }]} />
        <Banner>Offline or restricted ACs are skipped or clamped and shown per AC. There is no rollback across ACs.</Banner>
      </Modal>
    </div>
  );
}

/* ───── Admin · CSV import (FR-A18) ───── */
export function ImportCsvModal({ open, onClose }: { open: boolean; onClose: () => void }) {
  const toast = useToast();
  const [step, setStep] = useState(0);
  const close = () => { setStep(0); onClose(); };
  return (
    <Modal wide open={open} onClose={close} title={`Import units from CSV · ${step + 1} / 2`} footer={step === 0 ? <><Btn onClick={close}>Cancel</Btn><Btn variant="primary" onClick={() => setStep(1)}>Preview →</Btn></> : <><Btn onClick={() => setStep(0)}>← Back</Btn><Btn variant="primary" onClick={() => { close(); toast("22 rows imported · 2 skipped · undo available for 30 min"); }}>Import 22 rows</Btn></>}>
      <Steps steps={["Upload & mapping", "Preview"]} current={step} />
      {step === 0 ? <>
        <Field label="Customer" hint="Rows are created under this customer only. Properties / floors / rooms that do not exist are created."><Select><option>customer-a · Tan household</option><option>customer-b · Lim Trading</option></Select></Field>
        <div className="flex flex-wrap items-center justify-between gap-2 rounded-xl border border-dashed border-line p-3 text-[13px]"><span>📄 units-office-a.csv <span className="text-xs text-muted">· 24 rows · 9 columns · UTF-8 · 6 KB</span></span><Btn size="sm">Download template</Btn></div>
        <DataTable rowKey={(r) => r.c} rows={[{ c: "property", f: "Property name", req: true }, { c: "floor", f: "Floor", req: false }, { c: "room", f: "Room / space", req: true }, { c: "unit_name", f: "Unit name", req: true }, { c: "model", f: "Model", req: true }, { c: "installed_on", f: "Install date (warranty)", req: false }]} cols={[{ key: "c", label: "CSV column", render: (r) => <code>{r.c}</code> }, { key: "f", label: "Field", render: (r) => <Select className="w-auto"><option>{r.f}</option></Select> }, { key: "r", label: "", render: (r) => <Badge tone={r.req ? "primary" : "muted"}>{r.req ? "Required" : "Optional"}</Badge> }]} />
      </> : <>
        <div className="grid-fluid" style={{ ["--min" as string]: "140px" }}><Kpi label="Rows" value={24} /><Kpi label="Will be created" value={22} tone="ok" /><Kpi label="Errors (skipped)" value={2} tone="crit" /></div>
        <DataTable rowKey={(r) => r.r + ""} rows={[{ r: 7, m: "Unknown model “CS-9.9K”", st: "Error" }, { r: 15, m: "Duplicate unit name “WS-AC 03” in Workstations", st: "Error" }, { r: 2, m: "Office A › 3F › Meeting room › Meeting AC 2", st: "New" }]} cols={[{ key: "r", label: "Row", render: (r) => r.r }, { key: "m", label: "Result", render: (r) => r.m }, { key: "s", label: "", render: (r) => <Badge tone={r.st === "Error" ? "crit" : "ok"}>{r.st}</Badge> }]} />
        <Banner tone="warn">Rows with errors are skipped (VALIDATION). The preview expires after 30 minutes; importing an expired or changed preview is a CONFLICT.</Banner>
      </>}
    </Modal>
  );
}

/* ───── Admin · Warranty & coverage (FR-A19) ───── */
export function WarrantyTab() {
  const toast = useToast();
  const rows = [
    { u: "Living room AC", id: "unit-non-rto · Home A", c: "customer-a", m: "CS-3.5K", end: "2026-10-28", k: "—", st: "Ends in 27 d · no contract", tone: "warn" as const },
    { u: "Bedroom AC", id: "unit-online-rto · Home A", c: "customer-a", m: "CS-2.5K", end: "2026-11-30", k: "contract-rto-a", st: "Covered by contract", tone: "ok" as const },
    { u: "Lobby AC", id: "unit-limited · Office A", c: "customer-a", m: "CS-5.0K", end: "2027-03-31", k: "contract-rto-a", st: "Covered", tone: "ok" as const },
    { u: "Rooftop AC", id: "unit-p-rooftop", c: "customer-b", m: "VRF-12", end: "2026-08-31", k: "—", st: "No coverage", tone: "crit" as const },
  ];
  return (
    <div className="flex flex-col gap-4">
      <div className="grid-fluid" style={{ ["--min" as string]: "180px" }}><Kpi label="Under warranty" value={38} sub="of 52 units" /><Kpi label="Warranty ends ≤ 90 days" value={5} tone="warn" sub="2 within 30 days" /><Kpi label="Out of warranty, no contract" value={4} tone="crit" sub="Offer maintenance" /><Kpi label="Maintenance contract" value={31} sub="contract-rto-a, contract-b …" /></div>
      <div className="split">
        <Card title="Units by coverage end" action={<Btn size="sm" onClick={() => toast("CSV exported (demo)")}>Export CSV</Btn>}>
          <DataTable rowKey={(r) => r.id} rows={rows} cols={[{ key: "u", label: "Unit", render: (r) => <div><b>{r.u}</b><div className="text-[11px] text-muted">{r.id}</div></div> }, { key: "c", label: "Customer", render: (r) => r.c, hideBelow: "sm" }, { key: "m", label: "Model", render: (r) => r.m, hideBelow: "md" }, { key: "e", label: "Warranty end", render: (r) => r.end }, { key: "k", label: "Contract", render: (r) => r.k, hideBelow: "md" }, { key: "s", label: "Status", render: (r) => <Badge tone={r.tone}>{r.st}</Badge> }, { key: "a", label: "", render: (r) => (r.k === "—" ? <Btn size="sm" variant="primary" onClick={() => toast("Renewal offer drafted")}>Renewal offer</Btn> : null) }]} />
          <p className="mt-2 text-[11px] text-muted">Warranty end comes from the unit (install date + model warranty). Coverage = warranty OR an active maintenance contract.</p>
        </Card>
        <Card title="Warranty on jobs">
          <Banner>job-a06 (Bedroom AC) replaced a fan motor while under warranty — parts cost is claimable from the manufacturer.</Banner>
          <div className="mt-3"><SummaryList items={[["Job", "job-a06"], ["Part", "Fan motor FM-25 ×1"], ["Claim", "MYR 210.00 · not filed"]]} /></div>
          <Btn size="sm" className="mt-3" onClick={() => toast("Claim marked as filed")}>Mark claim filed</Btn>
        </Card>
      </div>
    </div>
  );
}

/* ───── Admin · Firmware campaigns (FR-A20) ───── */
export function FirmwareCampaigns() {
  const toast = useToast();
  const [modal, setModal] = useState(false);
  const [paused, setPaused] = useState(false);
  const waves = [["Wave 1 · 10 %", 100, "5 / 5 succeeded"], ["Wave 2 · 50 %", 96, "24 / 25 · 1 failed (old version kept)"], ["Wave 3 · 100 %", 0, "starts when wave 2 ≥ 95 % success"]] as const;
  return (
    <div className="split-rev">
      <Card title="Campaigns" action={<Btn size="sm" variant="primary" onClick={() => setModal(true)}>+ New campaign</Btn>} className="self-start">
        <ListRow selected><div className="min-w-0 flex-1"><b className="text-[13px]">fw-2.0.0-rollout</b><div className="text-[11px] text-muted">v2.4.1 → v2.0.0-demo · 50 devices</div></div><Badge tone={paused ? "warn" : "primary"}>{paused ? "Paused" : "Running"}</Badge></ListRow>
        <ListRow><div className="min-w-0 flex-1"><b className="text-[13px]">fw-1.9.3-hotfix</b><div className="text-[11px] text-muted">completed 08-30</div></div><Badge tone="ok">Done</Badge></ListRow>
      </Card>
      <Card title="fw-2.0.0-rollout" sub="Auto-pause if failures exceed 5 % within a wave" action={<Btn size="sm" onClick={() => { setPaused((p) => !p); toast(paused ? "Campaign resumed" : "Campaign paused", "warn"); }}>{paused ? "Resume" : "Pause"}</Btn>}>
        <div className="flex flex-col gap-3">{waves.map(([l, p, d]) => <div key={l}><div className="mb-1 flex justify-between text-xs"><b>{l}</b><span className="text-muted">{d}</span></div><UtilBar pct={p} tone={p >= 95 ? "ok" : "primary"} /></div>)}</div>
        <div className="mt-3"><SummaryList items={[["Skipped", "2 devices · active diagnostic run / open tamper"], ["Failed", "AC-DEMO-0003 · timeout · old version kept"]]} /></div>
      </Card>
      <Modal open={modal} onClose={() => setModal(false)} title="New firmware campaign" footer={<><Btn onClick={() => setModal(false)}>Cancel</Btn><Btn variant="primary" onClick={() => { setModal(false); toast("Campaign scheduled"); }}>Schedule</Btn></>}>
        <div className="grid-fluid" style={{ ["--min" as string]: "180px" }}><Field label="Model"><Select><option>Split 1.5HP (demo)</option></Select></Field><Field label="Target version"><Select><option>v2.0.0-demo</option></Select></Field></div>
        <Field label="Waves (ascending, ends at 100 %)"><Input defaultValue="10 %, 50 %, 100 %" /></Field>
        <div className="grid-fluid" style={{ ["--min" as string]: "180px" }}><Field label="Auto-pause at failures above"><Input defaultValue="5 %" /></Field><Field label="Start"><Input type="datetime-local" defaultValue="2026-10-03T02:00" /></Field></div>
        <Banner>Devices with an active diagnostic run, firmware operation or open tamper are skipped with a reason.</Banner>
      </Modal>
    </div>
  );
}

/* ───── Admin · Contractor payouts (FR-A23) ───── */
export function ContractorPayouts() {
  const toast = useToast();
  const [st, setSt] = useState<"Draft" | "Approved" | "Paid">("Approved");
  return (
    <div className="split-rev">
      <Card title="Statements · September 2026" action={<Btn size="sm" onClick={() => toast("Draft statements regenerated for September")}>Generate</Btn>} className="self-start">
        <ListRow selected><div className="min-w-0 flex-1"><b className="text-[13px]">contractor-a</b><div className="text-[11px] text-muted">stmt-2026-09-a · 6 jobs</div></div><Badge tone={st === "Paid" ? "ok" : st === "Draft" ? "muted" : "primary"}>{st}</Badge></ListRow>
        <ListRow><div className="min-w-0 flex-1"><b className="text-[13px]">contractor-b</b><div className="text-[11px] text-muted">stmt-2026-09-b · 3 jobs</div></div><Badge>Draft</Badge></ListRow>
      </Card>
      <Card title="contractor-a · stmt-2026-09-a" sub="Priced by the rate card effective at acceptance · pay date 10-10" action={<span className="flex gap-2">{st === "Draft" && <Btn size="sm" variant="primary" onClick={() => setSt("Approved")}>Approve</Btn>}{st === "Approved" && <Btn size="sm" variant="primary" onClick={() => { setSt("Paid"); toast("Marked paid"); }}>Mark paid</Btn>}</span>}>
        <SummaryList items={[["Jobs accepted in September", "6"], ["Gross", "2,810.00 MYR"], ["Deductions", "− 120.00 MYR (job-p07 rework)"], ["Total", "2,690.00 MYR"], ["Open question", "job-p07 deduction — “caused by a missing part from HQ stock”"]]} />
        <div className="mt-3 flex flex-wrap gap-2"><Btn size="sm" onClick={() => toast("Answered · adjustment +120.00 MYR added to the next draft")}>Answer with adjustment</Btn><Btn size="sm" onClick={() => toast("Answered without adjustment")}>Answer</Btn></div>
        <p className="mt-2 text-[11px] text-muted">Contractors see approved and paid statements only (Partner › Payouts).</p>
      </Card>
    </div>
  );
}

export { Toggle };
