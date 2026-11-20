// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { actionMessage, type ActionFailure } from "@ac/web/lib/actionMessage";
import type { ClientUserRow } from "@ac/web/lib/assets";

// Users of the customer (FR-C19, IR417, IR461, IR508) in API mode: which actions each row offers, the add dialog with
// its local checks and the Core API's field refusals, and the role, disable, enable and remove dialogs with a reason
type Done = { ok: true; value: null } | ({ ok: false } & ActionFailure);
const actions = vi.hoisted(() => ({
  inviteUser: vi.fn<(customerId: string, email: string, role: string, name: string) => Promise<Done>>(),
  updateUser: vi.fn<(id: string, version: number, customerId: string, email: string, role: string, status: string | null, reason: string) => Promise<Done>>(),
  removeUser: vi.fn<(id: string, version: number, reason: string) => Promise<Done>>(),
  resendInvite: vi.fn<(id: string) => Promise<Done>>(),
}));
vi.mock("../app/customer/users/actions", () => actions);
const runs = vi.hoisted((): { toasts: string[] } => ({ toasts: [] }));
vi.mock("@ac/web/lib/useAction", () => ({
  useAction: () => [false, <T,>(fn: () => Promise<{ ok: true; value: T } | ({ ok: false } & ActionFailure)>, ok: string | ((v: T) => string), after?: (v: T) => void, failed?: (f: ActionFailure) => void) => {
    void fn().then((r) => {
      if (r.ok) { runs.toasts.push(typeof ok === "string" ? ok : ok(r.value)); after?.(r.value); } else failed?.(r);
    });
  }] as const,
}));

import { UsersView } from "../app/customer/users/_components/users-view";

const row = (id: string, over: Partial<ClientUserRow> = {}): ClientUserRow => ({
  id, version: 3, email: `${id}@example.com`, name: id, role: "member", status: "active", sub: `${id}@example.com`, lastSignIn: "—", channels: "—",
  lastOwner: false, you: false, membershipId: `m-${id}`, revoked: false, ...over,
});
const ROWS = [
  row("Tan", { role: "owner", you: true, lastOwner: true }),
  row("Mei"),
  row("Ali", { role: "owner" }),
  row("guest", { name: null, status: "invited", sub: "invited 30 Sept 2026 by HQ" }),
  row("Old", { status: "disabled" }),
  row("Gone", { revoked: true }),
];
const live = (rows = ROWS, customer = "Demo Customer A") => ({ customer, rows, customerId: "cust-a", all: rows.map((r) => ({ id: r.id, email: r.email })) });
const dialog = () => within(screen.getByRole("dialog"));
const rowOf = (text: string) => {
  const found = screen.getAllByRole("row").find((r) => r.textContent.includes(text));
  if (!found) throw new Error(`no row ${text}`);
  return within(found);
};
const buttons = (text: string) => rowOf(text).queryAllByRole("button").map((b) => b.textContent);
const failure = (code: string, fieldErrors: Record<string, string> = {}): { ok: false } & ActionFailure => ({ ok: false, messageKey: `error.${code}`, code, fieldErrors });

beforeEach(() => {
  runs.toasts = [];
  for (const fn of Object.values(actions)) fn.mockReset().mockResolvedValue({ ok: true, value: null });
});
afterEach(() => {
  cleanup();
  vi.unstubAllEnvs();
});

describe("UsersView", () => {
  it("shows Page unavailable to a member (no data)", () => {
    render(<UsersView live={null} />);
    expect(screen.getByText("Page unavailable")).toBeDefined();
    expect(screen.queryByRole("button", { name: /Add user/ })).toBeNull();
  });

  it("offers each row only what its user allows", () => {
    render(<UsersView live={live()} />);
    expect(screen.getByRole("heading", { name: "Users of Demo Customer A" })).toBeDefined();
    expect(rowOf("Tan").getByText("Your account")).toBeDefined();
    expect(rowOf("Tan").getByText("Tan (you)")).toBeDefined();
    expect(buttons("Tan")).toStrictEqual([]);
    expect(buttons("Mei")).toStrictEqual(["Make owner", "Disable", "Remove"]);
    expect(buttons("Ali")).toStrictEqual(["Make member", "Disable", "Remove"]);
    expect(buttons("guest@")).toStrictEqual(["Resend set-up e-mail", "Make owner", "Remove"]);
    expect(rowOf("guest@").getAllByText("Not signed in yet").length).toBeGreaterThan(0);
    expect(buttons("Old")).toStrictEqual(["Make owner", "Enable", "Remove"]);
    expect(rowOf("Old").getAllByText("Disabled").length).toBeGreaterThan(0);
    // HQ ended the access: only removing is left
    expect(buttons("Gone")).toStrictEqual(["Remove"]);
    expect(rowOf("Gone").getByText("Only HQ can give the access back")).toBeDefined();
    expect(rowOf("Gone").getAllByText("Access ended by HQ").length).toBeGreaterThan(0);
  });

  it("the last active owner other than me can't be demoted, disabled or removed, also when HQ ended the access", () => {
    render(<UsersView live={live([row("Ali", { role: "owner", lastOwner: true }), row("Bob", { role: "owner", lastOwner: true, revoked: true })], "")} />);
    expect(screen.getByRole("heading", { name: "Users" })).toBeDefined();
    expect(buttons("Ali")).toStrictEqual([]);
    expect(rowOf("Ali").getByText("Last active owner")).toBeDefined();
    expect(buttons("Bob")).toStrictEqual([]);
  });

  it("resends the set-up e-mail by id", async () => {
    render(<UsersView live={live()} />);
    fireEvent.click(rowOf("guest@").getByRole("button", { name: "Resend set-up e-mail" }));
    await waitFor(() => { expect(runs.toasts).toStrictEqual(["Set-up e-mail sent again to guest@example.com"]); });
    expect(actions.resendInvite).toHaveBeenCalledWith("guest");
  });

  it("adds a user only with a name and a new address, focusing the first field to fix", async () => {
    render(<UsersView live={live()} />);
    fireEvent.click(screen.getByRole("button", { name: "Add user" }));
    expect(screen.getByRole("dialog").textContent).toContain("Add a user to Demo Customer A");
    // the button keeps the focus, so only the refusal moves it to a field (the dialog opens on Name by itself)
    const add = () => {
      const button = dialog().getByRole("button", { name: "Add user" });
      button.focus();
      fireEvent.click(button);
    };
    add();
    expect(dialog().getByText("A name of 1–120 characters")).toBeDefined();
    expect(dialog().getByText("A valid email address (up to 254 characters)")).toBeDefined();
    await waitFor(() => { expect(document.activeElement).toBe(dialog().getByLabelText(/^Name/)); });
    fireEvent.change(dialog().getByLabelText(/^Name/), { target: { value: " Lim Ka Wai " } });
    fireEvent.change(dialog().getByLabelText(/^Email/), { target: { value: "MEI@example.com" } });
    add();
    expect(dialog().getByText("Already a user of this customer")).toBeDefined();
    await waitFor(() => { expect(document.activeElement).toBe(dialog().getByLabelText(/^Email/)); });
    expect(actions.inviteUser).not.toHaveBeenCalled();
    fireEvent.change(dialog().getByLabelText(/^Email/), { target: { value: "lim@example.com" } });
    fireEvent.change(dialog().getByRole("combobox", { name: "Role" }), { target: { value: "owner" } });
    add();
    await waitFor(() => { expect(screen.queryByRole("dialog")).toBeNull(); });
    expect(actions.inviteUser).toHaveBeenCalledWith("cust-a", "lim@example.com", "owner", " Lim Ka Wai ");
    expect(runs.toasts).toStrictEqual(["Lim Ka Wai added — a set-up e-mail is on its way to lim@example.com"]);
  });

  it("shows the Core API's refusal under its field until that field is edited", async () => {
    actions.inviteUser.mockResolvedValue(failure("CONFLICT", { email: "conflict" }));
    render(<UsersView live={live([], "")} />);
    fireEvent.click(screen.getByRole("button", { name: "Add user" }));
    expect(screen.getByRole("dialog").textContent).toContain("Add a user");
    expect(screen.getByRole("dialog").textContent).not.toContain("Add a user to");
    fireEvent.change(dialog().getByLabelText(/^Name/), { target: { value: "Lim" } });
    fireEvent.change(dialog().getByLabelText(/^Email/), { target: { value: "lim@example.com" } });
    fireEvent.click(dialog().getByRole("button", { name: "Add user" }));
    const email = dialog().getByLabelText(/^Email/);
    await waitFor(() => { expect(document.activeElement).toBe(email); });
    const text = actionMessage(failure("CONFLICT", { email: "conflict" }));
    expect(dialog().getByText(text)).toBeDefined();
    // editing another field keeps it; editing the address clears it
    fireEvent.change(dialog().getByLabelText(/^Name/), { target: { value: "Lim K" } });
    expect(dialog().getByText(text)).toBeDefined();
    fireEvent.change(email, { target: { value: "lim2@example.com" } });
    expect(dialog().queryByText(text)).toBeNull();
    expect(runs.toasts).toStrictEqual([]);
  });

  it("changes the role to the other one with a reason, at the row version", async () => {
    render(<UsersView live={live()} />);
    fireEvent.click(rowOf("Ali").getByRole("button", { name: "Make member" }));
    expect(screen.getByRole("dialog").textContent).toContain("Change role of Ali");
    expect(dialog().getByRole<HTMLSelectElement>("combobox", { name: "Role" }).value).toBe("member");
    fireEvent.click(dialog().getByRole("button", { name: "Change role" }));
    expect(dialog().getByText("A reason of 1–1000 characters")).toBeDefined();
    await waitFor(() => { expect(document.activeElement).toBe(dialog().getByLabelText(/^Reason/)); });
    fireEvent.change(dialog().getByLabelText(/^Reason/), { target: { value: "handed over" } });
    fireEvent.click(dialog().getByRole("button", { name: "Change role" }));
    await waitFor(() => { expect(screen.queryByRole("dialog")).toBeNull(); });
    expect(actions.updateUser).toHaveBeenCalledWith("Ali", 3, "cust-a", "Ali@example.com", "member", null, "handed over");
    fireEvent.click(rowOf("Mei").getByRole("button", { name: "Make owner" }));
    fireEvent.change(dialog().getByLabelText(/^Reason/), { target: { value: "new manager" } });
    fireEvent.click(dialog().getByRole("button", { name: "Change role" }));
    await waitFor(() => { expect(runs.toasts).toStrictEqual(["Ali is now a member", "Mei is now an owner"]); });
    expect(actions.updateUser).toHaveBeenLastCalledWith("Mei", 3, "cust-a", "Mei@example.com", "owner", null, "new manager");
  });

  it("disables and enables with a reason, keeping the role", async () => {
    render(<UsersView live={live()} />);
    fireEvent.click(rowOf("Ali").getByRole("button", { name: "Disable" }));
    expect(dialog().getByText("Their access ends now. You can enable them again later.")).toBeDefined();
    expect(dialog().queryByRole("combobox", { name: "Role" })).toBeNull();
    fireEvent.change(dialog().getByLabelText(/^Reason/), { target: { value: "on leave" } });
    fireEvent.click(dialog().getByRole("button", { name: "Disable" }));
    await waitFor(() => { expect(screen.queryByRole("dialog")).toBeNull(); });
    expect(actions.updateUser).toHaveBeenLastCalledWith("Ali", 3, "cust-a", "Ali@example.com", "owner", "disabled", "on leave");
    fireEvent.click(rowOf("Old").getByRole("button", { name: "Enable" }));
    expect(screen.getByRole("dialog").textContent).toContain("Enable Old");
    fireEvent.change(dialog().getByLabelText(/^Reason/), { target: { value: "back" } });
    fireEvent.click(dialog().getByRole("button", { name: "Enable" }));
    await waitFor(() => { expect(screen.queryByRole("dialog")).toBeNull(); });
    expect(actions.updateUser).toHaveBeenLastCalledWith("Old", 3, "cust-a", "Old@example.com", "member", "active", "back");
    expect(runs.toasts).toStrictEqual(["Ali disabled — they can no longer sign in", "Old enabled again"]);
  });

  it("removes a user with a reason; the warning says whether they had signed in, and focus goes to the list", async () => {
    render(<UsersView live={live()} />);
    fireEvent.click(rowOf("guest@").getByRole("button", { name: "Remove" }));
    expect(screen.getByRole("dialog").textContent).toContain("Remove guest@example.com");
    expect(dialog().getByText("Their account is removed before they signed in.")).toBeDefined();
    fireEvent.click(dialog().getByRole("button", { name: "Cancel" }));
    expect(screen.queryByRole("dialog")).toBeNull();
    fireEvent.click(rowOf("Gone").getByRole("button", { name: "Remove" }));
    expect(dialog().getByText("Their access ends now and they leave your user list.")).toBeDefined();
    fireEvent.change(dialog().getByLabelText(/^Reason/), { target: { value: " left the company " } });
    fireEvent.click(dialog().getByRole("button", { name: "Remove" }));
    await waitFor(() => { expect(screen.queryByRole("dialog")).toBeNull(); });
    expect(actions.removeUser).toHaveBeenCalledWith("Gone", 3, " left the company ");
    expect(runs.toasts).toStrictEqual(["Gone removed"]);
    await waitFor(() => { expect(document.activeElement?.id).toBe("users-list"); });
  });

  it("a refused change shows the reason refusal under the field, and the dialog stays", async () => {
    const refused = failure("VALIDATION", { reason: "too_long" });
    actions.removeUser.mockResolvedValue(refused);
    render(<UsersView live={live()} />);
    fireEvent.click(rowOf("Mei").getByRole("button", { name: "Remove" }));
    fireEvent.change(dialog().getByLabelText(/^Reason/), { target: { value: "x" } });
    fireEvent.click(dialog().getByRole("button", { name: "Remove" }));
    // Reason is the dialog's only field, so where the focus ends says nothing: the email refusal above checks the focus
    await waitFor(() => { expect(dialog().getByText(actionMessage(refused))).toBeDefined(); });
    expect(dialog().getByLabelText(/^Reason/).getAttribute("aria-invalid")).toBe("true");
    expect(runs.toasts).toStrictEqual([]);
  });

  it("a refusal on no field of the dialog leaves the focus where it was and shows nothing under the fields", async () => {
    actions.updateUser.mockResolvedValue(failure("CONFLICT", { status: "last_owner" }));
    render(<UsersView live={live()} />);
    fireEvent.click(rowOf("Ali").getByRole("button", { name: "Disable" }));
    fireEvent.change(dialog().getByLabelText(/^Reason/), { target: { value: "on leave" } });
    const disable = dialog().getByRole("button", { name: "Disable" });
    fireEvent.click(disable);
    await waitFor(() => { expect(actions.updateUser).toHaveBeenCalledOnce(); });
    // a focus move the refusal asked for would land on the next frame, after this
    disable.focus();
    await new Promise((resolve) => { requestAnimationFrame(resolve); });
    expect(document.activeElement).toBe(disable);
    expect(dialog().getByLabelText(/^Reason/).getAttribute("aria-invalid")).toBeNull();
    expect(screen.getByRole("dialog").querySelector(".text-crit")).toBeNull();
    expect(dialog().getByRole("button", { name: "Disable" })).toBe(disable);
    expect(runs.toasts).toStrictEqual([]);
  });

  it("names the local Mailpit inbox only in development", () => {
    vi.stubEnv("NODE_ENV", "development");
    render(<UsersView live={live()} />);
    expect(screen.getByText(/in development it arrives in the local Mailpit inbox/)).toBeDefined();
    cleanup();
    vi.unstubAllEnvs(); // also in afterEach, should the first check fail
    render(<UsersView live={live()} />);
    expect(screen.getByText("The set-up e-mail comes from the sign-in service.")).toBeDefined();
  });
});
