// @vitest-environment jsdom
import { useState } from "react";
import { afterEach, describe, expect, it } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { Modal } from "@ac/web/components/ui";

afterEach(cleanup);

function Harness({ fields = true, inner = false }: { fields?: boolean; inner?: boolean }) {
  const [open, setOpen] = useState(false);
  const [second, setSecond] = useState(false);
  return (
    <>
      <button onClick={() => setOpen(true)}>Open</button>
      <Modal open={open} title="Edit" onClose={() => setOpen(false)} footer={<><button>Cancel</button><button onClick={() => setSecond(true)}>Save</button></>}>
        {fields ? <input aria-label="Name" /> : <p>Delete this?</p>}
      </Modal>
      {inner && <Modal open={second} title="Inner" onClose={() => setSecond(false)} footer={<button>OK</button>}><p>Sure?</p></Modal>}
    </>
  );
}
const open = () => { const b = screen.getByRole("button", { name: "Open" }); b.focus(); fireEvent.click(b); return b; };
const key = (k: string, shift = false) => fireEvent.keyDown(document.activeElement ?? document.body, { key: k, shiftKey: shift });

// IR361: the WAI-ARIA dialog pattern (UX-06: focus returns to the trigger)
describe("the modal dialog's keyboard behaviour", () => {
  it("moves focus to the first field, cycles Tab inside and returns focus to the trigger on Escape", () => {
    render(<Harness />);
    const trigger = open();
    expect(document.activeElement).toBe(screen.getByRole("textbox", { name: "Name" }));
    screen.getByRole("button", { name: "Save" }).focus();
    key("Tab"); // past the last control: back to the first (✕)
    expect(document.activeElement).toBe(screen.getByRole("button", { name: "Close" }));
    key("Tab", true); // before the first: to the last
    expect(document.activeElement).toBe(screen.getByRole("button", { name: "Save" }));
    key("Escape");
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(document.activeElement).toBe(trigger);
  });
  it("focuses the first footer button (Cancel) when the body has no control", () => {
    render(<Harness fields={false} />);
    open();
    expect(document.activeElement).toBe(screen.getByRole("button", { name: "Cancel" }));
  });
  it("lets only the innermost of stacked dialogs answer Escape", () => {
    render(<Harness inner />);
    open();
    const save = screen.getByRole("button", { name: "Save" });
    save.focus(); // a keyboard user reaches Save, then presses it
    fireEvent.click(save);
    expect(screen.getAllByRole("dialog")).toHaveLength(2);
    expect(document.activeElement).toBe(screen.getByRole("button", { name: "OK" }));
    key("Escape");
    expect(screen.getAllByRole("dialog")).toHaveLength(1); // the outer one stays
    expect(document.activeElement).toBe(screen.getByRole("button", { name: "Save" })); // back to its trigger
  });
});
