import { describe, expect, it } from "vitest";
import { partnerInviteError, partnerUserRows, reasonError, statusFilter, type ApiPartnerUser } from "@ac/web/lib/partnerUsers";
import { PARTNER_USERS_SEED } from "@ac/web/lib/partnerUsersDemo";

const user = (p: Partial<ApiPartnerUser>): ApiPartnerUser => ({
  id: "u", version: 1, organizationId: "org", membershipId: null, email: "x@example.com", displayName: null, partnerRole: "technician", status: "invited",
  lastSignInAt: null, accessUntil: null, invitedAt: "2026-09-12T02:00:00Z", invitedByMembershipId: "contractor-a", ...p,
});

describe("partnerUserRows (FR-P11)", () => {
  it("sorts by name, marks the caller's own account and gives it no actions", () => {
    const rows = partnerUserRows(PARTNER_USERS_SEED, "contractor-a");
    expect(rows.map((r) => r.name ?? r.email)).toEqual(["contractor-a", "new.tech@example.com", "tech-external-a"]);
    const self = rows.find((r) => r.you)!;
    expect(self.actions).toEqual({ resend: false, edit: false, disable: false, enable: false, remove: false });
  });
  it("offers resend, edit and remove on an invite; disable and remove on an active technician", () => {
    const rows = partnerUserRows(PARTNER_USERS_SEED, "contractor-a");
    expect(rows.find((r) => r.status === "invited")!.actions).toEqual({ resend: true, edit: true, disable: false, enable: false, remove: true });
    const tech = rows.find((r) => r.name === "tech-external-a")!;
    expect(tech.actions).toEqual({ resend: false, edit: false, disable: true, enable: false, remove: true });
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
    expect(r.actions).toEqual({ resend: false, edit: false, disable: false, enable: false, remove: true });
    expect(partnerUserRows([user({ id: "e", displayName: "Old", status: "active", membershipId: "m", accessUntil: "2026-10-01T00:00:00Z" })], null, undefined, new Date("2026-09-15T00:00:00Z"))[0].ended).toBe(false);
  });
  it("enables a disabled account; an invite has no access end yet", () => {
    const [d, i] = partnerUserRows([user({ id: "d", displayName: "A", status: "disabled", membershipId: "m" }), user({ id: "i", displayName: "B" })], null);
    expect(d.actions.enable).toBe(true);
    expect(i.access).toBe("—");
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
  it("reads only known statuses from the URL", () => {
    expect(statusFilter("disabled")).toBe("disabled");
    expect(statusFilter("gone")).toBeNull();
    expect(statusFilter(undefined)).toBeNull();
  });
});
