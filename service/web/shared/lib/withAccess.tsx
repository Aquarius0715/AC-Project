import "server-only";
import { CoreError, corePermissions } from "@ac/web/lib/dal";
import { NeedsAccess } from "@ac/web/components/NeedsAccess";

/** Renders an HQ page, or — when one of its reads is refused (FORBIDDEN) — which permission the role lacks, instead of
 * "could not be loaded" with Try again (IR581, IR582). `permissions` lists what the page's reads accept, its own read
 * first: the first one the role does not hold is named. `what` is the sidebar's name of the page. */
export async function withAccess(what: string, permissions: string[], render: () => Promise<React.ReactNode>): Promise<React.ReactNode> {
  try {
    return await render();
  } catch (e) {
    if (!(e instanceof CoreError && e.error.code === "FORBIDDEN")) throw e;
    const held = await corePermissions();
    return <NeedsAccess what={what} permission={permissions.find((p) => !held.has(p)) ?? permissions[0]} />;
  }
}
