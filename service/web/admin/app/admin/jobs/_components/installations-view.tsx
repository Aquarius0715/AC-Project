"use client";

import Link from "next/link";
import { useId, useState } from "react";
import { Badge, Banner, Btn, Card, DataTable, EmptyState, Field, Input, Modal, Page, Select, Textarea } from "@ac/web/components/ui";
import { Icon } from "@ac/web/components/Icon";
import { useI18n } from "@ac/web/components/I18n";
import { showTime, type T } from "@ac/web/lib/i18n";
import { useAction } from "@ac/web/lib/useAction";
import { useUrlPatch } from "@ac/web/lib/useUrlPatch";
import { fieldErrorTexts, type ActionFailure } from "@ac/web/lib/actionMessage";
import { focusField } from "@ac/web/lib/focusField";
import { codePoints } from "@ac/web/lib/text";
import { declineError } from "@ac/web/lib/installationRequests";
import { approveInstallationRequest, declineInstallationRequest } from "../actions";
import type { InstallationsLive } from "../_lib/load";
import { JobsTabs } from "./jobs-header";

type Row = InstallationsLive["rows"][number];
/** The units still to make for a request: its number less the jobs it has. */
const remaining = (r: Row) => Math.max(0, r.quantity - r.jobIds.length);
const STATUS_LABEL: Record<string, string> = { all: "All", submitted: "Submitted", approved: "Approved", declined: "Declined", cancelled: "Cancelled" };

/** Installation requests (FR-C20, FR-A06, DEC-107) from the Core API: HQ reads a customer's request and either makes the
planned units and the installation jobs for it (each job takes the request's 3 preferred times and goes to the usual offer
and booking) or declines it with a reason. The units still to make are the request's number less its jobs. */
export function InstallationsView({ live }: Readonly<{ live: InstallationsLive }>) {
  const i = useI18n(), { t } = i;
  const patch = useUrlPatch();
  const [dialog, setDialog] = useState<{ kind: "approve" | "decline"; row: Row } | null>(null);
  const close = () => { setDialog(null); };
  return (
    <Page className="max-w-[1440px]">
      <JobsTabs tab="installations" counts={live.counts} q={{}} />
      <div className="flex flex-wrap items-center gap-2 text-[13px]">
        <Select aria-label={t("Status")} className="w-auto" value={live.status} onChange={(e) => { patch({ status: e.target.value === "submitted" ? null : e.target.value }); }}>{live.statuses.map((s) => <option key={s} value={s}>{t("Status: {status}", { status: t(STATUS_LABEL[s] ?? s) })}</option>)}</Select>
        <span className="ml-auto text-xs text-muted-foreground">{t("{n} requests", { n: live.rows.length })}</span>
      </div>
      <Card pad={false}>
        {live.rows.length === 0 ? <EmptyState title={t("No installation requests")}>{t("A customer owner sends one from Installations in the customer app.")}</EmptyState> : (
          <DataTable rows={live.rows} rowKey={(r) => r.id} cols={[
            { key: "n", label: t("Request"), render: (r) => <div><b>{r.number}</b><div className="text-xs text-muted-foreground">{r.customer} · {r.place}</div></div> },
            { key: "q", label: t("Units"), render: (r) => <div>{r.quantity} · {r.model}<div className="text-xs text-muted-foreground">{r.withIot ? t("with an IoT device") : t("without an IoT device")}</div></div> },
            { key: "t", label: t("Preferred times"), render: (r) => <ul className="text-xs">{r.slots.map((s) => <li key={s.startAt}>{showTime(s.startAt, i.display)}</li>)}</ul>, hideBelow: "md" },
            { key: "s", label: t("Status"), render: (r) => <div><Badge tone={r.tone}>{r.statusLabel}</Badge>{r.declineReason && <div className="mt-1 text-xs text-muted-foreground">{r.declineReason}</div>}{r.note && <div className="mt-1 text-xs text-muted-foreground">{r.note}</div>}</div> },
            { key: "j", label: t("Jobs"), render: (r) => (r.jobIds.length === 0 ? "—" : <div className="flex flex-wrap gap-1">{r.jobIds.map((id) => <Link key={id} className="text-primary underline" href={`/admin/jobs?jobId=${id}`}>{id.slice(0, 8)}</Link>)}</div>) },
            { key: "a", label: t("Actions"), render: (r) => (
              <div className="flex flex-wrap gap-1">
                {live.canApprove && (r.status === "submitted" || r.status === "approved") && remaining(r) > 0 && <Btn size="sm" variant="ghost" onClick={() => { setDialog({ kind: "approve", row: r }); }}><Icon name="check" />{t("Make units and jobs")}</Btn>}
                {live.canDecline && r.status === "submitted" && <Btn size="sm" variant="ghost" onClick={() => { setDialog({ kind: "decline", row: r }); }}><Icon name="close" />{t("Decline")}</Btn>}
              </div>
            ) },
          ]} />
        )}
      </Card>
      <Banner>{t("Making units creates a planned unit and an installation job for each aircon; the job follows the usual offer, booking and report flow. Declining needs a reason the owner sees.")}</Banner>
      {dialog?.kind === "approve" && <ApproveDialog live={live} row={dialog.row} count={remaining(dialog.row)} onClose={close} />}
      {dialog?.kind === "decline" && <DeclineDialog row={dialog.row} onClose={close} />}
    </Page>
  );
}

/** What the Core API refused when the units were made: the first message on the times or the unit, else the general one. */
function refusalText(f: ActionFailure, t: T): string {
  return Object.values(fieldErrorTexts(f, { preferredSlots: "x", alternativeSlots: "x", unitId: "x" }, t))[0] ?? t("The units could not be made; what was made stays in the list.");
}

function ApproveDialog({ live, row, count, onClose }: Readonly<{ live: InstallationsLive; row: Row; count: number; onClose: () => void }>) {
  const { t } = useI18n();
  const [pending, run] = useAction();
  const [modelId, setModelId] = useState(row.modelId ?? live.models[0]?.id ?? "");
  const [names, setNames] = useState(Array.from({ length: count }, (_, n) => `${row.place.split(" › ").at(-1) ?? "AC"} · ${String(row.jobIds.length + n + 1)}`));
  const [symptom, setSymptom] = useState(t("Install a new air conditioner (request {number})", { number: row.number }));
  const [tried, setTried] = useState(false);
  const [server, setServer] = useState<string | undefined>();
  const ids = { model: useId(), names: useId(), symptom: useId() };
  const namesError = names.some((n) => n.trim() === "" || codePoints(n.trim()) > 120) ? t("A name of 1–120 characters for each unit") : null;
  const symptomError = codePoints(symptom.trim()) < 10 || codePoints(symptom.trim()) > 2000 ? t("Symptoms must be 10–2000 characters") : null;
  const submit = () => {
    setTried(true);
    if (!modelId) { focusField(ids.model); return; }
    if (namesError) { focusField(ids.names); return; }
    if (symptomError) { focusField(ids.symptom); return; }
    run(() => approveInstallationRequest({ modelId, names, symptom, customerOrgId: row.customerOrgId, propertyId: row.propertyId, requestId: row.id, slots: row.slots, spaceId: live.spaceOf[row.id] ?? null, contactWindow: row.contactWindow ?? "" }), (v) => t("{n} units and installation jobs made for request {number}", { n: v.made, number: row.number }), onClose, (f) => { setServer(refusalText(f, t)); });
  };
  return (
    <Modal open wide onClose={onClose} title={t("Make units and jobs for request {number}", { number: row.number })} footer={<><Btn onClick={onClose}>{t("Cancel")}</Btn><Btn variant="primary" disabled={pending} onClick={submit}>{t("Make {n} units", { n: count })}</Btn></>}>
      <p className="text-[13px] text-muted-foreground">{row.customer} · {row.place} · {row.withIot ? t("with an IoT device") : t("without an IoT device")}</p>
      <Field label={t("Model")} error={tried && !modelId ? t("Choose a model") : undefined}><Select id={ids.model} value={modelId} onChange={(e) => { setModelId(e.target.value); }}><option value="">{t("Choose a model")}</option>{live.models.map((m) => <option key={m.id} value={m.id}>{m.name}</option>)}</Select></Field>
      <Field label={t("Unit names")} error={tried ? namesError ?? undefined : undefined}>
        <div className="flex flex-col gap-1.5">{names.map((n, k) => <Input key={k} id={k === 0 ? ids.names : undefined} aria-label={t("Unit {n}", { n: k + 1 })} value={n} maxLength={120} onChange={(e) => { setNames(names.map((x, j) => (j === k ? e.target.value : x))); }} />)}</div>
      </Field>
      <Field label={t("Work description (10–2000 characters)")} error={tried ? symptomError ?? undefined : undefined}><Textarea id={ids.symptom} value={symptom} onChange={(e) => { setSymptom(e.target.value); }} /></Field>
      <Banner>{t("Each job takes the owner's preferred times: {times}.", { times: row.slots.map((s) => s.startAt.slice(0, 10)).join(", ") })}</Banner>
      {server && <Banner tone="crit">{server}</Banner>}
    </Modal>
  );
}

function DeclineDialog({ row, onClose }: Readonly<{ row: Row; onClose: () => void }>) {
  const { t } = useI18n();
  const [pending, run] = useAction();
  const [reason, setReason] = useState("");
  const [tried, setTried] = useState(false);
  const id = useId();
  const problem = declineError(reason, t);
  const submit = () => {
    setTried(true);
    if (problem) { focusField(id); return; }
    run(() => declineInstallationRequest(row.id, row.version, reason), t("Request {number} declined", { number: row.number }), onClose);
  };
  return (
    <Modal open onClose={onClose} title={t("Decline request {number}", { number: row.number })} footer={<><Btn onClick={onClose}>{t("Cancel")}</Btn><Btn variant="danger" disabled={pending} onClick={submit}>{t("Decline")}</Btn></>}>
      <Field label={t("Reason the owner sees")} error={tried ? problem ?? undefined : undefined}><Textarea id={id} value={reason} onChange={(e) => { setReason(e.target.value); }} placeholder={t("e.g. no capacity this month")} /></Field>
    </Modal>
  );
}
