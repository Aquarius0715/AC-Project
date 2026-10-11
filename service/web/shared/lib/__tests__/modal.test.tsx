// @vitest-environment jsdom
import { useState } from "react";
import { afterEach, describe, expect, it } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
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
const escape = () => fireEvent.keyDown(document.activeElement ?? document.body, { key: "Escape" });

// IR361 on the shadcn/ui Dialog (DEC-75, IR401): where focus goes on open and on close, and which dialog answers
// Escape. Tab cycling inside the dialog is a browser behaviour (Base UI's focus guards hand focus back on the next frame);
// the keyboard E2E covers it.
describe("the modal dialog's keyboard behaviour", () => {
  it("moves focus to the first field and returns it to the trigger on Escape", async () => {
    render(<Harness />);
    const trigger = open();
    await waitFor(() => expect(document.activeElement).toBe(screen.getByRole("textbox", { name: "Name" })));
    expect(screen.getByRole("dialog", { name: "Edit" })).toBeTruthy();
    escape();
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    await waitFor(() => expect(document.activeElement).toBe(trigger));
  });
  it("focuses the first footer button (Cancel) when the body has no control", async () => {
    render(<Harness fields={false} />);
    open();
    await waitFor(() => expect(document.activeElement).toBe(screen.getByRole("button", { name: "Cancel" })));
  });
  it("lets only the innermost of stacked dialogs answer Escape", async () => {
    render(<Harness inner />);
    open();
    const save = await screen.findByRole("button", { name: "Save" });
    save.focus(); // a keyboard user reaches Save, then presses it
    fireEvent.click(save);
    // both are open; behind the inner one the outer is hidden from assistive technology, as a modal dialog asks
    await waitFor(() => expect(screen.getAllByRole("dialog", { hidden: true })).toHaveLength(2));
    expect(screen.getByRole("dialog").getAttribute("aria-labelledby")).toBeTruthy();
    expect(screen.getByRole("dialog", { name: "Inner" })).toBeTruthy();
    await waitFor(() => expect(document.activeElement).toBe(screen.getByRole("button", { name: "OK" })));
    escape();
    await waitFor(() => expect(screen.getAllByRole("dialog", { hidden: true })).toHaveLength(1)); // the outer one stays
    expect(screen.getByRole("dialog", { name: "Edit" })).toBeTruthy();
    await waitFor(() => expect(document.activeElement).toBe(screen.getByRole("button", { name: "Save" }))); // back to its trigger
  });
});
