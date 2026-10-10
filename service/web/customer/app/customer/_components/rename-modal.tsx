"use client";

// Rename a location or an AC (FR-C02, FR-C03, Figma Client 02b / 02c, IR109): only the name changes, trimmed 1–120
// and unique among its siblings (locations.rename with the fetched version). Units & locations renames properties and
// spaces; the unit screen renames its AC (DD-C03, IR315). The Phase 1A demo only confirms with a toast.
import { useState } from "react";
import { Btn, Field, Input, Modal, SummaryList, useToast } from "@ac/web/components/ui";
import { useT } from "@ac/web/components/I18n";
import { useAction } from "@ac/web/lib/useAction";
import { nameError } from "@ac/web/lib/assets";
import { renameLocation } from "../properties/actions";

export type Renaming = { kind: "property" | "space" | "unit"; id: string; version: number; name: string; where: string };

export function RenameModal({ r, onClose, demo = false }: { r: Renaming; onClose: () => void; demo?: boolean }) {
  const t = useT();
  const toast = useToast();
  const [pending, run] = useAction();
  const [name, setName] = useState(r.name);
  const [err, setErr] = useState<string | undefined>();
  const save = () => {
    const e = nameError(name, t);
    if (e) return setErr(e);
    if (demo) {
      toast(t("Renamed to “{name}”", { name: name.trim() }));
      return onClose();
    }
    run(() => renameLocation(r.kind, r.id, r.version, name), t("Renamed to “{name}”", { name: name.trim() }), onClose, (f) => {
      if (f.messageKey === "error.duplicateSiblingName") setErr(t("“{name}” already exists there — choose another name. Your input is kept.", { name: name.trim() }));
      else if (f.messageKey === "error.versionConflict") setErr(t(r.kind === "unit" ? "Someone changed this AC in the meantime — the page now shows its current name. Your input is kept." : "Someone renamed it in the meantime — the tree now shows the current name. Your input is kept."));
      else setErr(undefined);
    });
  };
  return (
    <Modal open onClose={onClose} title={t("Rename {name}", { name: r.name })} footer={<><Btn onClick={onClose}>{t("Cancel")}</Btn><Btn variant="primary" disabled={pending || name.trim() === r.name} onClick={save}>{t("Save name")}</Btn></>}>
      <div className="flex flex-col gap-3">
        <p className="text-[13px] text-muted">{t("Only the name changes. Floors, rooms and units stay as they are — HQ manages the structure.")}</p>
        <SummaryList items={[[t(r.kind === "unit" ? "AC" : "Location"), r.where]]} />
        <Field label={t("New name")} hint={t("1–120 characters, unique among its siblings. HQ and technicians see the new name too.")} error={err}><Input value={name} onChange={(e) => { setName(e.target.value); setErr(undefined); }} autoFocus /></Field>
      </div>
    </Modal>
  );
}
