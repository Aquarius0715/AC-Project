// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, renderHook } from "@testing-library/react";

// a tap changes the pins at once; the saved list replaces them, and a refused save puts back the pins from before the
// tap and says so (IR819, R836)
const pins = vi.hoisted(() => ({ togglePinned: vi.fn() }));
const toast = vi.hoisted(() => vi.fn());
vi.mock("../app/customer/_lib/pins", () => pins);
vi.mock("@ac/web/components/ui", () => ({ useToast: () => toast }));
vi.mock("@ac/web/components/I18n", () => ({ useT: () => (s: string) => s }));

import { usePins } from "../app/customer/_lib/usePins";

afterEach(cleanup);

describe("usePins", () => {
  type Result = { ok: true; value: string[] } | { ok: false; messageKey: string; code: string; fieldErrors: Record<string, string> };

  it("shows the pin at once and then the list the server saved", async () => {
    const save = Promise.withResolvers<Result>();
    pins.togglePinned.mockReturnValueOnce(save.promise);
    const { result } = renderHook(() => usePins(["unit-1"]));
    act(() => { result.current[1]("unit-2"); });
    expect(result.current[0]).toStrictEqual(["unit-2", "unit-1"]);
    await act(async () => { save.resolve({ ok: true, value: ["unit-2"] }); await save.promise; });
    expect(result.current[0]).toStrictEqual(["unit-2"]);
    expect(toast).not.toHaveBeenCalled();
  });

  it("puts the pins back and warns when the save is refused", async () => {
    const save = Promise.withResolvers<Result>();
    pins.togglePinned.mockReturnValueOnce(save.promise);
    const { result } = renderHook(() => usePins(["unit-1"]));
    act(() => { result.current[1]("unit-1"); });
    expect(result.current[0]).toStrictEqual([]);
    await act(async () => {
      save.resolve({ ok: false, messageKey: "error.unavailable", code: "UNAVAILABLE", fieldErrors: {} });
      await save.promise;
    });
    expect(result.current[0]).toStrictEqual(["unit-1"]);
    expect(toast).toHaveBeenCalledWith("The pin was not saved — try again.", "warn");
  });
});
