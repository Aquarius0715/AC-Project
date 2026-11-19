// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import type { ActionFailure } from "@ac/web/lib/actionMessage";
import { partnerUserDemo } from "@ac/web/lib/partnerUsersDemo";

// The Users demo (FR-P11): contractor-a's accounts in the tab's store, the status filter, the store's changes, and
// its refusals answered with the API's codes
const toasts = vi.hoisted((): { texts: string[]; failed: ActionFailure[] } => ({ texts: [], failed: [] }));
type Result = { ok: true; value: null } | ({ ok: false } & ActionFailure);
vi.mock("@ac/web/lib/useAction", () => ({
  useAction: () => [false, (fn: () => Promise<Result>, ok: string, after?: () => void, failed?: (f: ActionFailure) => void) => {
    void fn().then((r) => {
      if (r.ok) { toasts.texts.push(ok); after?.(); } else { toasts.failed.push(r); failed?.(r); }
    });
  }] as const,
}));

import { UsersDemo } from "../app/partner/users/_components/users-demo";

beforeEach(() => { partnerUserDemo.reset(); toasts.texts = []; toasts.failed = []; });
afterEach(() => { cleanup(); vi.restoreAllMocks(); });

const rowOf = (text: string) => {
  const row = screen.getAllByRole("row").find((r) => r.textContent.includes(text));
  if (!row) throw new TypeError(`no row ${text}`);
  return within(row);
};
const box = (name: RegExp) => screen.getByRole("textbox", { name });
const submit = (name: string) => {
  const b = within(screen.getByRole("dialog")).getAllByRole("button", { name }).at(-1);
  if (!b) throw new TypeError(`no ${name}`);
  fireEvent.click(b);
};
/** Adds Lim (lim@example.com), a technician with the access end offered first. */
const addLim = () => {
  fireEvent.click(screen.getByRole("button", { name: "Add user" }));
  fireEvent.change(box(/^Name/), { target: { value: "Lim" } });
  fireEvent.change(box(/^Email/), { target: { value: "lim@example.com" } });
  submit("Add user");
};

describe("Users demo", () => {
  it("lists the seed, filters by status, and adds, disables, moves the access of and removes in the store, and re-sends without sending", async () => {
    render(<UsersDemo />);
    expect(screen.getByRole("heading", { name: "Users of contractor-a" })).toBeTruthy();
    expect(rowOf("contractor-a@ac.local").getByText("contractor-a (you)")).toBeTruthy();
    expect(screen.getByText("Nothing is sent in the demo.")).toBeTruthy();
    fireEvent.change(screen.getByRole("combobox", { name: "Status" }), { target: { value: "invited" } });
    expect(screen.getAllByRole("row").filter((r) => r.textContent.includes("@"))).toHaveLength(1);
    expect(rowOf("new.tech@example.com").getByText("New Tech")).toBeTruthy();
    fireEvent.change(screen.getByRole("combobox", { name: "Status" }), { target: { value: "" } });
    addLim();
    await waitFor(() => { expect(screen.queryByRole("dialog")).toBeNull(); });
    expect(rowOf("lim@example.com").getAllByText("Not signed in yet")).toHaveLength(2);
    fireEvent.click(rowOf("lim@example.com").getByRole("button", { name: "Resend set-up e-mail" }));
    fireEvent.click(rowOf("tech-external-a@").getByRole("button", { name: "Disable" }));
    fireEvent.change(box(/^Reason/), { target: { value: "on leave" } });
    submit("Disable");
    await waitFor(() => { expect(rowOf("tech-external-a@").getAllByText("Disabled")).toHaveLength(2); });
    fireEvent.click(rowOf("lim@example.com").getByRole("button", { name: "Access" }));
    fireEvent.change(box(/^Reason/), { target: { value: "longer" } });
    submit("Save access");
    await waitFor(() => { expect(screen.queryByRole("dialog")).toBeNull(); });
    fireEvent.click(rowOf("lim@example.com").getByRole("button", { name: "Remove" }));
    fireEvent.change(box(/^Reason/), { target: { value: "typo" } });
    submit("Remove");
    await waitFor(() => { expect(screen.queryByText("lim@example.com")).toBeNull(); });
    expect(toasts.texts).toStrictEqual([
      "Lim added — nothing is sent in the demo",
      "Set-up e-mail re-sent to lim@example.com — nothing is sent in the demo",
      "tech-external-a disabled — they can no longer sign in",
      expect.stringMatching(/^Access of Lim now ends \d{4}-\d{2}-\d{2}$/),
      "Lim removed",
    ]);
    expect(toasts.failed).toStrictEqual([]);
  });

  it("answers the store's refusals with the API's codes: a taken address on its field, a missing user, any other refusal as a conflict", async () => {
    vi.spyOn(partnerUserDemo, "invite").mockReturnValueOnce("errors.email_taken");
    vi.spyOn(partnerUserDemo, "setStatus").mockReturnValueOnce("errors.last_contractor");
    render(<UsersDemo />);
    addLim();
    await waitFor(() => { expect(box(/^Email/).getAttribute("aria-invalid")).toBe("true"); });
    fireEvent.click(within(screen.getByRole("dialog")).getByRole("button", { name: "Cancel" }));
    await waitFor(() => { expect(screen.queryByRole("dialog")).toBeNull(); });
    fireEvent.click(rowOf("tech-external-a@").getByRole("button", { name: "Disable" }));
    fireEvent.change(box(/^Reason/), { target: { value: "on leave" } });
    submit("Disable");
    await waitFor(() => { expect(toasts.failed).toHaveLength(2); });
    vi.spyOn(partnerUserDemo, "remove").mockReturnValueOnce("error.notFound");
    fireEvent.click(within(screen.getByRole("dialog")).getByRole("button", { name: "Cancel" }));
    await waitFor(() => { expect(screen.queryByRole("dialog")).toBeNull(); });
    fireEvent.click(rowOf("new.tech@example.com").getByRole("button", { name: "Remove" }));
    fireEvent.change(box(/^Reason/), { target: { value: "gone" } });
    submit("Remove");
    await waitFor(() => { expect(toasts.failed).toHaveLength(3); });
    expect(toasts.failed).toStrictEqual([
      { ok: false, code: "VALIDATION", messageKey: "errors.email_taken", fieldErrors: { email: "errors.email_taken" } },
      { ok: false, code: "CONFLICT", messageKey: "errors.last_contractor", fieldErrors: {} },
      { ok: false, code: "NOT_FOUND", messageKey: "error.notFound", fieldErrors: {} },
    ]);
    expect(toasts.texts).toStrictEqual([]);
  });
});
