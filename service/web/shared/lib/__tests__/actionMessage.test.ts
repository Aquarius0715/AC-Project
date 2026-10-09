import { describe, expect, it } from "vitest";
import { actionMessage } from "@ac/web/lib/actionMessage";

const failure = (over: Partial<Parameters<typeof actionMessage>[0]>) => ({ code: "VALIDATION", messageKey: "error.validation", fieldErrors: {}, ...over });

describe("actionMessage (one toast line per failed Server Action)", () => {
  it("reads the first field error with a known or humanized key", () => {
    expect(actionMessage(failure({ fieldErrors: { reason: "error.length" } }))).toBe("Reason is too short or too long");
    expect(actionMessage(failure({ fieldErrors: { "query.filters.to": "error.range" } }))).toBe("To is out of range");
    expect(actionMessage(failure({ fieldErrors: { calibratedAt: "error.future" } }))).toBe("Calibrated at must not be in the future");
    expect(actionMessage(failure({ fieldErrors: { unit: "errors.unit_mismatch" } }))).toBe("Unit — unit mismatch");
  });
  it("explains version conflicts, domain errors and bare codes", () => {
    expect(actionMessage(failure({ code: "CONFLICT", messageKey: "error.versionConflict" }))).toMatch(/changed this in the meantime/);
    expect(actionMessage(failure({ code: "CONFLICT", messageKey: "errors.operation_not_running" }))).toBe("Operation not running");
    expect(actionMessage(failure({ code: "NOT_FOUND", messageKey: "error.notFound" }))).toBe("This item no longer exists or is outside your scope");
    expect(actionMessage(failure({ code: "FORBIDDEN", messageKey: "error.forbidden" }))).toMatch(/permission/);
    expect(actionMessage(failure({ code: "RATE_LIMITED", messageKey: "error.rateLimited" }))).toBe("RATE_LIMITED: Rate limited");
  });
});
