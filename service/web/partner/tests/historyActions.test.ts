import { beforeEach, describe, expect, it, vi } from "vitest";

// the contractor's job communication (FR-P07, DD-P07): the note is saved at the job's version, then the preview is
// rendered; a refused note stops before the preview (a CONFLICT shows the latest job), and a refused preview after a
// saved note says the note was kept
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
  return { coreOp: vi.fn(), coreDisplay: vi.fn(), CoreError };
});
const cache = vi.hoisted(() => ({ refresh: vi.fn() }));
vi.mock("server-only", () => ({}));
vi.mock("next/cache", () => cache);
vi.mock("@ac/web/lib/dal", () => dal);

import { communicate, type Communication } from "../app/partner/history/actions";

const input = (over: Partial<Communication> = {}): Communication => ({
  jobId: "job-1", version: 3, saveNote: true, message: "  Moved to Friday.  ", visibility: "internal", templateKey: "schedule_change", channel: "inApp", recipientMembershipId: "mem-hq", ...over,
});
const preview = { id: "pv-1", templateKey: "schedule_change", channel: "inApp", recipientMembershipId: "mem-hq", deliveryState: "preview", occurredAt: "2026-11-18T02:00:00Z" };
const calls = (): unknown[] => dal.coreOp.mock.calls.map((call: unknown[]) => call[0]);

beforeEach(() => {
  vi.clearAllMocks();
  // Tokyo (UTC+9), not the default Kuala Lumpur (UTC+8), so the user's own zone shows in the preview's time
  dal.coreDisplay.mockResolvedValue({ locale: "en", timeZone: "Asia/Tokyo" });
  dal.coreOp.mockImplementation((operation: string) => Promise.resolve(operation === "notifications.preview" ? preview : {}));
});

describe("communicate", () => {
  it("saves the trimmed note at the job's version, then previews the same message and shows the latest job", async () => {
    const result = await communicate(input());
    expect(dal.coreOp).toHaveBeenNthCalledWith(1, "jobs.addNote", { jobId: "job-1", visibility: "internal", message: "Moved to Friday." }, { write: true, expectedVersion: 3 });
    expect(dal.coreOp).toHaveBeenNthCalledWith(2, "notifications.preview", { target: { kind: "job", id: "job-1" }, templateKey: "schedule_change", channel: "inApp", recipientMembershipId: "mem-hq", message: "Moved to Friday." });
    expect(result.ok && result.value.noteSaved).toBe(true);
    // 02:00 UTC is 11:00 in the user's display time zone
    expect(result.ok ? result.value.preview.at : null).toContain("2026, 11:00");
    expect(cache.refresh).toHaveBeenCalledOnce();
  });

  it("only previews a job that takes no notes, without a version and without refreshing", async () => {
    const result = await communicate(input({ saveNote: false, version: null }));
    expect(calls()).toStrictEqual(["notifications.preview"]);
    expect(result.ok && result.value.noteSaved).toBe(false);
    expect(cache.refresh).not.toHaveBeenCalled();
  });

  it("sends a note without a known version with no expected version", async () => {
    await communicate(input({ version: null }));
    // JSON drops an expected version left undefined; any number would show here
    expect(JSON.stringify(dal.coreOp.mock.calls[0]?.[2])).toBe(`{"write":true}`);
  });

  it("stops at a refused note: no preview, the form is kept, and a CONFLICT shows the latest job", async () => {
    dal.coreOp.mockRejectedValueOnce(new dal.CoreError("FORBIDDEN"));
    await expect(communicate(input())).resolves.toStrictEqual({ ok: false, code: "FORBIDDEN", messageKey: "error.forbidden", fieldErrors: {}, retryAfterSeconds: null, stage: "note", noteSaved: false });
    expect(calls()).toStrictEqual(["jobs.addNote"]);
    expect(cache.refresh).not.toHaveBeenCalled();
    dal.coreOp.mockRejectedValueOnce(new dal.CoreError("CONFLICT"));
    await expect(communicate(input())).resolves.toMatchObject({ ok: false, code: "CONFLICT", stage: "note", noteSaved: false });
    expect(cache.refresh).toHaveBeenCalledOnce();
  });

  it("says the note was saved when the preview after it is refused, and shows the job with the new note", async () => {
    dal.coreOp.mockImplementation((operation: string) => (operation === "notifications.preview" ? Promise.reject(new dal.CoreError("VALIDATION")) : Promise.resolve({})));
    await expect(communicate(input())).resolves.toMatchObject({ ok: false, code: "VALIDATION", stage: "preview", noteSaved: true });
    expect(cache.refresh).toHaveBeenCalledOnce();
  });

  it("reports a refused preview without a note as nothing saved, and an unreachable Core API as unavailable", async () => {
    dal.coreOp.mockRejectedValueOnce(new dal.CoreError("NOT_FOUND"));
    await expect(communicate(input({ saveNote: false }))).resolves.toMatchObject({ ok: false, code: "NOT_FOUND", stage: "preview", noteSaved: false });
    dal.coreOp.mockRejectedValueOnce(new TypeError("fetch failed"));
    await expect(communicate(input({ saveNote: false }))).resolves.toMatchObject({ ok: false, code: "UNAVAILABLE", stage: "preview", noteSaved: false });
    expect(cache.refresh).not.toHaveBeenCalled();
  });
});
