// @vitest-environment jsdom
import { useState } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, render, screen } from "@testing-library/react";
import { useRefreshThen } from "@ac/web/lib/useRefreshThen";

// the router of the page: a change that succeeded asks it to read the screen again (IR681)
const refresh = vi.hoisted(() => vi.fn());
vi.mock("next/navigation", () => ({ useRouter: () => ({ refresh }) }));

beforeEach(() => { refresh.mockClear(); });
afterEach(cleanup);

// a screen with its own transition flag: `pending` is true while the action and the new read run
function Screen({ then }: Readonly<{ then: () => void }>) {
  const [pending, setPending] = useState(false);
  const refreshThen = useRefreshThen(pending);
  return (
    <>
      <button type="button" onClick={() => { setPending(true); refreshThen(then); }}>change</button>
      <button type="button" onClick={() => { setPending(false); }}>shown</button>
    </>
  );
}

describe("useRefreshThen", () => {
  it("reads the page again once and runs the notice only when the transition is over", async () => {
    const then = vi.fn();
    render(<Screen then={then} />);
    await act(() => { screen.getByRole("button", { name: "change" }).click(); return Promise.resolve(); });
    expect([refresh.mock.calls.length, then.mock.calls.length]).toStrictEqual([1, 0]); // the screen is not shown yet
    await act(() => { screen.getByRole("button", { name: "shown" }).click(); return Promise.resolve(); });
    expect([refresh.mock.calls.length, then.mock.calls.length]).toStrictEqual([1, 1]); // shown: the notice comes
    await act(() => { screen.getByRole("button", { name: "shown" }).click(); return Promise.resolve(); });
    expect(then.mock.calls).toHaveLength(1); // once
  });
});
