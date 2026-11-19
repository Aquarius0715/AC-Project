import { beforeEach, describe, expect, it, vi } from "vitest";
import { isValidElement } from "react";
import { showSpan } from "@ac/web/lib/i18n";

// the contractor's assignment (FR-P03, jobs.assign): sent as a write at the job's version with the trimmed reason only
// when one is given; the saved slot comes back in the display zone; a CONFLICT re-reads the page, another refusal does
// not. The page renders the fixture demo outside API mode and the live view with single URL keys in API mode.
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
  return { coreOp: vi.fn(), coreDisplay: vi.fn(), apiMode: vi.fn(), CoreError };
});
const cache = vi.hoisted(() => ({ refresh: vi.fn() }));
const load = vi.hoisted(() => ({ loadSchedule: vi.fn() }));
vi.mock("server-only", () => ({}));
vi.mock("next/cache", () => cache);
vi.mock("next/server", () => ({ connection: () => Promise.resolve() }));
vi.mock("@ac/web/lib/dal", () => dal);
vi.mock("../app/partner/schedule/_lib/load", () => load);
vi.mock("../app/partner/schedule/_components/schedule-demo", () => ({ ScheduleDemo: () => null }));
vi.mock("../app/partner/schedule/_components/schedule-view", () => ({ ScheduleView: () => null }));

import { assignTechnician } from "../app/partner/schedule/actions";
import PartnerSchedulePage from "../app/partner/schedule/page";
import { ScheduleDemo } from "../app/partner/schedule/_components/schedule-demo";
import { ScheduleView } from "../app/partner/schedule/_components/schedule-view";
import Loading from "../app/partner/schedule/loading";
import ErrorState from "../app/partner/schedule/error";
import { RouteError, RouteLoading } from "@ac/web/components/RouteStates";

const display = { locale: "en", timeZone: "Asia/Kuala_Lumpur" } as const;
const slot = { startAt: "2026-09-21T02:00:00Z", endAt: "2026-09-21T04:00:00Z" };

beforeEach(() => {
  vi.clearAllMocks();
  dal.coreOp.mockResolvedValue({});
  dal.coreDisplay.mockResolvedValue(display);
  load.loadSchedule.mockResolvedValue({ marker: "live" });
});

describe("assignTechnician", () => {
  it("writes jobs.assign at the job's version with the trimmed reason, re-reads, and returns the slot in the display zone", async () => {
    await expect(assignTechnician("job-1", 7, "m-2", slot, "  sick leave  ")).resolves.toStrictEqual({ ok: true, value: { slot: showSpan(slot.startAt, slot.endAt, display) } });
    expect(dal.coreOp).toHaveBeenCalledExactlyOnceWith("jobs.assign", { jobId: "job-1", technicianMembershipId: "m-2", ...slot, reason: "sick leave" }, { write: true, expectedVersion: 7 });
    expect(cache.refresh).toHaveBeenCalledOnce();
    // a blank reason is left out, not sent empty
    await assignTechnician("job-1", 7, "m-2", slot, " ".repeat(3));
    expect(dal.coreOp).toHaveBeenLastCalledWith("jobs.assign", { jobId: "job-1", technicianMembershipId: "m-2", ...slot }, { write: true, expectedVersion: 7 });
  });

  it("re-reads on a CONFLICT only, returns each refusal with its fields, and reports an unreachable API as unavailable", async () => {
    const conflict = new dal.CoreError("CONFLICT");
    conflict.error.fieldErrors.startAt = "errors.slot_not_agreed";
    dal.coreOp.mockRejectedValueOnce(conflict);
    await expect(assignTechnician("job-1", 7, "m-2", slot, "")).resolves.toStrictEqual({ ok: false, code: "CONFLICT", messageKey: "error.conflict", fieldErrors: { startAt: "errors.slot_not_agreed" }, retryAfterSeconds: null });
    expect(cache.refresh).toHaveBeenCalledOnce();
    dal.coreOp.mockRejectedValueOnce(new dal.CoreError("VALIDATION"));
    await expect(assignTechnician("job-1", 7, "m-2", slot, "")).resolves.toMatchObject({ ok: false, code: "VALIDATION" });
    dal.coreOp.mockRejectedValueOnce(new TypeError("fetch failed"));
    await expect(assignTechnician("job-1", 7, "m-2", slot, "")).resolves.toMatchObject({ ok: false, code: "UNAVAILABLE" });
    expect(cache.refresh).toHaveBeenCalledOnce();
  });
});

type Query = Record<string, string | string[] | undefined>;
const page = (query: Query) => PartnerSchedulePage({ params: Promise.resolve({}), searchParams: Promise.resolve(query) });

describe("Partner schedule page and route states", () => {
  it("renders the demo with the job outside API mode, the view with single URL keys in API mode, and names the schedule in its states", async () => {
    dal.apiMode.mockReturnValue(false);
    const demo = await page({ jobId: "job-p12" });
    expect(isValidElement(demo) && demo.type).toBe(ScheduleDemo);
    expect(isValidElement(demo) && demo.props).toStrictEqual({ jobId: "job-p12" });
    expect(load.loadSchedule).not.toHaveBeenCalled();
    dal.apiMode.mockReturnValue(true);
    const view = await page({ jobId: "job-1", sort: ["dueAt", "status"] });
    // a repeated key passes its first value on; JSON drops the absent slot
    expect(JSON.stringify(load.loadSchedule.mock.calls[0]?.[0])).toBe(`{"jobId":"job-1","sort":"dueAt"}`);
    expect(isValidElement(view) && view.type).toBe(ScheduleView);
    expect(isValidElement(view) && view.props).toStrictEqual({ live: { marker: "live" } });
    const loading = Loading();
    expect(loading.type).toBe(RouteLoading);
    expect(loading.props).toStrictEqual({ what: "the schedule" });
    const error = new Error("boom");
    const retry = vi.fn();
    const failed = ErrorState({ error, retry });
    expect(failed.type).toBe(RouteError);
    expect(failed.props).toStrictEqual({ what: "The schedule", error, retry });
  });
});
