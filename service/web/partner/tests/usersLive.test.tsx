// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { partnerUserRows, type ApiPartnerUser } from "@ac/web/lib/partnerUsers";

// Users in API mode (FR-P11): each row action calls its Server Action with the row's id, version, e-mail and role,
// and the status filter goes to the URL
const ok = () => Promise.resolve({ ok: true as const, value: null });
const actions = vi.hoisted(() => ({ invitePartnerUser: vi.fn(), setPartnerAccess: vi.fn(), setPartnerUserStatus: vi.fn(), removePartnerUser: vi.fn(), resendPartnerInvite: vi.fn() }));
vi.mock("../app/partner/users/actions", () => actions);
const patch = vi.hoisted(() => vi.fn());
vi.mock("@ac/web/lib/useUrlPatch", () => ({ useUrlPatch: () => patch }));
vi.mock("@ac/web/lib/useAction", () => ({
  useAction: () => [false, (fn: () => Promise<{ ok: boolean }>, _ok: string, after?: () => void) => { void fn().then((r) => { if (r.ok) after?.(); }); }] as const,
}));

import { UsersLive } from "../app/partner/users/_components/users-live";

beforeEach(() => { for (const fn of [...Object.values(actions), patch]) fn.mockReset(); });
afterEach(cleanup);

const user = (id: string, over: Partial<ApiPartnerUser> = {}): ApiPartnerUser => ({
  id, version: 7, organizationId: "org-a", membershipId: `m-${id}`, email: `${id}@example.com`, displayName: id, partnerRole: "technician",
  status: "active", lastSignInAt: null, accessUntil: "2026-12-31T15:59:59Z", invitedAt: "2026-09-01T00:00:00Z", invitedByMembershipId: "m-boss", ...over,
});
const USERS = [user("boss", { partnerRole: "contractor" }), user("amy"), user("ben", { status: "invited" }), user("cal", { status: "disabled" })];
const rowOf = (text: string) => {
  const row = screen.getAllByRole("row").find((r) => r.textContent.includes(text));
  if (!row) throw new TypeError(`no row ${text}`);
  return within(row);
};
const reasonAndSubmit = async (reason: string, name: string) => {
  fireEvent.change(screen.getByRole("textbox", { name: /^Reason/ }), { target: { value: reason } });
  const b = within(screen.getByRole("dialog")).getAllByRole("button", { name }).at(-1);
  if (!b) throw new TypeError(`no ${name}`);
  fireEvent.click(b);
  await waitFor(() => { expect(screen.queryByRole("dialog")).toBeNull(); });
};

describe("Users live", () => {
  it("passes each row's change to its Server Action and the status filter to the URL", async () => {
    for (const fn of Object.values(actions)) fn.mockImplementation(ok);
    render(<UsersLive live={{ company: "Cool Co", status: null, today: "2026-09-15", rows: partnerUserRows(USERS, "m-boss"), all: USERS.map((u) => ({ id: u.id, email: u.email })) }} />);
    expect(screen.getByText("The set-up e-mail comes from the sign-in service.")).toBeTruthy();
    fireEvent.change(screen.getByRole("combobox", { name: "Status" }), { target: { value: "disabled" } });
    expect(patch).toHaveBeenCalledWith({ status: "disabled" });
    fireEvent.click(rowOf("ben@").getByRole("button", { name: "Resend set-up e-mail" }));
    expect(actions.resendPartnerInvite).toHaveBeenCalledWith("ben");
    fireEvent.click(rowOf("amy@").getByRole("button", { name: "Access" }));
    await reasonAndSubmit("extended", "Save access");
    expect(actions.setPartnerAccess).toHaveBeenCalledWith("amy", 7, "amy@example.com", "technician", "2026-12-31T15:59:59.000Z", "extended");
    fireEvent.click(rowOf("cal@").getByRole("button", { name: "Enable" }));
    await reasonAndSubmit("back", "Enable");
    expect(actions.setPartnerUserStatus).toHaveBeenCalledWith("cal", 7, "cal@example.com", "technician", "active", "back");
    fireEvent.click(rowOf("cal@").getByRole("button", { name: "Remove" }));
    await reasonAndSubmit("left", "Remove");
    expect(actions.removePartnerUser).toHaveBeenCalledWith("cal", 7, "left");
    fireEvent.click(screen.getByRole("button", { name: "Add user" }));
    fireEvent.change(screen.getByRole("textbox", { name: /^Name/ }), { target: { value: "Lim" } });
    fireEvent.change(screen.getByRole("textbox", { name: /^Email/ }), { target: { value: "lim@example.com" } });
    fireEvent.change(screen.getByRole("combobox", { name: /^Role/ }), { target: { value: "contractor" } });
    const add = within(screen.getByRole("dialog")).getAllByRole("button", { name: "Add user" }).at(-1);
    if (!add) throw new TypeError("no Add user");
    fireEvent.click(add);
    await waitFor(() => { expect(actions.invitePartnerUser).toHaveBeenCalledWith("lim@example.com", "contractor", "Lim", null); });
  });
});
