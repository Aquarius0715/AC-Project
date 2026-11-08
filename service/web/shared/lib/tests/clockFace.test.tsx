// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { ClockFace } from "@ac/web/screens/shell/demo/_components/demo-frame";

afterEach(cleanup);

// IR641: the Demo clock moves by a minute or by 1, 2, 4, 8, 12 and 24 hours
describe("the Demo clock", () => {
  it("has a button for each jump and tells the jump that was pressed", () => {
    const onJump = vi.fn();
    render(<ClockFace time="2026-09-14 09:41 UTC" onJump={onJump} />);
    expect(screen.getAllByRole("button").map((b) => b.textContent)).toStrictEqual(["+1 min", "+1 hour", "+2 hours", "+4 hours", "+8 hours", "+12 hours", "+24 hours"]);
    fireEvent.click(screen.getByRole("button", { name: "+24 hours" }));
    fireEvent.click(screen.getByRole("button", { name: "+1 min" }));
    expect(onJump.mock.calls).toStrictEqual([[1440], [1]]);
  });

  it("disables every button while an action runs", () => {
    render(<ClockFace time="2026-09-14 09:41 UTC" onJump={vi.fn()} disabled />);
    expect(screen.getAllByRole("button").every((b) => b instanceof HTMLButtonElement && b.disabled)).toBe(true);
  });
});
