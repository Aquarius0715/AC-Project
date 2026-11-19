import { beforeEach, describe, expect, it, vi } from "vitest";
import { isValidElement } from "react";

// The Server Actions of the partner Users page (FR-P11): each write goes to its Core API operation with the trimmed
// texts, the row version where one is needed, and re-reads the page whatever happens. The page renders the demo outside
// API mode, the Page unavailable view for a technician session (FORBIDDEN), and otherwise the rows of the URL's status
// with the own membership marked; its route states name the screen.
const dal = vi.hoisted(() => {
  interface DomainError { code: string; messageKey: string; fieldErrors: Record<string, string>; correlationId?: string; retryAfterSeconds: number | null }
  class CoreError extends Error {
    readonly error: DomainError;
    constructor(error: DomainError | string, options?: ErrorOptions) {
      const e = typeof error === "string" ? { code: error, messageKey: `error.${error.toLowerCase()}`, fieldErrors: {}, retryAfterSeconds: null } : error;
      super(e.code, options);
      this.name = "CoreError";
      this.error = e;
    }
  }
  return {
    coreOp: vi.fn(), apiMode: vi.fn(), coreAll: vi.fn(), corePrincipal: vi.fn(), coreDisplay: vi.fn(), coreIdentity: vi.fn(), coreNow: vi.fn(), CoreError,
  };
});
const cache = vi.hoisted(() => ({ refresh: vi.fn() }));
vi.mock("server-only", () => ({}));
vi.mock("next/cache", () => cache);
vi.mock("next/server", () => ({ connection: () => Promise.resolve() }));
vi.mock("@ac/web/lib/dal", () => dal);
vi.mock("../app/partner/users/_components/users-demo", () => ({ UsersDemo: () => null }));
vi.mock("../app/partner/users/_components/users-live", () => ({ UsersLive: () => null }));

import { redirect } from "next/navigation";
import { invitePartnerUser, removePartnerUser, resendPartnerInvite, setPartnerAccess, setPartnerUserStatus } from "../app/partner/users/actions";
import PartnerUsersPage from "../app/partner/users/page";
import { UsersDemo } from "../app/partner/users/_components/users-demo";
import { UsersLive } from "../app/partner/users/_components/users-live";
import UsersLoading from "../app/partner/users/loading";
import UsersErrorState from "../app/partner/users/error";
import { RouteError, RouteLoading } from "@ac/web/components/RouteStates";
import type { ApiPartnerUser } from "@ac/web/lib/partnerUsers";

beforeEach(() => {
  vi.resetAllMocks();
  dal.coreOp.mockResolvedValue({ id: "pu-1" });
});

const done = { ok: true, value: null };
const save = (body: Record<string, unknown>, options: Record<string, unknown>) => ["partnerUsers.save", body, options];

describe("user actions", () => {
  it("send each write with trimmed texts at the row version, return null and re-read each time", async () => {
    await expect(invitePartnerUser(" lim@example.com ", "technician", " Lim ", "2026-12-31T15:59:59Z")).resolves.toStrictEqual(done);
    expect(dal.coreOp).toHaveBeenLastCalledWith(...save({ email: "lim@example.com", partnerRole: "technician", displayName: "Lim", accessUntil: "2026-12-31T15:59:59Z" }, { write: true }));
    await expect(invitePartnerUser("boss@example.com", "contractor", "Boss", null)).resolves.toStrictEqual(done);
    expect(dal.coreOp).toHaveBeenLastCalledWith(...save({ email: "boss@example.com", partnerRole: "contractor", displayName: "Boss" }, { write: true }));
    await expect(setPartnerAccess("pu-1", 7, "amy@example.com", "technician", "2027-01-31T15:59:59Z", " extended ")).resolves.toStrictEqual(done);
    expect(dal.coreOp).toHaveBeenLastCalledWith(...save({ id: "pu-1", email: "amy@example.com", partnerRole: "technician", accessUntil: "2027-01-31T15:59:59Z", reason: "extended" }, { write: true, expectedVersion: 7 }));
    await expect(setPartnerUserStatus("pu-1", 8, "amy@example.com", "technician", "disabled", " left ")).resolves.toStrictEqual(done);
    expect(dal.coreOp).toHaveBeenLastCalledWith(...save({ id: "pu-1", email: "amy@example.com", partnerRole: "technician", status: "disabled", reason: "left" }, { write: true, expectedVersion: 8 }));
    await expect(removePartnerUser("pu-1", 9, " gone ")).resolves.toStrictEqual(done);
    expect(dal.coreOp).toHaveBeenLastCalledWith("partnerUsers.remove", { id: "pu-1", reason: "gone" }, { write: true, expectedVersion: 9 });
    await expect(resendPartnerInvite("pu-2")).resolves.toStrictEqual(done);
    expect(dal.coreOp).toHaveBeenLastCalledWith("partnerUsers.resendInvite", { id: "pu-2" }, { write: true });
    expect(cache.refresh).toHaveBeenCalledTimes(6);
  });

  it("return a refusal with its code, key, fields and wait, an outage as unavailable, let the sign-in redirect through, and re-read after each", async () => {
    dal.coreOp.mockRejectedValueOnce(new dal.CoreError({ code: "VALIDATION", messageKey: "error.validation", fieldErrors: { accessUntil: "error.range" }, retryAfterSeconds: null, correlationId: "c-1" }));
    await expect(setPartnerAccess("pu-1", 7, "amy@example.com", "technician", "2030-01-01T00:00:00Z", "far")).resolves.toStrictEqual({ ok: false, code: "VALIDATION", messageKey: "error.validation", fieldErrors: { accessUntil: "error.range" }, retryAfterSeconds: null });
    dal.coreOp.mockRejectedValueOnce(new dal.CoreError("CONFLICT"));
    await expect(removePartnerUser("pu-1", 7, "gone")).resolves.toMatchObject({ ok: false, code: "CONFLICT" });
    dal.coreOp.mockRejectedValueOnce(new TypeError("fetch failed"));
    await expect(resendPartnerInvite("pu-2")).resolves.toStrictEqual({ ok: false, code: "UNAVAILABLE", messageKey: "error.unavailable", fieldErrors: {} });
    dal.coreOp.mockImplementationOnce(() => { redirect("/bff/auth/login"); });
    await expect(setPartnerUserStatus("pu-1", 7, "amy@example.com", "technician", "active", "back")).rejects.toMatchObject({ digest: expect.stringMatching(/^NEXT_REDIRECT/) as unknown });
    expect(cache.refresh).toHaveBeenCalledTimes(4);
  });
});

const user = (id: string, over: Partial<ApiPartnerUser> = {}): ApiPartnerUser => ({
  id, version: 1, organizationId: "org-a", membershipId: `m-${id}`, email: `${id}@example.com`, displayName: id, partnerRole: "technician",
  status: "active", lastSignInAt: null, accessUntil: null, invitedAt: "2026-09-01T00:00:00Z", invitedByMembershipId: "m-boss", ...over,
});
const USERS = [user("boss", { partnerRole: "contractor" }), user("amy", { accessUntil: "2026-09-15T15:59:59Z" }), user("ben", { status: "disabled" })];
const page = (status?: string | string[]) => PartnerUsersPage({ params: Promise.resolve({}), searchParams: Promise.resolve(status === undefined ? {} : { status }) });
const liveProps = (element: unknown) => {
  if (!isValidElement<{ live: { rows: { id: string; status: string; you: boolean; ended: boolean }[] } & Record<string, unknown> }>(element) || element.type !== UsersLive) throw new TypeError("not UsersLive");
  return element.props.live;
};

// API mode with a signed-in contractor of Cool Co
const api = () => {
  dal.apiMode.mockReturnValue(true);
  dal.coreAll.mockResolvedValue(USERS);
  dal.corePrincipal.mockResolvedValue({ membershipId: "m-boss" });
  dal.coreDisplay.mockResolvedValue({ locale: "en", timeZone: "Asia/Kuala_Lumpur" });
  dal.coreIdentity.mockResolvedValue({ organizationName: "Cool Co" });
  // 16:30 UTC is already the next day in Kuala Lumpur
  dal.coreNow.mockResolvedValue(new Date("2026-09-15T16:30:00Z"));
};

describe("users page", () => {
  it("renders the demo outside API mode without reading the Core API", async () => {
    dal.apiMode.mockReturnValue(false);
    const demo = await page();
    expect(isValidElement(demo) && demo.type).toBe(UsersDemo);
    expect(dal.coreAll).not.toHaveBeenCalled();
  });

  it("lists every user with the own row marked, an access end passed by the business clock as ended, the company, the e-mails for the duplicate check and today in Kuala Lumpur", async () => {
    api();
    const live = liveProps(await page());
    expect(dal.coreAll).toHaveBeenCalledExactlyOnceWith("partnerUsers.list");
    expect(live.company).toBe("Cool Co");
    expect(live.status).toBeNull();
    expect(live.today).toBe("2026-09-16");
    expect(live.all).toStrictEqual(USERS.map((u) => ({ id: u.id, email: u.email })));
    expect(live.rows.map((r) => [r.id, r.you, r.ended])).toStrictEqual([["amy", false, true], ["ben", false, false], ["boss", true, false]]);
  });

  it("keeps only the rows of the URL's status (the first one given) and ignores an unknown status", async () => {
    api();
    const disabled = liveProps(await page(["disabled", "active"]));
    expect(disabled.status).toBe("disabled");
    expect(disabled.rows.map((r) => r.id)).toStrictEqual(["ben"]);
    expect(disabled.all).toHaveLength(3);
    const active = liveProps(await page("active"));
    expect(active.rows.map((r) => r.id)).toStrictEqual(["amy", "boss"]);
    const unknown = liveProps(await page("everyone"));
    expect(unknown.status).toBeNull();
    expect(unknown.rows).toHaveLength(3);
  });

  it("shows Page unavailable for a FORBIDDEN list (a technician session) and fails on any other error", async () => {
    api();
    dal.coreAll.mockRejectedValueOnce(new dal.CoreError("FORBIDDEN"));
    const unavailable = await page();
    expect(isValidElement(unavailable) && unavailable.type).toBe(UsersLive);
    expect(isValidElement(unavailable) && unavailable.props).toStrictEqual({ live: null });
    const outage = new dal.CoreError("UNAVAILABLE");
    dal.coreAll.mockRejectedValueOnce(outage);
    await expect(page()).rejects.toBe(outage);
    const failed = new TypeError("fetch failed");
    dal.coreAll.mockRejectedValueOnce(failed);
    await expect(page()).rejects.toBe(failed);
  });
});

describe("route states", () => {
  it("name the users while loading and on an error", () => {
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
