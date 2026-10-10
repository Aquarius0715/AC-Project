// Digital MRV demo (FR-A14, DATA_SOURCE=api): Scope 2 reports, their versions and emission factors projected for
// /admin/mrv. Pure code shared by the Server Component and the client view; texts in the display language (`t` / `i`).
// Report periods are Kuala Lumpur business time and say so; review times are in the display time zone (IR297).
import { klInstant, klLocal, klStamp, one, saving, summaryView, type ApiBaseline, type ApiEnergySummary, type BoundaryId, type SummaryView } from "@ac/web/lib/energy";
import { EN, showTime, translator, type I18n, type T } from "@ac/web/lib/i18n";

const en = translator("en");

/** MRVConditions of service-contracts.ts. */
export type MRVConditions = {
  from: string; to: string; unitIds: string[]; baselineId: string; baselineVersion: number; factorId: string; factorVersion: number; boundaryId: BoundaryId; boundary: string; organizationId: string;
};
/** MRVPreview / MRVReport of service-contracts.ts. */
export type ApiMRVPreview = { conditions: MRVConditions; summary: ApiEnergySummary; incomplete: boolean; scope: "scope_2"; isDemo: true };
export type ApiMRVReport = ApiMRVPreview & {
  id: string; version: number; createdAt: string; status: "draft" | "demo_reviewed"; evidenceIds: string[];
  reviewHistory: { userId: string; reportVersion: number; comment: string; at: string }[];
};
/** EmissionFactor of service-contracts.ts. */
export type ApiFactor = { id: string; version: number; createdAt: string; region: string; year: number; kgCO2ePerKWh: number; source: string; isDemo: true };

export type ReportRow = { id: string; version: number; status: ApiMRVReport["status"]; org: string; units: string; period: string; result: string; incomplete: boolean };

export function reportRows(rs: ApiMRVReport[], orgs: { id: string; name: string }[], t: T = en): ReportRow[] {
  const org = new Map(orgs.map((o) => [o.id, o.name]));
  return rs.map((r) => ({
    id: r.id, version: r.version, status: r.status, org: org.get(r.conditions.organizationId) ?? t("customer"),
    units: t(r.conditions.unitIds.length === 1 ? "1 unit" : "{n} units", { n: r.conditions.unitIds.length }),
    period: `${klStamp(r.conditions.from)} → ${klStamp(r.conditions.to)}`, incomplete: r.incomplete,
    result: r.incomplete || r.summary.totals.emissionsKg === null ? t("Calculation incomplete") : `${one(r.summary.totals.emissionsKg)} kgCO₂e`,
  }));
}

/** A report version's status as the screen words it. */
export const statusWord = (s: ApiMRVReport["status"] | string, t: T = en) => (s === "demo_reviewed" ? t("demo reviewed") : s === "draft" ? t("draft") : s);

/** The demo review history (DD-A14 step 5): each review at its time in the display time zone. */
export const reviewItems = (r: ApiMRVReport, i: I18n = EN) =>
  r.reviewHistory.map((h) => ({ time: showTime(h.at, i.display), title: `“${h.comment}”`, detail: i.t("version {n} · {user}", { n: h.reportVersion, user: h.userId.slice(0, 8) }) }));

/** The figures of a preview or a stored version: energy and emissions vs the baseline stay separate (DD-A14 step 5). */
export type MRVView = {
  incomplete: boolean; electricity: string; emissions: string; energyVsBaseline: string; energyPercent: string; emissionsVsBaseline: string; summary: SummaryView; conditions: [string, string][];
};

export function mrvView(p: ApiMRVPreview, orgs: { id: string; name: string }[], units: { id: string; label: string }[], i: I18n = EN): MRVView {
  const { t: tr } = i;
  const t = p.summary.totals;
  const s = summaryView(p.summary, i);
  const unit = new Map(units.map((u) => [u.id, u.label]));
  const b = p.summary.baselineSnapshot;
  const f = p.summary.factorSnapshot;
  const incomplete = p.incomplete || t.kWh === null || t.emissionsKg === null;
  return {
    incomplete, summary: s,
    electricity: t.kWh === null ? tr("Calculation incomplete") : `${one(t.kWh)} kWh`,
    emissions: t.emissionsKg === null || incomplete ? tr("Calculation incomplete") : `${one(t.emissionsKg)} kgCO₂e`,
    energyVsBaseline: saving(t.savedKWh, " kWh", one, tr), energyPercent: saving(t.savingPercentage, "%", one, tr), emissionsVsBaseline: saving(t.savedEmissionsKg, " kgCO₂e", one, tr),
    conditions: [
      [tr("Organization"), orgs.find((o) => o.id === p.conditions.organizationId)?.name ?? p.conditions.organizationId],
      [tr("Units"), p.conditions.unitIds.map((id) => unit.get(id) ?? id.slice(0, 8)).join(", ")],
      [tr("Period"), `${klStamp(p.conditions.from)} → ${klStamp(p.conditions.to)} (Asia/Kuala_Lumpur)`],
      [tr("Boundary"), `${p.conditions.boundaryId} — ${p.conditions.boundary}`],
      [tr("Baseline"), b ? `v${p.conditions.baselineVersion} · ${b.method} · ${b.baselineKWh === null ? tr("no value") : `${one(b.baselineKWh)} kWh`}` : `v${p.conditions.baselineVersion}`],
      [tr("Emission factor"), f ? `v${p.conditions.factorVersion} · ${f.region} · ${f.year} · ${f.kgCO2ePerKWh} kgCO₂e/kWh` : tr("Calculation incomplete — factor missing")],
      [tr("Factor source"), f?.source ?? "—"], [tr("Coverage"), s.coverage],
    ],
  };
}

/** The new-report form (DD-A14 fields): baseline and factor are chosen with their versions. */
export type MRVDraft = { organizationId: string; unitIds: string[]; from: string; to: string; baseline: string; factor: string; boundaryId: BoundaryId; boundary: string };
const ref = (s: string) => {
  const [id, v] = s.split("@");
  return { id, version: Number(v) };
};
export const versionKey = (id: string, version: number) => `${id}@${version}`;

export function mrvDraftErrors(d: MRVDraft, t: T = en): Record<string, string> {
  const e: Record<string, string> = {};
  if (!d.organizationId) e.organizationId = t("Choose a customer organization");
  if (d.unitIds.length < 1 || d.unitIds.length > 100) e.unitIds = t("Choose 1–100 units");
  if (!d.from || !d.to || d.from >= d.to) e.period = t("The end must be after the start");
  if (!d.baseline) e.baseline = t("Choose a baseline version");
  if (!d.factor) e.factor = t("Choose an emission factor version");
  if (d.boundary.trim().length < 1 || d.boundary.length > 500) e.boundary = t("1–500 characters");
  return e;
}

export function mrvConditions(d: MRVDraft): MRVConditions {
  const b = ref(d.baseline);
  const f = ref(d.factor);
  return {
    from: klInstant(d.from), to: klInstant(d.to), unitIds: d.unitIds, baselineId: b.id, baselineVersion: b.version, factorId: f.id, factorVersion: f.version,
    boundaryId: d.boundaryId, boundary: d.boundary.trim(), organizationId: d.organizationId,
  };
}

/** A draft that starts from a stored report version (a new version keeps its conditions until changed). */
export function mrvDraftOf(c?: MRVConditions, baseline?: ApiBaseline): MRVDraft {
  if (!c) return { organizationId: "", unitIds: [], from: "", to: "", baseline: "", factor: "", boundaryId: "ac_input_electricity", boundary: baseline?.boundary ?? "" };
  return {
    organizationId: c.organizationId, unitIds: c.unitIds, from: klLocal(c.from), to: klLocal(c.to), baseline: versionKey(c.baselineId, c.baselineVersion),
    factor: versionKey(c.factorId, c.factorVersion), boundaryId: c.boundaryId, boundary: c.boundary,
  };
}

/** factors.save rules (IR factors.save item 1) checked before the call. */
export type FactorDraft = { region: string; year: string; kgCO2ePerKWh: string; source: string };
export function factorErrors(d: FactorDraft, t: T = en): Record<string, string> {
  const e: Record<string, string> = {};
  if (d.region.trim().length < 1 || d.region.trim().length > 120) e.region = t("1–120 characters");
  const y = Number(d.year);
  if (!(Number.isInteger(y) && y >= 2000 && y <= 2100)) e.year = t("A year 2000–2100");
  const k = Number(d.kgCO2ePerKWh);
  if (!(d.kgCO2ePerKWh.trim() !== "" && k > 0 && k <= 10)) e.kgCO2ePerKWh = t("Above 0 and at most 10 kgCO₂e/kWh");
  if (d.source.trim().length < 1 || d.source.trim().length > 500) e.source = t("1–500 characters, stating the demo source");
  return e;
}
export const factorInput = (d: FactorDraft, id?: string) => ({
  ...(id ? { id } : {}), region: d.region.trim(), year: Number(d.year), kgCO2ePerKWh: Number(d.kgCO2ePerKWh), source: d.source.trim(), isDemo: true as const,
});
