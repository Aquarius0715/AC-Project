// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";

// the Phase 1A demo of the payout statements (FR-P10): switching the statement, and a question to HQ on the deduction
// line that needs a text before it is sent
const toast = vi.hoisted(() => vi.fn());
vi.mock("@ac/web/components/ui", async (importOriginal) => ({ ...(await importOriginal<Record<string, unknown>>()), useToast: () => toast }));

import { PayoutsDemo } from "../app/partner/payouts/_components/payouts-demo";

afterEach(() => { cleanup(); toast.mockReset(); });

const send = () => { fireEvent.click(screen.getByRole("button", { name: "Send" })); };

describe("Payouts demo", () => {
  it("opens the newest statement and switches to a paid one", () => {
    render(<PayoutsDemo />);
    expect(screen.getByText("September 2026 · stmt-2026-09")).toBeTruthy();
    expect(screen.getByText("1,990.00 MYR")).toBeTruthy();
    fireEvent.click(screen.getByText("August 2026"));
    expect(screen.getByText("August 2026 · stmt-2026-08")).toBeTruthy();
    expect(screen.getByText("Rate card rc-contractor-a v3 · Paid 09-15")).toBeTruthy();
    expect(screen.getByText("1,760.00 MYR")).toBeTruthy();
    // the card's own badge says Paid, beside the two paid rows of the list
    expect(screen.getAllByText("Paid")).toHaveLength(3);
  });

  it("offers Ask HQ on the deduction line only, refuses an empty question, then sends and closes", () => {
    render(<PayoutsDemo />);
    expect(screen.getAllByRole("button", { name: "Ask HQ" })).toHaveLength(1);
    fireEvent.click(screen.getByRole("button", { name: "Ask HQ" }));
    const dialog = screen.getByRole("dialog");
    expect(within(dialog).getByRole("combobox", { name: "Line" })).toHaveProperty("disabled", true);
    expect(within(dialog).queryByText("Enter your question")).toBeNull();
    fireEvent.change(within(dialog).getByRole("textbox"), { target: { value: " ".repeat(3) } });
    send();
    expect(screen.getByText("Enter your question")).toBeTruthy();
    expect(toast).not.toHaveBeenCalled();
    fireEvent.change(within(dialog).getByRole("textbox"), { target: { value: "The part came from HQ stock" } });
    expect(screen.queryByText("Enter your question")).toBeNull();
    send();
    expect(toast).toHaveBeenCalledExactlyOnceWith("Question sent — HQ answers on the statement");
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("closes the question dialog on Cancel and on Escape without sending", async () => {
    render(<PayoutsDemo />);
    fireEvent.click(screen.getByRole("button", { name: "Ask HQ" }));
    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    expect(screen.queryByRole("dialog")).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Ask HQ" }));
    fireEvent.keyDown(document.activeElement ?? document.body, { key: "Escape" });
    await vi.waitFor(() => { expect(screen.queryByRole("dialog")).toBeNull(); });
    expect(toast).not.toHaveBeenCalled();
  });
});
