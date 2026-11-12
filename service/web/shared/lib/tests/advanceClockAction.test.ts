// @vitest-environment node
import { beforeEach, describe, expect, it, vi } from "vitest";

// the Demo panel's Server Action calls the Core API through the DAL; refresh re-renders the page
const dal = vi.hoisted(() => {
  class CoreError extends Error {
    error: unknown;
    constructor(error: unknown, options?: ErrorOptions) { super("core", options); this.name = "CoreError"; this.error = error; }
  }
  return { coreOp: vi.fn(), coreClockFresh: vi.fn(), CoreError };
});
vi.mock("server-only", () => ({}));
vi.mock("next/cache", () => ({ refresh: vi.fn() }));
vi.mock("next/headers", () => ({ cookies: vi.fn() }));
vi.mock("next/navigation", () => ({ redirect: vi.fn() }));
vi.mock("@ac/web/lib/dal", () => dal);

import { advanceClock } from "@ac/web/screens/shell/demo/actions";
import { CLOCK_JUMPS } from "@ac/web/lib/demoPanel";

beforeEach(() => { dal.coreOp.mockReset(); dal.coreClockFresh.mockReset(); });

// IR641 added the 2–24 hour buttons, but the action still took a minute and an hour only
describe("the Demo clock action", () => {
  it.each(CLOCK_JUMPS.map((m) => [m]))("moves the clock by the %i-minute jump", async (minutes) => {
    const from = new Date("2026-11-12T01:00:00Z");
    const to = new Date(from.getTime() + minutes * 60_000);
    dal.coreClockFresh.mockResolvedValueOnce(from).mockResolvedValue(to);
    dal.coreOp.mockResolvedValueOnce(null);
    const result = await advanceClock(minutes);
    expect(result.ok).toBe(true);
    expect(dal.coreOp).toHaveBeenCalledExactlyOnceWith("demo.advanceClock", { to: to.toISOString() }, { write: true });
  });

  it("refuses a jump the panel does not offer", async () => {
    const result = await advanceClock(5);
    expect(result).toMatchObject({ ok: false, code: "VALIDATION", fieldErrors: { minutes: "error.invalid" } });
    expect(dal.coreOp).not.toHaveBeenCalled();
  });
});
