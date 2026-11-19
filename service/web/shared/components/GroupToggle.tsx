"use client";
import { Plain } from "@ac/web/components/ui";
import { useT } from "@ac/web/components/I18n";

/** “Select all (n)” / “Clear” for one group of a unit picker, named for the group for screen readers (UI/UX loop R60); 44 px on a touch screen like the other text actions (IR618). */
export function GroupToggle({ ids, chosen, group, disabled, onToggle }: Readonly<{ ids: string[]; chosen: string[]; group: string; disabled?: boolean; onToggle: (ids: string[], on: boolean) => void }>) {
  const t = useT();
  const isAll = ids.every((id) => chosen.includes(id));
  return (
    <Plain type="button" touch disabled={disabled} aria-label={t(isAll ? "Clear the units of {group}" : "Select all units of {group}", { group })}
      className="text-[11px] font-semibold text-primary hover:underline disabled:pointer-events-none disabled:opacity-50" onClick={() => { onToggle(ids, !isAll); }}>
      {t(isAll ? "Clear" : "Select all ({n})", { n: ids.length })}
    </Plain>
  );
}
