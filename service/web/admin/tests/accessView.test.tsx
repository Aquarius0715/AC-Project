// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import type { ComponentProps } from "react";
import { memberRows, type ApiMember, type ApiOrganization } from "@ac/web/lib/members";
import type { ActionFailure } from "@ac/web/lib/actionMessage";

// The HQ access manager (FR-A03, DD-A03): the filters and the selected membership in the URL, the membership form (an
// existing user or a new person, role defaults, the organization by role and employment, scopes, the valid period,
// the permission matrix with Write including Read and the self-grant rule), the first field to fix on a refused save,
// and revoke / sign out everywhere / delete with a reason. The Phase 1A demo moves locally.
const actions = vi.hoisted(() => ({
  saveMember: vi.fn<(...args: unknown[]) => Promise<unknown>>(), revokeMember: vi.fn<(...args: unknown[]) => Promise<unknown>>(),
  deleteMember: vi.fn<(...args: unknown[]) => Promise<unknown>>(), signOutEverywhere: vi.fn<(...args: unknown[]) => Promise<unknown>>(),
}));
const nav = vi.hoisted(() => ({ patch: vi.fn<(p: Record<string, string | null>) => void>(), toasts: [] as string[] }));
vi.mock("../app/admin/settings/access/actions", () => actions);
vi.mock("@ac/web/lib/useUrlPatch", () => ({ useUrlPatch: () => nav.patch }));
type Result = { ok: true; value: unknown } | ({ ok: false } & ActionFailure);
vi.mock("@ac/web/lib/useAction", () => ({
  useAction: () => [false, (fn: () => Promise<Result>, ok: string | ((v: unknown) => string), after?: (v: unknown) => void) => {
    void fn().then((r) => {
      if (!r.ok) return;
      nav.toasts.push(typeof ok === "string" ? ok : ok(r.value));
      after?.(r.value);
    });
  }] as const,
}));
// the search-selects are lists here: their own search is tested with the UI components
vi.mock("@ac/web/components/ui", async (importOriginal) => ({
  ...(await importOriginal<Record<string, unknown>>()),
  useToast: () => (m: string) => { nav.toasts.push(m); },
  SearchSelect: ({ value, onChange, options, label }: Readonly<{ value: string; onChange: (v: string) => void; options: { id: string; label: string }[]; label: string }>) => (
    <select aria-label={label} value={value} onChange={(e) => { onChange(e.target.value); }}>{options.map((o) => <option key={o.id} value={o.id}>{o.label}</option>)}</select>
  ),
}));
vi.mock("@ac/web/components/TimeSelect", async (importOriginal) => ({
  ...(await importOriginal<Record<string, unknown>>()),
  DateTimeSelect: ({ value, onChange, id, disabled }: Readonly<{ value: string; onChange: (v: string) => void; id?: string; disabled?: boolean }>) => (
    <input id={id} aria-label="date and time" disabled={disabled} value={value} onChange={(e) => { onChange(e.target.value); }} />
  ),
}));

import { AccessView } from "../app/admin/settings/access/_components/access-view";
import { AccessDemo } from "../app/admin/settings/access/_components/access-demo";

type Live = ComponentProps<typeof AccessView>["live"];
const NOW = "2026-09-14T01:00:00Z";
const member = (id: string, over: Partial<ApiMember> = {}): ApiMember => ({
  id, version: 2, updatedAt: "2026-09-01T00:00:00Z", userId: `user-${id}`, displayName: `Person ${id}`, organizationId: "org-hq", role: "admin", employment: null,
  permissions: ["dashboard.read", "asset.read"], scopes: [], scopeVersion: 4, validFrom: "2026-01-01T00:00:00Z", validUntil: null, ...over,
});
const orgs: ApiOrganization[] = [
  { id: "org-hq", name: "HQ", kind: "operator", status: "active" }, { id: "org-c", name: "Cool Partners", kind: "contractor", status: "active" },
  { id: "org-a", name: "Demo Customer A", kind: "customer", status: "active" },
];
const members = [
  member("hq"), member("tech", { role: "technician", employment: "external", organizationId: "org-c", permissions: ["alert.read"], scopes: [{ kind: "unit", id: "u-1" }, { kind: "property", id: "p-gone-1234" }], validUntil: "2026-12-31T16:00:00Z" }),
  member("old", { validUntil: "2026-08-01T00:00:00Z", revokedAt: "2026-08-01T00:00:00Z" }), member("ended", { validUntil: "2026-06-01T00:00:00Z" }),
];
function liveOf(selected: string, over: Partial<Live> = {}): Live {
  return {
    now: NOW, canWrite: true, canForce: false, selfUserId: "user-me", tenantId: "tenant-1", scope: {}, rows: memberRows(members, orgs, new Date(NOW)), selected,
    users: [{ id: "user-me", name: "Me" }, { id: "user-hq", name: "Person hq" }], orgs, properties: [{ id: "p-1", name: "Tower" }], units: [{ id: "u-1", name: "Lobby AC" }, { id: "u-2", name: "Office AC" }], ...over,
  };
}
const button = (name: string | RegExp) => screen.getByRole<HTMLButtonElement>("button", { name });
const dialog = () => within(screen.getByRole("dialog"));
const flush = () => act(() => new Promise<void>((r) => { setTimeout(r, 0); }));
const frame = () => act(() => new Promise<void>((r) => { requestAnimationFrame(() => { r(); }); }));
const permission = (code: string) => screen.getByRole("checkbox", { name: (name) => name.endsWith(code) });
const ok = { ok: true, value: null };
/** A valid-period picker, by its field ID (the Field around a picker is a labelled group). */
function picker(key: "validFrom" | "validUntil"): HTMLInputElement {
  const el = document.getElementById(`member-${key}`);
  if (!(el instanceof HTMLInputElement)) throw new TypeError(`no ${key} picker`);
  return el;
}
const isOn = (code: string) => permission(code).getAttribute("aria-checked") === "true";
const target = () => screen.getByRole<HTMLSelectElement>("combobox", { name: "Target" });
const formRole = (name: string) => screen.getAllByRole("button", { name }).at(-1) ?? document.body; // the form's, after the filter's

beforeEach(() => {
  nav.patch.mockReset(); nav.toasts = [];
  for (const f of Object.values(actions)) f.mockReset();
});
afterEach(cleanup);

describe("the membership list", () => {
  it("filters in the URL, searches the list, and opens a membership", () => {
    render(<AccessView live={liveOf("none")} />);
    expect(screen.getByText("Choose a membership")).toBeDefined();
    expect([...screen.getByLabelText<HTMLSelectElement>("Filter by organization").options].map((o) => o.textContent)).toStrictEqual(["Organization: All", "Cool Partners", "HQ"]);
    fireEvent.change(screen.getByLabelText("Filter by organization"), { target: { value: "org-c" } });
    expect(nav.patch).toHaveBeenLastCalledWith({ organizationId: "org-c", membershipId: null });
    fireEvent.change(screen.getByLabelText("Filter by organization"), { target: { value: "" } });
    expect(nav.patch).toHaveBeenLastCalledWith({ organizationId: null, membershipId: null });
    fireEvent.click(button("Technician"));
    expect(nav.patch).toHaveBeenLastCalledWith({ role: "technician", membershipId: null });
    fireEvent.click(button("Revoked"));
    expect(nav.patch).toHaveBeenLastCalledWith({ status: "revoked", membershipId: null });
    expect(screen.getByText("Inactive", { selector: "[data-slot=badge], span" })).toBeDefined();
    fireEvent.change(screen.getByPlaceholderText("Search name or organization…"), { target: { value: "cool" } });
    expect(screen.queryByText("Person hq")).toBeNull();
    fireEvent.click(button(/Person tech/));
    expect(nav.patch).toHaveBeenLastCalledWith({ membershipId: "tech" });
    fireEvent.change(screen.getByPlaceholderText("Search name or organization…"), { target: { value: "nobody" } });
    expect(screen.getByText("No membership matches these filters.")).toBeDefined();
    fireEvent.click(button(/New membership/));
    expect(nav.patch).toHaveBeenLastCalledWith({ membershipId: "new" });
  });

  it("goes back to every role and status, and says when the URL's membership is not listed", () => {
    const { rerender } = render(<AccessView live={liveOf("missing", { scope: { role: "admin", status: "active" } })} />);
    fireEvent.click(button("All roles"));
    expect(nav.patch).toHaveBeenLastCalledWith({ role: null, membershipId: null });
    fireEvent.click(button("All"));
    expect(nav.patch).toHaveBeenLastCalledWith({ status: null, membershipId: null });
    expect(screen.getByText("That membership is not in this list")).toBeDefined();
    fireEvent.click(button("Clear the filters"));
    expect(nav.patch).toHaveBeenLastCalledWith({ role: null, status: null, organizationId: null });
    rerender(<AccessView live={liveOf("missing")} />);
    expect(screen.queryByRole("button", { name: "Clear the filters" })).toBeNull(); // no filter hides it
  });
});

describe("a saved membership", () => {
  it("edits an HQ membership's permissions, Write including Read, and saves at its version", async () => {
    actions.saveMember.mockResolvedValue({ ok: true, value: { id: "hq", version: 3 } });
    render(<AccessView live={liveOf("hq")} />);
    expect(screen.getByText(/· scope version 4 · version 2$/)).toBeDefined();
    expect(screen.getByText("Admins act on every customer of the tenant.", { exact: false })).toBeDefined();
    expect(screen.getByLabelText<HTMLSelectElement>(/^User/).disabled).toBe(true); // fixed after creation
    fireEvent.click(permission("asset.write"));
    fireEvent.click(permission("asset.read"));
    expect([isOn("asset.write"), isOn("asset.read")]).toStrictEqual([true, true]); // Read stays on while Write is on
    fireEvent.click(permission("asset.write"));
    expect([isOn("asset.write"), isOn("asset.read")]).toStrictEqual([false, true]);
    fireEvent.click(permission("dashboard.read"));
    fireEvent.click(permission("billing.payment"));
    fireEvent.change(picker("validUntil"), { target: { value: "2027-01-01T00:00" } });
    fireEvent.change(screen.getByLabelText(/^Change reason/), { target: { value: " Payments desk " } });
    fireEvent.click(button("Save changes"));
    await flush();
    expect(actions.saveMember).toHaveBeenCalledWith(expect.objectContaining({ id: "hq", role: "admin", permissions: ["asset.read", "billing.payment"], scopes: [{ kind: "tenant", id: "tenant-1" }], validUntil: "2026-12-31T16:00:00.000Z", reason: "Payments desk" }), 2);
    expect(nav.toasts).toStrictEqual(["Access saved — open screens pick up the new scope on their next action"]);
    expect(nav.patch).toHaveBeenLastCalledWith({ membershipId: "hq" });
  });

  it("focuses the first field to fix on a refused save", async () => {
    render(<AccessView live={liveOf("hq")} />);
    fireEvent.change(picker("validUntil"), { target: { value: "2025-01-01T00:00" } });
    fireEvent.click(button("Save changes"));
    await frame();
    expect(document.activeElement?.id).toBe("member-validUntil");
    expect(screen.getByText("Must be after valid from")).toBeDefined();
    expect(screen.getByText("A change reason is required (1–1000 characters)")).toBeDefined();
    expect(actions.saveMember).not.toHaveBeenCalled();
  });

  it("edits a technician's scopes, keeping their target names, and needs an end date for an external one", async () => {
    actions.saveMember.mockResolvedValue({ ok: true, value: { id: "tech", version: 3 } });
    render(<AccessView live={liveOf("tech")} />);
    expect(screen.getByText("Technician · external")).toBeDefined();
    expect(screen.getByText("Unit · Lobby AC")).toBeDefined();
    expect(screen.getByText("Property · p-gone-1")).toBeDefined(); // a target no longer listed: its ID
    expect([...target().options].map((o) => o.value)).toStrictEqual(["u-2"]); // not added twice
    fireEvent.change(screen.getByLabelText(/^Add scope/), { target: { value: "organization" } });
    expect([...target().options].map((o) => o.textContent)).toStrictEqual(["Demo Customer A"]);
    expect(button("Add").disabled).toBe(true);
    fireEvent.change(target(), { target: { value: "org-a" } });
    fireEvent.click(button("Add"));
    expect(screen.getByText("Customer organization · Demo Customer A")).toBeDefined();
    fireEvent.change(screen.getByLabelText(/^Add scope/), { target: { value: "property" } });
    expect([...target().options].map((o) => o.textContent)).toStrictEqual(["Tower"]);
    const removes = screen.getAllByRole("button", { name: "Remove scope" });
    for (const x of removes) fireEvent.click(x);
    expect(screen.getByText("No targets — zero business targets.")).toBeDefined();
    fireEvent.change(picker("validUntil"), { target: { value: "" } });
    expect(screen.getByText("Required for external technicians")).toBeDefined();
    fireEvent.change(screen.getByLabelText(/^Change reason/), { target: { value: "Moved" } });
    fireEvent.click(button("Save changes"));
    await frame();
    expect(screen.getByText("Add at least one unit, property or customer organization")).toBeDefined();
    expect(screen.getByText("External technicians need an end date")).toBeDefined();
    expect(document.activeElement?.id).toBe("member-scopes");
    expect(actions.saveMember).not.toHaveBeenCalled();
  });

  it("revokes active access with a reason, signs the user out everywhere, and deletes the account with force", async () => {
    actions.revokeMember.mockResolvedValue({ ok: true, value: { id: "tech", version: 3 } });
    actions.signOutEverywhere.mockResolvedValue(ok);
    actions.deleteMember.mockResolvedValue(ok);
    render(<AccessView live={liveOf("tech", { canForce: true })} />);
    fireEvent.click(button("Revoke access…"));
    expect(dialog().getByText(/A job assigned to this technician that has not started is released/)).toBeDefined();
    expect(dialog().getByRole<HTMLButtonElement>("button", { name: "Revoke" }).disabled).toBe(true);
    fireEvent.change(dialog().getByLabelText(/^Reason/), { target: { value: "Left the company" } });
    fireEvent.click(dialog().getByRole("button", { name: "Revoke" }));
    await flush();
    expect(actions.revokeMember).toHaveBeenCalledWith(expect.objectContaining({ id: "tech", reason: "Left the company", validFrom: "2026-01-01T00:00:00Z" }), 2);
    expect(screen.queryByRole("dialog")).toBeNull();
    fireEvent.click(button("Revoke access…"));
    fireEvent.click(dialog().getByRole("button", { name: "Cancel" }));
    fireEvent.click(button("Sign out everywhere…"));
    fireEvent.change(dialog().getByLabelText(/^Reason/), { target: { value: "Lost phone" } });
    fireEvent.click(dialog().getByRole("button", { name: "Sign out everywhere" }));
    await flush();
    expect(actions.signOutEverywhere).toHaveBeenCalledWith("tech", "Lost phone");
    fireEvent.click(button("Sign out everywhere…"));
    fireEvent.click(dialog().getByRole("button", { name: "Cancel" }));
    fireEvent.click(button("Delete account…"));
    fireEvent.click(dialog().getByRole("checkbox", { name: /Force delete/ }));
    fireEvent.change(dialog().getByLabelText(/^Type Person tech to confirm/), { target: { value: "Person tech" } });
    fireEvent.change(dialog().getByLabelText(/^Reason/), { target: { value: "Duplicate" } });
    fireEvent.click(dialog().getByRole("button", { name: "Delete" }));
    await flush();
    expect(actions.deleteMember).toHaveBeenCalledWith("tech", 2, "Duplicate", true);
    expect(nav.patch).toHaveBeenLastCalledWith({ membershipId: null });
    fireEvent.click(button("Delete account…"));
    fireEvent.click(dialog().getByRole("button", { name: "Cancel" }));
    expect(nav.toasts).toStrictEqual(["Access revoked — the membership is now inactive", "Signed out everywhere — every session of Person tech has ended", "Account deleted"]);
  });

  it("refuses a permission the role may not hold, moves the start, closes dialogs with Escape, and deletes without force", async () => {
    actions.deleteMember.mockResolvedValue(ok);
    const odd = member("odd", { role: "technician", employment: "internal", permissions: ["alert.read", "asset.read"], scopes: [{ kind: "unit", id: "u-1" }] });
    render(<AccessView live={liveOf("odd", { rows: memberRows([odd], orgs, new Date(NOW)) })} />);
    fireEvent.change(picker("validFrom"), { target: { value: "2026-02-01T00:00" } });
    fireEvent.change(screen.getByLabelText(/^Change reason/), { target: { value: "x" } });
    fireEvent.click(button("Save changes"));
    expect(screen.getByText("Not allowed for this role: asset.read")).toBeDefined();
    for (const name of ["Revoke access…", "Sign out everywhere…"]) {
      fireEvent.click(button(name));
      fireEvent.keyDown(screen.getByRole("dialog"), { key: "Escape" });
      expect(screen.queryByRole("dialog")).toBeNull();
    }
    fireEvent.click(button("Delete account…"));
    expect(dialog().queryByRole("checkbox", { name: /Force delete/ })).toBeNull(); // no data.forceDelete
    fireEvent.change(dialog().getByLabelText(/^Reason/), { target: { value: "Mistake" } });
    fireEvent.click(dialog().getByRole("button", { name: "Delete" }));
    await flush();
    expect(actions.deleteMember).toHaveBeenCalledWith("odd", 2, "Mistake", false);
  });

  it("explains a revoked membership, offers no revoke for ended access, and is read only without identity.write", () => {
    const { rerender } = render(<AccessView live={liveOf("old")} />);
    expect(screen.getByText(/^Access revoked .*: the sign-in is off/)).toBeDefined();
    expect(screen.queryByRole("button", { name: "Revoke access…" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Sign out everywhere…" })).toBeNull();
    rerender(<AccessView live={liveOf("hq", { canWrite: false })} />);
    expect(screen.getByText("Read only — editing needs “Edit users & roles”.")).toBeDefined();
    for (const name of [/New membership/, "Save changes", "Revoke access…", "Delete account…", "Add"]) expect(screen.queryByRole("button", { name })).toBeNull();
    expect(picker("validFrom").disabled).toBe(true);
  });
});

describe("a new membership", () => {
  it("creates a contractor membership for an existing user, the organization as its scope", async () => {
    actions.saveMember.mockResolvedValue({ ok: true, value: { id: "m-new", version: 1 } });
    render(<AccessView live={liveOf("new")} />);
    expect(screen.getByText("Contractors act for their own organization.")).toBeDefined();
    fireEvent.click(button("Create membership"));
    await frame();
    expect(document.activeElement?.id).toBe("member-userId");
    expect(screen.getByText("Choose a user")).toBeDefined();
    expect(screen.getByText("Choose the organization")).toBeDefined();
    fireEvent.change(screen.getByLabelText(/^User/), { target: { value: "user-hq" } });
    expect([...screen.getByLabelText<HTMLSelectElement>(/^Organization/).options].map((o) => o.textContent)).toStrictEqual(["Select…", "Cool Partners"]);
    fireEvent.change(screen.getByLabelText(/^Organization/), { target: { value: "org-c" } });
    expect(screen.getByText("Contractors act for their own organization (Cool Partners).")).toBeDefined();
    fireEvent.change(screen.getByLabelText(/^Change reason/), { target: { value: "Onboarding" } });
    fireEvent.click(button("Create membership"));
    await flush();
    expect(actions.saveMember).toHaveBeenCalledWith(expect.objectContaining({ userId: "user-hq", organizationId: "org-c", role: "contractor", employment: null, scopes: [{ kind: "organization", id: "org-c" }], permissions: ["partner.accept", "partner.assign", "partner.review"] }), undefined);
    expect(nav.toasts).toStrictEqual(["Membership created"]);
  });

  it("creates a new person with a technician membership, by employment, and refuses an admin permission for a technician", async () => {
    actions.saveMember.mockResolvedValue({ ok: true, value: { id: "m-new", version: 1 } });
    render(<AccessView live={liveOf("new")} />);
    fireEvent.click(button("New person"));
    fireEvent.click(button("Create membership"));
    for (const text of ["A name of 1–120 characters", "A valid email address (up to 254 characters)"]) expect(screen.getByText(text)).toBeDefined();
    fireEvent.change(screen.getByLabelText(/^Name/), { target: { value: " Tia Tech " } });
    fireEvent.change(screen.getByLabelText(/^Email/), { target: { value: "tia@example.com" } });
    fireEvent.click(formRole("Technician"));
    expect([...screen.getByLabelText<HTMLSelectElement>(/^Organization/).options].map((o) => o.textContent)).toStrictEqual(["Select…", "HQ"]); // internal: HQ's field team
    fireEvent.change(screen.getByLabelText(/^Employment/), { target: { value: "external" } });
    expect([...screen.getByLabelText<HTMLSelectElement>(/^Organization/).options].map((o) => o.textContent)).toStrictEqual(["Select…", "Cool Partners"]);
    fireEvent.change(screen.getByLabelText(/^Organization/), { target: { value: "org-c" } });
    fireEvent.change(target(), { target: { value: "u-2" } });
    fireEvent.click(button("Add"));
    fireEvent.change(picker("validUntil"), { target: { value: "2026-12-01T00:00" } });
    fireEvent.change(screen.getByLabelText(/^Change reason/), { target: { value: "Contract crew" } });
    fireEvent.click(button("Create membership"));
    await flush();
    expect(actions.saveMember).toHaveBeenCalledWith(expect.objectContaining({
      userId: "00000000-0000-0000-0000-000000000000", "newUser": { email: "tia@example.com", displayName: "Tia Tech" }, role: "technician", employment: "external", organizationId: "org-c", scopes: [{ kind: "unit", id: "u-2" }],
    }), undefined);
    fireEvent.click(button("Existing user"));
    expect(screen.getByLabelText(/^User/)).toBeDefined();
  });

  it("starts an HQ membership on the tenant with the role defaults, and keeps the self-grant rule", () => {
    render(<AccessView live={liveOf("new")} />);
    fireEvent.click(formRole("Admin"));
    expect(screen.getByText("Admins act on every customer of the tenant.", { exact: false })).toBeDefined();
    expect([isOn("asset.write"), isOn("identity.write"), isOn("restriction.override")]).toStrictEqual([true, false, false]); // the HQ defaults
    fireEvent.change(screen.getByLabelText(/^User/), { target: { value: "user-me" } });
    expect(screen.getByText(/cannot be granted to yourself/)).toBeDefined();
    fireEvent.click(permission("identity.write"));
    expect(isOn("identity.write")).toBe(false); // blocked for oneself
    fireEvent.change(screen.getByLabelText(/^User/), { target: { value: "user-hq" } });
    fireEvent.click(permission("identity.write"));
    fireEvent.click(permission("control.execute"));
    expect([isOn("identity.write"), isOn("identity.read"), isOn("control.execute")]).toStrictEqual([true, true, false]);
    fireEvent.click(formRole("Contractor"));
    expect(isOn("asset.read")).toBe(false); // a contractor's defaults: the partner actions only
    expect(isOn("partner.accept")).toBe(true);
  });
});

describe("the access demo", () => {
  it("checks the reason, the external end date and the self-grant, and saves and revokes locally", () => {
    render(<AccessDemo />);
    const save = () => { fireEvent.click(button("Save changes")); };
    save();
    expect(screen.getByText("Change reason is required (1–1000 characters).")).toBeDefined();
    fireEvent.change(screen.getByLabelText(/^Change reason/), { target: { value: "x" } });
    save();
    expect(screen.getByText("Saved — scopeVersion incremented and recorded in the audit log.")).toBeDefined();
    fireEvent.change(screen.getByPlaceholderText("Search name, email, organization…"), { target: { value: "external-b" } });
    fireEvent.click(button(/^tech-external-b/));
    fireEvent.change(screen.getByLabelText(/^Change reason/), { target: { value: "x" } });
    save();
    expect(screen.getByText(/External technicians need a “Valid until” end date/)).toBeDefined();
    fireEvent.change(screen.getByLabelText(/^Valid until/), { target: { value: "2026-12-31" } });
    save();
    fireEvent.change(screen.getByPlaceholderText("Search name, email, organization…"), { target: { value: "" } });
    fireEvent.click(button(/^hq-operator/));
    fireEvent.click(screen.getByRole("checkbox", { name: "restriction.override" }));
    const demoOn = (code: string) => screen.getByRole("checkbox", { name: code }).getAttribute("aria-checked") === "true";
    fireEvent.click(screen.getByRole("checkbox", { name: "asset.read" })); // locked while Write is on (an HQ default)
    expect([demoOn("asset.write"), demoOn("asset.read")]).toStrictEqual([true, true]);
    fireEvent.click(screen.getByRole("checkbox", { name: "asset.write" }));
    fireEvent.click(screen.getByRole("checkbox", { name: "asset.read" }));
    expect([demoOn("asset.write"), demoOn("asset.read")]).toStrictEqual([false, false]);
    fireEvent.click(screen.getByRole("checkbox", { name: "asset.write" })); // Write turns Read on with it
    expect(demoOn("asset.read")).toBe(true);
    fireEvent.change(screen.getByLabelText(/^Change reason/), { target: { value: "x" } });
    save();
    expect(screen.getByText("FORBIDDEN — you cannot grant yourself restriction.override.")).toBeDefined();
    fireEvent.click(button("Technician"));
    fireEvent.click(button("Revoke access…"));
    fireEvent.click(dialog().getByRole("button", { name: "Cancel" }));
    fireEvent.click(button("Revoke access…"));
    fireEvent.keyDown(screen.getByRole("dialog"), { key: "Escape" });
    fireEvent.click(button("Revoke access…"));
    fireEvent.click(dialog().getByRole("button", { name: "Revoke" }));
    fireEvent.click(button(/New user/));
    expect(screen.queryByRole("button", { name: "Revoke access…" })).toBeNull();
    expect(nav.toasts).toStrictEqual(["Access saved", "Access saved", "Access revoked — user is now Inactive"]);
  });
});
