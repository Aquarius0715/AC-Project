// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, fireEvent, render, screen, within } from "@testing-library/react";

// The Phase 1A demo's Report a problem on the finished plan visit (job-c02). Its own file: the demo remembers a rated or
// reported job for the module's life, so after maintenanceDemo.test.tsx rates job-c02 it offers neither again.
const toast = vi.hoisted(() => vi.fn<(text: string, tone?: string) => void>());
vi.mock("@ac/web/components/ui", async (importOriginal) => ({ ...(await importOriginal<Record<string, unknown>>()), useToast: () => toast }));

import { MaintenanceDemo } from "../app/customer/maintenance/_components/maintenance-demo";
import { jobActions } from "@ac/web/lib/jobs";

const dialog = () => screen.getByRole("dialog");

beforeEach(() => {
  toast.mockReset();
  localStorage.clear();
  jobActions.reset();
});
afterEach(() => { cleanup(); });

describe("MaintenanceDemo Report a problem", () => {
  // first: a refused report leaves job-c02 open to rate and report
  it("is refused in the dialog when the job is no longer completed", () => {
    render(<MaintenanceDemo jobId="job-c02" />);
    fireEvent.click(within(screen.getByText("Completed report").closest("div") ?? document.body).getAllByRole("button", { name: "Report a problem" }).at(-1) ?? document.body);
    act(() => { jobActions.cancel("job-c02", "elsewhere"); });
    fireEvent.change(within(dialog()).getByRole("textbox", { name: /Details/ }), { target: { value: "Water on the floor again" } });
    fireEvent.click(within(dialog()).getByRole("button", { name: "Send report" }));
    expect(within(dialog()).getByText("CONFLICT — only completed jobs can be reported")).toBeDefined();
    expect(toast).not.toHaveBeenCalled();
  });

  it("checks the details, opens the follow-up under HQ review and offers neither rating nor report again", () => {
    render(<MaintenanceDemo />);
    // the banner's Report a problem, then Cancel
    fireEvent.click(screen.getByRole("button", { name: "Report a problem" }));
    expect(within(dialog()).getByText("Report a problem — job-c02")).toBeDefined();
    fireEvent.click(within(dialog()).getByRole("button", { name: "Cancel" }));
    const card = screen.getByText("Completed report").closest("div") ?? document.body;
    fireEvent.click(within(card).getAllByRole("button", { name: "Report a problem" }).at(-1) ?? document.body);
    fireEvent.click(within(dialog()).getByRole("button", { name: "Send report" }));
    expect(within(dialog()).getByText("Describe the problem (10–2000 characters)")).toBeDefined();
    fireEvent.click(within(dialog()).getByRole("button", { name: "New damage" }));
    fireEvent.change(within(dialog()).getByRole("textbox", { name: /Details/ }), { target: { value: "Water on the floor again" } });
    fireEvent.change(within(dialog()).getByLabelText(/Preferred visit/), { target: { value: "2026-09-26" } });
    fireEvent.click(within(dialog()).getByRole("button", { name: "Send report" }));
    const [text, tone] = toast.mock.lastCall ?? [];
    expect([tone, text]).toStrictEqual(["warn", expect.stringMatching(/^Problem reported — follow-up job-\S+ is under HQ review$/)]);
    expect(screen.getAllByText("Under HQ review").length).toBeGreaterThan(0);
    expect(screen.queryByText(/Work finished — job-c02/)).toBeNull();
  });
});
