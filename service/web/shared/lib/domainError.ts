// The DomainError body of a refused Core API or BFF call (service-contracts.ts), validated where it enters: a field the
// body leaves out takes its empty value, and a body that is not a DomainError at all reads as unavailable.
import { z } from "zod";

const domainErrorSchema = z.object({
  code: z.string(),
  messageKey: z.string(),
  fieldErrors: z.record(z.string(), z.string()).default({}),
  correlationId: z.string().default(""),
  retryAfterSeconds: z.number().nullable().default(null),
});
export type DomainError = z.infer<typeof domainErrorSchema>;

const UNAVAILABLE: DomainError = { code: "UNAVAILABLE", messageKey: "error.unavailable", fieldErrors: {}, correlationId: "", retryAfterSeconds: null };

/** The refusal a response body says, or "unavailable" when it says none. */
export function domainError(body: unknown): DomainError {
  const parsed = domainErrorSchema.safeParse(body);
  return parsed.success ? parsed.data : UNAVAILABLE;
}
