import { beforeEach, describe, expect, it, vi } from "vitest";

// the payouts screen (FR-P10): the company's approved and paid statements, the selected statement's rate card and the
// label of each line's job. Billing prices a line at the card in effect when its job was accepted, which is before the
// statement's pay date, so the header names the card in effect at the pay date, and no card when none was.
const dal = vi.hoisted(() => {
  class CoreError extends Error {
    readonly error: { code: string };
    constructor(code: string, options?: ErrorOptions) { super(code, options); this.name = "CoreError"; this.error = { code }; }
  }
  return {
    CoreError, coreAll: vi.fn(), coreOp: vi.fn(), coreDisplay: vi.fn(), coreIdentity: vi.fn(),
    orAbsent: async <T,>(read: Promise<T>): Promise<T | null> => {
      try { return await read; } catch (error) {
        if (error instanceof CoreError && (error.error.code === "FORBIDDEN" || error.error.code === "NOT_FOUND")) return null;
        throw error;
      }
    },
  };
});
vi.mock("server-only", () => ({}));
vi.mock("@ac/web/lib/dal", () => dal);

import { loadPayouts } from "../app/partner/payouts/_lib/load";
import type { ApiPayoutStatement } from "@ac/web/lib/partnerPayouts";

const statement = (id: string, period: string, status: "approved" | "paid", payDate: string): ApiPayoutStatement => ({
  id, period, status, payDate, version: 1, contractorOrgId: "org-a", currency: "MYR", grossMinor: 10_000, deductionsMinor: 0, netMinor: 10_000,
  lines: [], queries: [], approvedByMembershipId: null, paidAt: status === "paid" ? payDate : null,
});
const october = statement("st-oct-0001", "2026-10", "approved", "2026-11-25T00:00:00Z");
const september = statement("st-sep-0001", "2026-09", "paid", "2026-10-25T00:00:00Z");
const card = (id: string, version: number, effectiveFrom: string) => ({ id, version, effectiveFrom });

describe("partner payouts loader", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    dal.coreDisplay.mockResolvedValue({ locale: "en", timeZone: "Asia/Kuala_Lumpur" });
    dal.coreIdentity.mockResolvedValue({ displayName: "Owner", organizationName: "Contractor A" });
    dal.coreAll.mockResolvedValue([october, september]);
    dal.coreOp.mockImplementation((operation: string) => Promise.resolve(operation === "rateCards.list"
      ? { items: [card("rc-new-0001", 3, "2026-12-01T00:00:00Z"), card("rc-mid-0001", 2, "2026-10-01T00:00:00Z")] }
      : { items: [] }));
  });

  it("names the card in effect at the pay date, not a newer one", async () => {
    const live = await loadPayouts({});
    expect(live.selected?.statement.id).toBe("st-oct-0001");
    expect(live.selected?.sub).toBe("Rate card rc rc-mid-0 v2 · pays 25 Nov 2026");
    expect(live.selected?.kpis[1]?.sub).toBe("rate card rc-mid-0 v2");
  });

  it("names no card when none was in effect at the pay date: none priced a line", async () => {
    dal.coreOp.mockImplementation((operation: string) => Promise.resolve(operation === "rateCards.list" ? { items: [card("rc-new-0001", 3, "2026-12-01T00:00:00Z")] } : { items: [] }));
    const live = await loadPayouts({});
    expect(live.selected?.sub).toBe("Rate card set by HQ · pays 25 Nov 2026");
    expect(live.selected?.kpis[1]?.sub).toBe("rate card set by HQ");
  });

  it("filters by period and status, selects the asked statement and ignores an unknown status", async () => {
    const paid = await loadPayouts({ status: "paid" });
    expect(paid.status).toBe("paid");
    expect(paid.selected?.statement.id).toBe("st-sep-0001");
    expect(paid.selected?.sub).toBe("Rate card rc rc-mid-0 v2 · paid 25 Oct 2026");
    const asked = await loadPayouts({ statementId: "st-sep-0001" });
    expect(asked.selected?.statement.id).toBe("st-sep-0001");
    const unknown = await loadPayouts({ status: "draft", period: "2026-10" });
    expect([unknown.status, unknown.period, unknown.selected?.statement.id]).toStrictEqual(["", "2026-10", "st-oct-0001"]);
    expect(unknown.periods.map((p) => p.value)).toStrictEqual(["2026-10", "2026-09"]);
  });

  it("selects nothing and reads no card when no statement is shown", async () => {
    const live = await loadPayouts({ period: "2025-01" });
    expect(live.selected).toBeNull();
    expect(live.statements).toStrictEqual([]);
    expect(dal.coreOp).not.toHaveBeenCalledWith("rateCards.list", expect.anything());
  });
});
