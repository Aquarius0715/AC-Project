// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, fireEvent, render, screen, within } from "@testing-library/react";

// The Phase 1A demo of Users (FR-C19, IR417, IR461; DATA_SOURCE≠API): the seed's three users, adding a user with the
// local checks, resending the set-up e-mail, role and status changes and removing, all applied at once; the store
// refuses to demote, disable or remove the last active owner. A member gets Page unavailable.
const toast = vi.hoisted(() => vi.fn());
vi.mock("@ac/web/components/ui", async (importOriginal) => ({ ...(await importOriginal<Record<string, unknown>>()), useToast: () => toast }));

import { UsersDemo } from "../app/customer/users/_components/users-demo";
import { CURRENT_CLIENT, clientUserActions } from "@ac/web/lib/clientUsers";

const rowOf = (text: string) => {
  const found = screen.getAllByRole("row").find((r) => r.textContent.includes(text));
  if (!found) throw new Error(`no row ${text}`);
  return within(found);
};
const names = () => screen.getAllByRole("row").slice(1).map((r) => r.querySelector("b")?.textContent);
const dialog = () => within(screen.getByRole("dialog"));
const add = () => { fireEvent.click(dialog().getByRole("button", { name: "Add user" })); };

beforeEach(() => {
  toast.mockReset();
  sessionStorage.clear();
  clientUserActions.reset();
  CURRENT_CLIENT.role = "owner";
});
afterEach(() => { cleanup(); });

describe("UsersDemo", () => {
  it("shows Page unavailable to a member", () => {
    CURRENT_CLIENT.role = "member";
    render(<UsersDemo />);
    expect(screen.getByText("Page unavailable")).toBeDefined();
    expect(screen.queryByRole("table")).toBeNull();
  });

  it("lists the seed: my own row has no actions, an invited user shows who invited them", () => {
    render(<UsersDemo />);
    expect(names()).toStrictEqual(["Tan Wei (you)", "Mei Tan", "guest@example.com"]);
    expect(rowOf("Tan Wei").getByText("Your account")).toBeDefined();
    expect(rowOf("Tan Wei").queryAllByRole("button")).toHaveLength(0);
    expect(rowOf("guest@").getByText("invited 2026-09-29 by hq-operator")).toBeDefined();
    expect(rowOf("guest@").getByText("Not signed in yet")).toBeDefined();
    expect(rowOf("guest@").getByText("—")).toBeDefined();
    expect(rowOf("guest@").queryAllByRole("button").map((b) => b.textContent)).toStrictEqual(["Resend set-up e-mail", "Make owner", "Remove"]);
    expect(rowOf("Mei Tan").queryAllByRole("button").map((b) => b.textContent)).toStrictEqual(["Make owner", "Disable", "Remove"]);
  });

  it("adds a user only with a name and a new address, and Cancel clears the form", () => {
    render(<UsersDemo />);
    fireEvent.click(screen.getByRole("button", { name: "Add user" }));
    add();
    expect(dialog().getByText("A name of 1–120 characters")).toBeDefined();
    expect(dialog().getByText("Enter an e-mail address")).toBeDefined();
    fireEvent.change(dialog().getByLabelText(/^Name/), { target: { value: " Lim Ka Wai " } });
    fireEvent.change(dialog().getByLabelText(/^Email/), { target: { value: "MEI.TAN@example.com" } });
    add();
    expect(dialog().getByText(/This person is already a user of customer-a \(Mei Tan · Member\)/)).toBeDefined();
    expect(toast).not.toHaveBeenCalled();
    fireEvent.click(dialog().getByRole("button", { name: "Cancel" }));
    fireEvent.click(screen.getByRole("button", { name: "Add user" }));
    expect(dialog().getByLabelText<HTMLInputElement>(/^Name/).value).toBe("");
    expect(dialog().queryByText("A name of 1–120 characters")).toBeNull();
    fireEvent.change(dialog().getByLabelText(/^Name/), { target: { value: " Lim Ka Wai " } });
    fireEvent.change(dialog().getByLabelText(/^Email/), { target: { value: "lim@example.com" } });
    fireEvent.change(dialog().getByRole("combobox", { name: "Role" }), { target: { value: "owner" } });
    add();
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(toast).toHaveBeenCalledWith("Lim Ka Wai added — nothing is sent in the demo");
    expect(rowOf("Lim Ka Wai").getByText("Owner")).toBeDefined();
    expect(rowOf("Lim Ka Wai").getByText("Not signed in yet")).toBeDefined();
  });

  it("a name over 120 characters is refused, and nothing is added", () => {
    render(<UsersDemo />);
    fireEvent.click(screen.getByRole("button", { name: "Add user" }));
    fireEvent.change(dialog().getByLabelText(/^Name/), { target: { value: "x".repeat(121) } });
    fireEvent.change(dialog().getByLabelText(/^Email/), { target: { value: "lim@example.com" } });
    add();
    expect(dialog().getByText("A name of 1–120 characters")).toBeDefined();
    fireEvent.click(dialog().getByRole("button", { name: "Cancel" }));
    expect(names()).toHaveLength(3);
    expect(toast).not.toHaveBeenCalled();
  });

  it("resends the set-up e-mail, changes role and status, and removes at once", () => {
    render(<UsersDemo />);
    fireEvent.click(rowOf("guest@").getByRole("button", { name: "Resend set-up e-mail" }));
    expect(toast).toHaveBeenLastCalledWith("Set-up e-mail re-sent to guest@example.com — nothing is sent in the demo", undefined);
    fireEvent.click(rowOf("Mei Tan").getByRole("button", { name: "Make owner" }));
    expect(toast).toHaveBeenLastCalledWith("Mei Tan is now an owner", undefined);
    expect(rowOf("Mei Tan").getByText("Owner")).toBeDefined();
    fireEvent.click(rowOf("Mei Tan").getByRole("button", { name: "Make member" }));
    expect(toast).toHaveBeenLastCalledWith("Mei Tan is now a member", undefined);
    fireEvent.click(rowOf("Mei Tan").getByRole("button", { name: "Disable" }));
    expect(toast).toHaveBeenLastCalledWith("Mei Tan disabled", undefined);
    expect(rowOf("Mei Tan").getByText("Disabled")).toBeDefined();
    fireEvent.click(rowOf("Mei Tan").getByRole("button", { name: "Enable" }));
    expect(toast).toHaveBeenLastCalledWith("Mei Tan enabled again", undefined);
    fireEvent.click(rowOf("guest@").getByRole("button", { name: "Make owner" }));
    expect(toast).toHaveBeenLastCalledWith("guest@example.com is now an owner", undefined);
    fireEvent.click(rowOf("guest@").getByRole("button", { name: "Remove" }));
    expect(toast).toHaveBeenLastCalledWith("guest@example.com removed", undefined);
    expect(names()).toStrictEqual(["Tan Wei (you)", "Mei Tan"]);
  });

  it("the store refuses to leave the customer without an active owner, and the toast says why", () => {
    render(<UsersDemo />);
    // Mei becomes the only owner besides me; once I am disabled, she is the last active owner
    fireEvent.click(rowOf("Mei Tan").getByRole("button", { name: "Make owner" }));
    act(() => { clientUserActions.setStatus(CURRENT_CLIENT.userId, "disabled"); });
    fireEvent.click(rowOf("Mei Tan").getByRole("button", { name: "Make member" }));
    expect(toast).toHaveBeenLastCalledWith("CONFLICT — the last active owner cannot be demoted", "warn");
    fireEvent.click(rowOf("Mei Tan").getByRole("button", { name: "Disable" }));
    expect(toast).toHaveBeenLastCalledWith("CONFLICT — the last active owner cannot be disabled", "warn");
    fireEvent.click(rowOf("Mei Tan").getByRole("button", { name: "Remove" }));
    expect(toast).toHaveBeenLastCalledWith("CONFLICT — the last active owner cannot be removed", "warn");
    expect(rowOf("Mei Tan").getByText("Owner")).toBeDefined();
    expect(rowOf("Mei Tan").getByText("Active")).toBeDefined();
  });
});
