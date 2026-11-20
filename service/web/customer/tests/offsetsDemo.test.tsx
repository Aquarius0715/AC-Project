// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";

// The Phase 1A demo of carbon offsets (FR-C13): a positive amount only, the quote, the confirmation that needs the
// acknowledgement, the recorded demo request and Start another.
const toast = vi.hoisted(() => vi.fn());
vi.mock("@ac/web/components/ui", async (importOriginal) => ({ ...(await importOriginal<Record<string, unknown>>()), useToast: () => toast }));

import { OffsetsDemo } from "../app/customer/energy/offsets/_components/offsets-demo";

const button = (name: string) => screen.getByRole("button", { name });

beforeEach(() => { toast.mockReset(); });
afterEach(() => { cleanup(); });

describe("OffsetsDemo", () => {
  it("needs a positive amount, quotes it, and records the acknowledged request", () => {
    render(<OffsetsDemo />);
    const what = button("What is a carbon offset?");
    fireEvent.click(what);
    expect(screen.getByText(/An offset retires a carbon credit/)).toBeDefined();
    fireEvent.click(what);
    expect(screen.queryByText(/An offset retires a carbon credit/)).toBeNull();
    const amount = screen.getByRole("spinbutton");
    for (const bad of ["0", "-2", ""]) {
      fireEvent.change(amount, { target: { value: bad } });
      expect(button("Get demo quote").hasAttribute("disabled")).toBe(true);
    }
    fireEvent.change(amount, { target: { value: "2.5" } });
    fireEvent.click(button("Get demo quote"));
    expect(screen.getByText("2.5 kgCO2e")).toBeDefined();
    fireEvent.click(button("Back"));
    fireEvent.click(button("Get demo quote"));
    fireEvent.click(button("Continue"));
    expect(button("Confirm demo request").hasAttribute("disabled")).toBe(true);
    fireEvent.click(button("Back"));
    fireEvent.click(button("Continue"));
    fireEvent.click(screen.getByRole("checkbox"));
    fireEvent.click(button("Confirm demo request"));
    expect(toast).toHaveBeenCalledWith("Demo request recorded");
    expect(screen.getByText(/offset-0232/)).toBeDefined();
    fireEvent.click(button("Start another"));
    expect(screen.getByText("How much would you like to offset?")).toBeDefined();
    fireEvent.click(button("Get demo quote"));
    fireEvent.click(button("Continue"));
    expect(button("Confirm demo request").hasAttribute("disabled")).toBe(true);
  });
});
