import { beforeEach, describe, expect, it, vi } from "vitest";
import type { ApiContractFull } from "@ac/web/lib/contracts";

// contracts.save (FR-A07): a new contract without an expected version, the next version at the version it was read at
const dal = vi.hoisted(() => {
  interface DomainError { code: string; messageKey: string; fieldErrors: Record<string, string>; retryAfterSeconds: number | null }
  class CoreError extends Error {
    readonly error: DomainError;
    constructor(error: DomainError, options?: ErrorOptions) {
      super(error.code, options);
      this.name = "CoreError";
      this.error = error;
    }
  }
  return { CoreError, coreOp: vi.fn<(op: string, input?: unknown, options?: unknown) => Promise<unknown>>() };
});
const cache = vi.hoisted(() => ({ refresh: vi.fn() }));
vi.mock("server-only", () => ({}));
vi.mock("next/cache", () => cache);
vi.mock("@ac/web/lib/dal", () => dal);

import { saveContract } from "../app/admin/billing/contracts/actions";

const refused = (code: string) => new dal.CoreError({ code, messageKey: `error.${code.toLowerCase()}`, fieldErrors: {}, retryAfterSeconds: null });
const contract = (id: string, version: number): ApiContractFull => ({
  id, version, activeRestrictionIds: [], hasUnresolvedRecovery: false, customerId: "cust-a", customerOrgId: "org-a", unitIds: ["u-1"], planType: "general",
  startAt: "2025-12-31T16:00:00Z", endAt: "2026-12-31T16:00:00Z", priceMinor: 5000, currency: "MYR", restrictionEligible: false, rulesVersion: null,
});

beforeEach(() => { dal.coreOp.mockReset(); cache.refresh.mockReset(); });

describe("contracts.save", () => {
  it("creates a contract without a version, saves the next version at the version read, and returns a refusal", async () => {
    const input = { customerId: "cust-a", unitIds: ["u-1"], planType: "general" as const, startAt: "a", endAt: "b", priceMinor: 5000, currency: "MYR" as const, restrictionEligible: false, rulesVersion: null };
    dal.coreOp.mockResolvedValueOnce(contract("k-new", 1));
    expect(await saveContract(input)).toStrictEqual({ ok: true, value: { id: "k-new", version: 1 } });
    expect(JSON.stringify(dal.coreOp.mock.lastCall?.[2])).toBe('{"write":true}');
    dal.coreOp.mockResolvedValueOnce(contract("k-1", 3));
    expect(await saveContract({ ...input, id: "k-1" }, 2)).toStrictEqual({ ok: true, value: { id: "k-1", version: 3 } });
    expect(dal.coreOp.mock.lastCall?.[2]).toStrictEqual({ write: true, expectedVersion: 2 });
    dal.coreOp.mockRejectedValueOnce(refused("CONFLICT"));
    expect(await saveContract({ ...input, id: "k-1" }, 2)).toMatchObject({ ok: false, code: "CONFLICT" });
    expect(cache.refresh).toHaveBeenCalledTimes(3);
  });
});
