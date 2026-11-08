"use client";

import { useState } from "react";
import { Btn, Field, Modal, Textarea } from "@ac/web/components/ui";
import { useT } from "@ac/web/components/I18n";

/** The dialog of a delete that names why (DEC-103): what goes with the record and what stops it, and a required reason
(1–1000). Confirm gets the trimmed reason. */
export function DeleteDialog({ title, text, pending, onClose, onConfirm }: Readonly<{ title: string; text: string; pending: boolean; onClose: () => void; onConfirm: (reason: string) => void }>) {
  const t = useT();
  const [reason, setReason] = useState("");
  const [tried, setTried] = useState(false);
  return (
    <Modal open onClose={onClose} title={title} footer={<><Btn onClick={onClose}>{t("Cancel")}</Btn><Btn variant="danger" disabled={pending} onClick={() => { setTried(true); if (reason.trim()) onConfirm(reason.trim()); }}>{t("Delete")}</Btn></>}>
      <p className="text-[13px]">{text}</p>
      <Field label={t("Reason")} error={tried && !reason.trim() ? t("A reason is required") : undefined}><Textarea value={reason} maxLength={1000} onChange={(e) => { setReason(e.target.value); }} /></Field>
    </Modal>
  );
}
