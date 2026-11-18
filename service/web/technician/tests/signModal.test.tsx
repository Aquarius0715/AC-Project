// @vitest-environment jsdom
import { useState } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";

vi.mock("@ac/web/components/I18n", () => ({ useT: () => (s: string) => s }));

import { SignModal } from "../app/technician/jobs/[id]/_components/sign-modal";

// the customer sign-off dialog: a signature drawn on an earlier pad must not let an empty pad be saved (R1030), and the
// "customer not available" path needs a name, a reason and a JPEG/PNG site photo
const onSign = vi.fn<(form: FormData) => void>();
const pen = { beginPath: vi.fn(), moveTo: vi.fn(), lineTo: vi.fn(), stroke: vi.fn(), clearRect: vi.fn(), lineWidth: 0, lineCap: "", strokeStyle: "" };

function Harness() {
  const [open, setOpen] = useState(false);
  return (
    <>
      <button type="button" onClick={() => { setOpen(true); }}>Open</button>
      <SignModal open={open} pending={false} summary="draft v2" onClose={() => { setOpen(false); }} onSign={onSign} />
    </>
  );
}

const openDialog = async () => { fireEvent.click(screen.getByRole("button", { name: "Open" })); return screen.findByRole("dialog"); };
const pad = () => screen.getByLabelText("Signature pad");
const draw = () => {
  fireEvent.pointerDown(pad(), { clientX: 10, clientY: 10, pointerId: 1 });
  fireEvent.pointerMove(pad(), { clientX: 40, clientY: 30, pointerId: 1 });
  fireEvent.pointerUp(pad(), { pointerId: 1 });
};
const typeName = (name: string) => { fireEvent.change(screen.getByPlaceholderText("Ms Tan"), { target: { value: name } }); };
const save = () => { fireEvent.click(screen.getByRole("button", { name: "Save sign-off" })); };
const choose = (label: string) => { fireEvent.click(screen.getByRole("button", { name: label })); };
// jsdom has no 2D canvas: the pad draws on a recording pen and exports a fixed PNG
const stub = (name: "getContext" | "toDataURL" | "setPointerCapture", value: () => unknown) => {
  Object.defineProperty(HTMLCanvasElement.prototype, name, { value, configurable: true, writable: true });
};
beforeEach(() => {
  stub("getContext", () => pen);
  stub("toDataURL", () => "data:image/png;base64,c2lnbmVk");
  stub("setPointerCapture", vi.fn());
});
afterEach(() => {
  cleanup();
  onSign.mockReset();
  vi.clearAllMocks();
});

describe("SignModal", () => {
  it("sends the drawn signature and the trimmed signer name", async () => {
    render(<Harness />);
    await openDialog();
    typeName("  Ms Tan ");
    draw();
    save();
    expect(onSign).toHaveBeenCalledOnce();
    const form = onSign.mock.calls[0]?.[0];
    expect(form?.get("signerName")).toBe("Ms Tan");
    expect(form?.get("signature")).toBe("c2lnbmVk");
  });

  it("does not save an empty pad after the dialog was cancelled with a drawn signature", async () => {
    render(<Harness />);
    await openDialog();
    typeName("Ms Tan");
    draw();
    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    await waitFor(() => { expect(screen.queryByRole("dialog")).toBeNull(); });
    await openDialog();
    save();
    expect(onSign).not.toHaveBeenCalled();
    expect(screen.getByText("Ask the customer to sign in the box")).toBeTruthy();
  });

  it("does not save an empty pad after switching to “Customer not available” and back", async () => {
    render(<Harness />);
    await openDialog();
    typeName("Ms Tan");
    draw();
    choose("Customer not available");
    choose("Signature");
    save();
    expect(onSign).not.toHaveBeenCalled();
    expect(screen.getByText("Ask the customer to sign in the box")).toBeTruthy();
  });

  it("clears the pad and asks for a signature again", async () => {
    render(<Harness />);
    await openDialog();
    typeName("Ms Tan");
    const clear = screen.getByRole("button", { name: "Clear" });
    expect(clear.hasAttribute("disabled")).toBe(true);
    draw();
    expect(clear.hasAttribute("disabled")).toBe(false);
    fireEvent.click(clear);
    expect(pen.clearRect).toHaveBeenCalledWith(0, 0, 560, 160);
    save();
    expect(onSign).not.toHaveBeenCalled();
  });

  it("ignores a pointer move without a pressed pointer", async () => {
    render(<Harness />);
    await openDialog();
    typeName("Ms Tan");
    fireEvent.pointerMove(pad(), { clientX: 40, clientY: 30, pointerId: 1 });
    save();
    expect(onSign).not.toHaveBeenCalled();
  });

  it("counts nothing as drawn when the browser gives the pad no 2D context", async () => {
    stub("getContext", () => null);
    render(<Harness />);
    await openDialog();
    typeName("Ms Tan");
    draw();
    expect(screen.getByRole("button", { name: "Clear" }).hasAttribute("disabled")).toBe(true);
    save();
    expect(onSign).not.toHaveBeenCalled();
    expect(screen.getByText("Ask the customer to sign in the box")).toBeTruthy();
  });

  it("forgets a site photo whose file choice was cleared", async () => {
    render(<Harness />);
    await openDialog();
    typeName("Mr Lim");
    choose("Customer not available");
    fireEvent.change(screen.getByRole("textbox", { name: "Why the customer could not sign" }), { target: { value: "Away" } });
    const input = screen.getByLabelText("Site photo (JPEG/PNG)");
    fireEvent.change(input, { target: { files: [new File(["x"], "door.png", { type: "image/png" })] } });
    fireEvent.change(input, { target: { files: [] } });
    save();
    expect(onSign).not.toHaveBeenCalled();
    expect(screen.getByText("A site photo (JPEG/PNG, ≤5 MiB) is required when the customer is not available")).toBeTruthy();
  });

  it("needs the signer's name first", async () => {
    render(<Harness />);
    await openDialog();
    typeName(" ".repeat(3));
    draw();
    save();
    expect(onSign).not.toHaveBeenCalled();
    expect(screen.getByText("The signer's name is required")).toBeTruthy();
  });

  it("needs a reason and a JPEG/PNG site photo up to 5 MiB when the customer is not available", async () => {
    render(<Harness />);
    await openDialog();
    typeName("Mr Lim");
    choose("Customer not available");
    save();
    expect(screen.getByText("Say why the customer could not sign")).toBeTruthy();
    fireEvent.change(screen.getByRole("textbox", { name: "Why the customer could not sign" }), { target: { value: " Away on business " } });
    save();
    const photoError = "A site photo (JPEG/PNG, ≤5 MiB) is required when the customer is not available";
    expect(screen.getByText(photoError)).toBeTruthy();
    const input = screen.getByLabelText("Site photo (JPEG/PNG)");
    fireEvent.change(input, { target: { files: [new File(["x"], "door.gif", { type: "image/gif" })] } });
    save();
    expect(screen.getByText(photoError)).toBeTruthy();
    const big = new File(["x"], "door.jpg", { type: "image/jpeg" });
    Object.defineProperty(big, "size", { value: (5 << 20) + 1 });
    fireEvent.change(input, { target: { files: [big] } });
    save();
    expect(onSign).not.toHaveBeenCalled();
    const photo = new File(["x"], "door.png", { type: "image/png" });
    fireEvent.change(input, { target: { files: [photo] } });
    save();
    expect(onSign).toHaveBeenCalledOnce();
    const form = onSign.mock.calls[0]?.[0];
    expect(form?.get("signerName")).toBe("Mr Lim");
    expect(form?.get("absentReason")).toBe("Away on business");
    expect(form?.get("sitePhoto")).toBeInstanceOf(File);
    expect(form?.has("signature")).toBe(false);
    expect(screen.queryByText(photoError)).toBeNull();
  });
});
