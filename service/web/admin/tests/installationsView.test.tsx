// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { scopeLists } from "@ac/web/lib/scopeLists";
import type { ActionFailure } from "@ac/web/lib/actionMessage";

// HQ makes the units and jobs of an installation request (FR-A06, DEC-107); the action makes them in parallel, so after
// a refusal some pairs exist and the dialog must not offer the old number again (R835)
const actions = vi.hoisted(() => ({ approveInstallationRequest: vi.fn(), declineInstallationRequest: vi.fn() }));
vi.mock("../app/admin/jobs/actions", () => actions);
vi.mock("../app/admin/jobs/_components/jobs-header", () => ({ JobsTabs: () => null }));
vi.mock("@ac/web/lib/useUrlPatch", () => ({ useUrlPatch: () => vi.fn() }));
type Result = { ok: true; value: { made: number } } | ({ ok: false } & ActionFailure);
vi.mock("@ac/web/lib/useAction", () => ({
  useAction: () => [false, (fn: () => Promise<Result>, _ok: unknown, after?: () => void, failed?: (f: ActionFailure) => void) => {
    void fn().then((r) => { if (r.ok) after?.(); else failed?.(r); });
  }] as const,
}));

import { InstallationsView } from "../app/admin/jobs/_components/installations-view";
import type { InstallationsLive } from "../app/admin/jobs/_lib/load";

afterEach(cleanup);

type Row = InstallationsLive["rows"][number];
const row = (jobIds: string[]): Row => ({
  jobIds, id: "0193a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a5b", version: 1, number: "0193a1b2", place: "Home › Living", quantity: 3, model: "Daikin FTKF", withIot: false,
  status: "approved", statusLabel: "Approved", tone: "ok", firstTime: "—", note: null, declineReason: null, customerOrgId: "org-c",
  propertyId: "prop-1", modelId: "model-1", slots: [{ startAt: "2026-12-01T01:00:00Z", endAt: "2026-12-01T03:00:00Z" }], contactWindow: null, customer: "Demo Customer A",
});
const live = (r: Row): InstallationsLive => ({
  now: "2026-11-17T00:00:00Z", counts: { jobs: 0, plans: 0, contractors: 0 }, scope: scopeLists([], [], [], {}), units: [], q: {},
  canDecline: true, canApprove: true, status: "all", statuses: ["all", "submitted", "approved", "declined", "cancelled"], rows: [r],
  models: [{ id: "model-1", name: "Daikin FTKF" }], spaceOf: {},
});

describe("Installation request approval", () => {
  it("closes the dialog on a refusal and offers only the units still to make once the list is refreshed", async () => {
    actions.approveInstallationRequest.mockResolvedValue({ ok: false, code: "CONFLICT", messageKey: "error.conflict", fieldErrors: {} });
    const view = render(<InstallationsView live={live(row([]))} />);
    fireEvent.click(screen.getByRole("button", { name: /Make units and jobs/ }));
    fireEvent.click(screen.getByRole("button", { name: "Make 3 units" }));
    expect(await screen.findByText("The units could not be made; what was made stays in the list.")).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Make 3 units" })).toBeNull();
    view.rerender(<InstallationsView live={live(row(["job-1", "job-2"]))} />); // the action refreshed: 2 pairs were made
    fireEvent.click(screen.getByRole("button", { name: /Make units and jobs/ }));
    expect(screen.getByRole("button", { name: "Make 1 units" })).toBeTruthy();
    expect(screen.queryByText("The units could not be made; what was made stays in the list.")).toBeNull();
  });
});
