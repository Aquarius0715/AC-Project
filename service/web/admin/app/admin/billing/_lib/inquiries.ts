import "server-only";
import { coreAll, coreOp } from "@ac/web/lib/dal";
import type { ApiInquiry } from "@ac/web/lib/billing";

/** The customers' inquiries of the HQ billing screen (FR-A08), each state read on its own (IR713). The list sorts the
newest first across the states, so a single page of 100 lost an older inquiry still waiting for a reply behind newer
answered ones, from the Received tab and its count. Every received inquiry is read; the answered ones are the newest
100. */
export async function billingInquiries(): Promise<ApiInquiry[]> {
  const list = "inquiries.list";
  const [received, answered] = await Promise.all([
    coreAll<ApiInquiry>(list, { filters: { status: "received" } }),
    coreOp<{ items: ApiInquiry[] }>(list, { limit: 100, filters: { status: "answered" } }),
  ]);
  return [...received, ...answered.items];
}
