import "server-only";
import { coreOp } from "@ac/web/lib/dal";
import type { OpInput } from "@ac/web/lib/opTypes";
import { states, type RestrictionState } from "@ac/web/lib/restrictions";

type Scope = Omit<NonNullable<OpInput<"restrictions.list">["filters"]>, "status">;

/** The number of restrictions in each state of the scope (FR-A09, IR714): the total of restrictions.list per state.
The counts came from the listed rows, so a picked state left the other chips at 0.
Without a picked state, they stopped at the newest 100 restrictions. */
export async function stateCounts(scope: Scope): Promise<(state: RestrictionState) => number> {
  const totals = new Map(await Promise.all(states.map(async (status) => {
    const page = await coreOp<{ total: number }>("restrictions.list", { limit: 1, filters: { ...scope, status } });
    return [status, page.total] satisfies [RestrictionState, number];
  })));
  return (state) => totals.get(state) ?? 0;
}
