// Energy analysis (FR-A13, FR-C06, DATA_SOURCE=api): energy.summary and baselines projected for the screens, with the
// IR68 savings wording and the IR44 number rules. Pure code shared by Server Components and client views.

/** Round half away from zero to `digits` decimals (IR44: never Number.prototype.toFixed for rounding). */
export function roundAway(x: number, digits: number): number {
  const f = 10 ** digits;
  return (Math.sign(x) * Math.round(Math.abs(x) * f + Number.EPSILON)) / f;
}
const nf = (digits: number) => new Intl.NumberFormat("en-MY", { minimumFractionDigits: digits, maximumFractionDigits: digits });
/** One decimal (energy, percentages, kgCO₂e). */
export const one = (x: number) => nf(1).format(roundAway(x, 1));
/** An amount in minor units as "120.00 MYR". */
export const amount = (minor: number, currency: string) => `${nf(2).format(roundAway(minor / 100, 2))} ${currency}`;

/** IR68: >0 "Reduction {abs}", <0 "Increase {abs}", 0 "No change 0.0", null "Cannot calculate". */
export function saving(v: number | null, unit: string, fmt: (x: number) => string = one): string {
  if (v === null || !Number.isFinite(v)) return "Cannot calculate";
  const r = roundAway(v, 1);
  if (r === 0) return `No change ${fmt(0)}${unit}`;
  return `${r > 0 ? "Reduction" : "Increase"} ${fmt(Math.abs(v))}${unit}`;
}

export type BoundaryId = "ac_input_electricity" | "whole_building_electricity";
/** EnergyBaseline of service-contracts.ts. */
export type ApiBaseline = {
  id: string; version: number; createdAt: string; unitIds: string[]; period: { from: string; to: string }; method: "demo_fixed" | "demo_period_comparison";
  baselineKWh: number | null; quality: { kind: string; coverage: number | null }; boundaryId: BoundaryId; boundary: string; assumptions: string; source: string;
};
/** EnergySummary of service-contracts.ts. */
export type ApiEnergySummary = {
  period: { from: string; to: string }; unitIds: string[];
  totals: { kWh: number | null; amountMinor: number | null; savedKWh: number | null; deltaKWh: number | null; savingPercentage: number | null; savedAmountMinor: number | null; emissionsKg: number | null; savedEmissionsKg: number | null };
  currency: string; baselineRef: { id: string; version: number } | null; factorRef: { id: string; version: number } | null; baselineSnapshot: ApiBaseline | null;
  factorSnapshot: { id: string; version: number; region: string; year: number; kgCO2ePerKWh: number; source: string } | null;
  tariffVersion: string; boundaryId: BoundaryId; boundary: string; coverage: number | null; qualityWarnings: string[];
};

const KL = "Asia/Kuala_Lumpur";
export const klStamp = (iso: string) =>
  new Date(iso).toLocaleString("en-CA", { timeZone: KL, year: "numeric", month: "2-digit", day: "2-digit", hour: "2-digit", minute: "2-digit", hour12: false }).replace(",", "");
/** An instant as the datetime-local value in Kuala Lumpur ("2026-09-14T08:00"). */
export const klLocal = (iso: string) => klStamp(iso).replace(" ", "T");
/** A datetime-local value in Kuala Lumpur as an instant. */
export const klInstant = (local: string) => new Date(`${local}:00+08:00`).toISOString();

const warningText: Record<string, string> = {
  partial_coverage: "Partial coverage — some expected readings are missing or invalid",
  baseline_not_comparable: "Baseline not comparable — unit set, boundary, period length or coverage differ, so no difference is shown",
  modeled_baseline: "Modeled baseline — a demo assumption, not a measurement",
  prorated_modeled_baseline: "Modeled baseline prorated to the period",
  factor_missing: "Emission factor missing — emissions cannot be calculated",
  boundary_mismatch: "Boundary differs from the baseline",
};
export const warning = (code: string) => warningText[code] ?? code;

export type SummaryView = {
  actual: string; baseline: string; difference: string; percent: string; cost: string; savedCost: string; emissions: string; savedEmissions: string;
  coverage: string; conditions: [string, string][]; warnings: string[]; comparable: boolean; increase: boolean;
};

export function summaryView(s: ApiEnergySummary): SummaryView {
  const t = s.totals;
  const b = s.baselineSnapshot;
  const f = s.factorSnapshot;
  return {
    actual: t.kWh === null ? "No valid readings" : `${one(t.kWh)} kWh`,
    baseline: !b ? "No baseline" : b.baselineKWh === null ? "No value" : `${one(b.baselineKWh)} kWh`,
    difference: saving(t.savedKWh, " kWh"), percent: saving(t.savingPercentage, "%"),
    cost: t.amountMinor === null ? "—" : amount(t.amountMinor, s.currency),
    savedCost: t.savedAmountMinor === null ? "Cannot calculate" : saving(t.savedAmountMinor, "", (x) => amount(x, s.currency)),
    emissions: t.emissionsKg === null ? "—" : `${one(t.emissionsKg)} kgCO₂e`, savedEmissions: saving(t.savedEmissionsKg, " kgCO₂e"),
    coverage: s.coverage === null ? "—" : `${one(s.coverage * 100)}%`, comparable: t.savedKWh !== null, increase: (t.savedKWh ?? 0) < 0,
    conditions: [
      ["Period", `${klStamp(s.period.from)} → ${klStamp(s.period.to)} (Asia/Kuala_Lumpur)`],
      ["Boundary", `${s.boundaryId} — ${s.boundary}`],
      ["Baseline", b ? `v${b.version} · ${b.method} · ${klStamp(b.period.from)} → ${klStamp(b.period.to)} · ${b.unitIds.length} units` : "none selected"],
      ...(b ? [["Baseline assumptions", b.assumptions] as [string, string]] : []),
      ["Emission factor", f ? `${f.region} ${f.year} v${f.version} · ${f.kgCO2ePerKWh} kgCO₂e/kWh · ${f.source}` : "missing"],
      ["Tariff", s.tariffVersion], ["Coverage", s.coverage === null ? "—" : `${one(s.coverage * 100)}% of expected readings`],
    ],
    warnings: s.qualityWarnings.map(warning),
  };
}

export type BaselineRow = { id: string; version: number; method: ApiBaseline["method"]; value: string; units: string; period: string; boundaryId: BoundaryId; b: ApiBaseline };
export function baselineRows(bs: ApiBaseline[]): BaselineRow[] {
  return bs.map((b) => ({
    id: b.id, version: b.version, method: b.method, value: b.baselineKWh === null ? "no value" : `${one(b.baselineKWh)} kWh`,
    units: `${b.unitIds.length} unit${b.unitIds.length === 1 ? "" : "s"}`, period: `${klStamp(b.period.from)} → ${klStamp(b.period.to)}`, boundaryId: b.boundaryId, b,
  }));
}

/** The baseline form (DD-A13 fields); the Repository assigns versions. */
export type BaselineDraft = { unitIds: string[]; from: string; to: string; method: ApiBaseline["method"]; baselineKWh: string; boundaryId: BoundaryId; boundary: string; assumptions: string; source: string };
export function baselineDraft(b?: ApiBaseline): BaselineDraft {
  return {
    unitIds: b?.unitIds ?? [], from: b ? klLocal(b.period.from) : "", to: b ? klLocal(b.period.to) : "", method: b?.method ?? "demo_fixed",
    baselineKWh: b?.baselineKWh === null || b?.baselineKWh === undefined ? "" : String(b.baselineKWh), boundaryId: b?.boundaryId ?? "ac_input_electricity",
    boundary: b?.boundary ?? "", assumptions: b?.assumptions ?? "", source: b?.source ?? "",
  };
}
/** baselines.save rules (IR baselines.save item 3) checked before the call; the API checks them again. */
export function baselineErrors(d: BaselineDraft): Record<string, string> {
  const e: Record<string, string> = {};
  if (d.unitIds.length < 1 || d.unitIds.length > 100) e.unitIds = "Choose 1–100 units";
  if (!d.from || !d.to || d.from >= d.to) e.period = "The end must be after the start";
  else if (Date.parse(klInstant(d.to)) - Date.parse(klInstant(d.from)) > 366 * 24 * 3600 * 1000) e.period = "At most 366 days";
  if (d.method === "demo_fixed" && !(d.baselineKWh.trim() !== "" && Number.isFinite(+d.baselineKWh) && +d.baselineKWh >= 0)) e.baselineKWh = "A value ≥ 0 kWh";
  if (d.method === "demo_period_comparison" && d.boundaryId === "whole_building_electricity") e.boundaryId = "Period comparison measures AC input electricity only";
  if (d.boundary.trim().length < 1 || d.boundary.length > 500) e.boundary = "1–500 characters";
  if (d.assumptions.trim().length < 1 || d.assumptions.length > 2000) e.assumptions = "1–2000 characters";
  if (d.source.trim().length < 1 || d.source.length > 500) e.source = "1–500 characters";
  return e;
}
export function baselineInput(d: BaselineDraft, id?: string) {
  return {
    ...(id ? { id } : {}), unitIds: d.unitIds, period: { from: klInstant(d.from), to: klInstant(d.to) }, method: d.method,
    ...(d.method === "demo_fixed" ? { baselineKWh: Number(d.baselineKWh) } : {}), boundaryId: d.boundaryId, boundary: d.boundary.trim(), assumptions: d.assumptions.trim(), source: d.source.trim(),
  };
}
