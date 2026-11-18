"use client";

import { useCallback, useRef, useState } from "react";
import { Banner, Btn, Choice, Field, Input, Modal, Plain, Textarea } from "@ac/web/components/ui";
import { useT } from "@ac/web/components/I18n";
import { Icon } from "@ac/web/components/Icon";

/** The customer sign-off: a signature drawn on the pad, or, when the customer is not there, a reason and a site photo. */
export function SignModal({ open, pending, summary, onClose, onSign }: Readonly<{ open: boolean; pending: boolean; summary: string; onClose: () => void; onSign: (form: FormData) => void }>) {
  const t = useT();
  const canvas = useRef<HTMLCanvasElement | null>(null);
  const drawing = useRef(false);
  const [drawn, setDrawn] = useState(false);
  const [name, setName] = useState("");
  const [absent, setAbsent] = useState(false);
  const [reason, setReason] = useState("");
  const [photo, setPhoto] = useState<File | null>(null);
  const [error, setError] = useState<string | null>(null);
  // the pad is a new, empty canvas each time it mounts (the dialog unmounts its content when closed, and the pad is not
  // rendered while "Customer not available" is chosen), so "drawn" starts over with it: a stroke on an earlier pad must
  // not let an empty one be saved as the customer's signature (R1030)
  const pad = useCallback((el: HTMLCanvasElement | null) => {
    canvas.current = el;
    drawing.current = false;
    setDrawn(false);
  }, []);
  const point = (e: React.PointerEvent<HTMLCanvasElement>) => {
    const r = e.currentTarget.getBoundingClientRect();
    return [((e.clientX - r.left) / r.width) * e.currentTarget.width, ((e.clientY - r.top) / r.height) * e.currentTarget.height] as const;
  };
  const clear = () => { canvas.current?.getContext("2d")?.clearRect(0, 0, canvas.current.width, canvas.current.height); setDrawn(false); };
  const sign = () => {
    if (!name.trim()) { setError(t("The signer's name is required")); return; }
    const form = new FormData();
    form.set("signerName", name.trim());
    if (absent) {
      if (!reason.trim()) { setError(t("Say why the customer could not sign")); return; }
      if (!photo || !["image/jpeg", "image/png"].includes(photo.type) || photo.size > 5 << 20) { setError(t("A site photo (JPEG/PNG, ≤5 MiB) is required when the customer is not available")); return; }
      form.set("absentReason", reason.trim()); form.set("sitePhoto", photo);
    } else {
      if (!drawn || !canvas.current) { setError(t("Ask the customer to sign in the box")); return; }
      form.set("signature", canvas.current.toDataURL("image/png").split(",", 2)[1] ?? "");
    }
    setError(null);
    onSign(form);
  };
  return (
    <Modal open={open} onClose={onClose} title={t("Customer sign-off")} footer={<><Btn onClick={onClose}>{t("Cancel")}</Btn><Btn variant="primary" disabled={pending} onClick={sign}>{t("Save sign-off")}</Btn></>}>
      <p className="text-xs text-muted-foreground">{t("Report {summary}", { summary })}</p>
      <Field label={t("Signer name")}><Input value={name} maxLength={120} placeholder="Ms Tan" onChange={(e) => { setName(e.target.value); }} /></Field>
      <Field label={t("How")}><Choice value={absent ? "absent" : "sign"} onChange={(v) => { setAbsent(v === "absent"); }} options={[{ id: "sign", label: t("Signature") }, { id: "absent", label: t("Customer not available") }]} /></Field>
      {absent ? (
        <>
          <Field label={t("Why the customer could not sign")}><Textarea value={reason} maxLength={1000} onChange={(e) => { setReason(e.target.value); }} /></Field>
          <Field label={t("Site photo (JPEG/PNG)")}><Input type="file" accept="image/jpeg,image/png" onChange={(e) => { setPhoto(e.target.files?.[0] ?? null); }} /></Field>
        </>
      ) : (
        <div>
          <canvas ref={pad} width={560} height={160} aria-label={t("Signature pad")} className="h-32 w-full touch-none rounded-xl border border-dashed border-line bg-surface"
            onPointerDown={(e) => { drawing.current = true; const c = e.currentTarget.getContext("2d"); if (!c) { return; } const [x, y] = point(e); c.lineWidth = 2.5; c.lineCap = "round"; c.strokeStyle = "#0f172a"; c.beginPath(); c.moveTo(x, y); e.currentTarget.setPointerCapture(e.pointerId); }}
            onPointerMove={(e) => { const c = e.currentTarget.getContext("2d"); if (!c || !drawing.current) { return; } const [x, y] = point(e); c.lineTo(x, y); c.stroke(); setDrawn(true); }}
            onPointerUp={() => { drawing.current = false; }} />
          <div className="mt-1 flex justify-between text-[11px] text-muted-foreground"><span className="flex items-center gap-1"><Icon name="signature" />{t("Sign here")}</span><Plain type="button" disabled={!drawn} className="font-semibold text-primary disabled:text-muted-foreground" onClick={clear}>{t("Clear")}</Plain></div>
        </div>
      )}
      {error && <Banner tone="crit">{error}</Banner>}
      <p className="text-[11px] text-muted-foreground">{t("The sign-off is bound to this report version; saving the draft again clears it.")}</p>
    </Modal>
  );
}
