// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";

// The Phase 1A demo of the job history: the job choice, the event filter with its tones, and the preview form
const toast = vi.hoisted(() => vi.fn());
vi.mock("@ac/web/components/ui", async (importOriginal) => ({ ...(await importOriginal<Record<string, unknown>>()), useToast: () => toast }));

import { HistoryDemo } from "../app/partner/history/_components/history-demo";

beforeEach(() => { toast.mockReset(); });
afterEach(cleanup);

const titles = () => [...document.querySelectorAll("ol li .font-semibold")].map((e) => e.textContent);
const dotOf = (title: string) => screen.getByText(title).closest("li")?.children[1]?.className ?? "";
const tab = (name: RegExp) => { fireEvent.click(screen.getByRole("tab", { name })); };

describe("History demo", () => {
  it("filters the events by who sees them and colours rework and an accepted report", () => {
    render(<HistoryDemo />);
    expect(titles()).toHaveLength(9);
    expect(dotOf("Rework requested — missing evidence")).toContain("bg-warn");
    expect(dotOf("Report accepted")).toContain("bg-ok");
    expect(dotOf("Job accepted")).toContain("bg-primary");
    tab(/^Customer-visible/);
    expect(titles()).toStrictEqual(["Report accepted", "Work started (checked in)", "Schedule changed"]);
    tab(/^Internal/);
    expect(titles()).toStrictEqual(["Report v2 submitted", "Rework requested — missing evidence", "Report v1 submitted", "Technician assigned", "Job accepted", "Offer received from HQ"]);
  });

  it("asks for a job when none is chosen", () => {
    render(<HistoryDemo />);
    fireEvent.change(screen.getByRole("combobox", { name: "Job" }), { target: { value: "" } });
    expect(screen.getByText("Choose a job above to see its history.")).toBeTruthy();
    fireEvent.change(screen.getByRole("combobox", { name: "Job" }), { target: { value: "job-p04" } });
    expect(screen.getByText("Job history — job-p04")).toBeTruthy();
  });

  it("refuses an empty message, then adds the preview for the chosen role and visibility", () => {
    render(<HistoryDemo />);
    const save = screen.getByRole("button", { name: "Save note & create preview" });
    fireEvent.change(screen.getByRole("textbox"), { target: { value: "  " } });
    fireEvent.click(save);
    expect(screen.getByText("Message is required")).toBeTruthy();
    expect(toast).not.toHaveBeenCalled();
    expect(screen.getByText("Previews on job-p03 (2)")).toBeTruthy();
    fireEvent.change(screen.getByRole("textbox"), { target: { value: "On my way" } });
    fireEvent.change(screen.getByRole("combobox", { name: /^Recipient role/ }), { target: { value: "technician" } });
    fireEvent.click(screen.getByRole("button", { name: "Customer-visible" }));
    fireEvent.click(save);
    expect(toast).toHaveBeenCalledWith("Note saved + preview created");
    expect(screen.getByText("Previews on job-p03 (3)")).toBeTruthy();
    expect(screen.getAllByText(/^schedule_change → /).map((e) => e.textContent)).toStrictEqual(["schedule_change → technician", "schedule_change → customer"]);
    expect(screen.getByText("in-app · preview · customer · just now")).toBeTruthy();
    expect(screen.queryByText("Message is required")).toBeNull();
  });
});
