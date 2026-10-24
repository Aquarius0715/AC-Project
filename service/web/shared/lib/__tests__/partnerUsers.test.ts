import { describe, expect, it } from "vitest";
import { accessEnd, accessError, addDays, nameError, partnerInviteError, partnerUserRows, reasonError, statusFilter, type ApiPartnerUser } from "@ac/web/lib/partnerUsers";
import { PARTNER_USERS_SEED } from "@ac/web/lib/partnerUsersDemo";

const user = (p: Partial<ApiPartnerUser>): ApiPartnerUser => ({
  id: "u", version: 1, organizationId: "org", membershipId: null, email: "x@example.com", displayName: null, partnerRole: "technician", status: "invited",
  lastSignInAt: null, accessUntil: null, invitedAt: "2026-09-12T02:00:00Z", invitedByMembershipId: "contractor-a", ...p,
});

describe("partnerUserRows (FR-P11)", () => {
  it("sorts by name, marks the caller's own account and gives it no actions", () => {
    const rows = partnerUserRows(PARTNER_USERS_SEED, "contractor-a");
    expect(rows.map((r) => r.name ?? r.email)).toEqual(["contractor-a", "New Tech", "tech-external-a"]);
    const self = rows.find((r) => r.you)!;
    expect(self.actions).toEqual({ resend: false, access: false, disable: false, enable: false, remove: false });
  });
  it("offers resend, access and remove on a technician not signed in yet; access, disable and remove on an active one (IR461)", () => {
    const rows = partnerUserRows(PARTNER_USERS_SEED, "contractor-a");
    const added = rows.find((r) => r.status === "invited")!;
    expect(added.actions).toEqual({ resend: true, access: true, disable: false, enable: false, remove: true });
    expect(added.accessDate).toBe("2026-12-31");
    const tech = rows.find((r) => r.name === "tech-external-a")!;
    expect(tech.actions).toEqual({ resend: false, access: true, disable: true, enable: false, remove: true });
    expect(tech.access).toMatch(/2026/);
  });
  it("keeps the company's last active contractor: no disable, no remove", () => {
    const rows = partnerUserRows([user({ id: "boss", membershipId: "m1", partnerRole: "contractor", status: "active", displayName: "Boss" })], "someone-else");
    expect(rows[0].lastContractor).toBe(true);
    expect(rows[0].actions.disable || rows[0].actions.remove).toBe(false);
  });
  it("reads an active account whose access end has passed as ended: no disable, only remove", () => {
    const [r] = partnerUserRows([user({ id: "e", displayName: "Old", status: "active", membershipId: "m", accessUntil: "2026-10-01T00:00:00Z" })], null, undefined, new Date("2026-10-02T00:00:00Z"));
    expect(r.ended).toBe(true);
    expect(r.actions).toEqual({ resend: false, access: true, disable: false, enable: false, remove: true });
    expect(partnerUserRows([user({ id: "e", displayName: "Old", status: "active", membershipId: "m", accessUntil: "2026-10-01T00:00:00Z" })], null, undefined, new Date("2026-09-15T00:00:00Z"))[0].ended).toBe(false);
  });
  it("reads a technician not signed in yet whose access end has passed as ended: no resend until the end moves (IR461)", () => {
    const [r] = partnerUserRows([user({ id: "n", displayName: "Late", membershipId: "m", accessUntil: "2026-10-01T00:00:00Z" })], null, undefined, new Date("2026-10-02T00:00:00Z"));
    expect(r.ended).toBe(true);
    expect(r.actions).toEqual({ resend: false, access: true, disable: false, enable: false, remove: true });
  });
  it("keeps a technician who holds an assigned job: no disable, no remove until it is reassigned (IR432)", () => {
    const [r] = partnerUserRows([user({ id: "b", displayName: "Busy", status: "active", membershipId: "m", booked: true })], null);
    expect(r.booked).toBe(true);
    expect(r.actions).toEqual({ resend: false, access: true, disable: false, enable: false, remove: false }); // a later end is still set
  });
  it("enables a disabled account; an invite from before IR461 (no account) has no access end and no resend", () => {
    const [d, i] = partnerUserRows([user({ id: "d", displayName: "A", status: "disabled", membershipId: "m" }), user({ id: "i", displayName: "B" })], null);
    expect(d.actions.enable).toBe(true);
    expect([i.access, i.actions.resend, i.actions.access]).toEqual(["—", false, false]);
  });
});

describe("partner invite checks", () => {
  const us = [{ id: "a", email: "Tech@Example.com" }];
  it("refuses a bad or taken address, case-insensitive, but not the invite's own", () => {
    expect(partnerInviteError("nope", us)).toBe("A valid email address (up to 254 characters)");
    expect(partnerInviteError(" tech@example.com ", us)).toBe("Already a user of your company");
    expect(partnerInviteError("tech@example.com", us, undefined, "a")).toBeUndefined();
    expect(partnerInviteError("new@example.com", us)).toBeUndefined();
  });
  it("needs a reason of 1–1000 characters", () => {
    expect(reasonError("  ")).toBeDefined();
    expect(reasonError("x".repeat(1001))).toBeDefined();
    expect(reasonError("On leave")).toBeUndefined();
  });
  it("needs a name of 1–120 characters (IR461)", () => {
    expect(nameError(" ")).toBeDefined();
    expect(nameError("x".repeat(121))).toBeDefined();
    expect(nameError(" Lim Wei Ming ")).toBeUndefined();
  });
  it("takes an access end from the business day to a year ahead, closing at the end of that Kuala Lumpur day (IR461)", () => {
    expect([accessError("2026-09-14", "2026-09-15"), accessError("2026-09-15", "2026-09-15"), accessError(addDays("2026-09-15", 365), "2026-09-15"), accessError(addDays("2026-09-15", 366), "2026-09-15"), accessError("15/09/2026", "2026-09-15")])
      .toEqual(["Today or later", undefined, undefined, "At most a year ahead", "A date"]);
    expect(accessEnd("2026-12-31")).toBe("2026-12-31T15:59:59.000Z");
  });
  it("reads only known statuses from the URL", () => {
    expect(statusFilter("disabled")).toBe("disabled");
    expect(statusFilter("gone")).toBeNull();
    expect(statusFilter(undefined)).toBeNull();
  });
});
