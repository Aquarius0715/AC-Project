import { beforeEach, describe, expect, it, vi } from "vitest";
import { isValidElement, type ComponentProps, type ReactNode } from "react";
import type { ApiOffsetRecord } from "@ac/web/lib/offsets";

// The offset demo registry of HQ (FR-A15, SCR-A15): the Server Actions (a demo quote, its request, and the simulated
// purchase, retirement, failure and retry, each with its version) and the Server Component (the URL filters customer,
// status and created, the selected record, a recordId the list does not hold, the units of live customers).
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
  return {
    CoreError, apiMode: vi.fn(() => true), coreNow: vi.fn(() => Promise.resolve(new Date("2026-09-22T00:00:00Z"))),
    coreDisplay: vi.fn(() => Promise.resolve({ locale: "en", timeZone: "Asia/Kuala_Lumpur" })),
    corePermissions: vi.fn(() => Promise.resolve(new Set(["offset.read", "offset.write"]))),
    coreOp: vi.fn<(op: string, input?: unknown, options?: unknown) => Promise<unknown>>(),
    coreAllPage: vi.fn<(op: string) => Promise<{ items: unknown[]; total: number }>>(),
  };
});
const cache = vi.hoisted(() => ({ refresh: vi.fn() }));
vi.mock("server-only", () => ({}));
vi.mock("next/cache", () => cache);
vi.mock("next/server", () => ({ connection: () => Promise.resolve() }));
vi.mock("@ac/web/lib/dal", () => dal);
vi.mock("../app/admin/offsets/_components/offsets-view", () => ({ OffsetsView: () => null }));
vi.mock("../app/admin/offsets/_components/offsets-demo", () => ({ OffsetsDemo: () => null }));

import { quoteOffset, requestOffset, retryOffset, simulateOffset } from "../app/admin/offsets/actions";
import AdminOffsetsPage from "../app/admin/offsets/page";
import { OffsetsView } from "../app/admin/offsets/_components/offsets-view";
import { OffsetsDemo } from "../app/admin/offsets/_components/offsets-demo";
import OffsetsLoading from "../app/admin/offsets/loading";
import OffsetsErrorState from "../app/admin/offsets/error";
import { RouteError, RouteLoading } from "@ac/web/components/RouteStates";
import { NeedsAccess } from "@ac/web/components/NeedsAccess";

const refused = (code: string) => new dal.CoreError({ code, messageKey: `error.${code.toLowerCase()}`, fieldErrors: {}, retryAfterSeconds: null });
const record = (id: string): ApiOffsetRecord => ({
  id, version: 2, createdAt: "2026-09-20T01:00:00Z", quoteId: "quo-00001111", customerId: "cust-a", amountKg: 1.5, attempts: [], currentAttemptId: null,
  state: "demo_requested", previousState: null, purchaseRef: null, retirementRef: null, demoCertificateRef: null, eventHistory: [],
});
const props = (sp: Record<string, string> = {}) => ({ searchParams: Promise.resolve(sp), params: Promise.resolve({}) });
type Live = ComponentProps<typeof OffsetsView>["live"];
async function liveOf(sp: Record<string, string> = {}): Promise<Live> {
  const page: ReactNode = await AdminOffsetsPage(props(sp));
  if (!isValidElement<ComponentProps<typeof OffsetsView>>(page) || page.type !== OffsetsView) throw new TypeError("not OffsetsView");
  return page.props.live;
}
const listInput = () => dal.coreOp.mock.calls.find(([op]) => op === "offsets.list")?.[1];

beforeEach(() => {
  dal.apiMode.mockReturnValue(true);
  dal.corePermissions.mockResolvedValue(new Set(["offset.read", "offset.write"]));
  dal.coreOp.mockReset(); dal.coreAllPage.mockReset(); cache.refresh.mockReset();
  dal.coreAllPage.mockImplementation((op) => Promise.resolve(op === "customers.list"
    ? { items: [{ id: "cust-a", organizationId: "org-a", name: "Demo Customer A" }], total: 1 }
    : { items: [{ id: "u-1", displayName: "Lobby AC", customerOrgId: "org-a", archived: false }, { id: "u-2", displayName: "Old AC", customerOrgId: "org-a", archived: true }, { id: "u-3", displayName: "Stray AC", customerOrgId: "org-x", archived: false }], total: 3 }));
});

describe("offset Server Actions", () => {
  it("asks for a demo quote without a re-render, and requests it with the quote's version", async () => {
    const input = { customerId: "cust-a", purpose: "August", amountKg: 1.25, period: { from: "2026-08-01T00:00:00Z", to: "2026-09-01T00:00:00Z" }, unitIds: ["u-1"] };
    dal.coreOp.mockResolvedValueOnce({ id: "quo-1", version: 1 });
    expect(await quoteOffset(input)).toStrictEqual({ ok: true, value: { id: "quo-1", version: 1 } });
    expect(dal.coreOp).toHaveBeenLastCalledWith("offsets.preview", input, { write: true });
    expect(cache.refresh).not.toHaveBeenCalled(); // a quote is no record yet
    dal.coreOp.mockResolvedValueOnce({ id: "rec-1" });
    expect(await requestOffset("quo-1", 1)).toStrictEqual({ ok: true, value: "rec-1" });
    expect(dal.coreOp).toHaveBeenLastCalledWith("offsets.simulate", { event: "request", demoConfirmed: true, quoteId: "quo-1", quoteVersion: 1 }, { write: true });
    expect(cache.refresh).toHaveBeenCalledOnce();
  });

  it("simulates a step with a fresh event ID and the record's version, and retries the current attempt", async () => {
    dal.coreOp.mockResolvedValue({});
    expect(await simulateOffset("rec-1", 3, "att-1", "retire")).toStrictEqual({ ok: true, value: null });
    expect(await simulateOffset("rec-1", 3, "att-1", "retire")).toStrictEqual({ ok: true, value: null });
    const [first, second] = dal.coreOp.mock.calls.map(([, input]) => input);
    expect(first).toMatchObject({ event: "retire", recordId: "rec-1", attemptId: "att-1", demoConfirmed: true });
    expect(JSON.stringify(first)).not.toBe(JSON.stringify(second)); // each click is its own event
    expect(dal.coreOp.mock.calls[0]?.[2]).toStrictEqual({ write: true, expectedVersion: 3 });
    expect(await retryOffset("rec-1", 4, "att-1")).toStrictEqual({ ok: true, value: null });
    expect(dal.coreOp).toHaveBeenLastCalledWith("offsets.simulate", { event: "retry", demoConfirmed: true, recordId: "rec-1", attemptId: "att-1" }, { write: true, expectedVersion: 4 });
  });

  it("returns a refusal with its code, and still re-renders", async () => {
    dal.coreOp.mockRejectedValueOnce(refused("CONFLICT"));
    expect(await simulateOffset("rec-1", 3, "att-1", "fail")).toStrictEqual({ ok: false, code: "CONFLICT", messageKey: "error.conflict", fieldErrors: {}, retryAfterSeconds: null });
    expect(cache.refresh).toHaveBeenCalledOnce();
  });
});

describe("the offsets page (Server Component)", () => {
  it("shows the demo outside API mode, and its loading and error states", async () => {
    dal.apiMode.mockReturnValue(false);
    const page: ReactNode = await AdminOffsetsPage(props());
    expect(isValidElement(page) && page.type).toBe(OffsetsDemo);
    const loading = OffsetsLoading();
    expect(loading.type === RouteLoading && loading.props).toStrictEqual({ what: "offset records" });
    const error = new Error("down"), retry = vi.fn();
    const failed = OffsetsErrorState({ error, retry });
    expect(failed.type === RouteError && failed.props).toMatchObject({ error, retry });
  });

  it("reads the last 30 days by default and opens the first record", async () => {
    dal.coreOp.mockResolvedValue({ items: [record("rec-1"), record("rec-2")] });
    const live = await liveOf();
    expect(listInput()).toStrictEqual({ limit: 100, filters: { from: "2026-08-23T00:00:00.000Z" } });
    expect([live.tab, live.selectedId, live.recordMissing, live.canWrite, live.scope.created]).toStrictEqual(["records", "rec-1", false, true, "30d"]);
    expect([live.scope.customerId ?? null, live.scope.status ?? null]).toStrictEqual([null, null]); // no customer or status filter
    expect(live.rows.map((r) => [r.id, r.customer])).toStrictEqual([["rec-1", "Demo Customer A"], ["rec-2", "Demo Customer A"]]);
    expect(live.units).toStrictEqual([{ id: "u-1", label: "Lobby AC", customerId: "cust-a" }, { id: "u-3", label: "Stray AC", customerId: "" }]); // archived units are left out
  });

  it("takes the customer, a known status and all time from the URL, and flags a recordId the list does not hold", async () => {
    dal.coreOp.mockResolvedValue({ items: [record("rec-1")] });
    dal.corePermissions.mockResolvedValue(new Set(["offset.read"]));
    const live = await liveOf({ customerId: "cust-a", status: "failed", created: "all", recordId: "rec-gone", tab: "market" });
    expect(listInput()).toStrictEqual({ limit: 100, filters: { customerId: "cust-a", status: "failed" } });
    expect([live.tab, live.selectedId, live.recordMissing, live.canWrite, live.scope]).toStrictEqual(["market", null, true, false, { customerId: "cust-a", status: "failed", created: "all" }]);
    dal.coreOp.mockClear();
    const other = await liveOf({ status: "nonsense", created: "7d", recordId: "rec-1" });
    expect(listInput()).toStrictEqual({ limit: 100, filters: { from: "2026-09-15T00:00:00.000Z" } });
    expect([other.selectedId, other.recordMissing]).toStrictEqual(["rec-1", false]);
  });

  it("selects nothing when no record matches", async () => {
    dal.coreOp.mockResolvedValue({ items: [] });
    const live = await liveOf({ customerId: "cust-a" });
    expect([live.rows, live.selectedId, live.recordMissing]).toStrictEqual([[], null, false]);
  });

  it("names offset.read when the records are refused (IR582)", async () => {
    dal.coreOp.mockRejectedValue(refused("FORBIDDEN"));
    dal.corePermissions.mockResolvedValue(new Set());
    const page: ReactNode = await AdminOffsetsPage(props());
    expect(isValidElement(page) && page.type === NeedsAccess && page.props).toStrictEqual({ what: "Offsets", permission: "offset.read" });
  });
});
