"use client";

// The technician's “Scan unit QR” dialog in API mode (FR-T13, DD-T13, Figma Technician 01-6): the camera is simulated —
// the label code, a device serial or the unit ID is typed, or one of the user's units is “scanned” — and
// units.resolveQr (through the BFF) answers the unit and the user's next open job on it; units.get and jobs.get fill
// the matched card. Units outside the user's assignments and unknown labels read as “Page unavailable” (NOT_FOUND), so
// the one-tap scans are the units of the user's open jobs (jobs.list), not every unit in scope (DD-T13, IR254).
import Link from "next/link";
import { useRef, useState } from "react";
import { Badge, Banner, Btn, Input, Modal } from "./ui";
import { callOp, OpError } from "@ac/web/lib/ops";
import { useNow, useOp } from "@ac/web/lib/useOp";
import { klTime } from "@ac/web/lib/devices";
import { qrMatch, qrRefusal, scanCode, type ApiQrJob, type ApiQrResolution, type ApiQrUnit, type QrMatch } from "@ac/web/lib/techQr";

const OPEN = ["assigned", "in_progress", "on_hold", "rework_requested"]; // the jobs units.resolveQr opens (IR230)

type State = { kind: "idle" } | { kind: "busy" } | { kind: "matched"; code: string; at: string; match: QrMatch } | { kind: "refused"; title: string; text: string };

export function QrScan({ open, onClose }: { open: boolean; onClose: () => void }) {
  const [code, setCode] = useState("");
  const [state, setState] = useState<State>({ kind: "idle" });
  const input = useRef<HTMLInputElement>(null);
  const now = useNow();
  const units = useOp<{ items: { id: string; displayName: string }[] }, { id: string; displayName: string }[]>("units.list", { limit: 100 }, [], (p) => p.items, open);
  const jobUnits = useOp<{ items: { unitId: string }[] }, string[]>("jobs.list", { filters: { statuses: OPEN }, limit: 100 }, [], (p) => p.items.map((j) => j.unitId), open);
  const assigned = units.data.filter((u) => jobUnits.data.includes(u.id));
  const close = () => { setState({ kind: "idle" }); setCode(""); onClose(); };
  const scan = async (raw: string) => {
    const c = scanCode(raw);
    if (!c) return setState({ kind: "refused", ...qrRefusal({ code: "VALIDATION", messageKey: "error.validation" }) });
    setState({ kind: "busy" });
    try {
      const r = await callOp<ApiQrResolution>("units.resolveQr", { code: c });
      let locked = false;
      const [unit, job] = await Promise.all([
        callOp<ApiQrUnit>("units.get", { id: r.unitId }).catch((e: unknown) => { locked = e instanceof OpError && e.error.code === "FORBIDDEN"; return null; }), // before the work window (IR94)
        r.jobId ? callOp<ApiQrJob>("jobs.get", { jobId: r.jobId }).catch(() => null) : Promise.resolve(null),
      ]);
      const at = (now ?? new Date()).toISOString();
      const name = units.data.find((u) => u.id === r.unitId)?.displayName;
      setState({ kind: "matched", code: c, at, match: qrMatch(r, unit, job, Date.parse(at), { name, locked }) });
    } catch (e) {
      if (!(e instanceof OpError)) throw e;
      setState({ kind: "refused", ...qrRefusal(e.error) });
    }
  };
  const m = state.kind === "matched" ? state.match : null;
  return (
    <Modal open={open} onClose={close} title="Scan unit QR" wide footer={
      <>
        <Btn variant="ghost" onClick={() => input.current?.focus()}>Enter ID manually</Btn>
        {m ? <Link href={m.unitHref} onClick={close} className="inline-flex items-center rounded-control border border-line bg-surface px-3.5 py-2 text-[13px] font-semibold hover:bg-surface2">Open unit</Link> : <Btn disabled>Open unit</Btn>}
        {m?.jobHref ? <Link href={m.jobHref} onClick={close} className="inline-flex items-center rounded-control border border-primary bg-primary px-3.5 py-2 text-[13px] font-semibold text-white">Open job →</Link> : <Btn variant="primary" disabled>Open job →</Btn>}
      </>
    }>
      <div className="grid gap-4 sm:grid-cols-[minmax(0,1fr)_minmax(0,1.2fr)]">
        <div className="flex flex-col gap-2 rounded-xl bg-ink/90 p-4 text-white">
          <div className="grid h-36 place-items-center rounded-lg border-2 border-dashed border-ok/70 text-[11px] text-white/70">camera (simulated) — point it at the unit label</div>
          <form className="flex gap-2" onSubmit={(e) => { e.preventDefault(); void scan(code); }}>
            <Input ref={input} aria-label="Label code, device serial or unit ID" value={code} onChange={(e) => setCode(e.target.value)} placeholder="Label code, device serial or unit ID" className="bg-white text-ink" />
            <Btn type="submit" variant="primary" disabled={state.kind === "busy"}>Scan</Btn>
          </form>
          {assigned.length > 0 && (
            <div className="text-[11px] text-white/80">
              Labels on the units of your open jobs:
              <div className="mt-1 flex flex-wrap gap-1">{assigned.map((u) => <button key={u.id} type="button" onClick={() => { setCode(`ac-unit:${u.id}`); void scan(`ac-unit:${u.id}`); }} className="rounded-full border border-white/30 px-2 py-0.5 hover:bg-white/10">{u.displayName}</button>)}</div>
            </div>
          )}
        </div>
        <div className="flex min-w-0 flex-col gap-2 text-[13px]">
          {state.kind === "idle" && <p className="text-muted">Scan the label on the unit, or enter its label code, device serial or unit ID. Only units in your assignments are shown.</p>}
          {state.kind === "busy" && <p className="text-muted" aria-busy>Reading the label…</p>}
          {state.kind === "refused" && <Banner tone={state.title === "Page unavailable" ? "warn" : "crit"}><b>{state.title}</b> — {state.text}</Banner>}
          {m && state.kind === "matched" && (
            <>
              <p className="flex flex-wrap items-center gap-2"><Badge tone="ok">✓ Matched</Badge><span className="text-xs text-muted">label {state.code.length > 24 ? `${state.code.slice(0, 24)}…` : state.code} · scanned {klTime(state.at).slice(11)}</span></p>
              <b className="text-[15px]">{m.title}</b>
              {m.sub && <span className="text-xs text-muted">{m.sub}</span>}
              {m.job ? <div className={m.job.tone === "ok" ? "rounded-xl bg-ok-soft p-3 text-ok" : "rounded-xl bg-primary-soft p-3 text-primary"}><b className="block">{m.job.label}</b><span className="text-xs">{m.job.text}</span></div>
                : <div className="rounded-xl bg-surface2 p-3 text-xs text-muted">No open job of yours on this unit right now — the unit register is still readable.</div>}
            </>
          )}
          <p className="mt-auto text-[11px] text-muted">Not in your assignments → “Page unavailable”, no unit data (same as a direct link). Unknown or damaged label → enter the unit ID manually.</p>
        </div>
      </div>
    </Modal>
  );
}
