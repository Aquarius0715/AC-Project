// /settings/preferences (FR-X01, FR-X08, DDC-07, SCR-X-settings-preferences, IR246): in API mode a Server Component reads
// the signed-in user's preferences (preferences.get), two-step verification (twoFactor.get) and, for a client, the
// location consent (consents.get; none recorded yet = not granted); saving is Server Actions (./actions.ts). The time zone
// offsets are worked out here at the demo clock, and while two-step verification is off the setup key becomes the
// authenticator QR code for the signed-in user (IR247). The Phase 1A demo keeps its browser-only state.
import { connection } from "next/server";
import { apiMode, coreIdentity, coreNow, coreOp, CoreError, getSession } from "@ac/web/lib/dal";
import { zoneOptions } from "@ac/web/lib/preferences";
import { otpauthUri, qrPath } from "@ac/web/lib/twoFactor";
import { PreferencesDemo } from "./_components/preferences-demo";
import { PreferencesView, type PreferencesLive } from "./_components/preferences-view";

/** The consent, or null when none was given yet (NOT_FOUND). */
async function consentOrNull() {
  try {
    return await coreOp<NonNullable<PreferencesLive["consent"]>>("consents.get", { purpose: "location_automation" });
  } catch (error) {
    if (error instanceof CoreError && error.error.code === "NOT_FOUND") return null;
    throw error;
  }
}

export default async function PreferencesPage() {
  await connection();
  if (!apiMode()) return <PreferencesDemo />;
  const session = await getSession();
  const isClient = session?.role === "client";
  const [prefs, twoFactor, consent, now, identity] = await Promise.all([
    coreOp<PreferencesLive["prefs"]>("preferences.get", {}),
    coreOp<PreferencesLive["twoFactor"]>("twoFactor.get", {}),
    isClient ? consentOrNull() : Promise.resolve(null),
    coreNow(),
    coreIdentity(),
  ]);
  const qr = !twoFactor.enabled && twoFactor.setupKey ? qrPath(otpauthUri(twoFactor.setupKey, identity.displayName || "account")) : null;
  return <PreferencesView live={{ prefs, twoFactor, consent, qr, client: isClient, hq: session?.role === "admin", zones: zoneOptions(prefs.timezone, now) }} />;
}
