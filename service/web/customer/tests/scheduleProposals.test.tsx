// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import type { ActionFailure } from "@ac/web/lib/actionMessage";
import type { ApiAutomation } from "@ac/web/lib/clientAutomations";
import type { StartProposal } from "@ac/web/lib/contracts.gen";

// Schedule suggestions on the customer Automations page: a card per proposal whose AC and rule version are current,
// Approve sends the proposal's own id, weekday and version, and the empty text follows the cards actually shown (a
// proposal of an archived AC or of a rule changed since is hidden, R1106)
type Approved = { ok: true; value: null } | ({ ok: false } & ActionFailure);
const actions = vi.hoisted(() => ({ approveStartProposal: vi.fn<(selection: unknown) => Promise<Approved>>() }));
vi.mock("../app/customer/automations/actions", () => actions);
const runs = vi.hoisted((): { toasts: string[] } => ({ toasts: [] }));
vi.mock("@ac/web/lib/useAction", () => ({
  useAction: () => [false, (fn: () => Promise<Approved>, ok: string) => {
    void fn().then((r) => { if (r.ok) runs.toasts.push(ok); });
  }] as const,
}));

import { ScheduleProposals } from "../app/customer/automations/_components/schedule-proposals";

const RULE = "11111111-1111-4111-8111-111111111111";
const UNIT = "22222222-2222-4222-8222-222222222222";
const PROPOSAL = "33333333-3333-4333-8333-333333333333";
const rule: ApiAutomation = {
  id: RULE, version: 3, createdAt: "2026-09-01T00:00:00Z", updatedAt: "2026-09-01T00:00:00Z", name: "Weekday pre-cool", unitIds: [UNIT], ownerMembershipId: "m-1",
  timezone: "Asia/Kuala_Lumpur", enabled: true, priority: 0, disabledReason: null, onlyIf: null, lastRun: null,
  kind: "schedule", weekdays: [1, 3], startLocal: "18:00", endLocal: "22:00", endsNextDay: false,
  startAction: { kind: "set_temperature", celsius: 25 }, endAction: { kind: "set_power", power: false },
};
const proposal = (over: Partial<StartProposal> = {}): StartProposal => ({
  id: PROPOSAL, automationId: RULE, unitId: UNIT, version: 3, weekday: 3, from: "18:00", to: "17:30", endLocal: "22:00", occurrences: 4,
  supportDates: ["2026-09-02", "2026-09-09", "2026-09-16"], ...over,
});
const units = [{ id: UNIT, name: "Bedroom AC" }];
const EMPTY = "No schedule suggestions yet.";

beforeEach(() => {
  runs.toasts = [];
  actions.approveStartProposal.mockReset().mockResolvedValue({ ok: true, value: null });
});
afterEach(() => { cleanup(); });

describe("ScheduleProposals", () => {
  it("shows a current proposal with its evidence, and Approve sends its id, weekday and version", async () => {
    render(<ScheduleProposals proposals={[proposal()]} units={units} rules={[rule]} />);
    expect(screen.getByRole("heading", { level: 3 }).textContent).toBe("Bedroom AC · Weekday pre-cool");
    expect(screen.getByText("Wednesday 18:00 → 17:30")).toBeTruthy();
    expect(screen.getByText("On 3 of the last 4 occurrences of Wednesday, the AC was confirmed to turn on manually within 15 minutes of 17:30.")).toBeTruthy();
    expect(screen.getByText("Confirmed on 2026-09-02, 2026-09-09, 2026-09-16 · ends at 22:00 (Asia/Kuala_Lumpur)")).toBeTruthy();
    expect(screen.queryByText(EMPTY)).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Approve schedule change" }));
    await vi.waitFor(() => { expect(runs.toasts).toStrictEqual(["Schedule change approved"]); });
    expect(actions.approveStartProposal.mock.calls).toStrictEqual([[{ id: RULE, proposalId: PROPOSAL, weekday: 3, version: 3 }]]);
  });

  it("says there is none when the API has none", () => {
    render(<ScheduleProposals proposals={[]} units={units} rules={[rule]} />);
    expect(screen.getByText(EMPTY)).toBeTruthy();
  });

  it("hides a proposal of an AC not listed or of a rule changed since, and then says there is none", () => {
    render(<ScheduleProposals proposals={[proposal({ unitId: "archived-ac" }), proposal({ id: "p-old", version: 2 }), proposal({ id: "p-gone", automationId: "deleted-rule" })]} units={units} rules={[rule]} />);
    expect(screen.queryByRole("button", { name: "Approve schedule change" })).toBeNull();
    expect(screen.getByText(EMPTY)).toBeTruthy();
  });
});
