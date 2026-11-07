// @vitest-environment jsdom
import { renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { useOpenedAt } from "@ac/web/lib/useOpenedAt";

afterEach(() => {
  vi.useRealTimers();
});

describe("useOpenedAt", () => {
  it("is the time of the first render and does not move on later renders", () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date("2026-11-07T10:00:00Z"));
    const { result, rerender } = renderHook(() => useOpenedAt());
    const first = result.current;
    expect(first).toBe(Date.parse("2026-11-07T10:00:00Z"));
    vi.setSystemTime(new Date("2026-11-07T10:05:00Z"));
    rerender();
    expect(result.current, "the opening time is kept").toBe(first);
  });
});
