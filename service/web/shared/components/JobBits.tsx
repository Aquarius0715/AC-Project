"use client";

import { Badge, Input, Select, cx } from "./ui";
import type { JobStatus, Origin, Slot } from "@ac/web/lib/jobs";
import { statusLabel } from "@ac/web/lib/jobs";
import { useT } from "./I18n";

/** Where a job came from — shown on every role (IR113), in the display language. */
export function OriginBadge({ origin }: { origin: Origin }) {
  const t = useT();
  return origin === "plan" ? (
    <span className="inline-flex items-center gap-1 whitespace-nowrap rounded-full bg-[#f3ebff] px-2 py-0.5 text-[11px] font-semibold leading-4 text-[#6d28d9]"><span aria-hidden>↻</span>{t("Periodic plan")}</span>
  ) : (
    <span className="inline-flex items-center gap-1 whitespace-nowrap rounded-full bg-[#e0f7f5] px-2 py-0.5 text-[11px] font-semibold leading-4 text-[#0b6e66]"><span aria-hidden>✉</span>{t("Client request")}</span>
  );
}

const statusTone: Record<JobStatus, Parameters<typeof Badge>[0]["tone"]> = {
  requested: "primary", time_proposed: "warn", offered: "primary", accepted: "primary", assigned: "primary", in_progress: "warn", submitted: "primary", rework_requested: "warn", completed: "ok", cancelled: "muted", on_hold: "unknown",
};
const statusIcon: Partial<Record<JobStatus, string>> = { requested: "○", time_proposed: "⇄", offered: "○", accepted: "◔", assigned: "◔", in_progress: "↻", submitted: "◷", rework_requested: "↻", completed: "✓", cancelled: "×", on_hold: "■" };
export function JobStatusBadge({ s }: { s: JobStatus }) {
  const t = useT();
  return <Badge tone={statusTone[s]} icon={statusIcon[s]}>{t(statusLabel[s])}</Badge>;
}

export const RANKS = ["1st", "2nd", "3rd"];
export function Rank({ i }: { i: number }) {
  const t = useT();
  return <span className="inline-grid w-10 shrink-0 place-items-center rounded-lg bg-primary-soft py-0.5 text-xs font-semibold text-primary">{t(RANKS[i])}</span>;
}

export const WINDOWS = ["09:00–11:00", "10:00–12:00", "13:00–15:00", "14:00–16:00", "16:00–18:00"];

/** Three preferred times (1st = most preferred). Used by the request, decline and reschedule dialogs. */
export function PreferredSlotsInput({ value, onChange, error, label = "Preferred times — give 3 options (1st = most preferred)", hint }: { value: Slot[]; onChange: (v: Slot[]) => void; error?: string; label?: string; hint?: string }) {
  const set = (i: number, p: Partial<Slot>) => onChange(value.map((s, k) => (k === i ? { ...s, ...p } : s)));
  return (
    <fieldset className="flex flex-col gap-2">
      <legend className="mb-1 text-xs font-semibold text-ink">{label}</legend>
      {value.map((s, i) => (
        <div key={i} className="flex items-center gap-2">
          <Rank i={i} />
          <Input type="date" aria-label={`${RANKS[i]} preferred date`} value={s.date} onChange={(e) => set(i, { date: e.target.value })} className="min-w-0 flex-1" />
          <Select aria-label={`${RANKS[i]} preferred window`} value={s.win} onChange={(e) => set(i, { win: e.target.value })} className="min-w-0 flex-1">{WINDOWS.map((w) => <option key={w}>{w}</option>)}</Select>
        </div>
      ))}
      {error ? <span className="text-xs font-medium text-crit">✕ {error}</span> : <span className="text-[11px] text-muted">{hint ?? "3 different times, at least 1 day ahead. HQ books one of them. If none can be met, HQ proposes another time — you accept or decline it."}</span>}
    </fieldset>
  );
}

/** Validation shared by all preferred-time inputs (IR113): 3 slots, distinct, after the demo “today”. */
export function preferredError(v: Slot[], today = "2026-09-22"): string | undefined {
  if (v.length !== 3 || v.some((s) => !s.date)) return "Enter 3 preferred times";
  if (v.some((s) => s.date <= today)) return "Each time must be at least 1 day ahead";
  if (new Set(v.map((s) => s.date + s.win)).size !== 3) return "The 3 times must be different";
  return undefined;
}

export function SlotLine({ i, slot, struck, note }: { i: number; slot: Slot; struck?: boolean; note?: React.ReactNode }) {
  return (
    <div className="flex flex-wrap items-center gap-2 text-[13px]">
      <Rank i={i} />
      <span className={cx("font-semibold", struck && "text-muted line-through")}>{slot.date.slice(5)} · {slot.win}</span>
      {note && <span className="text-xs text-muted">{note}</span>}
    </div>
  );
}
