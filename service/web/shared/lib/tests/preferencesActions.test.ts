// @vitest-environment node
import { beforeEach, describe, expect, it, vi } from "vitest";

// the Server Actions of /settings/preferences call the Core API through the DAL; refresh re-renders the page
const dal = vi.hoisted(() => {
  class CoreError extends Error {
    error: DomainError;
    constructor(error: DomainError, options?: ErrorOptions) { super(error.code, options); this.name = "CoreError"; this.error = error; }
  }
  return { coreOp: vi.fn(), CoreError };
});
const cache = vi.hoisted(() => ({ refresh: vi.fn() }));
vi.mock("server-only", () => ({}));
vi.mock("next/cache", () => cache);
vi.mock("@ac/web/lib/dal", () => dal);

import { disableTwoFactor, enableTwoFactor, savePreferences } from "@ac/web/screens/shell/settings/preferences/actions";
import { domainError, type DomainError } from "@ac/web/lib/domainError";

const PREFS = { locale: "en" as const, timezone: "Asia/Kuala_Lumpur" };
const refusal = (code: DomainError["code"], messageKey: string) => new dal.CoreError(domainError({ code, messageKey, fieldErrors: {}, retryAfterSeconds: null }));

beforeEach(() => {
  dal.coreOp.mockReset();
  cache.refresh.mockReset();
});

describe("savePreferences (FR-X01)", () => {
  it("saves the preferences, then the changed location consent with its version, and re-renders", async () => {
    dal.coreOp.mockResolvedValue({});
    expect(await savePreferences(PREFS, { granted: true, version: 3 })).toStrictEqual({ ok: true, value: null });
    expect(dal.coreOp.mock.calls).toStrictEqual([
      ["preferences.update", PREFS, { write: true }],
      ["consents.update", { purpose: "location_automation", granted: true }, { write: true, expectedVersion: 3 }],
    ]);
    expect(cache.refresh).toHaveBeenCalledOnce();
  });

  it("re-renders when the preferences were saved but the consent was refused, so the page shows what landed", async () => {
    dal.coreOp.mockResolvedValueOnce({}).mockRejectedValueOnce(refusal("RATE_LIMITED", "error.rateLimited"));
    expect(await savePreferences(PREFS, { granted: false, version: 1 })).toMatchObject({ ok: false, code: "RATE_LIMITED" });
    expect(cache.refresh).toHaveBeenCalled();
  });

  it("does not re-render when the preferences themselves were refused", async () => {
    dal.coreOp.mockRejectedValueOnce(refusal("VALIDATION", "error.validation"));
    expect(await savePreferences(PREFS, { granted: true, version: 0 })).toMatchObject({ ok: false, code: "VALIDATION" });
    expect(dal.coreOp).toHaveBeenCalledOnce();
    expect(cache.refresh).not.toHaveBeenCalled();
  });
});

describe("two-step verification (IR144)", () => {
  it("returns the recovery codes of twoFactor.enable once", async () => {
    dal.coreOp.mockResolvedValueOnce({ recoveryCodes: ["a1", "b2"] });
    expect(await enableTwoFactor("123456")).toStrictEqual({ ok: true, value: ["a1", "b2"] });
    expect(dal.coreOp).toHaveBeenCalledExactlyOnceWith("twoFactor.enable", { code: "123456" }, { write: true });
  });

  it("passes a wrong code on and re-renders only after a version conflict", async () => {
    dal.coreOp.mockRejectedValueOnce(refusal("VALIDATION", "error.twoFactorCode"));
    expect(await disableTwoFactor("000000")).toMatchObject({ ok: false, code: "VALIDATION", messageKey: "error.twoFactorCode" });
    expect(cache.refresh).not.toHaveBeenCalled();
    dal.coreOp.mockRejectedValueOnce(refusal("CONFLICT", "error.conflict"));
    expect(await disableTwoFactor("000000")).toMatchObject({ ok: false, code: "CONFLICT" });
    expect(cache.refresh).toHaveBeenCalledOnce();
  });

  it("answers UNAVAILABLE when the Core API is unreachable", async () => {
    dal.coreOp.mockRejectedValueOnce(new TypeError("fetch failed"));
    expect(await enableTwoFactor("123456")).toMatchObject({ ok: false, code: "UNAVAILABLE" });
  });
});
