// Browser client for the BFF operation relay (/bff/ops/<operation>). Errors are thrown as DomainError values with the
// code / messageKey / fieldErrors of service-contracts.ts.
import type { OpArgs } from "@ac/web/lib/opTypes";
import { randomId } from "@ac/web/lib/randomId";

export interface DomainError { code: string; messageKey: string; fieldErrors: Record<string, string>; correlationId: string; retryAfterSeconds: number | null }

export class OpError extends Error {
  constructor(public readonly status: number, public readonly error: DomainError) {
    super(`${error.code}: ${error.messageKey}`);
  }
}

export interface WriteOptions { expectedVersion?: number; idempotencyKey?: string }

/** One operation through the BFF; the input is typed by the contract (IR313). */
export async function callOp<T>(...[operation, input, options = {}]: OpArgs<WriteOptions & { write?: boolean }>): Promise<T> {
  const headers: Record<string, string> = { "Content-Type": "application/json" };
  if (options.write) headers["Idempotency-Key"] = options.idempotencyKey ?? randomId(); // also over HTTP (DEC-93)
  if (options.expectedVersion !== undefined) headers["X-Expected-Version"] = String(options.expectedVersion);
  const res = await fetch(`/bff/ops/${operation}`, { method: "POST", body: JSON.stringify(input ?? {}), headers });
  const body = await res.json().catch(() => null);
  if (!res.ok) {
    throw new OpError(res.status, body ?? { code: "UNAVAILABLE", messageKey: "error.unavailable", fieldErrors: {}, correlationId: "", retryAfterSeconds: null });
  }
  return (body as { data: T }).data;
}
