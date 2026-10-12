// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, render, screen } from "@testing-library/react";
import { toast } from "sonner";
import { ToastProvider } from "@ac/web/components/ui";
import { useAction } from "@ac/web/lib/useAction";

type Result = { ok: true; value: string } | { ok: false; code: string; messageKey: string; fieldErrors: Record<string, string>; retryAfterSeconds?: number | null };

const assign = vi.fn();
beforeEach(() => {
  assign.mockClear();
  // the page the action runs on; jsdom cannot navigate, so the sign-in redirect is caught here
  vi.stubGlobal("location", { ...window.location, pathname: "/admin/units", search: "?customerId=c-1&tab=users", assign });
});
afterEach(() => {
  cleanup();
  toast.dismiss(); // Sonner's store outlives the Toaster: an open toast would be replayed in the next test
  vi.unstubAllGlobals();
});

function Runner({ result, after, failed }: { result: Result; after: (v: string) => void; failed: () => void }) {
  const [pending, run] = useAction();
  return <button disabled={pending} onClick={() => run(async () => result, (v: string) => `Saved ${v}`, after, failed)}>go</button>;
}

const press = async (result: Result) => {
  const after = vi.fn(), failed = vi.fn();
  render(<ToastProvider><Runner result={result} after={after} failed={failed} /></ToastProvider>);
  await act(async () => { screen.getByRole("button", { name: "go" }).click(); });
  return { after, failed };
};

// useAction runs a Server Action: a toast and `after` on success, the readable refusal otherwise (IR269), and a
// refused session goes to sign-in and back to this page (IR385)
describe("useAction", () => {
  it("shows the success toast and runs after", async () => {
    const { after, failed } = await press({ ok: true, value: "unit-1" });
    expect([(await screen.findByRole("status")).textContent, after.mock.calls, failed.mock.calls.length, assign.mock.calls.length]).toEqual(["Saved unit-1", [["unit-1"]], 0, 0]);
  });

  it("shows a refusal as an error toast and runs failed", async () => {
    const { after, failed } = await press({ ok: false, code: "CONFLICT", messageKey: "error.versionConflict", fieldErrors: {} });
    expect((await screen.findByRole("alert")).textContent).not.toBe("");
    expect([after.mock.calls.length, failed.mock.calls.length, assign.mock.calls.length]).toEqual([0, 1, 0]);
  });

  it("sends a session HQ signed out everywhere to sign-in, back to this page", async () => {
    const { after, failed } = await press({ ok: false, code: "UNAUTHENTICATED", messageKey: "errors.session_revoked", fieldErrors: {} });
    expect(assign.mock.calls).toEqual([["/login?error=revoked&returnTo=%2Fadmin%2Funits%3FcustomerId%3Dc-1%26tab%3Dusers"]]);
    await act(() => new Promise((r) => setTimeout(r, 20))); // past the tick on which Sonner would add a toast
    expect([after.mock.calls.length, failed.mock.calls.length, screen.queryByRole("alert")?.textContent ?? ""]).toEqual([0, 0, ""]);
  });

  it("sends an expired session to sign-in too", async () => {
    await press({ ok: false, code: "UNAUTHENTICATED", messageKey: "error.membershipInvalid", fieldErrors: {} });
    expect(assign.mock.calls).toEqual([["/login?error=expired&returnTo=%2Fadmin%2Funits%3FcustomerId%3Dc-1%26tab%3Dusers"]]);
  });
});
