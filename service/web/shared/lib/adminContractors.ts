// HQ contractor register (FR-A21, DD-A21, IR131, IR133, Figma Admin 06-9 699:20918) from the Core API: one row per
// contractor organization (with or without a profile), the 90-day KPI tiles, the profile facts, the rate card in
// effect and the scheduled ones, the technicians with their certificates, and the profile / rate card forms. Pure code
// shared by the server loader and the client view; Vitest covers it.
import { klTime } from "@ac/web/lib/devices";
import { qualificationLabel } from "@ac/web/lib/partnerJobDetail";

export type ApiKpis = { period: { from: string; to: string }; offerAcceptance: number | null; arrivalInWindow: number | null; firstTimeAccepted: number | null; averageRating: number | null; ratingCount: number; reworkRate: number | null };
export type ApiProfile = {
  id: string; version: number; organizationId: string; name: string; status: "active" | "suspended"; registrationNo: string; serviceAreas: string[]; contactEmail: string;
  insuranceValidUntil: string | null; delegation: { from: string; to: string }; rateCardId: string | null; suspendedReason: string | null; kpis: ApiKpis; createdAt: string; updatedAt: string;
};
export type WorkType = "periodic_inspection" | "repair_base" | "emergency" | "rework_deduction";
export type ApiRateCard = { id: string; version: number; contractorOrgId: string; effectiveFrom: string; currency: "MYR" | "USD"; lines: { workType: WorkType; amountMinor: number; note: string | null }[] };
export type ApiCertificate = {
  id: string; version: number; membershipId: string; organizationId: string; code: string; name: string; number: string; issuedAt: string; expiresAt: string; fileName: string | null;
  status: "valid" | "expiring" | "expired" | "pending_verification" | "rejected"; renewalOf: string | null; createdAt: string;
  verifiedAt?: string | null; trainingRequestedAt?: string | null;
};
export type ApiTechnician = { id: string; displayName: string; organizationId: string; role: string; validUntil: string | null; qualifications: { code: string; validFrom: string; validUntil: string; revokedAt: string | null }[] };
type Tone = "ok" | "warn" | "crit" | "primary" | "muted";

const day = (iso: string) => klTime(iso).slice(0, 10);
const pct = (v: number | null) => (v === null ? "—" : `${Math.round(v)} %`);

/** One contractor organization of the list (Figma “contractor-a · KL, Selangor · 2 technicians · Active”). */
export type ContractorRow = { id: string; name: string; sub: string; badge: { label: string; tone: Tone } };
export function contractorRows(orgs: { id: string; name: string }[], profiles: ApiProfile[], technicians: ApiTechnician[]): ContractorRow[] {
  return [...orgs].sort((a, b) => a.name.localeCompare(b.name)).map((o) => {
    const p = profiles.find((x) => x.organizationId === o.id);
    const n = technicians.filter((t) => t.organizationId === o.id).length;
    const techs = `${n} technician${n === 1 ? "" : "s"}`;
    if (!p) return { id: o.id, name: o.name, sub: `no contractor profile yet · ${techs}`, badge: { label: "No profile", tone: "muted" } };
    return {
      id: o.id, name: p.name, sub: `${p.serviceAreas.join(", ")} · ${p.status === "suspended" ? "offers suspended" : techs}`,
      badge: p.status === "active" ? { label: "Active", tone: "ok" } : { label: "Suspended", tone: "crit" },
    };
  });
}

/** The 90-day KPI tiles (IR131 item 6); SLA targets are per plan type (SLA tab), so the tiles name the measure. */
export function kpiTiles(k: ApiKpis): { label: string; value: string; sub: string; tone?: "warn" | "crit" }[] {
  return [
    { label: "Offer acceptance", value: pct(k.offerAcceptance), sub: k.offerAcceptance === null ? "no answered offers · 90 d" : "of answered or expired offers · 90 d" },
    { label: "Arrival in window", value: pct(k.arrivalInWindow), sub: k.arrivalInWindow === null ? "no check-ins · 90 d" : "check-ins inside the scheduled slot" },
    { label: "Report accepted first time", value: pct(k.firstTimeAccepted), sub: k.firstTimeAccepted === null ? "no reviewed reports · 90 d" : "first submitted version accepted" },
    { label: "Customer rating", value: k.averageRating === null ? "—" : `${k.averageRating.toFixed(1)} ★`, sub: `${k.ratingCount} rating${k.ratingCount === 1 ? "" : "s"} (Client)` },
    { label: "Rework rate", value: pct(k.reworkRate), sub: k.reworkRate === null ? "no completed jobs · 90 d" : "completed jobs with a rework follow-up", tone: k.reworkRate !== null && k.reworkRate > 10 ? "warn" : undefined },
  ];
}

/** The profile facts (Figma: Registration, Service areas, Delegation period, Contact, Insurance). */
export function profileFacts(p: ApiProfile, now: number): [string, string][] {
  const ins = p.insuranceValidUntil;
  return [
    ["Registration", p.registrationNo], ["Service areas", p.serviceAreas.join(", ")], ["Delegation period", `${day(p.delegation.from)} – ${day(p.delegation.to)}`],
    ["Contact", p.contactEmail], ["Insurance", !ins ? "not recorded — the delegation runs a year" : Date.parse(ins) <= now ? `expired ${day(ins)}` : `valid to ${day(ins)}`],
  ];
}

export const WORK_TYPES: { id: WorkType; label: string }[] = [
  { id: "periodic_inspection", label: "Periodic inspection (per unit)" }, { id: "repair_base", label: "Repair — base visit" },
  { id: "emergency", label: "Emergency call-out (< 4 h)" }, { id: "rework_deduction", label: "Rework deduction (2nd return)" },
];
/** “450.00”, a rework deduction as “− 120.00”. */
export const rateAmount = (l: { workType: WorkType; amountMinor: number }) => `${l.workType === "rework_deduction" && l.amountMinor > 0 ? "− " : ""}${(l.amountMinor / 100).toFixed(2)}`;

/** The rate card in effect (rateCardId) and the versions scheduled after it. */
export function rateCardView(cards: ApiRateCard[], rateCardId: string | null, now: number) {
  const current = cards.find((c) => c.id === rateCardId) ?? null;
  const lines = (c: ApiRateCard) => WORK_TYPES.flatMap((w) => c.lines.filter((l) => l.workType === w.id).map((l) => ({ label: w.label, amount: rateAmount(l), note: l.note })));
  return {
    current: current ? { title: `Rate card v${current.version} (from ${day(current.effectiveFrom)})`, currency: current.currency, lines: lines(current) } : null,
    scheduled: cards.filter((c) => Date.parse(c.effectiveFrom) > now).sort((a, b) => Date.parse(a.effectiveFrom) - Date.parse(b.effectiveFrom)).map((c) => ({ id: c.id, title: `v${c.version} from ${day(c.effectiveFrom)} · scheduled`, currency: c.currency, lines: lines(c) })),
    nextVersion: Math.max(0, ...cards.map((c) => c.version)) + 1,
  };
}

/** One technician of the contractor with the qualifications and the certificate state (Figma “1 expiring · 10-15”). */
export type TechRow = { id: string; name: string; sub: string; badge: { label: string; tone: Tone } };
export function technicianRows(technicians: ApiTechnician[], certs: ApiCertificate[], now: number): TechRow[] {
  return technicians.map((t) => {
    const ended = t.validUntil && Date.parse(t.validUntil) <= now;
    const quals = t.qualifications.filter((q) => !q.revokedAt);
    const sub = ended ? `membership ended ${klTime(t.validUntil!).slice(5, 10)}` : quals.map((q) => qualificationLabel(q.code).replace(/ (unit )?work$/, "")).join(" · ") || "no qualifications";
    const mine = certs.filter((c) => c.membershipId === t.id);
    const pending = mine.filter((c) => c.status === "pending_verification").length;
    const expiring = mine.filter((c) => c.status === "expiring");
    const soonest = [...quals].filter((q) => Date.parse(q.validUntil) > now).sort((a, b) => Date.parse(a.validUntil) - Date.parse(b.validUntil))[0];
    let badge: TechRow["badge"];
    if (ended) badge = { label: "Expired", tone: "muted" };
    else if (pending) badge = { label: `${pending} awaiting verification`, tone: "primary" };
    else if (expiring.length) badge = { label: `${expiring.length} expiring · ${klTime(expiring.map((c) => c.expiresAt).sort()[0]).slice(5, 10)}`, tone: "warn" };
    else if (!quals.length || !soonest) badge = { label: quals.length ? "Expired" : "None", tone: quals.length ? "crit" : "muted" };
    else if (Date.parse(soonest.validUntil) - now <= 30 * 86_400_000) badge = { label: `expiring · ${klTime(soonest.validUntil).slice(5, 10)}`, tone: "warn" };
    else badge = { label: "Valid", tone: "ok" };
    return { id: t.id, name: t.displayName, sub, badge };
  });
}

/** The certificates waiting for HQ (Verify uploads), oldest first. */
export const pendingCertificates = (certs: ApiCertificate[]) => certs.filter((c) => c.status === "pending_verification").sort((a, b) => Date.parse(a.createdAt) - Date.parse(b.createdAt));

/** The profile form (IR131 item 1): registration 1–64, 1–20 distinct service areas of 1–80, a contact e-mail. */
export function profileErrors(f: { registrationNo: string; serviceAreas: string; contactEmail: string }): Record<string, string> {
  const e: Record<string, string> = {};
  const r = f.registrationNo.trim().length;
  if (r < 1 || r > 64) e.registrationNo = "1–64 characters.";
  const areas = areasOf(f.serviceAreas);
  if (areas.length < 1 || areas.length > 20 || areas.some((a) => a.length > 80) || new Set(areas.map((a) => a.toLowerCase())).size !== areas.length) e.serviceAreas = "1–20 different areas, comma-separated.";
  if (!/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(f.contactEmail.trim()) || f.contactEmail.trim().length > 254) e.contactEmail = "A valid e-mail address.";
  return e;
}
export const areasOf = (v: string) => v.split(",").map((a) => a.trim()).filter(Boolean);

/** The rate card form (IR131 item 3): effective from a future time, 1–4 lines of different work types, amounts ≥ 0. */
export function rateCardErrors(f: { effectiveFrom: string | null; lines: { workType: string; amount: string; note: string }[] }, now: number): Record<string, string> {
  const e: Record<string, string> = {};
  if (!f.effectiveFrom || Date.parse(f.effectiveFrom) <= now) e.effectiveFrom = "Effective from a future date.";
  if (f.lines.length < 1 || f.lines.length > 4) e.lines = "1–4 lines.";
  else if (new Set(f.lines.map((l) => l.workType)).size !== f.lines.length) e.lines = "Each work type once.";
  else if (f.lines.some((l) => !/^\d+(\.\d{1,2})?$/.test(l.amount.trim()))) e.lines = "Amounts are non-negative with at most two decimals.";
  else if (f.lines.some((l) => l.note.trim().length > 200)) e.lines = "Notes up to 200 characters.";
  return e;
}
/** The first day of next month in the display zone (00:00 KL), the default start of a new rate card. */
export function nextMonthStart(now: number): string {
  const kl = new Date(now + 8 * 3600_000);
  return new Date(Date.UTC(kl.getUTCFullYear(), kl.getUTCMonth() + 1, 1) - 8 * 3600_000).toISOString();
}

/** Readable refusals of the contractor register writes. */
export function contractorRefusal(f: { code: string; messageKey: string; fieldErrors: Record<string, string> }): string {
  const keys: Record<string, string> = {
    "errors.profile_exists": "this organization already has a contractor profile",
    "error.notContractor": "the organization is not a contractor", "error.organizationFixed": "a profile’s organization cannot change",
    "error.invalidState": "nothing to change in the current state", "error.versionConflict": "the record changed meanwhile — the latest version is shown",
    "error.count": "wrong number of entries", "error.invalid": "not a valid value", "error.length": "too long", "error.required": "required", "error.past": "must be in the future",
  };
  const fields = Object.entries(f.fieldErrors ?? {}).map(([k, v]) => `${k}: ${keys[v] ?? v}`);
  if (fields.length) return fields.join(" · ");
  if (keys[f.messageKey]) return `${f.code === "CONFLICT" ? "Not saved (CONFLICT): " : ""}${keys[f.messageKey]}.`;
  if (f.code === "NOT_FOUND") return "The contractor or the certificate no longer exists.";
  return `${f.code} — ${f.messageKey}`;
}
