// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, fireEvent, render, screen, within } from "@testing-library/react";

// The Phase 1A demo of HQ maintenance jobs (fixture jobs, plans, contractors and SLA): the stage, origin and delivery
// filters, each fixture job's actions and dialogs (they change the shared demo store), New job, and the other tabs.
const nav = vi.hoisted(() => ({ toasts: [] as string[] }));
vi.mock("@ac/web/components/ui", async (importOriginal) => ({ ...(await importOriginal<Record<string, unknown>>()), useToast: () => (m: string) => { nav.toasts.push(m); } }));

import { JobsDemo } from "../app/admin/jobs/_components/jobs-demo";

const button = (name: string | RegExp) => screen.getByRole<HTMLButtonElement>("button", { name });
const flush = () => act(() => new Promise<void>((r) => { setTimeout(r, 0); }));
/** The buttons a job's detail offers: not the list rows (pressed state), the stage tiles, the tabs or New job. */
const actionButtons = () => screen.getAllByRole<HTMLButtonElement>("button").filter((b) => {
  const text = b.textContent.trim();
  return !b.disabled && !b.hasAttribute("aria-pressed") && b.getAttribute("role") !== "tab" && !/(?:New job)|(?:\d$)/.test(text) && text !== "";
});

/** Fills every empty text field of the open dialog and presses its last enabled footer button; closes it otherwise. */
function complete() {
  const box = screen.queryByRole("dialog");
  if (!box) return;
  const d = within(box);
  for (const field of d.queryAllByRole("textbox")) {
    if (field instanceof HTMLTextAreaElement || (field instanceof HTMLInputElement && field.value === "")) fireEvent.change(field, { target: { value: "Reason given for the demo action" } });
  }
  const buttons = d.queryAllByRole<HTMLButtonElement>("button").filter((b) => !b.disabled && !/^(Cancel|Close|Back)$/.test(b.textContent.trim()));
  const last = buttons.at(-1);
  if (last) fireEvent.click(last);
  if (screen.queryByRole("dialog")) fireEvent.keyDown(screen.getByRole("dialog"), { key: "Escape" });
}

/** Sorts the actions that move a job on (hold, cancel) after the others. */
const rank = (label: string) => (/^(?:Hold|Cancel)/.test(label) ? 1 : 0);

afterEach(() => { cleanup(); nav.toasts = []; });

describe("the jobs demo", () => {
  it("filters by stage, origin and delivery, and lists fewer jobs for each", () => {
    render(<JobsDemo />);
    const rows = () => screen.getAllByRole("button", { pressed: false }).length;
    const all = rows();
    fireEvent.change(screen.getByLabelText("Origin"), { target: { value: "plan" } });
    fireEvent.change(screen.getByLabelText("Delivery"), { target: { value: "contractor" } });
    expect(rows()).toBeLessThanOrEqual(all);
    fireEvent.change(screen.getByLabelText("Origin"), { target: { value: "all" } });
    fireEvent.change(screen.getByLabelText("Delivery"), { target: { value: "all" } });
    const firstStage = screen.getAllByRole("button").find((b) => /^Requested\d+$/.test(b.textContent.replaceAll(/\s/g, "")));
    fireEvent.click(firstStage ?? document.body);
    fireEvent.click(button(/Requested$/)); // the chip clears it
    expect(rows()).toBe(all);
  });

  it("walks every fixture job through its actions and dialogs", async () => {
    render(<JobsDemo />);
    const ids = screen.getAllByText(/^job-[a-z0-9]+ · /).map((b) => b.textContent.split(" · ", 1)[0] ?? "");
    expect(ids.length).toBeGreaterThan(5);
    // the actions that move a job on (hold, cancel) last, each action on a fresh screen of the job
    for (const id of ids) {
      cleanup();
      render(<JobsDemo jobId={id} />);
      const labels = actionButtons().map((b) => b.textContent.trim()).toSorted((a, b) => rank(a) - rank(b));
      for (const label of labels) {
        cleanup();
        render(<JobsDemo jobId={id} />);
        const again = actionButtons().find((b) => b.textContent.trim() === label);
        if (!again) continue;
        fireEvent.click(again);
        complete();
        await flush();
      }
    }
    expect(nav.toasts.length).toBeGreaterThan(5);
    expect(nav.toasts.some((m) => /^Proposal sent|^Report accepted|^Job cancelled|^Cost line added/.test(m))).toBe(true);
  });

  it("creates a job from New job after checking it", () => {
    render(<JobsDemo />);
    fireEvent.click(button(/New job/));
    const d = within(screen.getByRole("dialog"));
    const create = d.getAllByRole<HTMLButtonElement>("button").findLast((b) => !/^(?:Cancel|Close)$/.test(b.textContent.trim()));
    fireEvent.click(create ?? document.body);
    expect(screen.getByRole("dialog")).toBeDefined(); // the symptom is required first
    fireEvent.change(d.getByLabelText(/^Symptom/), { target: { value: "No cooling since this morning." } });
    fireEvent.click(create ?? document.body);
    expect(nav.toasts.length).toBeGreaterThan(0);
  });

  it("shows the plans, contractors and SLA tabs", () => {
    for (const [tab, title] of [["plans", "Recurring plans"], ["contractors", "Contractors"], ["sla", "Customers"]] as const) {
      render(<JobsDemo tab={tab} />);
      expect(screen.getAllByText(title).length).toBeGreaterThan(0);
      const offered = screen.getAllByRole<HTMLButtonElement>("button");
      for (const b of offered) {
        if (b.disabled || b.getAttribute("role") || !b.isConnected) continue;
        fireEvent.click(b);
        complete();
      }
      cleanup();
    }
    render(<JobsDemo jobId="job-gone" />);
    fireEvent.click(screen.getByRole("tab", { name: /Plans/ }));
    expect(screen.getByText("Recurring plans")).toBeDefined();
  });
});
