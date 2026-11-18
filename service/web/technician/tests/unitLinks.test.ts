import { beforeEach, describe, expect, it, vi } from "vitest";

// the alert evidence and diagnostic control screens link back to the unit with the URL's jobId: one encoded value, so
// "x&alertId=other" adds no parameter to the link
const dal = vi.hoisted(() => {
  class CoreError extends Error {
    readonly error: { code: string; messageKey: string; fieldErrors: Record<string, string> };
    readonly status: number;
    constructor(code: string, options: ErrorOptions & { messageKey: string; status: number }) {
      super(code, options);
      this.name = "CoreError";
      this.error = { code, messageKey: options.messageKey, fieldErrors: {} };
      this.status = options.status;
    }
  }
  return {
    CoreError, coreNow: vi.fn(), coreDisplay: vi.fn(), corePermissions: vi.fn(), coreOp: vi.fn(), coreAll: vi.fn(), coreItems: vi.fn(),
    orWhenRefused: async <T, F>(read: Promise<T>, fallback: F): Promise<T | F> => read.catch(() => fallback),
  };
});
vi.mock("server-only", () => ({}));
vi.mock("@ac/web/lib/dal", () => dal);

import { loadAlerts } from "../app/technician/units/[id]/alerts/_lib/load";
import { loadControl } from "../app/technician/units/[id]/control/_lib/load";

const JOB = "x&alertId=other";
const UNIT_HREF = "/technician/units/unit-1?jobId=x%26alertId%3Dother";
const notStarted = () => Promise.reject(new dal.CoreError("FORBIDDEN", { messageKey: "errors.assignment_not_started", status: 403 }));

beforeEach(() => {
  vi.clearAllMocks();
  dal.coreNow.mockResolvedValue(new Date("2026-09-14T01:35:00Z"));
  dal.coreDisplay.mockResolvedValue({ locale: "en", timeZone: "Asia/Kuala_Lumpur" });
  dal.corePermissions.mockResolvedValue(new Set<string>());
});

describe("technician unit links", () => {
  it("alert evidence links back to the unit with the job encoded", async () => {
    dal.coreAll.mockImplementation(notStarted);
    dal.coreItems.mockImplementation(notStarted);
    dal.coreOp.mockImplementation(notStarted);
    const page = await loadAlerts("unit-1", { jobId: JOB });
    expect(page).toMatchObject({ kind: "unavailable", unitHref: UNIT_HREF });
  });

  it("diagnostic control links back to the unit with the job encoded", async () => {
    dal.coreOp.mockImplementation(notStarted);
    const page = await loadControl("unit-1", JOB);
    expect(page).toMatchObject({ kind: "not_started", unitHref: UNIT_HREF });
  });
});
