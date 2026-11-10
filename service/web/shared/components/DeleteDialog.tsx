"use client";

import { useState } from "react";
import { Btn, Check, Field, Input, Modal, Textarea } from "@ac/web/components/ui";
import { useT } from "@ac/web/components/I18n";

/** The state of the "Force delete" option of a delete dialog (data.forceDelete): on, and the name typed to confirm it. */
export interface ForceState { on: boolean; typed: string }
export const noForce: ForceState = { on: false, typed: "" };

/** Force is ready to go when it is off, or on with the record's name typed exactly. */
export function forceReady(force: ForceState, name: string): boolean {
  return !force.on || force.typed.trim() === name;
}

/** The "Force delete" option, shown only to the holder of data.forceDelete: it also removes the accounts that have
history, so the name of the record has to be typed first. `what` says what is forced. */
export function ForceDeleteField({ name, what, force, onChange }: Readonly<{ name: string; what: string; force: ForceState; onChange: (next: ForceState) => void }>) {
  const t = useT();
  return (
    <div className="mt-3 rounded-md border border-border p-3">
      <Check checked={force.on} onChange={(on) => { onChange({ on, typed: "" }); }} label={<span><strong>{t("Force delete")}</strong> — {what}</span>} />
      {force.on && (
        <Field label={t("Type {name} to confirm", { name })} error={force.typed.trim() && !forceReady(force, name) ? t("The name does not match") : undefined}>
          <Input value={force.typed} onChange={(e) => { onChange({ on: true, typed: e.target.value }); }} />
        </Field>
      )}
    </div>
  );
}

/** The dialog of a delete that names why (DEC-103): what goes with the record and what stops it, and a required reason
(1–1000). Confirm gets the trimmed reason. With `force` (given only to the holder of data.forceDelete) it also offers the
forced delete and confirm gets `force` true. */
export function DeleteDialog({ title, text, pending, onClose, onConfirm, force }: Readonly<{ title: string; text: string; pending: boolean; onClose: () => void; onConfirm: (reason: string, force: boolean) => void; force?: { name: string; what: string } }>) {
  const t = useT();
  const [reason, setReason] = useState("");
  const [tried, setTried] = useState(false);
  const [forced, setForced] = useState(noForce);
  const ready = forceReady(forced, force?.name ?? "");
  return (
    <Modal open onClose={onClose} title={title} footer={<><Btn onClick={onClose}>{t("Cancel")}</Btn><Btn variant="danger" disabled={pending || !ready} onClick={() => { setTried(true); if (reason.trim()) onConfirm(reason.trim(), forced.on); }}>{t("Delete")}</Btn></>}>
      <p className="text-[13px]">{text}</p>
      <Field label={t("Reason")} error={tried && !reason.trim() ? t("A reason is required") : undefined}><Textarea value={reason} maxLength={1000} onChange={(e) => { setReason(e.target.value); }} /></Field>
      {force && <ForceDeleteField name={force.name} what={force.what} force={forced} onChange={setForced} />}
    </Modal>
  );
}
