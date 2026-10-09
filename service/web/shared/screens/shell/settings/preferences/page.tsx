// /settings/preferences (FR-X01, FR-X08, DDC-07, SCR-X-settings-preferences, IR246): in API mode a Server Component reads
// the signed-in user's preferences (preferences.get), two-step verification (twoFactor.get) and, for a client, the
// location consent (consents.get; none recorded yet = not granted); saving is Server Actions (./actions.ts). The time zone
// offsets are worked out here at the demo clock. The Phase 1A demo keeps its browser-only state.
import { connection } from "next/server";
import { apiMode, coreNow, coreOp, CoreError, getSession } from "@ac/web/lib/dal";
import { zoneOptions } from "@ac/web/lib/preferences";
import { PreferencesDemo } from "./_components/preferences-demo";
import { PreferencesView, type PreferencesLive } from "./_components/preferences-view";

export default async function PreferencesPage() {
  await connection();
  if (!apiMode()) return <PreferencesDemo />;
  const session = await getSession();
  const client = session?.role === "client";
  const [prefs, twoFactor, consent, now] = await Promise.all([
    coreOp<PreferencesLive["prefs"]>("preferences.get", {}),
    coreOp<PreferencesLive["twoFactor"]>("twoFactor.get", {}),
    client ? coreOp<NonNullable<PreferencesLive["consent"]>>("consents.get", { purpose: "location_automation" }).catch((e) => {
      if (e instanceof CoreError && e.error.code === "NOT_FOUND") return null;
      throw e;
    }) : Promise.resolve(null),
    coreNow(),
  ]);
  return <PreferencesView live={{ prefs, twoFactor, consent, client, zones: zoneOptions(prefs.timezone, now) }} />;
}
