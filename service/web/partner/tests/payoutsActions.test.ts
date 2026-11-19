import { beforeEach, describe, expect, it, vi } from "vitest";

// the contractor's question to HQ on a statement line (FR-P10, payouts.query): sent as a write at the statement's
// version; a refusal comes back as an action failure, and the page re-reads in both cases
const dal = vi.hoisted(() => {
  interface DomainError { code: string; messageKey: string; fieldErrors: Record<string, string>; retryAfterSeconds: number | null }
  class CoreError extends Error {
    readonly error: DomainError;
    constructor(code: string, options?: ErrorOptions) {
      super(code, options);
      this.name = "CoreError";
      this.error = { code, messageKey: `error.${code.toLowerCase()}`, fieldErrors: {}, retryAfterSeconds: null };
    }
  }
  return { coreOp: vi.fn(), CoreError };
});
const cache = vi.hoisted(() => ({ refresh: vi.fn() }));
vi.mock("server-only", () => ({}));
vi.mock("next/cache", () => cache);
vi.mock("@ac/web/lib/dal", () => dal);

import { askHq } from "../app/partner/payouts/actions";

beforeEach(() => {
  vi.clearAllMocks();
  dal.coreOp.mockResolvedValue({});
});

describe("askHq", () => {
  it("sends the question as a write at the statement's version and re-reads the page", async () => {
    await expect(askHq("stmt-1", 4, "line-2", "deduction", "Part came from HQ stock")).resolves.toStrictEqual({ ok: true, value: null });
    expect(dal.coreOp).toHaveBeenCalledExactlyOnceWith("payouts.query", { statementId: "stmt-1", lineId: "line-2", topic: "deduction", message: "Part came from HQ stock" }, { write: true, expectedVersion: 4 });
    expect(cache.refresh).toHaveBeenCalledOnce();
  });

  it("returns a refusal with its field errors, and still re-reads so a changed statement shows", async () => {
    const conflict = new dal.CoreError("CONFLICT");
    conflict.error.fieldErrors.message = "too long";
    dal.coreOp.mockRejectedValueOnce(conflict);
    await expect(askHq("stmt-1", 4, "line-2", "amount", "Why?")).resolves.toStrictEqual({ ok: false, code: "CONFLICT", messageKey: "error.conflict", fieldErrors: { message: "too long" }, retryAfterSeconds: null });
    expect(cache.refresh).toHaveBeenCalledOnce();
  });

  it("reports an unreachable Core API as unavailable", async () => {
    dal.coreOp.mockRejectedValueOnce(new TypeError("fetch failed"));
    await expect(askHq("stmt-1", 4, "line-2", "other", "Why?")).resolves.toMatchObject({ ok: false, code: "UNAVAILABLE" });
    expect(cache.refresh).toHaveBeenCalledOnce();
  });
});
