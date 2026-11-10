import "server-only";
import { CoreError, coreOp, orAbsent } from "@ac/web/lib/dal";
import type { ApiDevice } from "@ac/web/lib/devices";

/** One page of the registry: its devices (the URL's device first when it is not on the page), how many match, and the
cursor of the next page. */
export interface RegistryPage { items: ApiDevice[]; total: number; next: string | null }

interface Page { items: ApiDevice[]; total: number; nextCursor?: string | null }

/** HQ's device registry (FR-A04) as pages of devices.list on the server, sorted by serial, with the search (serial,
MQTT node ID or unit name) on the server too (IR723): it read the first 100 devices and searched only those, so HQ
could not reach the rest at the launch size. A device the URL names that is not on the page is read on its own, so a
link to any device opens it. A cursor that no longer holds (another search, the scope changed) reads the first page. */
export async function registryPage(search: string, cursor: string | null, deviceId: string | null): Promise<RegistryPage> {
  const read = (from: string | null) => coreOp<Page>("devices.list", {
    limit: 100, sort: { field: "serial", direction: "asc" }, ...(search && { filters: { search } }), ...(from && { cursor: from }),
  });
  const page = cursor
    ? await read(cursor).catch((error: unknown) => {
      if (error instanceof CoreError) return read(null);
      throw error;
    })
    : await read(null);
  const wanted = deviceId && page.items.every((d) => d.id !== deviceId) ? await orAbsent(coreOp<ApiDevice>("devices.get", { id: deviceId })) : null;
  return { items: wanted ? [wanted, ...page.items] : page.items, total: page.total, next: page.nextCursor ?? null };
}
