"use client";

import { useState } from "react";
import { Btn, Check } from "@ac/web/components/ui";
import { useI18n } from "@ac/web/components/I18n";
import { useOp } from "@ac/web/lib/useOp";
import { evidenceRows, MAX_EVIDENCE, type ApiEvidenceAttachment } from "@ac/web/lib/mrv";

type Units = { id: string; label: string }[];
type Period = { unitIds: string[]; from: string; to: string };

/** The candidates of attachments.list for the units and period (IR349), read only while `enabled`. */
function useCandidates({ unitIds, from, to }: Period, enabled: boolean) {
  return useOp<{ items: ApiEvidenceAttachment[] }, ApiEvidenceAttachment[]>("attachments.list", { unitIds, from, to, query: { limit: 100 } }, [], (d) => d.items, enabled);
}

/** “Evidence to attach” of a new report or version (DD-A14 item 6, Figma Admin 324:2, IR349): the chosen attachments as
 * chips, and “+ Add evidence” listing the photos of accepted work reports on the draft's units uploaded in its period.
 * mrv.saveDraft keeps up to 20. */
export function EvidencePicker({ ids, onChange, period, units }: { ids: string[]; onChange: (ids: string[]) => void; period: Period; units: Units }) {
  const i = useI18n();
  const { t } = i;
  const [open, setOpen] = useState(false);
  const ready = period.unitIds.length > 0 && !!period.from && !!period.to && period.from < period.to;
  const cands = useCandidates(period, ready && (open || ids.length > 0));
  const rows = evidenceRows(cands.data, units, i);
  const byId = new Map(rows.map((r) => [r.id, r]));
  return (
    <div role="group" aria-label={t("Evidence to attach")}>
      <p className="mb-1 text-[13px] font-semibold">{t("Evidence to attach")}</p>
      <div className="flex flex-wrap items-center gap-2">
        {ids.length === 0 && <span className="text-xs text-muted">{t("None yet.")}</span>}
        {ids.map((id) => {
          const name = byId.get(id)?.name ?? id.slice(0, 8);
          return (
            <span key={id} className="inline-flex items-center gap-1.5 rounded-full bg-surface2 px-2.5 py-1 text-xs">
              {name}<button type="button" aria-label={t("Remove {name}", { name })} className="text-muted hover:text-ink" onClick={() => onChange(ids.filter((x) => x !== id))}>✕</button>
            </span>
          );
        })}
        {ids.length < MAX_EVIDENCE && <Btn size="sm" variant="ghost" disabled={!ready} aria-expanded={open} onClick={() => setOpen((v) => !v)}>{t("+ Add evidence")}</Btn>}
      </div>
      {!ready && <p className="mt-1 text-[11px] text-muted">{t("Choose the units and the period first: the candidates are their work-report photos.")}</p>}
      {open && ready && (
        <div className="mt-2 flex flex-col gap-1.5 rounded-xl border border-line p-3">
          {cands.loading ? <p className="text-xs text-muted">{t("Loading the attachments…")}</p>
            : cands.error ? <p className="text-xs text-crit">{t("The attachments could not be loaded — try again.")}</p>
            : rows.length === 0 ? <p className="text-xs text-muted">{t("No photos of accepted work reports on these units in this period.")}</p>
            : rows.map((r) => (
              <Check key={r.id} checked={ids.includes(r.id)} disabled={!ids.includes(r.id) && ids.length >= MAX_EVIDENCE} onChange={(on) => onChange(on ? [...ids, r.id] : ids.filter((x) => x !== r.id))}
                label={<span><b>{r.name}</b><span className="block text-xs text-muted">{r.sub}</span></span>} />
            ))}
          <p className="text-[11px] text-muted">{t("Photos of accepted work reports on these units, uploaded in the period (attachments.list). Up to 20.")}</p>
        </div>
      )}
    </div>
  );
}

/** The evidence of a saved report version by name, unit and upload time; an attachment that is no longer a candidate
 * of the report's units and period keeps its ID. */
export function EvidenceList({ ids, period, units }: { ids: string[]; period: Period; units: Units }) {
  const i = useI18n();
  const { t } = i;
  const cands = useCandidates(period, ids.length > 0);
  const byId = new Map(evidenceRows(cands.data, units, i).map((r) => [r.id, r]));
  if (ids.length === 0) return <p className="text-xs text-muted">{t("No evidence attached.")}</p>;
  return (
    <ul className="flex flex-col gap-1 text-[13px]">
      {ids.map((id) => {
        const r = byId.get(id);
        return <li key={id}>{r ? <><b>{r.name}</b> <span className="text-xs text-muted">· {r.sub}</span></> : <span className="font-mono text-xs">{id}</span>}</li>;
      })}
    </ul>
  );
}
