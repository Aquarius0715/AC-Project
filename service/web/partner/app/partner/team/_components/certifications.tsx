"use client";

import { useState } from "react";
import { Badge, Banner, Btn, Card, cx, DataTable, Field, Input, LinkBtn, Modal, Select, SummaryList, Textarea } from "@ac/web/components/ui";
import { useT } from "@ac/web/components/I18n";
import { useAction } from "@ac/web/lib/useAction";
import { useUrlPatch } from "@ac/web/lib/useUrlPatch";
import { certificateErrors, certificateRefusal, fileSize, FILE_TYPES, type CertForm, type CertRow } from "@ac/web/lib/partnerCertificates";
import { requestTraining, submitCertificate } from "../actions";
import type { TeamLive } from "../_lib/load";

type Live = Extract<TeamLive, { notFound: false }>;
type Open = { kind: "renew" | "add"; row: CertRow | null } | { kind: "training" | "view"; row: CertRow } | null;

/** The Certifications tab (FR-P09, Figma Contractor 04-6/04-7): filters by technician, status and the expiring window;
 * the KPIs; one row per technician and qualification with the jobs an ending certificate blocks; the assignment
 * impact; renewals, new certificates and training requests. Texts in the user's display language; the dates come
 * formatted from the loader (IR277). */
export function Certifications({ live }: { live: Live }) {
  const t = useT();
  const patch = useUrlPatch();
  const c = live.certificates;
  const q = c.query;
  const [open, setOpen] = useState<Open>(null);
  const action = (r: CertRow) => (
    <div className="flex flex-wrap items-center justify-end gap-1.5">
      {r.actions.includes("renew") && <Btn size="sm" variant={r.status === "expiring" ? "primary" : "secondary"} onClick={() => setOpen({ kind: "renew", row: r })}>{t("Upload renewal")}</Btn>}
      {r.actions.includes("add") && <Btn size="sm" onClick={() => setOpen({ kind: "add", row: r })}>{t(r.status === "not_held" ? "Add certificate" : "Upload certificate")}</Btn>}
      {r.actions.includes("training") && <Btn size="sm" onClick={() => setOpen({ kind: "training", row: r })}>{t("Request training")}</Btn>}
      {r.actions.includes("view") && <Btn size="sm" variant="ghost" onClick={() => setOpen({ kind: "view", row: r })}>{t("View")}</Btn>}
      {r.renewalPending && <span className="text-[11px] text-primary">{t("Renewal pending HQ verification")}</span>}
      {r.training && <span className="text-[11px] text-muted">{r.training}</span>}
    </div>
  );
  return (
    <>
      <div className="flex flex-wrap items-center gap-2">
        <Select aria-label={t("Technician")} className="w-auto" value={q.membershipId ?? ""} onChange={(e) => patch({ membershipId: e.target.value || null })}>
          <option value="">{t("Technician: All")}</option>
          {c.people.map((p) => <option key={p.id} value={p.id}>{p.name}</option>)}
        </Select>
        <Select aria-label={t("Status")} className="w-auto" value={q.status ?? ""} onChange={(e) => patch({ status: e.target.value || null })}>
          <option value="">{t("Status: All")}</option>
          {c.statuses.map((s) => <option key={s.value} value={s.value}>{s.text}</option>)}
        </Select>
        <Select aria-label={t("Expiring within")} className="w-auto" value={String(q.within)} onChange={(e) => patch({ expiringWithinDays: e.target.value === "60" ? null : e.target.value })}>
          {c.windows.map((w) => <option key={w.value} value={w.value}>{t("Expiring within: {window}", { window: w.text })}</option>)}
        </Select>
      </div>
      <div className="grid-fluid" style={{ ["--min" as string]: "180px" }}>
        {c.kpis.map((k) => (
          <section key={k.label} aria-label={k.label} className="rounded-2xl border border-line bg-surface p-4">
            <div className="text-xs text-muted">{k.label}</div>
            <div className="text-2xl font-bold">{k.value}</div>
            {k.chip ? <span className="mt-1 inline-block rounded-full bg-warn-soft px-2 py-0.5 text-[11px] font-semibold text-warn">{k.chip}</span> : <div className="mt-1 text-[11px] text-muted">{k.sub}</div>}
          </section>
        ))}
      </div>
      <div className="split">
        <Card title={t("Certificates — {company}", { company: live.company })} action={<Btn size="sm" onClick={() => setOpen({ kind: "add", row: null })} disabled={!c.people.some((p) => p.active)}>{t("+ Add certificate")}</Btn>}>
          {c.rows.length === 0 ? <p className="py-4 text-[13px] text-muted">{t("No certificates match these filters.")}</p> : (
            <DataTable rowKey={(r) => r.key} rows={c.rows} cols={[
              { key: "t", label: t("Technician"), render: (r) => <b className="whitespace-nowrap">{r.technician}</b> },
              { key: "c", label: t("Certificate"), render: (r) => <span><b className="block">{r.name}</b><span className="block text-[11px] text-muted">{r.detail}</span></span> },
              { key: "e", label: t("Expires"), render: (r) => <span className="whitespace-nowrap">{r.expires}</span> },
              { key: "s", label: t("Status"), render: (r) => <Badge tone={r.badge.tone}>{r.badge.text}</Badge> },
              { key: "b", label: t("Blocks"), render: (r) => (r.blocks.length ? r.blocks.map((b) => b.text).join(", ") : "—") },
              { key: "a", label: t("Action"), render: action },
            ]} />
          )}
          <p className="mt-2 text-[11px] text-muted">{t("Certificates are checked when you assign a technician (members.eligible). An expired certificate makes the technician ineligible for jobs that need it; nothing is revoked automatically before expiry.")}</p>
        </Card>
        <Card title={t("Assignment impact")}>
          {c.impact ? (
            <div className="flex flex-col gap-3">
              <Banner tone="warn">{c.impact.text}</Banner>
              <SummaryList items={c.impact.rows} />
              <div><LinkBtn href={`/partner/schedule?jobId=${c.impact.jobId}`} size="sm">{t("Open assignment →")}</LinkBtn></div>
            </div>
          ) : <p className="text-[13px] text-muted">{t("No booked job depends on an expiring, expired or missing certificate.")}</p>}
          <p className="mt-3 text-[11px] text-muted">{t("Reminders are sent 60, 30 and 7 days before expiry to the contractor admin (Notifications).")}</p>
        </Card>
      </div>
      {open?.kind === "renew" || open?.kind === "add" ? <CertificateModal live={live} kind={open.kind} row={open.row} onClose={() => setOpen(null)} /> : null}
      {open?.kind === "training" && <TrainingModal row={open.row} onClose={() => setOpen(null)} />}
      {open?.kind === "view" && <ViewModal row={open.row} onClose={() => setOpen(null)} />}
    </>
  );
}

/** Upload renewal (Figma 04-7) or a new certificate: the days are Kuala Lumpur days; the file is PDF, JPG or PNG of at
 * most 10 MB; it stays “Pending HQ verification” and the current expiry counts until HQ approves (BR-P09). */
function CertificateModal({ live, kind, row, onClose }: { live: Live; kind: "renew" | "add"; row: CertRow | null; onClose: () => void }) {
  const t = useT();
  const [pending, run] = useAction();
  const c = live.certificates;
  const renew = kind === "renew" && row?.cert;
  const [form, setForm] = useState<CertForm>({
    membershipId: row?.technicianId ?? c.people.find((p) => p.active)?.id ?? "", code: row?.code ?? "demo_indoor", name: row?.name ?? c.codes[0].text,
    number: "", issued: "", expires: "", file: null,
  });
  const [file, setFile] = useState<File | null>(null);
  const [tried, setTried] = useState(false);
  const [server, setServer] = useState<Record<string, string>>({});
  const [refused, setRefused] = useState<string | null>(null);
  const local = tried ? certificateErrors(form, t) : {};
  const err = (k: string) => (local as Record<string, string | undefined>)[k] ?? server[k];
  const set = (p: Partial<CertForm>) => { setForm({ ...form, ...p }); setServer({}); setRefused(null); };
  const save = () => {
    setTried(true);
    if (Object.keys(certificateErrors(form, t)).length || !file) return;
    const data = new FormData();
    for (const [k, v] of Object.entries({ membershipId: form.membershipId, code: form.code, name: form.name, number: form.number, issued: form.issued, expires: form.expires })) data.set(k, v);
    if (renew) data.set("renewalOf", row!.cert!.id);
    data.set("file", file);
    run(() => submitCertificate(data), t("Submitted for HQ verification — it counts for eligibility once HQ approves it."), onClose,
      (f) => { const r = certificateRefusal(f, t); setServer(r.fields); setRefused(r.text); });
  };
  return (
    <Modal open onClose={onClose} title={renew ? t("Upload renewal — {name}", { name: row!.name }) : t("Add certificate")} footer={<>
      <Btn onClick={onClose}>{t("Cancel")}</Btn>
      <Btn variant="primary" disabled={pending} onClick={save}>{t("Submit for verification")}</Btn>
    </>}>
      {renew ? (
        <SummaryList items={[[t("Technician"), row!.technician], [t("Current"), t("{number} · expires {date}", { number: row!.cert!.number, date: row!.expires })]]} />
      ) : (
        <>
          <Field label={t("Technician")} error={err("membershipId")}>
            <Select value={form.membershipId} onChange={(e) => set({ membershipId: e.target.value })}>
              {c.people.filter((p) => p.active).map((p) => <option key={p.id} value={p.id}>{p.name}</option>)}
            </Select>
          </Field>
          <div className="grid-fluid" style={{ ["--min" as string]: "200px" }}>
            <Field label={t("Qualification")} error={err("code")}>
              <Select value={form.code} onChange={(e) => set({ code: e.target.value, name: c.codes.find((x) => x.code === e.target.value)?.text ?? form.name })}>
                {c.codes.map((x) => <option key={x.code} value={x.code}>{x.text}</option>)}
              </Select>
            </Field>
            <Field label={t("Certificate name")} error={err("name")}><Input value={form.name} maxLength={120} onChange={(e) => set({ name: e.target.value })} /></Field>
          </div>
        </>
      )}
      <div className="grid-fluid" style={{ ["--min" as string]: "180px" }}>
        <Field label={t("Issued on")} error={err("issued")}><Input type="date" value={form.issued} onChange={(e) => set({ issued: e.target.value })} /></Field>
        <Field label={t("Expires on")} error={err("expires")}><Input type="date" value={form.expires} min={form.issued || undefined} onChange={(e) => set({ expires: e.target.value })} /></Field>
      </div>
      <Field label={t("Certificate number")} error={err("number")}><Input value={form.number} maxLength={64} onChange={(e) => set({ number: e.target.value })} /></Field>
      <div className="flex flex-col gap-1 text-xs font-semibold text-ink">
        <span>{t("Certificate file")}</span>
        <label className={cx("block cursor-pointer rounded-xl border border-dashed px-4 py-4 text-center hover:bg-surface2", err("file") ? "border-crit" : "border-line")}>
          <input type="file" accept={FILE_TYPES.join(",")} className="sr-only" onChange={(e) => { const f = e.target.files?.[0] ?? null; setFile(f); set({ file: f ? { name: f.name, type: f.type, size: f.size } : null }); }} />
          <b className="block text-[13px]">{file ? `📄 ${file.name} · ${fileSize(file.size)}` : t("Choose a file")}</b>
          <span className="block text-[11px] font-normal text-muted">{t("PDF / JPG / PNG, up to 10 MB")}{file ? ` · ${t("Replace")}` : ""}</span>
        </label>
        {err("file") && <span className="font-medium text-crit">✕ {err("file")}</span>}
      </div>
      <Banner>{t(renew ? "The renewal is “Pending HQ verification” until HQ checks it. Until then the current expiry date is used for eligibility." : "The certificate is “Pending HQ verification” until HQ checks it; only then does it count for eligibility.")}</Banner>
      {refused && <Banner tone="crit">{refused}</Banner>}
    </Modal>
  );
}

/** Request training for an expiring or expired certificate (certificates.requestTraining, note 1–1000). */
function TrainingModal({ row, onClose }: { row: CertRow; onClose: () => void }) {
  const t = useT();
  const [pending, run] = useAction();
  const [note, setNote] = useState("");
  const [tried, setTried] = useState(false);
  const [refused, setRefused] = useState<string | null>(null);
  const bad = note.trim().length < 1 || note.trim().length > 1000;
  const save = () => {
    setTried(true);
    if (bad) return;
    run(() => requestTraining(row.cert!.id, row.cert!.version, note), t("Training requested."), onClose, (f) => setRefused(certificateRefusal(f, t).text ?? t("Check the input.")));
  };
  return (
    <Modal open onClose={onClose} title={t("Request training — {name}", { name: row.name })} footer={<>
      <Btn onClick={onClose}>{t("Cancel")}</Btn>
      <Btn variant="primary" disabled={pending} onClick={save}>{t("Request training")}</Btn>
    </>}>
      <SummaryList items={[[t("Technician"), row.technician], [t("Certificate"), `${row.name} · ${row.badge.text}`]]} />
      <Field label={t("Note (required, 1–1000 characters)")} error={tried && bad ? t("A note is required (1–1000 characters)") : undefined}>
        <Textarea value={note} maxLength={1000} onChange={(e) => setNote(e.target.value)} placeholder={t("Refresher course before the certificate expires.")} />
      </Field>
      {refused && <Banner tone="crit">{refused}</Banner>}
    </Modal>
  );
}

function ViewModal({ row, onClose }: { row: CertRow; onClose: () => void }) {
  const t = useT();
  return (
    <Modal open onClose={onClose} title={row.name} footer={<Btn onClick={onClose}>{t("Close")}</Btn>}>
      <SummaryList items={[[t("Technician"), row.technician], [t("Certificate number"), row.cert?.number ?? "—"], [t("Details"), row.detail], [t("Expires"), row.expires], [t("Status"), row.badge.text]]} />
      <p className="text-[11px] text-muted">{t("HQ keeps the uploaded file; ask HQ for a copy.")}</p>
    </Modal>
  );
}
