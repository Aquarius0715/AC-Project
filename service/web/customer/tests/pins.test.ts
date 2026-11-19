import { beforeEach, describe, expect, it, vi } from "vitest";

// the customer's pins are saved with the user's preferences (IR819): preferences.update must keep the stored language
// and time zone, a refusal keeps its code, and a stored value of the wrong shape reads as unavailable (R836)
const dal = vi.hoisted(() => {
  class CoreError extends Error {
    readonly error: { code: string; messageKey: string; fieldErrors: Record<string, string>; retryAfterSeconds: number | null };
    constructor(code: string, options?: ErrorOptions) {
      super(code, options);
      this.name = "CoreError";
      this.error = { code, messageKey: `error.${code}`, fieldErrors: {}, retryAfterSeconds: null };
    }
  }
  return { coreOp: vi.fn(), CoreError };
});
vi.mock("server-only", () => ({}));
vi.mock("@ac/web/lib/dal", () => dal);

import { togglePinned } from "../app/customer/_lib/pins";

const stored = (pinnedUnitIds?: string[]) => ({ locale: "ja", timezone: "Asia/Kuala_Lumpur", ...(pinnedUnitIds && { pinnedUnitIds }) });

beforeEach(() => { dal.coreOp.mockReset(); });

describe("togglePinned", () => {
  it("puts a new pin first and keeps the stored language and time zone", async () => {
    dal.coreOp.mockResolvedValueOnce(stored(["unit-1"])).mockResolvedValueOnce(null);
    await expect(togglePinned("unit-2")).resolves.toStrictEqual({ ok: true, value: ["unit-2", "unit-1"] });
    expect(dal.coreOp).toHaveBeenLastCalledWith(
      "preferences.update",
      { locale: "ja", timezone: "Asia/Kuala_Lumpur", pinnedUnitIds: ["unit-2", "unit-1"] },
      { write: true },
    );
  });

  it("unpins a pinned AC and starts from no pins when none are stored", async () => {
    dal.coreOp.mockResolvedValueOnce(stored(["unit-1", "unit-2"])).mockResolvedValueOnce(null);
    await expect(togglePinned("unit-1")).resolves.toStrictEqual({ ok: true, value: ["unit-2"] });
    dal.coreOp.mockResolvedValueOnce(stored()).mockResolvedValueOnce(null);
    await expect(togglePinned("unit-1")).resolves.toStrictEqual({ ok: true, value: ["unit-1"] });
  });

  it("returns the refusal of preferences.update with its code", async () => {
    dal.coreOp.mockResolvedValueOnce(stored([])).mockRejectedValueOnce(new dal.CoreError("VALIDATION"));
    await expect(togglePinned("unit-1")).resolves.toMatchObject({ ok: false, code: "VALIDATION", messageKey: "error.VALIDATION" });
  });

  it("does not write when the stored preferences have the wrong shape", async () => {
    dal.coreOp.mockResolvedValueOnce({ locale: "fr", timezone: "UTC" });
    await expect(togglePinned("unit-1")).resolves.toMatchObject({ ok: false, code: "UNAVAILABLE" });
    expect(dal.coreOp).toHaveBeenCalledOnce();
  });
});
