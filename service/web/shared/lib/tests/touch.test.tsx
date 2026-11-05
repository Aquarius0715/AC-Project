// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import { Btn, Choice, Input, Plain, Select, Tabs } from "@ac/web/components/ui";
import { Button } from "@ac/web/components/ui/button";

afterEach(cleanup);

// IR615: on a touch screen (pointer: coarse) the shared controls are at least 44 px tall and icon buttons 44×44; a
// mouse keeps the compact sizes, and Plain takes its parent's size. jsdom has no media queries, so this checks the
// classes; the smoke crawl's phone pass measures the controls under touch emulation.
describe("touch-sized controls", () => {
  const coarse = (el: HTMLElement) => el.className.split(/\s+/).filter((c) => c.startsWith("pointer-coarse:"));
  it("sizes every button, input, select, tab and choice to 44 px on a touch screen", () => {
    render(<>
      <Btn>Save</Btn>
      <Btn size="sm">Small</Btn>
      <Button size="icon-sm" aria-label="More" />
      <Input aria-label="Name" />
      <Select aria-label="Type"><option>A</option></Select>
      <Tabs value="a" onChange={vi.fn()} tabs={[{ id: "a", label: "One" }, { id: "b", label: "Two" }]} />
      <Choice value="x" onChange={vi.fn()} options={[{ id: "x", label: "Cool" }, { id: "y", label: "Dry" }]} />
    </>);
    for (const name of ["Save", "Small", "One", "Cool"]) expect(coarse(screen.getByRole(/One|Two/.test(name) ? "tab" : "button", { name })), name).toContain("pointer-coarse:min-h-11");
    expect(coarse(screen.getByRole("button", { name: "More" }))).toEqual(expect.arrayContaining(["pointer-coarse:min-h-11", "pointer-coarse:min-w-11"]));
    expect(coarse(screen.getByRole("textbox", { name: "Name" }))).toContain("pointer-coarse:min-h-11");
    expect(coarse(screen.getByRole("combobox", { name: "Type" }))).toContain("pointer-coarse:min-h-11");
  });
  it("leaves a plain button its parent's size", () => {
    render(<Plain>Open card</Plain>);
    const b = screen.getByRole("button", { name: "Open card" });
    expect(coarse(b)).toEqual(["pointer-coarse:min-h-0"]);
    expect(Object.hasOwn(b.dataset, "plain")).toBe(true);
  });
  it("sizes a Plain action marked touch to 44×44 (IR618)", () => {
    render(<Plain touch aria-label="Rename Home A">✎</Plain>);
    expect(coarse(screen.getByRole("button", { name: "Rename Home A" }))).toEqual(expect.arrayContaining(["pointer-coarse:min-h-11", "pointer-coarse:min-w-11", "pointer-coarse:justify-center"]));
  });
  it("sizes a chip or a tab built from Plain (IR617)", () => {
    render(<><Plain aria-pressed={false}>Offline</Plain><Plain role="tab" aria-selected>Today</Plain></>);
    for (const el of [screen.getByRole("button", { name: "Offline" }), screen.getByRole("tab", { name: "Today" })]) {
      expect(coarse(el)).toEqual(expect.arrayContaining(["pointer-coarse:min-h-11", "pointer-coarse:inline-flex", "pointer-coarse:items-center"]));
      expect(coarse(el)).not.toContain("pointer-coarse:min-h-0");
    }
  });
});
