// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import type { ActionFailure } from "@ac/web/lib/actionMessage";

// The contractor's quality review (FR-P05): the empty states, the submitted report, the evidence check, accept and
// return with the ticked items and a reason, the refusals, and the decided states
const actions = vi.hoisted(() => ({ reviewReport: vi.fn() }));
vi.mock("../app/partner/jobs/[id]/review/actions", () => actions);
const toasts = vi.hoisted((): { texts: string[] } => ({ texts: [] }));
type Result = { ok: true; value: { status: string } } | ({ ok: false } & ActionFailure);
vi.mock("@ac/web/lib/useAction", () => ({
  useAction: () => [false, (fn: () => Promise<Result>, ok: (v: { status: string }) => string, after?: () => void, failed?: (f: ActionFailure) => void) => {
    void fn().then((r) => {
      if (r.ok) { toasts.texts.push(ok(r.value)); after?.(); } else failed?.(r);
    });
  }] as const,
}));

import { ReviewView, type ReviewLive } from "../app/partner/jobs/[id]/review/_components/review-view";

beforeEach(() => { actions.reviewReport.mockReset(); toasts.texts = []; });
afterEach(cleanup);

type Report = NonNullable<ReviewLive["report"]>;
const report = (over: Partial<Report> = {}): Report => ({
  id: "r1", version: 2, author: "Tan", submitted: "18 Nov 16:40",
  items: [
    { id: "i1", label: "Filter", result: "normal", reason: null, photos: ["filter.jpg"] },
    { id: "i2", label: "Drain", result: null, reason: "blocked access", photos: [] },
  ],
  readings: [["Supply air", "13.9 °C"]], parts: [["Filter", "1 × stock"]], signOff: "Ms Lee", workText: "Cleaned the coil.", next: "Inspect in 6 months",
  photos: [{ id: "p1", name: "filter.jpg", url: "data:image/png;base64,AA==" }], timeOnSite: "2 h 35 m",
  evidence: [{ label: "Photos", value: "1", tone: "ok" }, { label: "Readings", value: "0", tone: "warn" }], acceptable: true, missing: [], allowed: true, blocked: null,
  versions: [{ title: "v2 · submitted", sub: "by Tan", badge: { label: "Reviewing", tone: "primary" } }, { title: "Due", sub: "09-23", badge: null }],
  last: null, rework: [{ label: "Drain — no result", checked: true }, { label: "Photos — add more evidence", checked: false }],
  ...over,
});
const live = (over: Partial<ReviewLive["job"]> = {}, r: Report | null = report()): ReviewLive => ({
  job: { id: "job-p05-aaaa-bbbb", version: 4, status: "submitted", type: "maintenance", unit: "Server room AC", ...over }, reviewer: "u-me", report: r,
});
const failure = (code: string, fieldErrors: Record<string, string> = {}): Result => ({ ok: false, messageKey: `error.${code}`, code, fieldErrors });
const click = (name: string | RegExp) => { fireEvent.click(screen.getByRole("button", { name })); };
const reasonBox = () => {
  const box = document.getElementById("return-reason");
  if (!(box instanceof HTMLTextAreaElement)) throw new TypeError("no reason box");
  return box;
};

describe("Review view without a report", () => {
  it("says a submitted report is outside the delegation, and that nothing is submitted for other statuses", () => {
    const view = render(<ReviewView live={live({}, null)} />);
    expect(screen.getByText("Report not readable")).toBeTruthy();
    expect(screen.getByText("The submitted report is outside your company's delegation window.")).toBeTruthy();
    expect(screen.getByText(/job-p05- · Quality review · Server room AC$/)).toBeTruthy();
    view.rerender(<ReviewView live={live({ status: "assigned", unit: null }, null)} />);
    expect(screen.getByText("Nothing submitted yet")).toBeTruthy();
    expect(screen.getByText(/^This job is assigned; a review opens once the technician submits the report\.$/i)).toBeTruthy();
    expect(screen.getByText(/job-p05- · Quality review$/)).toBeTruthy();
  });
});

describe("Review view with a submitted report", () => {
  it("shows the items, photos, parts, readings and evidence, and accepts the current version", async () => {
    actions.reviewReport.mockResolvedValue({ ok: true, value: { status: "completed" } });
    render(<ReviewView live={live()} />);
    expect(screen.getByText("Submitted report — version 2")).toBeTruthy();
    expect(screen.getByText("Tan · submitted 18 Nov 16:40")).toBeTruthy();
    expect(screen.getByText("No result")).toBeTruthy();
    expect(screen.getByText("Reason: blocked access")).toBeTruthy();
    expect(screen.getByText("Linked photos: filter.jpg")).toBeTruthy();
    expect(screen.getByRole("img", { name: "filter.jpg" }).getAttribute("src")).toBe("data:image/png;base64,AA==");
    expect(screen.getByText("Parts & refrigerant")).toBeTruthy();
    expect(screen.getByText("Ms Lee")).toBeTruthy();
    expect(screen.getByText("2 h 35 m")).toBeTruthy();
    expect(screen.getByText("Readings").querySelector("span")?.className).toContain("bg-warn");
    expect(screen.getByText("All required evidence present — acceptable.").className).toContain("bg-ok-soft");
    expect(screen.getByText("Reviewing")).toBeTruthy();
    click("Accept report");
    await waitFor(() => { expect(toasts.texts).toStrictEqual(["Report accepted — job completed"]); });
    expect(actions.reviewReport).toHaveBeenCalledWith("job-p05-aaaa-bbbb", 4, 2, "accept", null);
  });

  it("holds Accept back with the missing evidence, and says when there are no photos, parts or sign-off", () => {
    render(<ReviewView live={live({}, report({ acceptable: false, missing: ["Readings", "Photos"], photos: [], parts: [], signOff: null, submitted: null }))} />);
    const accept = screen.getByRole("button", { name: "Accept report" });
    expect(accept.hasAttribute("disabled")).toBe(true);
    expect(accept.getAttribute("title")).toBe("Readings; Photos");
    expect(screen.getByText("Not acceptable yet: Readings; Photos.").className).toContain("bg-warn-soft");
    expect(screen.getByText("No photos attached.")).toBeTruthy();
    expect(screen.queryByText("Parts & refrigerant")).toBeNull();
    expect(screen.getByText("not signed")).toBeTruthy();
    expect(screen.getByText("Tan")).toBeTruthy(); // no submitted time: the author alone
  });

  it("refuses an empty reason, then returns with the ticked items leading the reason and closes the dialog", async () => {
    actions.reviewReport.mockResolvedValue({ ok: true, value: { status: "rework_requested" } });
    render(<ReviewView live={live()} />);
    click("Return for rework…");
    const [drain, photos, ...rest] = screen.getAllByRole("checkbox");
    if (!drain || !photos) throw new Error("the dialog lists two rework items");
    expect([drain, photos, ...rest].map((b) => b.getAttribute("aria-checked"))).toStrictEqual(["true", "false"]);
    click("Return report");
    expect(screen.getByText("A reason is required (1–1000 characters)")).toBeTruthy();
    await waitFor(() => { expect(document.activeElement).toBe(reasonBox()); });
    expect(actions.reviewReport).not.toHaveBeenCalled();
    fireEvent.click(photos); // tick the photos
    fireEvent.click(drain); // untick the drain
    fireEvent.change(reasonBox(), { target: { value: "  Add a wide shot  " } });
    click("Return report");
    await waitFor(() => { expect(toasts.texts).toStrictEqual(["Returned for rework"]); });
    expect(actions.reviewReport).toHaveBeenCalledWith("job-p05-aaaa-bbbb", 4, 2, "return", "Rework: Photos — add more evidence. Add a wide shot");
    await waitFor(() => { expect(screen.queryByRole("button", { name: "Return report" })).toBeNull(); });
    // the dialog opens again with the loader's ticks and an empty reason
    click("Return for rework…");
    expect(screen.getAllByRole("checkbox").map((b) => b.getAttribute("aria-checked"))).toStrictEqual(["true", "false"]);
    expect(reasonBox().value).toBe("");
  });

  it("refuses a reason that is over 1000 characters with the ticked items, and Cancel or Escape closes the dialog", async () => {
    render(<ReviewView live={live()} />);
    click("Return for rework…");
    fireEvent.change(reasonBox(), { target: { value: "x".repeat(990) } });
    click("Return report");
    expect(screen.getByText("1017 / 1000 characters with the ticked items")).toBeTruthy();
    expect(actions.reviewReport).not.toHaveBeenCalled();
    click("Cancel");
    await waitFor(() => { expect(screen.queryByRole("button", { name: "Return report" })).toBeNull(); });
    click("Return for rework…");
    fireEvent.keyDown(reasonBox(), { key: "Escape" });
    await waitFor(() => { expect(screen.queryByRole("button", { name: "Return report" })).toBeNull(); });
  });

  it.each([
    ["CONFLICT", "This report version was already accepted or returned, or the job changed in the meantime; the page shows the latest state."],
    ["FORBIDDEN", "A contributor to this report version cannot approve it, or your company does not hold this delegation."],
    ["NOT_FOUND", "This job is outside your company’s delegation."],
  ])("shows the %s refusal in a banner", async (code, text) => {
    actions.reviewReport.mockResolvedValue(failure(code));
    render(<ReviewView live={live()} />);
    click("Accept report");
    expect(await screen.findByText(text)).toBeTruthy();
    expect(toasts.texts).toStrictEqual([]);
  });

  it("shows no banner for another failure (clearing an earlier one), and puts a refused reason on the reason field", async () => {
    const banner = () => screen.queryByText(/already accepted or returned|your company does not hold|^This job is outside/);
    actions.reviewReport.mockResolvedValue(failure("VALIDATION", { reason: "too long" }));
    render(<ReviewView live={live()} />);
    click("Return for rework…");
    fireEvent.change(reasonBox(), { target: { value: "Fine locally" } });
    click("Return report");
    expect(await screen.findByText("A reason is required (1–1000 characters)")).toBeTruthy();
    await waitFor(() => { expect(document.activeElement).toBe(reasonBox()); });
    expect(banner()).toBeNull();
    actions.reviewReport.mockResolvedValue(failure("CONFLICT"));
    click("Return report");
    await waitFor(() => { expect(banner()).not.toBeNull(); });
    actions.reviewReport.mockResolvedValue(failure("UNAVAILABLE"));
    click("Return report");
    await waitFor(() => { expect(banner()).toBeNull(); });
    expect(actions.reviewReport).toHaveBeenCalledTimes(3);
    expect(screen.getByRole("button", { name: "Return report" })).toBeTruthy(); // the dialog stays open to retry
  });
});

describe("Review view after or outside a decision", () => {
  it("cannot decide on a version the reviewer may not approve, and shows why", () => {
    render(<ReviewView live={live({}, report({ allowed: false, blocked: "You contributed to this version." }))} />);
    expect(screen.getByText("You contributed to this version.")).toBeTruthy();
    expect(screen.getByText("You cannot decide on this version.")).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Accept report" })).toBeNull();
  });

  it("says an accepted report's version, falling back to the current one", () => {
    const view = render(<ReviewView live={live({ status: "completed" }, report({ last: { decision: "accept", version: 1, reason: null } }))} />);
    expect(screen.getByText(/^Accepted → completed \(receipt\)\. Report v1 is now visible/)).toBeTruthy();
    expect(screen.getByText("Decided — the job is completed.")).toBeTruthy();
    view.rerender(<ReviewView live={live({ status: "completed" }, report({ blocked: "not shown" }))} />);
    expect(screen.getByText(/^Accepted → completed \(receipt\)\. Report v2 is now visible/)).toBeTruthy();
    expect(screen.queryByText("not shown")).toBeNull();
  });

  it("says a returned report's reason and the next version, or the next version alone", () => {
    const view = render(<ReviewView live={live({ status: "rework_requested" }, report({ last: { decision: "return", version: 2, reason: "Add photos" } }))} />);
    expect(screen.getByText("Returned for rework — “Add photos”. Tan can resume as v3.")).toBeTruthy();
    view.rerender(<ReviewView live={live({ status: "rework_requested" }, report())} />);
    expect(screen.getByText("Returned for rework. Tan can resume as v3.")).toBeTruthy();
  });
});
