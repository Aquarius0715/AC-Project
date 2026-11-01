// @vitest-environment jsdom
import { afterEach, describe, expect, it } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import { Choice, Field, Input } from "@ac/web/components/ui";

afterEach(cleanup);

// IR573: a field with an error marks its control invalid, and the error stays part of the control's name, so a screen
// reader names the field to fix; a group of choices is never marked (aria-invalid does not apply to buttons).
describe("a form field with an error", () => {
  it("marks its control invalid and names the error with it", () => {
    render(<Field label="Symptoms" error="Symptoms must be 10–2000 characters"><Input /></Field>);
    const box = screen.getByRole("textbox", { name: /Symptoms/ });
    expect(box.getAttribute("aria-invalid")).toBe("true");
    expect(box).toHaveProperty("labels");
    expect((box as HTMLInputElement).labels?.[0]?.textContent).toContain("Symptoms must be 10–2000 characters");
  });
  it("leaves a valid control and a group of choices unmarked", () => {
    render(<><Field label="Contact window" hint="Use HH:mm"><Input /></Field><Field label="Type" error="Pick one"><Choice value="a" onChange={() => {}} options={[{ id: "a", label: "Repair" }, { id: "b", label: "Preventive" }]} /></Field></>);
    expect(screen.getByRole("textbox", { name: /Contact window/ }).getAttribute("aria-invalid")).toBeNull();
    for (const b of screen.getAllByRole("button")) expect(b.getAttribute("aria-invalid")).toBeNull();
  });
});
