// HQ contracts (FR-A07, DATA_SOURCE=api): contracts.list, customers.list, properties.list and units.list projected for
// the contract editor. Pure code shared by the Server Component and the client view. Texts in the display language
// (`t` / `i`, IR300); contract periods are Kuala Lumpur business days, typed as such and written in the user's language.
import { businessDay, money, type ApiCustomer, type ApiProperty } from "@ac/web/lib/billing";
import { EN, translator, type I18n, type T } from "@ac/web/lib/i18n";

const en = translator("en");

export type PlanType = "rto" | "general" | "energy" | "environment";
/** Contract of service-contracts.ts. */
export type ApiContractFull = {
  id: string; version: number; activeRestrictionIds: string[]; hasUnresolvedRecovery: boolean; customerId: string; customerOrgId: string; unitIds: string[];
  planType: PlanType; startAt: string; endAt: string; priceMinor: number; currency: "MYR" | "USD"; restrictionEligible: boolean; rulesVersion: string | null;
};
/** The ACUnit fields the editor needs. */
export type ApiUnitBrief = { id: string; customerOrgId: string; propertyId: string; displayName: string; archived: boolean };

export const planLabel: Record<PlanType, string> = { rto: "RTO", general: "General", energy: "Energy", environment: "Environment" };
/** A plan type as the screen words it. */
export const planWord = (p: PlanType, t: T = en) => ({ rto: t("RTO"), general: t("General"), energy: t("Energy"), environment: t("Environment") })[p] ?? p;

export type ContractRow = {
  id: string; version: number; planType: PlanType; plan: string; customerId: string; cust: string; unitIds: string[]; units: string;
  priceMinor: number; currency: "MYR" | "USD"; price: string; startAt: string; endAt: string; start: string; end: string; period: string;
  restrictionEligible: boolean; rulesVersion: string | null; restrictionIds: string[]; recovery: boolean;
};
export type UnitOption = { id: string; customerId: string; label: string; where: string; otherContracts: string[] };

const KL = "Asia/Kuala_Lumpur";
/** The Kuala Lumpur calendar day of an instant (yyyy-mm-dd). */
export const klDay = (iso: string) => new Date(iso).toLocaleDateString("en-CA", { timeZone: KL });

/** The contracts list: `start` / `end` are the Kuala Lumpur days the date inputs take, `period` the same days as the user
 * reads them. */
export function contractRows(contracts: ApiContractFull[], customers: ApiCustomer[], i: I18n = EN): ContractRow[] {
  const { t, display: { locale } } = i;
  const name = new Map(customers.map((c) => [c.id, c.name]));
  return contracts.map((k) => ({
    id: k.id, version: k.version, planType: k.planType, plan: planWord(k.planType, t), customerId: k.customerId, cust: name.get(k.customerId) ?? t("customer"),
    unitIds: k.unitIds, units: t(k.unitIds.length === 1 ? "1 unit" : "{n} units", { n: k.unitIds.length }), priceMinor: k.priceMinor, currency: k.currency,
    price: money(k.priceMinor, k.currency), startAt: k.startAt, endAt: k.endAt, start: klDay(k.startAt), end: klDay(k.endAt), period: `${businessDay(k.startAt, locale)} → ${businessDay(k.endAt, locale)}`,
    restrictionEligible: k.restrictionEligible, rulesVersion: k.rulesVersion, restrictionIds: k.activeRestrictionIds, recovery: k.hasUnresolvedRecovery,
  }));
}

/** The customers' active units for the checklist; a unit on another current contract is labelled, not hidden (DD-A07). */
export function unitOptions(units: ApiUnitBrief[], customers: ApiCustomer[], properties: ApiProperty[], contracts: ApiContractFull[]): UnitOption[] {
  const customerOfOrg = new Map(customers.map((c) => [c.organizationId, c.id]));
  const property = new Map(properties.map((p) => [p.id, p.name]));
  const contractsOf = new Map<string, string[]>();
  for (const k of contracts) for (const u of k.unitIds) contractsOf.set(u, [...(contractsOf.get(u) ?? []), k.id]);
  return units.filter((u) => !u.archived && customerOfOrg.has(u.customerOrgId)).map((u) => ({
    id: u.id, customerId: customerOfOrg.get(u.customerOrgId)!, label: u.displayName, where: property.get(u.propertyId) ?? "", otherContracts: contractsOf.get(u.id) ?? [],
  }));
}

export type ContractDraft = { customerId: string; unitIds: string[]; planType: PlanType; start: string; end: string; price: string; currency: "MYR" | "USD"; restrictionEligible: boolean; rulesVersion: string };

/** Client-side checks of contracts.save (IR rules for contracts.save item 1), worded in the display language; the API
 * checks them again. */
export function draftErrors(d: ContractDraft, t: T = en): Partial<Record<keyof ContractDraft, string>> {
  const e: Partial<Record<keyof ContractDraft, string>> = {};
  if (!d.customerId) e.customerId = t("Choose a customer");
  if (d.unitIds.length === 0) e.unitIds = t("Include at least one unit");
  if (!d.start) e.start = t("Required");
  if (!d.end || (d.start && d.end <= d.start)) e.end = t("End must be after start");
  if (!/^\d+(\.\d{1,2})?$/.test(d.price.trim())) e.price = t("A price ≥ 0 with at most 2 decimals");
  if (d.restrictionEligible && d.planType !== "rto") e.restrictionEligible = t("Only RTO contracts can be restriction eligible");
  if (d.restrictionEligible && (d.rulesVersion.trim().length < 1 || d.rulesVersion.trim().length > 64)) e.rulesVersion = t("Required when eligible (1–64 characters)");
  return e;
}

/** A Kuala Lumpur day as the instant to save; an unchanged day keeps the stored instant (no silent time shift). */
export function dayInstant(day: string, original?: string): string {
  return original && klDay(original) === day ? original : `${day}T00:00:00+08:00`;
}

/** "120.5" → 12050 (validated by draftErrors first). */
export const priceMinor = (price: string) => Math.round(Number(price.trim()) * 100);
