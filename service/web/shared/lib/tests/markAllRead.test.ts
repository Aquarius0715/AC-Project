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
vi.mock("next/cache", () => ({ refresh: vi.fn() }));
vi.mock("@ac/web/lib/dal", () => dal);

import { markAllRead } from "@ac/web/screens/shell/notifications/actions";
import { domainError, type DomainError } from "@ac/web/lib/domainError";

beforeEach(() => dal.coreOp.mockReset());

// IR379: "Mark all as read" is one write, so it spends one of the member's writes a minute (IR375); IR637: it names
// no IDs, so the notifications older than the newest 100 the page lists are marked too
describe("Mark all as read", () => {
  it("asks for every unread notification in one write", async () => {
    dal.coreOp.mockResolvedValueOnce({ markedIds: ["n1", "n2", "n3"], skippedIds: [] });
    expect(await markAllRead()).toStrictEqual({ marked: 3, skipped: 0, failure: null });
    expect(dal.coreOp).toHaveBeenCalledExactlyOnceWith("notifications.markAllRead", { all: true }, { write: true });
  });
  it("passes a refusal on, with the wait of the write limit", async () => {
    dal.coreOp.mockRejectedValueOnce(new dal.CoreError(domainError({ code: "RATE_LIMITED", messageKey: "error.rateLimited", fieldErrors: {}, retryAfterSeconds: 12 })));
    expect(await markAllRead()).toStrictEqual({ marked: 0, skipped: 0, failure: { code: "RATE_LIMITED", messageKey: "error.rateLimited", fieldErrors: {}, retryAfterSeconds: 12 } });
  });
  it("marks nothing when the Core API is unreachable", async () => {
    dal.coreOp.mockRejectedValueOnce(new TypeError("fetch failed"));
    const result = await markAllRead();
    expect(result.failure?.code).toBe("UNAVAILABLE");
  });
});
