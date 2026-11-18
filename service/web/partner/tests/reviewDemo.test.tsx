// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";

// The Phase 1A demo review: fixed report rows, accept, and return with a required reason; nothing is saved
const toast = vi.hoisted(() => vi.fn());
vi.mock("@ac/web/components/ui", async (importOriginal) => ({ ...(await importOriginal<Record<string, unknown>>()), useToast: () => toast }));

import { ReviewDemo } from "../app/partner/jobs/[id]/review/_components/review-demo";

beforeEach(() => { toast.mockReset(); });
afterEach(cleanup);

const click = (name: string) => { fireEvent.click(screen.getByRole("button", { name })); };
const decisions = () => screen.queryAllByRole("button", { name: /^(Accept report|Return for rework…)$/ });

describe("Review demo", () => {
  it("shows the fixed report with the not-applicable reason, and accepting completes it once", () => {
    render(<ReviewDemo id="job-p05" />);
    expect(screen.getByText(/job-p05 · Quality review$/)).toBeTruthy();
    expect(screen.getAllByText(/^Reason: /).map((e) => e.textContent)).toStrictEqual(["Reason: unit has no exposed wiring in this installation"]);
    expect(screen.getAllByText(/^Photo \d$/)).toHaveLength(3);
    click("Accept report");
    expect(screen.getByText(/^Accepted → completed \(receipt\)\. Report v1/)).toBeTruthy();
    expect(toast).toHaveBeenCalledWith("Report accepted");
    expect(decisions()).toStrictEqual([]);
  });

  it("asks for a reason before returning, Cancel or Escape keep the report pending, and a reason returns it", async () => {
    render(<ReviewDemo id="job-p05" />);
    click("Return for rework…");
    expect(screen.queryByText("A reason is required")).toBeNull();
    click("Return report");
    expect(screen.getByText("A reason is required")).toBeTruthy();
    click("Cancel");
    await waitFor(() => { expect(screen.queryByRole("button", { name: "Return report" })).toBeNull(); });
    click("Return for rework…");
    fireEvent.keyDown(screen.getByRole("textbox"), { key: "Escape" });
    await waitFor(() => { expect(screen.queryByRole("button", { name: "Return report" })).toBeNull(); });
    expect(decisions()).toHaveLength(2);
    expect(toast).not.toHaveBeenCalled();
    click("Return for rework…");
    fireEvent.change(screen.getByRole("textbox"), { target: { value: " ".repeat(3) } });
    click("Return report");
    expect(screen.getByText("A reason is required")).toBeTruthy();
    fireEvent.change(screen.getByRole("textbox"), { target: { value: "Add a filter photo" } });
    expect(screen.queryByText("A reason is required")).toBeNull();
    click("Return report");
    expect(screen.getByText("Returned for rework. tech-external-a can resume as v2.")).toBeTruthy();
    expect(toast).toHaveBeenCalledWith("Returned for rework", "warn");
    expect(decisions()).toStrictEqual([]);
  });
});
