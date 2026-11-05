import { z } from "zod";

/** What every successful Core API answer carries: the operation's `data` and, for reads, the clock it was read at. */
const envelope = z.object({ data: z.unknown(), meta: z.object({ snapshotAt: z.string().optional() }).optional() }).refine((answer) => answer.data !== undefined);

export interface Answer<T> { data: T; meta?: { snapshotAt?: string } }

/** The answer body when it has the envelope's shape (data present, meta well formed), else null. The type of `data` is
the caller's: the operation catalog (contracts.gen.ts) types each answer. */
export function answerOf<T>(body: unknown): Answer<T> | null {
  const parsed = envelope.safeParse(body);
  return parsed.success ? parsed.data as Answer<T> : null; // eslint-disable-line @typescript-eslint/no-unsafe-type-assertion -- the envelope is validated; data is typed by the operation catalog until per-operation answer schemas are generated from the contract
}
