// @vitest-environment jsdom
import { useState } from "react";
import { afterEach, describe, expect, it } from "vitest";
import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { Btn } from "@ac/web/components/ui";

afterEach(cleanup);

// IR622: the button a user pressed is disabled while its action runs. It keeps the focus (aria-disabled, not the
// disabled attribute, which drops the focus to the page) and ignores further presses; a button disabled without the
// focus stays natively disabled, out of the Tab order.
function Pending() {
  const [pending, setPending] = useState(false);
  const [clicks, setClicks] = useState(0);
  return (
    <>
      <Btn disabled={pending} onClick={() => { setPending(true); setClicks((n) => n + 1); }}>Save</Btn>
      <Btn disabled>Idle</Btn>
      <Btn>Next</Btn>
      <p>Clicks {clicks}</p>
    </>
  );
}

describe("a button disabled while it has the focus", () => {
  it("keeps the focus, marked aria-disabled, and ignores further presses", () => {
    render(<Pending />);
    const save = screen.getByRole("button", { name: "Save" });
    act(() => save.focus());
    fireEvent.click(save);
    expect(document.activeElement).toBe(save);
    expect([save.getAttribute("aria-disabled"), save.hasAttribute("disabled")]).toEqual(["true", false]);
    fireEvent.click(save);
    fireEvent.keyDown(save, { key: "Enter" });
    expect(screen.getByText("Clicks 1")).toBeTruthy();
  });
  it("is natively disabled again once the focus moves on, and so is a button disabled without it", () => {
    render(<Pending />);
    const save = screen.getByRole("button", { name: "Save" });
    act(() => save.focus());
    fireEvent.click(save);
    act(() => screen.getByRole("button", { name: "Next" }).focus());
    expect(save.hasAttribute("disabled")).toBe(true);
    expect(screen.getByRole("button", { name: "Idle" }).hasAttribute("disabled")).toBe(true);
  });
});
