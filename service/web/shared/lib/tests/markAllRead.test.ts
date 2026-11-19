// @vitest-environment node
import { beforeEach, describe, expect, it, vi } from "vitest";

// the inbox's Server Action calls the Core API through the DAL; refresh re-renders the page
const dal = vi.hoisted(() => {
  class CoreError extends Error {
    error: DomainError;
    constructor(error: DomainError, options?: ErrorOptions) { super(error.code, options); this.name = "CoreError"; this.error = error; }
  }
  return { coreOp: vi.fn(), CoreError };
});
vi.mock("server-only", () => ({}));
const cache = vi.hoisted(() => ({ refresh: vi.fn() }));
vi.mock("next/cache", () => cache);
vi.mock("@ac/web/lib/dal", () => dal);

import { markAllRead, markRead } from "@ac/web/screens/shell/notifications/actions";
import { domainError, type DomainError } from "@ac/web/lib/domainError";

beforeEach(() => {
  dal.coreOp.mockReset();
  cache.refresh.mockReset();
});

// IR380: one notification is marked with its row version; the list is re-read whether or not the write landed
describe("Mark as read", () => {
  it("sends the row version and re-renders after a success and after a refusal", async () => {
    dal.coreOp.mockResolvedValueOnce({});
    expect(await markRead("n1", 4)).toBeNull();
    expect(dal.coreOp).toHaveBeenCalledExactlyOnceWith("notifications.markRead", { id: "n1" }, { write: true, expectedVersion: 4 });
    dal.coreOp.mockRejectedValueOnce(new dal.CoreError(domainError({ code: "CONFLICT", messageKey: "error.conflict", fieldErrors: {}, retryAfterSeconds: null })));
    expect(await markRead("n1", 4)).toMatchObject({ code: "CONFLICT", messageKey: "error.conflict" });
    expect(cache.refresh).toHaveBeenCalledTimes(2);
  });
});

// IR379: "Mark all as read" is one write for the whole list, so it spends one of the member's writes a minute (IR375)
describe("Mark all as read", () => {
  it("sends the listed notifications in one write", async () => {
    dal.coreOp.mockResolvedValueOnce({ markedIds: ["n1", "n3"], skippedIds: ["n2"] });
    expect(await markAllRead(["n1", "n2", "n3"])).toStrictEqual({ marked: 2, skipped: 1, failure: null });
    expect(dal.coreOp).toHaveBeenCalledExactlyOnceWith("notifications.markAllRead", { ids: ["n1", "n2", "n3"] }, { write: true });
  });
  it("passes a refusal on, with the wait of the write limit", async () => {
    dal.coreOp.mockRejectedValueOnce(new dal.CoreError(domainError({ code: "RATE_LIMITED", messageKey: "error.rateLimited", fieldErrors: {}, retryAfterSeconds: 12 })));
    expect(await markAllRead(["n1"])).toStrictEqual({ marked: 0, skipped: 0, failure: { ok: false, code: "RATE_LIMITED", messageKey: "error.rateLimited", fieldErrors: {}, retryAfterSeconds: 12 } }); // actionFailure marks it ok: false (IR686)
  });
  it("marks nothing when the Core API is unreachable", async () => {
    dal.coreOp.mockRejectedValueOnce(new TypeError("fetch failed"));
    const result = await markAllRead(["n1"]);
    expect(result.failure?.code).toBe("UNAVAILABLE");
  });
});
