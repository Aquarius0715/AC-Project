// @vitest-environment node
import { beforeEach, describe, expect, it, vi } from "vitest";

// the inbox's Server Action calls the Core API through the DAL; refresh re-renders the page
const dal = vi.hoisted(() => {
  class CoreError extends Error {
    constructor(public error: { code: string; messageKey: string; fieldErrors: Record<string, string>; retryAfterSeconds: number | null }) { super(error.code); }
  }
  return { coreOp: vi.fn(), CoreError };
});
vi.mock("server-only", () => ({}));
vi.mock("next/cache", () => ({ refresh: vi.fn() }));
vi.mock("@ac/web/lib/dal", () => dal);

import { markAllRead } from "@ac/web/screens/shell/notifications/actions";

beforeEach(() => dal.coreOp.mockReset());

// IR379: "Mark all as read" is one write for the whole list, so it spends one of the member's writes a minute (IR375)
describe("Mark all as read", () => {
  it("sends the listed notifications in one write", async () => {
    dal.coreOp.mockResolvedValueOnce({ markedIds: ["n1", "n3"], skippedIds: ["n2"] });
    expect(await markAllRead(["n1", "n2", "n3"])).toEqual({ marked: 2, skipped: 1, failure: null });
    expect(dal.coreOp).toHaveBeenCalledTimes(1);
    expect(dal.coreOp).toHaveBeenCalledWith("notifications.markAllRead", { ids: ["n1", "n2", "n3"] }, { write: true });
  });
  it("passes a refusal on, with the wait of the write limit", async () => {
    dal.coreOp.mockRejectedValueOnce(new dal.CoreError({ code: "RATE_LIMITED", messageKey: "error.rateLimited", fieldErrors: {}, retryAfterSeconds: 12 }));
    expect(await markAllRead(["n1"])).toEqual({ marked: 0, skipped: 0, failure: { code: "RATE_LIMITED", messageKey: "error.rateLimited", fieldErrors: {}, retryAfterSeconds: 12 } });
  });
  it("marks nothing when the Core API is unreachable", async () => {
    dal.coreOp.mockRejectedValueOnce(new TypeError("fetch failed"));
    expect((await markAllRead(["n1"])).failure?.code).toBe("UNAVAILABLE");
  });
});
