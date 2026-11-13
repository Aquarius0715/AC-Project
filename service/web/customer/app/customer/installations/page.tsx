// /customer/installations (FR-C20, SCR-C20, DEC-107): in API mode a Server Component checks the session's client role
// (members get the shared Page unavailable view, FORBIDDEN, IR112) and reads installationRequests.list with the names of
// the properties, spaces and models. New requests and cancelling a submitted one are Server Actions (actions.ts); HQ
// decides in Admin › Jobs. The Phase 1A demo stores nothing: a request needs the Core API.
import { connection } from "next/server";
import { apiMode, coreAll, coreDisplay, corePrincipal } from "@ac/web/lib/dal";
import { i18nOf } from "@ac/web/lib/i18n";
import type { ApiPropertyRow, ApiSpaceRow } from "@ac/web/lib/assets";
import type { ApiCapability } from "@ac/web/lib/devices";
import { requestRows, type InstallationRequest } from "@ac/web/lib/installationRequests";
import { InstallationsDemo } from "./_components/installations-demo";
import { InstallationsView } from "./_components/installations-view";

export default async function CustomerInstallationsPage() {
  await connection();
  if (!apiMode()) return <InstallationsDemo />;
  const me = await corePrincipal();
  if (me.clientRole !== "owner") return <InstallationsView live={null} />;
  const [requests, properties, spaces, models, display] = await Promise.all([
    coreAll<InstallationRequest>("installationRequests.list"), coreAll<ApiPropertyRow>("properties.list"),
    coreAll<ApiSpaceRow>("spaces.list"), coreAll<ApiCapability>("capabilities.list"), coreDisplay(),
  ]);
  const i = i18nOf(display);
    return <InstallationsView live={{ rows: requestRows(requests, { models, properties, spaces }, i), properties: properties.filter((p) => !p.archived), spaces: spaces.filter((s) => !s.archived), now: new Date().toISOString(), models }} />;
}
