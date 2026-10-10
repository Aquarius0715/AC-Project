// The contractor's certifications (FR-P09, DD-P09, Figma Contractor 04-6/04-7) from the Core API: one row per
// technician and qualification — the certificate on file (certificates.list) with its renewal, the qualification HQ
// granted without a certificate on file, or a qualification a booked job needs that the technician does not hold —
// with the jobs an ending certificate blocks, the KPIs, the assignment impact and the renewal / training forms.
// Pure code shared by the server loader and the client view; Vitest covers it. Texts in the display language (`t` /
// `i`, IR277); certificate dates are Kuala Lumpur days, a job's time an instant in the display time zone.
import { EN, translator, type I18n, type T } from "@ac/web/lib/i18n";
import { businessDay } from "@ac/web/lib/clientBilling";
import { klDay, type Slot } from "@ac/web/lib/partnerOverview";
import { qualificationLabel } from "@ac/web/lib/partnerJobDetail";
import type { ApiCertificate } from "@ac/web/lib/adminContractors";
import { activeNow, klStart, QUALIFICATION_CODES, type ApiTeamMember } from "@ac/web/lib/partnerTeam";

const en = translator("en");
const DAY = 86_400_000;
export const WINDOWS = [30, 60, 90] as const;
export const CERT_STATUSES = ["valid", "expiring", "expired", "pending", "rejected", "not_held"] as const;
export type CertStatus = (typeof CERT_STATUSES)[number];
/** A certificate as certificates.list returns it (Certificate of service-contracts.ts). */
export type ApiCert = ApiCertificate;
/** A booked job of a technician with the qualifications its unit's maintenance scope needs (IR123 item 3). */
export type CertJob = { jobId: string; short: string; technicianId: string; slot: Slot; text: string; unit: string; required: string[] };

/** The tab's conditions in the URL (DD-P09): a technician, a status and the expiring window (30 / 60 / 90 days,
 * default 60). */
export function certQuery(sp: { membershipId?: string; status?: string; expiringWithinDays?: string }, technicianIds: string[]) {
  const within = WINDOWS.find((w) => String(w) === sp.expiringWithinDays) ?? 60;
  return {
    membershipId: sp.membershipId && technicianIds.includes(sp.membershipId) ? sp.membershipId : null,
    status: (CERT_STATUSES as readonly string[]).includes(sp.status ?? "") ? (sp.status as CertStatus) : null, within,
  };
}

export type CertRow = {
  key: string; technicianId: string; technician: string; code: string; name: string; detail: string; expires: string; expiresAt: string | null;
  status: CertStatus; badge: { text: string; tone: "ok" | "warn" | "crit" | "primary" | "muted" }; days: number | null;
  blocks: { jobId: string; text: string; day: string }[]; cert: { id: string; version: number; number: string } | null; renewalPending: boolean;
  training: string | null; actions: ("view" | "renew" | "training" | "add")[];
};

/** The rows (Figma 04-6): per technician and qualification the certificate on file — its latest one, with a pending
 * renewal noted — else the HQ grant without a certificate on file, else “not held” when a booked job needs it. A
 * valid certificate ending within the window is “Expiring”; the jobs that end after it are the ones it blocks. A
 * certificate keeps the name it was uploaded with; only an active technician can get a new one (IR133 item 1). */
export function certRows(members: ApiTeamMember[], certs: ApiCert[], jobs: CertJob[], within: number, nowMs: number, i: I18n = EN): CertRow[] {
  const { t, display: { locale } } = i;
  const rows: CertRow[] = [];
  const blocks = (m: string, code: string, endsAt: number) => jobs.filter((j) => j.technicianId === m && j.required.includes(code) && Date.parse(j.slot.endAt) > endsAt)
    .sort((a, b) => Date.parse(a.slot.startAt) - Date.parse(b.slot.startAt)).map((j) => {
      const day = klDay(new Date(Date.parse(j.slot.startAt) + 8 * 3600_000).toISOString().slice(0, 10), locale); // the visit's Kuala Lumpur day
      return { jobId: j.jobId, text: `${j.short} (${day})`, day };
    });
  const state = (endsAt: number): { status: CertStatus; days: number } => {
    const days = Math.ceil((endsAt - nowMs) / DAY);
    return { status: endsAt <= nowMs ? "expired" : days <= within ? "expiring" : "valid", days };
  };
  const badgeOf = (s: CertStatus, days: number | null): CertRow["badge"] => ({
    valid: { text: t("Valid"), tone: "ok" as const }, expiring: { text: t("Expiring · {n} d", { n: days ?? 0 }), tone: "warn" as const }, expired: { text: t("Expired"), tone: "crit" as const },
    pending: { text: t("Pending HQ verification"), tone: "primary" as const }, rejected: { text: t("Rejected"), tone: "crit" as const }, not_held: { text: t("Not held"), tone: "muted" as const },
  })[s];
  for (const m of members) {
    const active = activeNow(m, nowMs);
    const mine = certs.filter((c) => c.membershipId === m.id);
    const groups = new Map<string, ApiCert[]>();
    for (const c of mine) {
      const k = c.code === "other" ? `other:${c.id}` : c.code;
      groups.set(k, [...(groups.get(k) ?? []), c]);
    }
    // a renewal of an "other" certificate joins its original
    for (const [k, list] of [...groups]) {
      const of = list[0].renewalOf;
      if (k.startsWith("other:") && of && groups.has(`other:${of}`)) { groups.get(`other:${of}`)!.push(...list); groups.delete(k); }
    }
    const viaGrant = new Map<string, "pending" | "rejected">(); // no certificate on file yet: the HQ grant still decides (BR-P09)
    for (const [k, list] of groups) {
      const onFile = list.filter((c) => c.status === "valid" || c.status === "expiring" || c.status === "expired").sort((a, b) => Date.parse(b.expiresAt) - Date.parse(a.expiresAt))[0];
      const pending = list.filter((c) => c.status === "pending_verification").sort((a, b) => Date.parse(b.createdAt) - Date.parse(a.createdAt))[0];
      const rejected = list.filter((c) => c.status === "rejected").sort((a, b) => Date.parse(b.createdAt) - Date.parse(a.createdAt))[0];
      if (!onFile && (m.qualifications ?? []).some((q) => q.code === k && !q.revokedAt && q.validUntil)) { viaGrant.set(k, pending ? "pending" : "rejected"); continue; }
      const shown = onFile ?? pending ?? rejected!;
      const s = onFile ? state(Date.parse(onFile.expiresAt)) : { status: (pending ? "pending" : "rejected") as CertStatus, days: null };
      const ends = Date.parse(shown.expiresAt);
      const training = onFile?.trainingRequestedAt ? t("Training requested {date}", { date: businessDay(onFile.trainingRequestedAt, locale) }) : null;
      const actions: CertRow["actions"] = ["view"];
      if (active && !pending && (s.status === "expiring" || s.status === "expired" || s.status === "rejected")) actions.push("renew");
      if (active && onFile && !training && (s.status === "expiring" || s.status === "expired")) actions.push("training");
      rows.push({
        key: `${m.id}:${k}`, technicianId: m.id, technician: m.displayName, code: shown.code, name: shown.name || qualificationLabel(shown.code, t),
        detail: t("{number} · issued {date}", { number: shown.number, date: businessDay(shown.issuedAt, locale) }), expires: businessDay(shown.expiresAt, locale), expiresAt: shown.expiresAt,
        status: s.status, badge: badgeOf(s.status, s.days), days: s.days, blocks: s.status === "pending" || s.status === "rejected" ? [] : blocks(m.id, shown.code, ends),
        cert: { id: shown.id, version: shown.version, number: shown.number }, renewalPending: !!onFile && !!pending, training, actions,
      });
    }
    for (const code of QUALIFICATION_CODES) {
      if (groups.has(code) && !viaGrant.has(code)) continue;
      const upload = viaGrant.get(code);
      const grant = (m.qualifications ?? []).filter((q) => q.code === code && !q.revokedAt && q.validUntil).sort((a, b) => Date.parse(b.validUntil!) - Date.parse(a.validUntil!))[0];
      if (grant) {
        const s = state(Date.parse(grant.validUntil!));
        rows.push({
          key: `${m.id}:${code}`, technicianId: m.id, technician: m.displayName, code, name: qualificationLabel(code, t),
          detail: t(upload === "pending" ? "granted by HQ · the upload waits for HQ" : upload === "rejected" ? "granted by HQ · the upload was rejected" : "granted by HQ · no certificate on file"),
          expires: businessDay(grant.validUntil!, locale), expiresAt: grant.validUntil!, status: s.status, badge: badgeOf(s.status, s.days), days: s.days,
          blocks: blocks(m.id, code, Date.parse(grant.validUntil!)), cert: null, renewalPending: upload === "pending", training: null, actions: active && upload !== "pending" ? ["add"] : [],
        });
        continue;
      }
      const needed = blocks(m.id, code, -Infinity);
      if (needed.length) {
        rows.push({
          key: `${m.id}:${code}`, technicianId: m.id, technician: m.displayName, code, name: qualificationLabel(code, t), detail: t("not held"), expires: "—", expiresAt: null,
          status: "not_held", badge: badgeOf("not_held", null), days: null, blocks: needed, cert: null, renewalPending: false, training: null, actions: active ? ["add"] : [],
        });
      }
    }
  }
  const rank: Record<CertStatus, number> = { expired: 0, expiring: 1, not_held: 2, rejected: 3, pending: 4, valid: 5 };
  return rows.sort((a, b) => a.technician.localeCompare(b.technician) || rank[a.status] - rank[b.status] || a.name.localeCompare(b.name));
}

/** The four KPIs (Figma 04-6): in date, expiring within the window (the soonest named), expired, pending HQ
 * verification (renewals included). */
export function certKpis(rows: CertRow[], certs: ApiCert[], within: number, t: T = en) {
  const expiring = rows.filter((r) => r.status === "expiring").sort((a, b) => (a.days ?? 0) - (b.days ?? 0));
  const expired = rows.filter((r) => r.status === "expired");
  const ids = new Set(rows.map((r) => r.technicianId));
  return [
    { label: t("Valid"), value: rows.filter((r) => r.status === "valid" || r.status === "expiring").length, sub: t("in date"), chip: null },
    { label: t("Expiring ≤ {n} days", { n: within }), value: expiring.length, sub: null, chip: expiring[0] ? `${expiring[0].technician} · ${t("{n} d", { n: expiring[0].days ?? 0 })}` : null },
    { label: t("Expired"), value: expired.length, sub: [...new Set(expired.map((r) => r.technician))].join(", ") || "—", chip: null },
    { label: t("Pending HQ verification"), value: certs.filter((c) => c.status === "pending_verification" && ids.has(c.membershipId)).length, sub: t("uploads you sent"), chip: null },
  ];
}

/** The assignment impact (Figma 04-6): the first job an expiring, expired or missing qualification blocks. */
export function impact(rows: CertRow[], jobs: CertJob[], t: T = en) {
  const row = rows.filter((r) => r.blocks.length && (r.status === "expiring" || r.status === "expired" || r.status === "not_held"))
    .map((r) => ({ r, job: jobs.find((j) => j.jobId === r.blocks[0].jobId)! })).sort((a, b) => Date.parse(a.job.slot.startAt) - Date.parse(b.job.slot.startAt))[0];
  if (!row) return null;
  const { r, job } = row;
  const day = r.blocks[0].day;
  const text = r.status === "not_held" ? t("{name} does not hold {cert}. {job} ({unit}, {day}) needs it — add the certificate or reassign.", { name: r.technician, cert: r.name, job: job.short, unit: job.unit, day })
    : t(r.status === "expired" ? "{cert} of {name} expired {date}. {job} ({unit}, {day}) needs it — renew it or reassign." : "{cert} of {name} expires {date}. {job} ({unit}, {day}) needs it — renew before then or reassign.",
      { cert: r.name, name: r.technician, date: r.expires, job: job.short, unit: job.unit, day });
  return { text, jobId: job.jobId, rows: [[t("Job"), `${job.short} · ${job.unit}`], [t("Slot"), job.text], [t("Assigned"), r.technician], [t("Requires"), qualificationLabel(r.code, t)]] as [string, string][] };
}

export type CertForm = { membershipId: string; code: string; name: string; number: string; issued: string; expires: string; file: { name: string; type: string; size: number } | null };
const isDate = (d: string) => /^\d{4}-\d{2}-\d{2}$/.test(d) && !Number.isNaN(Date.parse(`${d}T00:00:00Z`));
export const FILE_TYPES = ["application/pdf", "image/jpeg", "image/png"];
/** The renewal / certificate form's own checks (IR133 item 1): a name of 1–120 and a number of 1–64 characters, the
 * issue day before the expiry day, and a PDF, JPEG or PNG of at most 10 MB. */
export function certificateErrors(f: CertForm, t: T = en): Partial<Record<"membershipId" | "name" | "number" | "issued" | "expires" | "file", string>> {
  const e: Partial<Record<"membershipId" | "name" | "number" | "issued" | "expires" | "file", string>> = {};
  if (!f.membershipId) e.membershipId = t("Choose a technician");
  const name = f.name.trim().length, number = f.number.trim().length;
  if (name < 1 || name > 120) e.name = t("1–120 characters");
  if (number < 1 || number > 64) e.number = t("1–64 characters");
  if (!isDate(f.issued)) e.issued = t("Choose the issue date");
  if (!isDate(f.expires)) e.expires = t("Choose the expiry date");
  else if (isDate(f.issued) && f.expires <= f.issued) e.expires = t("The expiry date must be after the issue date");
  if (!f.file) e.file = t("Attach the certificate file");
  else if (!FILE_TYPES.includes(f.file.type)) e.file = t("Use a PDF, JPG or PNG file");
  else if (f.file.size > 10_000_000) e.file = t("The file is larger than 10 MB");
  return e;
}
/** The instants a form's Kuala Lumpur days stand for: the start of the issue day and of the expiry day. */
export const certDates = (f: { issued: string; expires: string }) => ({ issuedAt: klStart(f.issued), expiresAt: klStart(f.expires) });

const FIELDS: Record<string, string> = { name: "name", number: "number", expiresAt: "expires", issuedAt: "issued", file: "file", code: "code", membershipId: "membershipId", note: "note" };
const TEXT: Record<string, string> = {
  "error.required": "Required", "error.length": "Too long", "error.range": "The expiry date must be after the issue date", "error.invalidFile": "Use a PDF, JPG or PNG file of at most 10 MB",
  "error.invalid": "Not a valid choice",
};
/** Readable refusals of certificates.submit and certificates.requestTraining: the fields they name, or the reason. */
export function certificateRefusal(f: { code: string; messageKey: string; fieldErrors: Record<string, string> }, t: T = en): { fields: Record<string, string>; text: string | null } {
  if (f.code === "NOT_FOUND") return { fields: {}, text: t("That technician or certificate is no longer in your company — reload the page.") };
  if (f.code === "CONFLICT") return { fields: {}, text: t("The certificate changed in the meantime — the page shows the latest state.") };
  if (f.code !== "VALIDATION") return { fields: {}, text: null };
  const fields = Object.fromEntries(Object.entries(f.fieldErrors).map(([k, v]) => [FIELDS[k] ?? k, TEXT[v] ? t(TEXT[v]) : v]));
  return { fields, text: Object.keys(fields).length ? null : t("Check the input.") };
}
/** “412 KB”, “2.4 MB”: a file's size as the form shows it. */
export const fileSize = (bytes: number) => (bytes < 1_000_000 ? `${Math.max(1, Math.round(bytes / 1000))} KB` : `${Math.round(bytes / 100_000) / 10} MB`);
