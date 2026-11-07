import { z } from "@ac/web/lib/zod";

export interface Answer<T> { data: T; meta?: { snapshotAt?: string } }

/** What every successful Core API answer carries: the operation's `data` and, for reads, the clock it was read at. The
type of `data` is the caller's: the operation catalog (contracts.gen.ts) types each answer, and per-operation answer
schemas are not generated from the contract yet. */
const envelope = <T>() => z.object({ data: z.custom<T>((data) => data !== undefined), meta: z.object({ snapshotAt: z.string().optional() }).optional() });

/** The answer body when it has the envelope's shape (data present, meta well formed), else null. */
export function answerOf<T>(body: unknown): Answer<T> | null {
  const parsed = envelope<T>().safeParse(body);
  return parsed.success ? parsed.data : null;
}
