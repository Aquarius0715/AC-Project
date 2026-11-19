// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import type { ActionFailure } from "@ac/web/lib/actionMessage";
import { accessEnd, addDays, partnerUserRows, type ApiPartnerUser } from "@ac/web/lib/partnerUsers";

// The contractor's company users (FR-P11): the rows' status and hints, the status filter, and the add, access,
// disable, enable and remove dialogs with their local checks, the Core API's field refusals and the toasts
const toasts = vi.hoisted((): { texts: string[]; failed: ActionFailure[] } => ({ texts: [], failed: [] }));
type Result = { ok: true; value: null } | ({ ok: false } & ActionFailure);
vi.mock("@ac/web/lib/useAction", () => ({
  useAction: () => [false, (fn: () => Promise<Result>, ok: string, after?: () => void, failed?: (f: ActionFailure) => void) => {
    void fn().then((r) => {
      if (r.ok) { toasts.texts.push(ok); after?.(); } else { toasts.failed.push(r); failed?.(r); }
    });
  }] as const,
}));

import { UsersView, type UsersData, type UsersOps } from "../app/partner/users/_components/users-view";

const ok = (): Promise<Result> => Promise.resolve({ ok: true, value: null });
const ops = () => ({ invite: vi.fn<UsersOps["invite"]>(ok), access: vi.fn<UsersOps["access"]>(ok), setStatus: vi.fn<UsersOps["setStatus"]>(ok), remove: vi.fn<UsersOps["remove"]>(ok), resend: vi.fn<UsersOps["resend"]>(ok) });
const user = (id: string, over: Partial<ApiPartnerUser> = {}): ApiPartnerUser => ({
  id, version: 3, organizationId: "org-a", membershipId: `m-${id}`, email: `${id}@example.com`, displayName: id, partnerRole: "technician",
  status: "active", lastSignInAt: null, accessUntil: null, invitedAt: "2026-09-01T00:00:00Z", invitedByMembershipId: "m-boss", ...over,
});
const NOW = new Date("2026-09-15T00:00:00Z");
const USERS: ApiPartnerUser[] = [
  user("boss", { partnerRole: "contractor" }),
  user("amy", { accessUntil: "2026-12-31T15:59:59Z" }),
  user("ben", { status: "invited", displayName: null }),
  user("cal", { status: "disabled" }),
  user("dee", { accessUntil: "2026-09-10T15:59:59Z" }),
  user("eve", { revokedAt: "2026-09-12T00:00:00Z" }),
  user("fay", { booked: true }),
];
const data = (over: Partial<UsersData> = {}): UsersData => ({
  company: "Cool Co", status: null, today: "2026-09-15", rows: partnerUserRows(USERS, "m-boss", undefined, NOW), all: USERS.map((u) => ({ id: u.id, email: u.email })), ...over,
});
const rowOf = (text: string) => {
  const row = screen.getAllByRole("row").find((r) => r.textContent.includes(text));
  if (!row) throw new TypeError(`no row ${text}`);
  return within(row);
};
const box = (name: RegExp) => screen.getByRole("textbox", { name });
const type = (el: HTMLElement, value: string) => { fireEvent.change(el, { target: { value } }); };
const dateBox = () => {
  const el = document.querySelector('input[type="date"]');
  if (!(el instanceof HTMLInputElement)) throw new TypeError("no date box");
  return el;
};
const footer = (name: string) => within(screen.getByRole("dialog")).getAllByRole("button", { name }).at(-1);
const submit = (name: string) => {
  const b = footer(name);
  if (!b) throw new TypeError(`no ${name}`);
  fireEvent.click(b);
};

beforeEach(() => { toasts.texts = []; toasts.failed = []; });
afterEach(() => { cleanup(); vi.unstubAllEnvs(); });

describe("Users view", () => {
  it("shows the unavailable card without data", () => {
    render(<UsersView data={null} ops={ops()} onStatus={vi.fn()} />);
    expect(screen.getByText("Page unavailable")).toBeTruthy();
    expect(screen.getByRole("link", { name: "Go to your home (or sign in)" }).getAttribute("href")).toBe("/partner");
  });

  it("shows each row's status, role and the hint for what is missing", () => {
    render(<UsersView data={data()} ops={ops()} onStatus={vi.fn()} sends />);
    expect(screen.getByRole("heading", { name: "Users of Cool Co" })).toBeTruthy();
    const boss = rowOf("boss@example.com");
    expect(boss.getByText("boss (you)")).toBeTruthy();
    expect(boss.getAllByText("Contractor")).toHaveLength(2);
    expect(boss.getByText("Your account")).toBeTruthy();
    expect(rowOf("amy@").getAllByText("Active")).toHaveLength(2);
    expect(rowOf("amy@").queryByText("Your account")).toBeNull();
    expect(rowOf("ben@").getAllByText("Not signed in yet")).toHaveLength(2);
    expect(rowOf("ben@").getByRole("button", { name: "Resend set-up e-mail" })).toBeTruthy();
    expect(rowOf("cal@").getAllByText("Disabled")).toHaveLength(2);
    expect(rowOf("cal@").getByRole("button", { name: "Enable" })).toBeTruthy();
    expect(rowOf("dee@").getAllByText("Access ended")).toHaveLength(2);
    expect(rowOf("eve@").getAllByText("Access ended by HQ")).toHaveLength(2);
    expect(rowOf("eve@").getByText("Only HQ can give the access back")).toBeTruthy();
    expect(rowOf("fay@").getByText("Has an assigned job — reassign it first")).toBeTruthy();
    expect(rowOf("fay@").queryByRole("button", { name: "Remove" })).toBeNull();
    expect(screen.getByText("The set-up e-mail comes from the sign-in service.")).toBeTruthy();
  });

  it("names the last active contractor, and shows a dash when a row has no action and no hint", () => {
    const only = [user("solo", { partnerRole: "contractor" }), user("gus", { membershipId: null })];
    const rows = partnerUserRows(only, "m-none", undefined, NOW);
    render(<UsersView data={data({ rows, company: "" })} ops={ops()} onStatus={vi.fn()} />);
    expect(screen.getByRole("heading", { name: "Users" })).toBeTruthy();
    expect(rowOf("solo@").getByText("Last active contractor")).toBeTruthy();
    expect(rowOf("solo@").queryByRole("button", { name: "Disable" })).toBeNull();
    expect(rowOf("gus@").getByRole("button", { name: "Disable" })).toBeTruthy();
    const none = { resend: false, access: false, disable: false, enable: false, remove: false };
    const [ivy] = partnerUserRows([user("ivy", { status: "disabled" })], null);
    if (!ivy) throw new TypeError("no row");
    cleanup();
    render(<UsersView data={data({ rows: [{ ...ivy, actions: none }] })} ops={ops()} onStatus={vi.fn()} />);
    expect(rowOf("ivy@").getAllByText("—").filter((el) => el.tagName === "SPAN")).toHaveLength(1);
  });

  it("shows the empty state, the filter and the set-up note of the demo and of development", () => {
    const onStatus = vi.fn();
    const view = render(<UsersView data={data({ rows: [], status: "disabled" })} ops={ops()} onStatus={onStatus} />);
    expect(screen.getByText("No users")).toBeTruthy();
    expect(screen.getByText("Nothing is sent in the demo.")).toBeTruthy();
    const filter = screen.getByRole("combobox", { name: "Status" });
    expect(filter instanceof HTMLSelectElement && filter.value).toBe("disabled");
    fireEvent.change(filter, { target: { value: "invited" } });
    fireEvent.change(filter, { target: { value: "" } });
    expect(onStatus.mock.calls).toStrictEqual([["invited"], [null]]);
    vi.stubEnv("NODE_ENV", "development");
    view.rerender(<UsersView data={data({ rows: [] })} ops={ops()} onStatus={onStatus} sends />);
    expect(screen.getByText(/in development it arrives in the local Mailpit inbox/)).toBeTruthy();
  });

  it("checks the add form top to bottom, then adds a technician with the access end", async () => {
    const o = ops();
    render(<UsersView data={data()} ops={o} onStatus={vi.fn()} sends />);
    fireEvent.click(screen.getByRole("button", { name: "Add user" }));
    expect(screen.getByRole("dialog", { name: "Add a user to Cool Co" })).toBeTruthy();
    expect(dateBox().value).toBe(addDays("2026-09-15", 90));
    type(dateBox(), "2026-09-14");
    submit("Add user");
    await waitFor(() => { expect(document.activeElement).toBe(box(/^Name/)); });
    expect(screen.getByText("A name of 1–120 characters")).toBeTruthy();
    expect(screen.getByText("A valid email address (up to 254 characters)")).toBeTruthy();
    expect(screen.getByText("Today or later")).toBeTruthy();
    type(box(/^Name/), "Lim");
    type(box(/^Email/), "AMY@example.com");
    submit("Add user");
    expect(screen.getByText("Already a user of your company")).toBeTruthy();
    type(box(/^Email/), " lim@example.com ");
    submit("Add user");
    await waitFor(() => { expect(document.activeElement).toBe(dateBox()); });
    type(dateBox(), "2026-10-01");
    submit("Add user");
    await waitFor(() => { expect(screen.queryByRole("dialog")).toBeNull(); });
    expect(o.invite).toHaveBeenCalledWith("lim@example.com", "technician", "Lim", accessEnd("2026-10-01"));
    expect(toasts.texts).toStrictEqual(["Lim added — a set-up e-mail is on its way to lim@example.com"]);
  });

  it("adds a contractor without an access end, and shows the Core API's field refusal until the field is edited", async () => {
    const o = ops();
    o.invite.mockResolvedValueOnce({ ok: false, code: "VALIDATION", messageKey: "errors.email_taken", fieldErrors: { email: "errors.email_taken", other: "x" } });
    render(<UsersView data={data({ company: "" })} ops={o} onStatus={vi.fn()} />);
    fireEvent.click(screen.getByRole("button", { name: "Add user" }));
    expect(screen.getByRole("dialog", { name: "Add a user to your company" })).toBeTruthy();
    fireEvent.change(screen.getByRole("combobox", { name: /^Role/ }), { target: { value: "contractor" } });
    expect(document.querySelector('input[type="date"]')).toBeNull();
    type(box(/^Name/), "Tan");
    type(box(/^Email/), "tan@example.com");
    submit("Add user");
    await waitFor(() => { expect(document.activeElement).toBe(box(/^Email/)); });
    expect(box(/^Email/).getAttribute("aria-invalid")).toBe("true");
    type(box(/^Name/), "Tan Ah");
    expect(box(/^Email/).getAttribute("aria-invalid")).toBe("true");
    type(box(/^Email/), "tan2@example.com");
    expect(box(/^Email/).getAttribute("aria-invalid")).toBeNull();
    submit("Add user");
    await waitFor(() => { expect(screen.queryByRole("dialog")).toBeNull(); });
    expect(o.invite).toHaveBeenLastCalledWith("tan2@example.com", "contractor", "Tan Ah", null);
    expect(toasts.texts).toStrictEqual(["Tan Ah added — nothing is sent in the demo"]);
  });

  it("moves a technician's access end, opening on the current one or three months ahead", async () => {
    const o = ops();
    const rows = partnerUserRows([...USERS, user("jo", { status: "invited", displayName: null })], "m-boss", undefined, NOW);
    render(<UsersView data={data({ rows })} ops={o} onStatus={vi.fn()} />);
    fireEvent.click(rowOf("amy@").getByRole("button", { name: "Access" }));
    expect(screen.getByRole("dialog", { name: "Access of amy" })).toBeTruthy();
    expect(dateBox().value).toBe("2026-12-31");
    submit("Save access");
    await waitFor(() => { expect(document.activeElement).toBe(box(/^Reason/)); });
    expect(screen.getByText("A reason of 1–1000 characters")).toBeTruthy();
    type(box(/^Reason/), "contract extended");
    submit("Save access");
    await waitFor(() => { expect(screen.queryByRole("dialog")).toBeNull(); });
    expect(o.access).toHaveBeenCalledWith(expect.objectContaining({ id: "amy" }), accessEnd("2026-12-31"), "contract extended");
    expect(toasts.texts).toStrictEqual(["Access of amy now ends 2026-12-31"]);
    fireEvent.click(rowOf("jo@").getByRole("button", { name: "Access" }));
    expect(screen.getByRole("dialog", { name: "Access of jo@example.com" })).toBeTruthy();
    expect(dateBox().value).toBe(addDays("2026-09-15", 90));
    fireEvent.click(within(screen.getByRole("dialog")).getByRole("button", { name: "Cancel" }));
    await waitFor(() => { expect(screen.queryByRole("dialog")).toBeNull(); });
  });

  it("disables and enables an account with a reason, and keeps the dialog on a refusal without a field", async () => {
    const o = ops();
    o.setStatus.mockResolvedValueOnce({ ok: false, code: "CONFLICT", messageKey: "error.conflict", fieldErrors: {} });
    render(<UsersView data={data()} ops={o} onStatus={vi.fn()} />);
    fireEvent.click(rowOf("amy@").getByRole("button", { name: "Disable" }));
    expect(screen.getByRole("dialog", { name: "Disable amy" })).toBeTruthy();
    expect(screen.getByText(/Their access ends now\. Enabling them again/)).toBeTruthy();
    type(box(/^Reason/), "on leave");
    submit("Disable");
    await waitFor(() => { expect(toasts.failed).toHaveLength(1); });
    expect(screen.getByRole("dialog")).toBeTruthy();
    submit("Disable");
    await waitFor(() => { expect(screen.queryByRole("dialog")).toBeNull(); });
    expect(o.setStatus).toHaveBeenLastCalledWith(expect.objectContaining({ id: "amy" }), "disabled", "on leave");
    fireEvent.click(rowOf("cal@").getByRole("button", { name: "Enable" }));
    expect(screen.getByRole("dialog", { name: "Enable cal" })).toBeTruthy();
    // the reason of the earlier dialog is cleared
    const reasonBox = box(/^Reason/);
    expect(reasonBox instanceof HTMLTextAreaElement && reasonBox.value).toBe("");
    type(box(/^Reason/), "back");
    submit("Enable");
    await waitFor(() => { expect(screen.queryByRole("dialog")).toBeNull(); });
    expect(o.setStatus).toHaveBeenLastCalledWith(expect.objectContaining({ id: "cal" }), "active", "back");
    expect(toasts.texts).toStrictEqual(["amy disabled — they can no longer sign in", "cal enabled again"]);
  });

  it("removes an account, with the invited wording, the reason refusal and the focus on the list", async () => {
    const o = ops();
    o.remove.mockResolvedValueOnce({ ok: false, code: "VALIDATION", messageKey: "errors.reason", fieldErrors: { reason: "errors.reason" } });
    const view = render(<UsersView data={data()} ops={o} onStatus={vi.fn()} />);
    fireEvent.click(rowOf("ben@").getByRole("button", { name: "Remove" }));
    expect(screen.getByRole("dialog", { name: "Remove ben@example.com" })).toBeTruthy();
    expect(screen.getByText("Their account is removed before they signed in.")).toBeTruthy();
    type(box(/^Reason/), "never started");
    submit("Remove");
    await waitFor(() => { expect(box(/^Reason/).getAttribute("aria-invalid")).toBe("true"); });
    submit("Remove");
    await waitFor(() => { expect(screen.queryByRole("dialog")).toBeNull(); });
    expect(o.remove).toHaveBeenLastCalledWith(expect.objectContaining({ id: "ben" }), "never started");
    expect(toasts.texts).toStrictEqual(["ben@example.com removed"]);
    view.rerender(<UsersView data={data({ rows: data().rows.filter((r) => r.id !== "ben") })} ops={o} onStatus={vi.fn()} />);
    await waitFor(() => { expect(document.activeElement?.id).toBe("users-list"); });
    fireEvent.click(rowOf("cal@").getByRole("button", { name: "Remove" }));
    expect(screen.getByText("Their access ends now and they leave your company's user list.")).toBeTruthy();
  });

  it("re-sends the set-up e-mail, saying when nothing is sent", async () => {
    const o = ops();
    const view = render(<UsersView data={data()} ops={o} onStatus={vi.fn()} sends />);
    fireEvent.click(rowOf("ben@").getByRole("button", { name: "Resend set-up e-mail" }));
    view.rerender(<UsersView data={data()} ops={o} onStatus={vi.fn()} />);
    fireEvent.click(rowOf("ben@").getByRole("button", { name: "Resend set-up e-mail" }));
    await waitFor(() => { expect(toasts.texts).toHaveLength(2); });
    expect(o.resend).toHaveBeenCalledWith(expect.objectContaining({ id: "ben" }));
    expect(toasts.texts).toStrictEqual(["Set-up e-mail sent again to ben@example.com", "Set-up e-mail re-sent to ben@example.com — nothing is sent in the demo"]);
  });
});
