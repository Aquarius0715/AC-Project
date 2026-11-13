"use client";

import Link from "next/link";
import { useId, useState } from "react";
import { Badge, Banner, Btn, Card, DataTable, Field, Input, LinkButton, Modal, Page, PageHead, Select, Textarea } from "@ac/web/components/ui";
import { Checkbox } from "@ac/web/components/ui/checkbox";
import { Icon } from "@ac/web/components/Icon";
import { useDisplay, useT } from "@ac/web/components/I18n";
import { useAction } from "@ac/web/lib/useAction";
import { without } from "@ac/web/lib/omit";
import { fieldErrorTexts, type ActionFailure } from "@ac/web/lib/actionMessage";
import { focusField } from "@ac/web/lib/focusField";
import { spacePath, type ApiPropertyRow, type ApiSpaceRow } from "@ac/web/lib/assets";
import type { ApiCapability } from "@ac/web/lib/devices";
import { contactError, preferredError } from "@ac/web/lib/customerMaintenance";
import { quantityError, type InstallationRow } from "@ac/web/lib/installationRequests";
import { codePoints } from "@ac/web/lib/text";
import { TimesInput, threeRows, toSlot, type Row } from "../../maintenance/_components/maintenance-view";
import { cancelInstallationRequest, createInstallationRequest } from "../actions";

type FieldKey = "property" | "quantity" | "times" | "contact" | "note";
const FIELDS: FieldKey[] = ["property", "quantity", "times", "contact", "note"];
const API_FIELDS: Record<string, FieldKey> = { propertyId: "property", spaceId: "property", quantity: "quantity", preferredSlots: "times", contactWindow: "contact", note: "note" };
const MAX_NOTE = 2000;

interface Live { rows: InstallationRow[]; properties: ApiPropertyRow[]; spaces: ApiSpaceRow[]; models: ApiCapability[]; now: string }

/** Installation requests (FR-C20, DEC-107) in API mode: the owner asks HQ to install new aircons — property and space, how
many, the model or “not decided”, whether an IoT device goes with each unit, 3 preferred times and a note — and sees the
state, HQ's reason when it declined, and the jobs HQ made. A submitted request can be cancelled. */
export function InstallationsView({ live }: Readonly<{ live: Live | null }>) {
  const t = useT();
  const [pending, run] = useAction();
  const [dialog, setDialog] = useState<"new" | InstallationRow | null>(null);
  if (!live) {
    return (
      <Page>
        <Card title={t("Page unavailable")}>
          <p className="text-[13px] text-muted-foreground">{t("This page doesn’t exist or you don’t have access to it.")}</p>
          <div className="mt-3"><LinkButton href="/customer" variant="primary">{t("Go to your home (or sign in)")}</LinkButton></div>
        </Card>
      </Page>
    );
  }
  const close = () => { setDialog(null); };
  return (
    <Page>
      <PageHead title={t("Installation requests")} sub={t("Ask HQ to install new air conditioners, with or without an IoT device. HQ plans the units and offers a visit time.")} action={<Btn variant="primary" onClick={() => { setDialog("new"); }}><Icon name="add" /> {t("New request")}</Btn>} />
      <Card pad={false}>
        <DataTable rows={live.rows} rowKey={(r) => r.id} cols={[
          { key: "n", label: t("Request"), render: (r) => <div><b>{r.number}</b><div className="text-xs text-muted-foreground">{r.place}</div></div> },
          { key: "q", label: t("Units"), render: (r) => <div>{r.quantity} · {r.model}<div className="text-xs text-muted-foreground">{r.withIot ? t("with an IoT device") : t("without an IoT device")}</div></div> },
          { key: "s", label: t("Status"), render: (r) => <div><Badge tone={r.tone}>{r.statusLabel}</Badge>{r.declineReason && <div className="mt-1 text-xs text-muted-foreground">{t("HQ: {reason}", { reason: r.declineReason })}</div>}</div> },
          { key: "t", label: t("First preferred time"), render: (r) => r.firstTime, hideBelow: "sm" },
          { key: "j", label: t("Jobs"), render: (r) => (r.jobIds.length === 0 ? "—" : <div className="flex flex-wrap gap-1">{r.jobIds.map((id) => <Link key={id} className="text-primary underline" href={`/customer/maintenance?jobId=${id}`}>{id.slice(0, 8)}</Link>)}</div>) },
          { key: "a", label: t("Actions"), render: (r) => (r.status === "submitted" ? <Btn size="sm" variant="ghost" onClick={() => { setDialog(r); }}><Icon name="close" />{t("Cancel request")}</Btn> : null) },
        ]} />
      </Card>
      <Banner>{t("HQ creates the units and the installation jobs, or declines with a reason. You can’t change a request; cancel it and send a new one. Only owners see this page.")}</Banner>
      {dialog === "new" && <NewRequest live={live} onClose={close} />}
      {dialog !== null && dialog !== "new" && (
        <Modal open onClose={close} title={t("Cancel request {number}", { number: dialog.number })} footer={<><Btn onClick={close}>{t("Keep it")}</Btn><Btn variant="danger" disabled={pending} onClick={() => { run(() => cancelInstallationRequest(dialog.id, dialog.version), t("Request {number} cancelled", { number: dialog.number }), close); }}>{t("Cancel request")}</Btn></>}>
          <Banner tone="warn">{t("HQ has not decided yet. Once cancelled, the request can’t be reopened.")}</Banner>
        </Modal>
      )}
    </Page>
  );
}

/** The new-request dialog: a failed submit takes the focus to the first field to fix, top to bottom. */
function NewRequest({ live, onClose }: Readonly<{ live: Live; onClose: () => void }>) {
  const t = useT();
  const { timeZone } = useDisplay();
  const [pending, run] = useAction();
  const now = Date.parse(live.now);
  const [propertyId, setPropertyId] = useState(live.properties[0]?.id ?? "");
  const [spaceId, setSpaceId] = useState("");
  const [quantity, setQuantity] = useState("1");
  const [modelId, setModelId] = useState("");
  const [withIot, setWithIot] = useState(false);
  const [rows, setRows] = useState<Row[]>(threeRows(now, timeZone));
  const [contact, setContact] = useState("");
  const [note, setNote] = useState("");
  const [tried, setTried] = useState(false);
  const [server, setServer] = useState<Partial<Record<FieldKey, string>>>({});
  const ids: Record<FieldKey, string> = { property: useId(), quantity: useId(), times: useId(), contact: useId(), note: useId() };
  const slots = rows.map((r) => toSlot(r, timeZone));
  const local: Partial<Record<FieldKey, string | null>> = {
    property: propertyId ? null : t("Choose a property"), quantity: quantityError(quantity, t), times: preferredError(slots, now, 3, t), contact: contactError(contact, t),
    note: codePoints(note.trim()) > MAX_NOTE ? t("Up to {max} characters", { max: MAX_NOTE }) : null,
  };
  const spaces = live.spaces.filter((s) => s.propertyId === propertyId);
  const edit = (k: FieldKey) => { setServer((s) => without(s, k)); };
  const shown = (k: FieldKey) => (tried ? local[k] : null) ?? server[k] ?? null;
  const submit = () => {
    setTried(true);
    const bad = FIELDS.find((k) => local[k]);
    if (bad) { focusField(ids[bad]); return; }
    const refused = (f: ActionFailure) => {
      const fields = fieldErrorTexts(f, API_FIELDS, t);
      setServer(fields);
      const first = FIELDS.find((k) => fields[k]);
      if (first) focusField(ids[first]);
    };
    run(() => createInstallationRequest({ modelId, note, propertyId, slots, spaceId, withIot, contactWindow: contact, quantity: Number(quantity) }), (v) => t("Request {number} sent — HQ will decide and offer a visit time.", { number: v.id.slice(0, 8) }), onClose, refused);
  };
  return (
    <Modal open wide onClose={onClose} title={t("New installation request")} footer={<><Btn onClick={onClose}>{t("Cancel")}</Btn><Btn variant="primary" disabled={pending} onClick={submit}>{t("Send request")}</Btn></>}>
      <Field label={t("Property")} error={shown("property")}><Select id={ids.property} value={propertyId} onChange={(e) => { setPropertyId(e.target.value); setSpaceId(""); edit("property"); }}>{live.properties.map((p) => <option key={p.id} value={p.id}>{p.name}</option>)}</Select></Field>
      <Field label={t("Space (optional)")}><Select value={spaceId} onChange={(e) => { setSpaceId(e.target.value); }}><option value="">{t("Any space of the property")}</option>{spaces.map((s) => <option key={s.id} value={s.id}>{spacePath(s.id, live.spaces)}</option>)}</Select></Field>
      <Field label={t("Number of units (1–10)")} error={shown("quantity")}><Input id={ids.quantity} inputMode="numeric" value={quantity} onChange={(e) => { setQuantity(e.target.value); edit("quantity"); }} /></Field>
      <Field label={t("Model")}><Select value={modelId} onChange={(e) => { setModelId(e.target.value); }}><option value="">{t("Not decided")}</option>{live.models.map((m) => <option key={m.id} value={m.id}>{m.manufacturer} {m.model}</option>)}</Select></Field>
      <label className="flex items-center gap-2 text-[13px]"><Checkbox className="h-4 w-4 accent-[#005bea]" checked={withIot} onCheckedChange={(v) => { setWithIot(v); }} />{t("Install an IoT device with each unit")}</label>
      <TimesInput rows={rows} onChange={(r) => { setRows(r); edit("times"); }} error={shown("times") ?? null} firstId={ids.times} />
      <Field label={t("Contact window (optional)")} hint={t("Use HH:mm for times (example: Weekdays 09:00-18:00) — no phone numbers or e-mail addresses")} error={shown("contact")}><Input id={ids.contact} value={contact} maxLength={200} onChange={(e) => { setContact(e.target.value); edit("contact"); }} /></Field>
      <Field label={t("Note (optional)")} hint={`${String(codePoints(note.trim()))} / ${String(MAX_NOTE)}`} error={shown("note")}><Textarea id={ids.note} value={note} onChange={(e) => { setNote(e.target.value); edit("note"); }} placeholder={t("e.g. two rooms on the second floor")} /></Field>
      <Banner>{t("Nothing is installed or booked yet. HQ reviews the request, creates the units and offers a visit time for you to accept.")}</Banner>
    </Modal>
  );
}
