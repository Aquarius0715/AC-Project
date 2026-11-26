import { beforeEach, describe, expect, it, vi } from "vitest";
import { isValidElement, type ComponentProps, type ReactNode } from "react";
import type { ApiMember } from "@ac/web/lib/members";

// HQ access & roles (FR-A03, SCR-A03): the Server Actions (save at the version read, revoke at the business clock,
// delete with a reason and force, sign out everywhere) and the Server Component (the URL filters, the users a new
// membership may be for, and the membership the URL opens).
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
  return {
    CoreError, apiMode: vi.fn(() => true), coreNow: vi.fn(() => Promise.resolve(new Date("2026-09-14T01:00:00Z"))),
    coreDisplay: vi.fn(() => Promise.resolve({ locale: "en", timeZone: "Asia/Kuala_Lumpur" })),
    corePermissions: vi.fn(() => Promise.resolve(new Set(["identity.read", "identity.write", "asset.read"]))),
    corePrincipal: vi.fn(() => Promise.resolve({ userId: "user-me", tenantId: "tenant-1" })),
    coreOp: vi.fn<(op: string, input?: unknown, options?: unknown) => Promise<unknown>>(),
    coreAll: vi.fn<(op: string, input?: unknown) => Promise<unknown>>(),
  };
});
const cache = vi.hoisted(() => ({ refresh: vi.fn() }));
vi.mock("server-only", () => ({}));
vi.mock("next/cache", () => cache);
vi.mock("next/server", () => ({ connection: () => Promise.resolve() }));
vi.mock("@ac/web/lib/dal", () => dal);
vi.mock("../app/admin/settings/access/_components/access-view", () => ({ AccessView: () => null }));
vi.mock("../app/admin/settings/access/_components/access-demo", () => ({ AccessDemo: () => null }));

import { deleteMember, revokeMember, saveMember, signOutEverywhere } from "../app/admin/settings/access/actions";
import AdminAccessPage from "../app/admin/settings/access/page";
import { AccessView } from "../app/admin/settings/access/_components/access-view";
import { AccessDemo } from "../app/admin/settings/access/_components/access-demo";
import AccessLoading from "../app/admin/settings/access/loading";
import AccessErrorState from "../app/admin/settings/access/error";
import { RouteError, RouteLoading } from "@ac/web/components/RouteStates";

const refused = (code: string) => new dal.CoreError({ code, messageKey: `error.${code.toLowerCase()}`, fieldErrors: {}, retryAfterSeconds: null });
const member = (id: string, over: Partial<ApiMember> = {}): ApiMember => ({
  id, version: 2, updatedAt: "2026-09-01T00:00:00Z", userId: `user-${id}`, displayName: `Person ${id}`, organizationId: "org-hq", role: "admin", employment: null,
  permissions: ["dashboard.read"], scopes: [], scopeVersion: 1, validFrom: "2026-01-01T00:00:00Z", validUntil: null, ...over,
});
const input = { organizationId: "org-hq", role: "admin" as const, employment: null, userId: "user-1", permissions: ["dashboard.read" as const], scopes: [], validFrom: "2026-01-01T00:00:00Z", validUntil: null, reason: "x" };

beforeEach(() => {
  dal.apiMode.mockReturnValue(true);
  dal.coreOp.mockReset(); dal.coreAll.mockReset(); cache.refresh.mockReset();
});

describe("access Server Actions", () => {
  it("saves a new membership without a version and a saved one at its version, and revokes it at the business clock", async () => {
    dal.coreOp.mockResolvedValue({ id: "m-1", version: 3 });
    expect(await saveMember(input)).toStrictEqual({ ok: true, value: { id: "m-1", version: 3 } });
    expect(JSON.stringify(dal.coreOp.mock.lastCall?.[2])).toBe('{"write":true}');
    await saveMember({ ...input, id: "m-1" }, 2);
    expect(dal.coreOp.mock.lastCall?.[2]).toStrictEqual({ write: true, expectedVersion: 2 });
    await revokeMember({ ...input, id: "m-1" }, 2);
    expect(dal.coreOp.mock.lastCall?.[1]).toMatchObject({ id: "m-1", validUntil: "2026-09-14T01:00:00.000Z" }); // when HQ confirms, not when the page loaded
    dal.coreOp.mockRejectedValueOnce(refused("FORBIDDEN"));
    expect(await saveMember(input)).toMatchObject({ ok: false, code: "FORBIDDEN" });
    expect(cache.refresh).toHaveBeenCalledTimes(4);
  });

  it("deletes an account with a trimmed reason, forced or not, and signs a user out everywhere", async () => {
    dal.coreOp.mockResolvedValue({});
    expect(await deleteMember("m-1", 2, " Mistake ")).toStrictEqual({ ok: true, value: null });
    expect(dal.coreOp.mock.lastCall).toStrictEqual(["members.delete", { id: "m-1", force: false, reason: "Mistake" }, { write: true, expectedVersion: 2 }]);
    await deleteMember("m-1", 2, "History", true);
    expect(dal.coreOp.mock.lastCall?.[1]).toStrictEqual({ id: "m-1", force: true, reason: "History" });
    expect(await signOutEverywhere("m-1", " Lost phone ")).toStrictEqual({ ok: true, value: null });
    expect(dal.coreOp.mock.lastCall).toStrictEqual(["members.revokeSessions", { membershipId: "m-1", reason: "Lost phone" }, { write: true }]);
    dal.coreOp.mockRejectedValueOnce(refused("CONFLICT")).mockRejectedValueOnce(refused("NOT_FOUND"));
    expect(await deleteMember("m-1", 2, "x")).toMatchObject({ ok: false, code: "CONFLICT" });
    expect(await signOutEverywhere("m-1", "x")).toMatchObject({ ok: false, code: "NOT_FOUND" });
    expect(cache.refresh).toHaveBeenCalledTimes(5);
  });
});

const props = (sp: Record<string, string> = {}) => ({ searchParams: Promise.resolve(sp), params: Promise.resolve({}) });
type Live = ComponentProps<typeof AccessView>["live"];
async function liveOf(sp: Record<string, string> = {}): Promise<Live> {
  const page: ReactNode = await AdminAccessPage(props(sp));
  if (!isValidElement<ComponentProps<typeof AccessView>>(page) || page.type !== AccessView) throw new TypeError("not AccessView");
  return page.props.live;
}
const everyone = [
  member("active"), member("ended", { validUntil: "2026-06-01T00:00:00Z" }), member("revoked", { validUntil: "2026-08-01T00:00:00Z", revokedAt: "2026-08-01T00:00:00Z" }),
  member("client", { role: "client" }), member("second", { userId: "user-active" }),
];
function core(listed: ApiMember[] = everyone) {
  dal.coreAll.mockImplementation((op, query) => {
    if (op === "members.list") return Promise.resolve(query === undefined ? everyone : listed);
    const pages = new Map<string, unknown[]>([
      ["organizations.list", [{ id: "org-hq", name: "HQ", kind: "operator", status: "active" }]], ["properties.list", [{ id: "p-1", name: "Tower" }]],
      ["units.list", [{ id: "u-1", displayName: "Lobby AC" }]],
    ]);
    return Promise.resolve(pages.get(op) ?? []);
  });
}

describe("the access page (Server Component)", () => {
  it("shows the demo outside API mode, and its loading and error states", async () => {
    dal.apiMode.mockReturnValue(false);
    const page: ReactNode = await AdminAccessPage(props());
    expect(isValidElement(page) && page.type).toBe(AccessDemo);
    expect(AccessLoading().type).toBe(RouteLoading);
    const error = new Error("down"), retry = vi.fn();
    const failed = AccessErrorState({ error, retry });
    expect(failed.type === RouteError && failed.props).toMatchObject({ error, retry });
  });

  it("lists HQ, contractor and technician memberships, active first, with the users once each, nothing open by default", async () => {
    core();
    const live = await liveOf();
    expect(dal.coreAll).toHaveBeenCalledWith("members.list", { filters: {} });
    expect(dal.coreAll).not.toHaveBeenCalledWith("members.list"); // unfiltered: the list itself holds every user
    expect(live.rows.map((r) => r.id)).toStrictEqual(["active", "second", "ended", "revoked"]);
    expect(live.users.map((u) => u.id)).toStrictEqual(["user-active", "user-ended", "user-revoked"]); // no client
    expect([live.selected, live.canWrite, live.canForce, live.selfUserId, live.tenantId, live.units]).toStrictEqual(["none", true, false, "user-me", "tenant-1", [{ id: "u-1", name: "Lobby AC" }]]);
  });

  it("filters by role, status and organization in the URL, offering every user to a new membership", async () => {
    core([member("t-1", { role: "technician", employment: "external", organizationId: "org-c" })]);
    const live = await liveOf({ role: "technician", status: "active", organizationId: "org-c", membershipId: "new" });
    expect(dal.coreAll).toHaveBeenCalledWith("members.list", { filters: { role: "technician", activeOnly: true, organizationId: "org-c" } });
    expect([live.selected, live.scope, live.users.length]).toStrictEqual(["new", { role: "technician", status: "active", organizationId: "org-c" }, 3]);
    core();
    const [inactive, revoked, bogus] = await Promise.all([liveOf({ status: "inactive" }), liveOf({ status: "revoked" }), liveOf({ role: "client", status: "bogus" })]);
    expect(inactive.rows.map((r) => r.id)).toStrictEqual(["ended"]);
    expect(revoked.rows.map((r) => r.id)).toStrictEqual(["revoked"]);
    expect(Object.values(bogus.scope).filter(Boolean)).toStrictEqual([]); // no filter from values the screen does not offer
  });

  it("opens the URL's membership, says when the list does not hold it, and opens a new form on an empty list", async () => {
    core();
    const [asked, gone] = await Promise.all([liveOf({ membershipId: "ended" }), liveOf({ membershipId: "gone" })]);
    expect([asked.selected, gone.selected]).toStrictEqual(["ended", "missing"]);
    core([]);
    const empty = await liveOf({ status: "inactive", role: "admin" });
    expect(empty.selected).toBe("new");
  });
});
