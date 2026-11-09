// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { DeleteDialog, forceReady, noForce } from "@ac/web/components/DeleteDialog";

afterEach(cleanup);

const show = (force?: { name: string; what: string }) => {
  const onConfirm = vi.fn<(reason: string, force: boolean) => void>();
  render(<DeleteDialog title="Delete customer" text="Delete it?" pending={false} onClose={vi.fn()} onConfirm={onConfirm} force={force} />);
  return onConfirm;
};
const why = () => { fireEvent.change(screen.getByRole("textbox", { name: /Reason/ }), { target: { value: "  registered by mistake " } }); };
const del = () => screen.getByRole("button", { name: "Delete" });

// data.forceDelete: the option exists only for the holder, and it needs the name typed first
describe("DeleteDialog force option", () => {
  it("offers no force option unless the holder is given one, and confirms with force false", () => {
    const onConfirm = show();
    expect(screen.queryByRole("checkbox")).toBeNull();
    why();
    fireEvent.click(del());
    expect(onConfirm).toHaveBeenCalledWith("registered by mistake", false);
  });
  it("keeps Delete off until the name is typed exactly, then confirms with force true", () => {
    const onConfirm = show({ name: "Demo Customer B", what: "also deletes its accounts" });
    why();
    expect(del().hasAttribute("disabled")).toBe(false);
    fireEvent.click(screen.getByRole("checkbox"));
    expect(del().hasAttribute("disabled")).toBe(true);
    fireEvent.change(screen.getByRole("textbox", { name: /Demo Customer B/ }), { target: { value: "Demo Customer" } });
    expect(del().hasAttribute("disabled")).toBe(true);
    expect(screen.getByText("The name does not match")).toBeTruthy();
    fireEvent.change(screen.getByRole("textbox", { name: /Demo Customer B/ }), { target: { value: "Demo Customer B" } });
    fireEvent.click(del());
    expect(onConfirm).toHaveBeenCalledWith("registered by mistake", true);
  });
  it("needs the reason even when forced", () => {
    const onConfirm = show({ name: "X", what: "forces" });
    fireEvent.click(screen.getByRole("checkbox"));
    fireEvent.change(screen.getByRole("textbox", { name: /Type X/ }), { target: { value: "X" } });
    fireEvent.click(del());
    expect(onConfirm).not.toHaveBeenCalled();
    expect(screen.getByText("A reason is required")).toBeTruthy();
  });
  it("is ready when force is off or the name matches", () => {
    expect(forceReady(noForce, "A")).toBe(true);
    expect(forceReady({ on: true, typed: "" }, "A")).toBe(false);
    expect(forceReady({ on: true, typed: " A " }, "A")).toBe(true);
  });
});
