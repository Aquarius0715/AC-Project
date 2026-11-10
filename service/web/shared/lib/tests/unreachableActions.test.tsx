// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { toast } from "sonner";
import type { unstable_rethrow as rethrow } from "next/navigation";
import { ToastProvider } from "@ac/web/components/ui";

// the inbox runs its Server Actions in its own transitions, not through useAction; a call that gets no answer (the
// browser offline, the app server restarting) rejects in the browser (IR718)
const actions = vi.hoisted(() => ({ markRead: vi.fn(), markAllRead: vi.fn() }));
vi.mock("@ac/web/screens/shell/notifications/actions", () => actions);
vi.mock("next/navigation", async (actual) => {
  const real = await actual<{ unstable_rethrow: typeof rethrow }>();
  return { unstable_rethrow: real.unstable_rethrow, useRouter: () => ({ refresh: vi.fn() }), usePathname: () => "/notifications" };
});

const { NotificationsView } = await import("@ac/web/screens/shell/notifications/_components/notifications-view");
const { orUnreachable, reached } = await import("@ac/web/lib/unreachable");

afterEach(() => {
  cleanup();
  toast.dismiss();
});

const rows = [{ id: "n-1", version: 1, t: "Alert on Bedroom AC", d: "", w: "10:00", read: false, href: "" }];

describe("an inbox action that gets no answer", () => {
  it("reads a single mark-read as unavailable and keeps the inbox", async () => {
    actions.markRead.mockRejectedValue(new TypeError("Failed to fetch"));
    render(<ToastProvider><NotificationsView rows={rows} /></ToastProvider>);
    await act(() => { fireEvent.click(screen.getByRole("button", { name: /Alert on Bedroom AC/ })); return Promise.resolve(); });
    const alert = await screen.findByRole("alert");
    expect(alert.textContent).toBe("Not marked as read: The service is unavailable — try again shortly");
  });

  it("reads mark all as read as unavailable and keeps the inbox", async () => {
    actions.markAllRead.mockRejectedValue(new TypeError("Failed to fetch"));
    render(<ToastProvider><NotificationsView rows={rows} /></ToastProvider>);
    await act(() => { fireEvent.click(screen.getByRole("button", { name: "Mark all as read" })); return Promise.resolve(); });
    const alerts = await screen.findAllByRole("alert");
    expect(alerts.map((a) => a.textContent)).toContain("Not marked as read: The service is unavailable — try again shortly");
    expect(screen.getByRole("button", { name: "Mark all as read" })).toBeTruthy();
  });
});

describe("reached and orUnreachable", () => {
  it("pass an answer through and read a call without one as the unavailable failure", async () => {
    const unavailable = { ok: false, code: "UNAVAILABLE", messageKey: "error.unavailable", fieldErrors: {} };
    expect(await reached(() => Promise.resolve({ ok: true, value: 1 }))).toStrictEqual({ ok: true, value: 1 });
    expect(await reached(() => Promise.reject(new TypeError("Failed to fetch")))).toStrictEqual(unavailable);
    expect(await orUnreachable(() => Promise.reject(new TypeError("Failed to fetch")), (failure) => ({ marked: 0, failure }))).toStrictEqual({ marked: 0, failure: unavailable });
  });
});
