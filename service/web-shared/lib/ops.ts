// Browser client for the BFF operation relay (/bff/ops/<operation>). Errors are thrown as DomainError values with the
// code / messageKey / fieldErrors of service-contracts.ts.
export type DomainError = { code: string; messageKey: string; fieldErrors: Record<string, string>; correlationId: string; retryAfterSeconds: number | null };

export class OpError extends Error {
  constructor(public readonly status: number, public readonly error: DomainError) {
    super(`${error.code}: ${error.messageKey}`);
  }
}

export type WriteOptions = { expectedVersion?: number; idempotencyKey?: string };

export async function callOp<T>(operation: string, input: unknown, opts: WriteOptions & { write?: boolean } = {}): Promise<T> {
  const headers: Record<string, string> = { "Content-Type": "application/json" };
  if (opts.write) headers["Idempotency-Key"] = opts.idempotencyKey ?? crypto.randomUUID();
  if (opts.expectedVersion !== undefined) headers["X-Expected-Version"] = String(opts.expectedVersion);
  const res = await fetch(`/bff/ops/${operation}`, { method: "POST", headers, body: JSON.stringify(input ?? {}), credentials: "same-origin" });
  const body = await res.json().catch(() => null);
  if (!res.ok) {
    throw new OpError(res.status, body ?? { code: "UNAVAILABLE", messageKey: "error.unavailable", fieldErrors: {}, correlationId: "", retryAfterSeconds: null });
  }
  return (body as { data: T }).data;
}
