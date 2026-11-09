import { describe, expect, it, vi } from "vitest";
import { CoreError } from "@ac/web/lib/dal";
import { actionFailure } from "@ac/web/lib/actionFailure";

// the DAL and Next's control-flow errors come from the server side: a redirect is thrown and must pass through
vi.mock("server-only", () => ({}));
vi.mock("next/headers", () => ({ cookies: () => Promise.resolve({ get: vi.fn() }) }));
vi.mock("next/navigation", () => ({
  redirect: (to: string) => { throw new Error(`NEXT_REDIRECT ${to}`); },
  unstable_rethrow: (error: unknown) => { if (Error.isError(error) && error.message.startsWith("NEXT_REDIRECT")) throw error; },
}));

describe("the failure a Server Action returns", () => {
  it("keeps the code, the key and the field errors of a refused call", () => {
    const refused = new CoreError({ code: "CONFLICT", messageKey: "error.versionConflict", fieldErrors: { a: "error.invalid" }, correlationId: "", retryAfterSeconds: 3 }, { status: 409 });
    expect(actionFailure(refused)).toStrictEqual({ ok: false, code: "CONFLICT", messageKey: "error.versionConflict", fieldErrors: { a: "error.invalid" }, retryAfterSeconds: 3 });
  });

  it("reads a call that failed without an answer (the Core API is unreachable) as unavailable, so the page keeps its edits", () => {
    expect(actionFailure(new TypeError("fetch failed"))).toStrictEqual({ ok: false, code: "UNAVAILABLE", messageKey: "error.unavailable", fieldErrors: {} });
  });

  it("lets Next's own control flow (a redirect to sign-in) through", () => {
    expect(() => actionFailure(new Error("NEXT_REDIRECT /login"))).toThrow("NEXT_REDIRECT /login");
  });
});
