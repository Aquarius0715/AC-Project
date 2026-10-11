// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, render, screen } from "@testing-library/react";
import { RouteError } from "@ac/web/components/RouteStates";

const assign = vi.fn();
beforeEach(() => {
  assign.mockClear();
  vi.stubGlobal("location", { ...window.location, pathname: "/customer/units/u-1", search: "?tab=readings", assign });
});
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const show = async (session: () => Promise<Response>) => {
  vi.stubGlobal("fetch", vi.fn(session));
  await act(async () => { render(<RouteError what="unit" error={new Error("refused")} retry={() => {}} />); });
  await act(async () => { await new Promise((r) => setTimeout(r, 0)); }); // the session check settles
};
const json = (body: unknown) => () => Promise.resolve(new Response(JSON.stringify(body)));

// a page that failed because its session was refused (IR385): the DAL dropped the session, so the route error state
// sends the browser to sign-in and back; any other failure stays on the page with "Try again"
describe("RouteError", () => {
  it("goes to sign-in when the session is gone in API mode", async () => {
    await show(json({ dataSource: "api", signedIn: false }));
    expect(assign.mock.calls).toEqual([["/login?error=expired&returnTo=%2Fcustomer%2Funits%2Fu-1%3Ftab%3Dreadings"]]);
  });

  it("stays with Try again while signed in, in the demo, or when the check fails", async () => {
    for (const session of [json({ dataSource: "api", signedIn: true }), json({ dataSource: "mock", signedIn: false }), () => Promise.reject(new Error("offline"))]) {
      await show(session);
      expect(screen.getAllByRole("button", { name: "Try again" }).length).toBeGreaterThan(0);
      cleanup();
    }
    expect(assign).not.toHaveBeenCalled();
  });
});
