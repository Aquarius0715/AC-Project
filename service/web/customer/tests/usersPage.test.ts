import { beforeEach, describe, expect, it, vi } from "vitest";
import { isValidElement, type ComponentProps } from "react";
import type { ApiClientUser } from "@ac/web/lib/assets";

// The customer Users page (FR-C19, IR417, IR461): the demo outside API mode, Page unavailable for a member, and for an
// owner the rows of clientUsers.list with the owner's customer, the organization name and every address for the
// duplicate check. Its Server Actions trim the texts, send the role change and the status change at the row version,
// leave the status out of a role change, and re-read the page whatever happens.
const dal = vi.hoisted(() => {
  interface DomainError { code: string; messageKey: string; fieldErrors: Record<string, string>; retryAfterSeconds: number | null }
  class CoreError extends Error {
    readonly error: DomainError;
    constructor(error: DomainError, options?: ErrorOptions) {
      super(error.code, options);
      this.name = "CoreError";
      this.error = error;
    }
  }
  return { coreOp: vi.fn(), apiMode: vi.fn(), coreAll: vi.fn(), corePrincipal: vi.fn(), coreDisplay: vi.fn(), coreIdentity: vi.fn(), CoreError };
});
const cache = vi.hoisted(() => ({ refresh: vi.fn() }));
vi.mock("server-only", () => ({}));
vi.mock("next/cache", () => cache);
vi.mock("next/server", () => ({ connection: () => Promise.resolve() }));
vi.mock("@ac/web/lib/dal", () => dal);
vi.mock("../app/customer/users/_components/users-demo", () => ({ UsersDemo: () => null }));
vi.mock("../app/customer/users/_components/users-view", () => ({ UsersView: () => null }));

import { inviteUser, removeUser, resendInvite, updateUser } from "../app/customer/users/actions";
import CustomerUsersPage from "../app/customer/users/page";
import { UsersDemo } from "../app/customer/users/_components/users-demo";
import { UsersView } from "../app/customer/users/_components/users-view";
import UsersLoading from "../app/customer/users/loading";
import UsersErrorState from "../app/customer/users/error";
import { RouteError, RouteLoading } from "@ac/web/components/RouteStates";

const user = (id: string, over: Partial<ApiClientUser> = {}): ApiClientUser => ({
  id, version: 2, customerId: "cust-a", membershipId: `m-${id}`, email: `${id}@example.com`, displayName: id, clientRole: "member", status: "active",
  lastSignInAt: null, allowedChannels: [], invitedAt: "2026-09-29T20:00:00Z", invitedByMembershipId: "m-hq", ...over,
});
const viewProps = (element: unknown) => {
  if (!isValidElement<ComponentProps<typeof UsersView>>(element) || element.type !== UsersView) throw new TypeError("not UsersView");
  return element.props.live;
};
const refusal = (code: string, fieldErrors: Record<string, string> = {}) => new dal.CoreError({ code, fieldErrors, messageKey: `error.${code}`, retryAfterSeconds: null });

beforeEach(() => {
  vi.resetAllMocks();
  dal.coreOp.mockResolvedValue({ id: "cu-new" });
  dal.apiMode.mockReturnValue(true);
  dal.corePrincipal.mockResolvedValue({ membershipId: "m-me", clientRole: "owner" });
  dal.coreDisplay.mockResolvedValue({ locale: "en", timeZone: "Asia/Kuala_Lumpur" });
  dal.coreIdentity.mockResolvedValue({ displayName: "Tan Wei", organizationName: "Demo Customer A" });
});

describe("users actions", () => {
  it("adds a user with the trimmed address and name, and returns no value", async () => {
    await expect(inviteUser("cust-a", "  lim@example.com ", "owner", " Lim Ka Wai ")).resolves.toStrictEqual({ ok: true, value: null });
    expect(dal.coreOp.mock.lastCall).toStrictEqual(["clientUsers.save", { customerId: "cust-a", clientRole: "owner", email: "lim@example.com", displayName: "Lim Ka Wai" }, { write: true }]);
    expect(cache.refresh).toHaveBeenCalledOnce();
  });

  it("a role change leaves the status out; a status change sends it; both go at the row version with the trimmed reason", async () => {
    await updateUser("cu-1", 4, "cust-a", " mei@example.com", "owner", null, " promoted ");
    expect(dal.coreOp.mock.lastCall).toStrictEqual(["clientUsers.save", { id: "cu-1", customerId: "cust-a", email: "mei@example.com", clientRole: "owner", reason: "promoted" }, { write: true, expectedVersion: 4 }]);
    await updateUser("cu-1", 5, "cust-a", "mei@example.com", "member", "disabled", "left");
    expect(dal.coreOp.mock.lastCall).toStrictEqual(["clientUsers.save", { id: "cu-1", customerId: "cust-a", email: "mei@example.com", clientRole: "member", status: "disabled", reason: "left" }, { write: true, expectedVersion: 5 }]);
  });

  it("removes at the row version and resends the set-up e-mail by id", async () => {
    await removeUser("cu-1", 3, " moved away ");
    expect(dal.coreOp.mock.lastCall).toStrictEqual(["clientUsers.remove", { id: "cu-1", reason: "moved away" }, { write: true, expectedVersion: 3 }]);
    await resendInvite("cu-2");
    expect(dal.coreOp.mock.lastCall).toStrictEqual(["clientUsers.resendInvite", { id: "cu-2" }, { write: true }]);
    expect(cache.refresh).toHaveBeenCalledTimes(2);
  });

  it("a refusal comes back with its code and fields, and the page is re-read", async () => {
    dal.coreOp.mockRejectedValueOnce(refusal("CONFLICT", { email: "taken" }));
    await expect(inviteUser("cust-a", "mei@example.com", "member", "Mei")).resolves.toStrictEqual({
      ok: false, code: "CONFLICT", messageKey: "error.CONFLICT", fieldErrors: { email: "taken" }, retryAfterSeconds: null,
    });
    dal.coreOp.mockRejectedValueOnce(new Error("socket hang up"));
    await expect(removeUser("cu-1", 3, "x")).resolves.toMatchObject({ ok: false, code: "UNAVAILABLE" });
    expect(cache.refresh).toHaveBeenCalledTimes(2);
  });
});

describe("users page", () => {
  it("renders the demo outside API mode without reading the Core API", async () => {
    dal.apiMode.mockReturnValue(false);
    const page = await CustomerUsersPage();
    expect(isValidElement(page) && page.type).toBe(UsersDemo);
    expect(dal.corePrincipal).not.toHaveBeenCalled();
  });

  it("shows Page unavailable to a member without reading the users", async () => {
    dal.corePrincipal.mockResolvedValue({ membershipId: "m-mei", clientRole: "member" });
    expect(viewProps(await CustomerUsersPage())).toBeNull();
    expect(dal.coreAll).not.toHaveBeenCalled();
  });

  it("gives an owner the rows, the own customer, the organization and every address", async () => {
    const users = [
      user("me", { customerId: "cust-own", membershipId: "m-me", clientRole: "owner", displayName: "Tan Wei" }),
      user("mei", { displayName: "Mei Tan", invitedByMembershipId: "m-me" }),
      user("guest", { displayName: null, membershipId: null, status: "invited", invitedByMembershipId: "m-hq" }),
      user("lim", { displayName: null, membershipId: "m-lim", status: "invited", invitedByMembershipId: "m-mei" }),
    ];
    dal.coreAll.mockResolvedValue(users);
    const live = viewProps(await CustomerUsersPage());
    expect(dal.coreAll).toHaveBeenCalledWith("clientUsers.list");
    expect(live?.customerId).toBe("cust-own");
    expect(live?.customer).toBe("Demo Customer A");
    expect(live?.all).toStrictEqual(users.map((u) => ({ id: u.id, email: u.email })));
    const byId = new Map(live?.rows.map((r) => [r.id, r]));
    expect(byId.get("me")).toMatchObject({ you: true, lastOwner: true });
    expect(byId.get("mei")).toMatchObject({ you: false, sub: "mei@example.com" });
    // who invited: HQ when the inviter is not a user of the customer, the user's name, or "you" for the owner who reads
    expect(byId.get("guest")?.sub).toBe("invited 30 Sept 2026 by HQ");
    expect(byId.get("lim")?.sub).toBe("invited 30 Sept 2026 by Mei Tan");
    expect(byId.get("mei")?.sub).not.toContain("invited");
  });

  it("names the inviter you, and takes the customer of the first row when the owner's own row is missing", async () => {
    dal.coreAll.mockResolvedValue([user("lim", { customerId: "cust-b", displayName: null, status: "invited", invitedByMembershipId: "m-me" })]);
    const live = viewProps(await CustomerUsersPage());
    expect(live?.customerId).toBe("cust-b");
    expect(live?.rows[0]?.sub).toBe("invited 30 Sept 2026 by you");
    dal.coreAll.mockResolvedValue([]);
    expect(viewProps(await CustomerUsersPage())?.customerId).toBe("");
  });

  it("names the screen in its loading and error states", () => {
    const error = new Error("boom");
    const retry = vi.fn();
    const loading = UsersLoading();
    expect(loading.type).toBe(RouteLoading);
    expect(loading.props).toStrictEqual({ what: "users" });
    const failed = UsersErrorState({ error, retry });
    expect(failed.type).toBe(RouteError);
    expect(failed.props).toStrictEqual({ what: "Users", error, retry });
  });
});
