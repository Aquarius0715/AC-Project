// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import { Check, Field, Input } from "@ac/web/components/ui";
import { focusField } from "@ac/web/lib/focusField";

afterEach(cleanup);
const frame = () => new Promise((r) => { requestAnimationFrame(() => { r(null); }); });

// IR586: a failed save takes focus to the field to fix, whether it is a control or a set of checkboxes.
describe("focusField", () => {
  it("focuses a control by its id", async () => {
    render(<Field label="Source" error="1–500 characters"><Input id="source" /></Field>);
    focusField("source");
    await frame();
    expect(document.activeElement).toBe(screen.getByRole("textbox", { name: /Source/ }));
  });
  it("focuses the first checkbox of a set", async () => {
    render(<div id="units"><Check label="Lobby AC" /><Check label="Bedroom AC" /></div>);
    focusField("units");
    await frame();
    expect(document.activeElement).toBe(screen.getAllByRole("checkbox")[0]);
  });
  it("does nothing for an unknown id", async () => {
    const spy = vi.spyOn(HTMLElement.prototype, "focus");
    focusField("missing");
    await frame();
    expect(spy).not.toHaveBeenCalled();
    spy.mockRestore();
  });
});
