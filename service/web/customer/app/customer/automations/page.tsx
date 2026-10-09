// /customer/automations (FR-C04, FR-C05, SCR-C04): in API mode a Server Component reads the customer's automations.list,
// automations.nextRuns of each enabled schedule rule (next run), units.list with properties.list and spaces.list (the AC
// names and places), consents.get (location_automation) and preferences.get (the timezone of new rules); the editor
// adds units.get of the customer's ACs for their actions. Save, switch on/off, delete and consent are Server Actions;
// the editor previews an unsaved schedule through the BFF (automations.nextRuns draft) and tests a saved event rule
// with automations.simulate at the current tick — nothing is sent (IR214). URL keys: automationId (an id, or new),
// tab. The Phase 1A demo keeps the fixtures.
import { connection } from "next/server";
import { apiMode, coreAll, coreOp, CoreError } from "@ac/web/lib/dal";
import { spacePath, type ApiPropertyRow, type ApiSpaceRow, type ApiUnitRow } from "@ac/web/lib/assets";
import type { ApiUnitDetail } from "@ac/web/lib/units";
import { consentCard, ruleCard, triggerOf, type ApiAutomation, type ApiConsent, type ApiOccurrence, type Caps } from "@ac/web/lib/clientAutomations";
import { AutomationsDemo } from "./_components/automations-demo";
import { AutomationsView, type AutomationsLive } from "./_components/automations-view";

export default async function CustomerAutomationsPage({ searchParams }: PageProps<"/customer/automations">) {
  await connection();
  if (!apiMode()) return <AutomationsDemo />;
  const sp = await searchParams;
  const editing = typeof sp.automationId === "string" && sp.automationId ? sp.automationId : null;
  const [prefs, rules, units, properties, spaces, consent] = await Promise.all([
    coreOp<{ timezone: string }>("preferences.get", {}), coreAll<ApiAutomation>("automations.list"), coreAll<ApiUnitRow>("units.list"),
    coreAll<ApiPropertyRow>("properties.list"), coreAll<ApiSpaceRow>("spaces.list"),
    coreOp<ApiConsent>("consents.get", { purpose: "location_automation" }).catch((e) => {
      if (e instanceof CoreError && e.error.code === "NOT_FOUND") return null; // no consent record: location rules unavailable
      throw e;
    }),
  ]);
  const propertyName = new Map(properties.map((p) => [p.id, p.name]));
  const acs = units.filter((u) => !u.archived).map((u) => {
    const property = propertyName.get(u.propertyId) ?? "";
    return { id: u.id, name: u.displayName, property, path: [property, spacePath(u.spaceId, spaces)].filter(Boolean).join(" › ") };
  }).sort((a, b) => a.path.localeCompare(b.path) || a.name.localeCompare(b.name));
  const ac = new Map(acs.map((u) => [u.id, u]));
  const nextRuns = new Map(await Promise.all(rules.filter((r) => r.kind === "schedule" && r.enabled).map(async (r) =>
    [r.id, (await coreOp<ApiOccurrence[]>("automations.nextRuns", { automationId: r.id, count: 8 })).find((o) => o.phase === "schedule_start")?.at ?? null] as const)));
  const granted = consent?.granted ?? false;
  const location = rules.filter((r) => triggerOf(r) === "location");
  // the editor's ACs with their capabilities (action options), read only while editing
  const caps: Record<string, Caps | null> = {};
  if (editing) {
    const details = await Promise.all(acs.map((u) => coreOp<ApiUnitDetail>("units.get", { id: u.id }).catch(() => null)));
    details.forEach((d, i) => { caps[acs[i].id] = d?.capabilities ?? null; });
  }
  const live: AutomationsLive = {
    rules, acs, caps, timezone: prefs.timezone || "Asia/Kuala_Lumpur", editing,
    cards: rules.map((r) => ruleCard(r, ac.get(r.unitIds[0]) ?? null, nextRuns.get(r.id) ?? null, granted)),
    consent: consent ? { version: consent.version, ...consentCard(consent, location.length, location.filter((r) => r.disabledReason === "consent_revoked").length) } : { version: null, ...consentCard(null, 0, 0) },
    nextRuns: Object.fromEntries(nextRuns),
  };
  return <AutomationsView live={live} />;
}
