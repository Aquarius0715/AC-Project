"use client";

// The evidence picker of an alert's resolution (IR327, Figma Admin 259:2): the heading says whether evidence is
// required, then one checkbox per candidate — remeasurements and device recoveries, the newest first — and the evidence
// attached to the alert at detection last (“none attached” when it has none). The rows come from alertEvidence.ts.
import { useT } from "./I18n";
import { evidenceHeading, togglePick, type EvidenceRow } from "@ac/web/lib/alertEvidence";

function Row({ title, sub, checked, disabled, onChange }: { title: string; sub: string; checked: boolean; disabled?: boolean; onChange?: (on: boolean) => void }) {
  return (
    <label className="flex items-start gap-2.5 border-t border-line py-2 text-[13px] first:border-0">
      <input type="checkbox" className="mt-0.5 h-4 w-4 shrink-0 accent-[#005bea]" checked={checked} disabled={disabled} onChange={(e) => onChange?.(e.target.checked)} />
      <span className="min-w-0"><b className="block font-semibold">{title}</b><span className="block text-[11px] text-muted">{sub}</span></span>
    </label>
  );
}

export function EvidencePicker({ rows, policyless, picked, onChange, loading, error }: { rows: EvidenceRow[]; policyless: boolean; picked: string[]; onChange: (ids: string[]) => void; loading?: boolean; error?: string | null }) {
  const t = useT();
  const h = evidenceHeading(policyless, t);
  const attached = rows.filter((r) => r.kind === "detection");
  const later = rows.filter((r) => r.kind !== "detection");
  const row = (r: EvidenceRow) => <Row key={r.id} title={r.title} sub={r.sub} checked={picked.includes(r.id)} onChange={(on) => onChange(togglePick(picked, r.id, on))} />;
  return (
    <fieldset className="flex min-w-0 flex-col rounded-xl border border-line px-3 pb-1 pt-2">
      <legend className="px-1 text-xs font-semibold">{h.title}</legend>
      <p className="mb-1 text-[11px] text-muted">{h.sub}</p>
      {loading ? <p className="py-2 text-xs text-muted">{t("Loading the evidence…")}</p> : (
        <div className="flex flex-col">
          {later.length ? later.map(row) : <p className="py-2 text-xs text-muted">{t("No remeasurement since detection yet — remeasure the unit or record an inspection.")}</p>}
          {attached.length ? attached.map(row) : <Row title={t("Evidence attached to this alert")} sub={t("none attached")} checked={false} disabled />}
        </div>
      )}
      {error && <p className="pb-2 text-xs font-medium text-crit">✕ {error}</p>}
    </fieldset>
  );
}
