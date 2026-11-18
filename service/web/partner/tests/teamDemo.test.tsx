// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";

// The Phase 1A demo of Team & capacity: the search by qualification or technician, Active only, the Certifications
// tab and the two demo dialogs with their toasts
const toast = vi.hoisted(() => vi.fn());
vi.mock("@ac/web/components/ui", async (importOriginal) => ({ ...(await importOriginal<Record<string, unknown>>()), useToast: () => toast }));

import { MockTeam } from "../app/partner/team/_components/mock-team";

const cards = () => screen.queryAllByText(/^tech-external-/, { selector: "h2, h3, [data-slot='card-title']" }).map((x) => x.textContent);
const search = (value: string) => { fireEvent.change(screen.getByPlaceholderText("Filter by qualification or technician"), { target: { value } }); };

beforeEach(() => {
  toast.mockReset();
  history.replaceState(null, "", "/partner/team");
  vi.stubGlobal("ResizeObserver", class { observe = vi.fn(); unobserve = vi.fn(); disconnect = vi.fn(); }); // jsdom has none
});
afterEach(() => { cleanup(); vi.unstubAllGlobals(); });

describe("Team demo", () => {
  it("finds a technician by qualification or by ID in any letter case, and says when nobody matches", () => {
    render(<MockTeam />);
    expect(cards()).toStrictEqual(["tech-external-a", "tech-external-a2"]);
    search("GENERAL");
    expect(cards()).toStrictEqual(["tech-external-a2"]);
    search("Tech-External-A2");
    expect(cards()).toStrictEqual(["tech-external-a2"]);
    search("gas");
    expect(cards()).toStrictEqual([]);
    expect(screen.getByText("No technician matches this qualification.")).toBeTruthy();
  });

  it("shows the expired member without Active only, with no utilization", () => {
    render(<MockTeam />);
    expect(screen.getByText(/1 expired member hidden \(Active only\)\./)).toBeTruthy();
    fireEvent.click(screen.getByRole("checkbox", { name: "Active only" }));
    expect(cards()).toStrictEqual(["tech-external-a", "tech-external-a2", "tech-external-old"]);
    expect(screen.queryByText(/expired member hidden/)).toBeNull();
    expect(screen.getAllByText("—").length).toBeGreaterThan(0);
  });

  it("saves the demo unavailable days and uploads a renewal from the Certifications tab", () => {
    render(<MockTeam />);
    fireEvent.click(screen.getByRole("button", { name: "Unavailable days" }));
    expect(screen.getByText(/Conflicts with 1 confirmed assignment \(job-c07/)).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    expect(toast).toHaveBeenCalledWith("Unavailable days saved — available hours set to 0");
    fireEvent.click(screen.getByRole("tab", { name: /Certifications/ }));
    expect(screen.getAllByText(/^(Valid|Expiring · 14 d|Expired)$/, { selector: "[data-slot='badge']" }).map((x) => x.textContent)).toStrictEqual(["Valid", "Expiring · 14 d", "Valid", "Expired"]);
    expect(screen.getAllByRole("button", { name: "Upload renewal" })).toHaveLength(1); // only the expiring row
    fireEvent.click(screen.getByRole("button", { name: "Upload renewal" }));
    fireEvent.click(screen.getByRole("button", { name: "Upload" }));
    expect(toast).toHaveBeenLastCalledWith("Renewal uploaded — pending HQ verification");
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("closes both dialogs with Cancel or Escape without a toast", () => {
    render(<MockTeam />);
    fireEvent.click(screen.getByRole("button", { name: "Unavailable days" }));
    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    expect(screen.queryByRole("dialog")).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Unavailable days" }));
    fireEvent.keyDown(screen.getByRole("dialog"), { key: "Escape" });
    expect(screen.queryByRole("dialog")).toBeNull();
    fireEvent.click(screen.getByRole("tab", { name: /Certifications/ }));
    fireEvent.click(screen.getByRole("button", { name: "Upload renewal" }));
    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    expect(screen.queryByRole("dialog")).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Upload renewal" }));
    fireEvent.keyDown(screen.getByRole("dialog"), { key: "Escape" });
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(toast).not.toHaveBeenCalled();
  });
});
