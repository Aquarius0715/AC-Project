import { describe, expect, it } from "vitest";
import { i18nOf, translator } from "@ac/web/lib/i18n";
import {
  allowedFor, allPermissions, isActive, MEMBER_FIELDS, memberDraft, memberErrors, memberFieldId, memberInput, memberRows, orgKind, resourceWord, roleDefaults, roleWord, validity, zonedLocal,
  type ApiMember, type MemberDraft,
} from "@ac/web/lib/members";

const ASIA_TOKYO = "Asia/Tokyo";
const TECHNICIAN = "technician";
const ALERT_READ = "alert.read";
const TEXT_2026_09_01T08_00 = "2026-09-01T08:00";
const IDENTITY_READ = "identity.read";
const CONTRACTOR = "contractor";
const TEXT_2026_09_15T00_00_00Z = "2026-09-15T00:00:00Z";

const MS = i18nOf({ locale: "ms", timeZone: ASIA_TOKYO });
const NOW = new Date("2026-09-15T01:00:00Z");
const member = (over: Partial<ApiMember>): ApiMember => {
  return {
    id: "m1", version: 2, updatedAt: "2026-09-01T00:00:00Z", userId: "u1", displayName: "Aina", organizationId: "org-p", role: TECHNICIAN, employment: "external",
    permissions: [ALERT_READ], scopes: [{ kind: "unit", id: "unit-1" }], scopeVersion: 3, validFrom: "2026-09-01T00:00:00Z", validUntil: "2026-12-31T16:00:00Z", ...over,
  };
};
const draft = (over: Partial<MemberDraft>): MemberDraft => {
  return {
    userId: "u1", organizationId: "org-p", role: TECHNICIAN, employment: "external", permissions: [ALERT_READ], scopes: [{ kind: "unit", id: "unit-1" }], validFrom: TEXT_2026_09_01T08_00, validUntil: "2027-01-01T00:00", reason: "Rotation",
    creatingUser: false, inviteEmail: "", inviteName: "", ...over,
  };
};

describe("HQ access & roles (FR-A03, BR-A03, IR107)", () => {
  it("has the 38 permissions and the role limits of members.save", () => {
    expect([allPermissions.length, allowedFor.contractor, roleDefaults.admin.includes("identity.write"), roleDefaults.admin.includes(IDENTITY_READ)]).toEqual([38, ["partner.accept", "partner.assign", "partner.review"], false, true]);
    expect([orgKind("admin", "internal"), orgKind(CONTRACTOR, "internal"), orgKind(TECHNICIAN, "external"), orgKind(TECHNICIAN, "internal")]).toEqual(["operator", CONTRACTOR, CONTRACTOR, "operator"]);
  });

  it("rows memberships without clients, with their valid period in the display time zone", () => {
    const rows = memberRows([member({}), member({ id: "m2", role: "client", employment: null }), member({ id: "m3", role: "admin", employment: null, organizationId: "x", validUntil: null })], [{ id: "org-p", name: "CoolFix", kind: CONTRACTOR, status: "active" }], NOW);
    expect(rows.map((r) => [r.id, r.label, r.active, r.validity])).toEqual([
      ["m1", "Technician · external · CoolFix", true, "1 Sept 2026, 8:00 am MYT → 1 Jan 2027, 12:00 am MYT"], ["m3", "Admin · organization", true, "1 Sept 2026, 8:00 am MYT → open-ended"],
    ]);
    expect([isActive(member({ validUntil: TEXT_2026_09_15T00_00_00Z }), NOW), isActive(member({ validFrom: "2026-09-16T00:00:00Z" }), NOW)]).toEqual([false, false]);
    expect(memberRows([member({ validUntil: TEXT_2026_09_15T00_00_00Z, revokedAt: TEXT_2026_09_15T00_00_00Z })], [], NOW).map((r) => [r.active, r.revoked])).toEqual([[false, true]]); // IR508
  });

  it("types the valid period in the display time zone and saves the same instants", () => {
    const tokyo = memberDraft(member({}), "", ASIA_TOKYO);
    expect([tokyo.validFrom, tokyo.validUntil, memberDraft(member({})).validFrom]).toEqual(["2026-09-01T09:00", "2027-01-01T01:00", TEXT_2026_09_01T08_00]);
    expect(memberInput({ ...tokyo, reason: " Rotation " }, "m1", ASIA_TOKYO)).toMatchObject({ id: "m1", employment: "external", validFrom: "2026-09-01T00:00:00.000Z", validUntil: "2026-12-31T16:00:00.000Z", reason: "Rotation" });
    expect(memberInput(draft({ role: "admin", validUntil: "" })).employment).toBeNull();
    expect(zonedLocal("2026-09-15T01:00:00Z", ASIA_TOKYO)).toBe("2026-09-15T10:00"); // a saved instant in the form's zone
    expect(memberDraft().role).toBe(CONTRACTOR);
    // a new membership starts now on the business clock, in the form's zone (IR622); an existing one keeps its own
    expect([memberDraft(undefined, "", "Asia/Kuala_Lumpur", "2026-09-23T09:38:00Z").validFrom, memberDraft().validFrom, memberDraft(member({}), "", "Asia/Kuala_Lumpur", "2026-09-23T09:38:00Z").validFrom])
      .toEqual(["2026-09-23T17:38", "", TEXT_2026_09_01T08_00]);
  });

  it("checks a membership before members.save (no self-grant of identity.write or restriction.override)", () => {
    expect(memberErrors(draft({}), { selfUserId: "me" })).toEqual({});
    expect(memberErrors(draft({ userId: "", organizationId: "", scopes: [], validFrom: "", validUntil: "", reason: " ", permissions: [ALERT_READ, "billing.read"] }), { selfUserId: "me" })).toEqual({
      userId: "Choose a user", organizationId: "Choose the organization", permissions: "Not allowed for this role: billing.read", scopes: "Add at least one unit, property or customer organization",
      validFrom: "Required", validUntil: "External technicians need an end date", reason: "A change reason is required (1–1000 characters)",
    });
    expect(memberErrors(draft({ role: "admin", permissions: ["asset.write"], validUntil: "2026-08-01T00:00" }), { selfUserId: "me" })).toEqual({ permissions: "asset.write needs asset.read", validUntil: "Must be after valid from" });
    expect(memberErrors(draft({ userId: "me", role: "admin", permissions: [IDENTITY_READ, "identity.write", "restriction.override"] }), { selfUserId: "me", current: member({ permissions: [IDENTITY_READ] }) }).permissions)
      .toBe("Another identity administrator must grant identity.write, restriction.override to you");
 
  });

  it("creates a new person with the membership: e-mail and name instead of a user, and no user ID yet (IR461)", () => {
    expect(memberErrors(draft({ userId: "", creatingUser: true, inviteEmail: "nope", inviteName: " " }), { selfUserId: "me" })).toEqual({
      "newUser.email": "A valid email address (up to 254 characters)", "newUser.displayName": "A name of 1–120 characters",
    });
    expect(memberErrors(draft({ userId: "", creatingUser: true, inviteEmail: "new@example.com", inviteName: "New Colleague" }), { selfUserId: "me" })).toEqual({});
    expect(memberInput(draft({ userId: "", creatingUser: true, inviteEmail: " new@example.com ", inviteName: " New Colleague " }))).toMatchObject({
      userId: "00000000-0000-0000-0000-000000000000", "newUser": { email: "new@example.com", displayName: "New Colleague" },
    });
    expect("newUser" in memberInput(draft({ creatingUser: true }), "m1")).toBe(false); // an existing membership keeps its user
  });
});

describe("HQ access & roles in Malay with the display time zone (IR302)", () => {
  it("words roles, resources, periods and checks", () => {
    const t = translator("ms");
    expect([roleWord(TECHNICIAN, "internal", t), roleWord(CONTRACTOR, null, t), resourceWord("Users & roles", t), resourceWord("Something new", t)]).toEqual(["Juruteknik · dalaman", "Kontraktor", "Pengguna & peranan", "Something new"]);
    expect(validity(member({ validUntil: null }), MS)).toBe("1 Sep 2026, 9:00 PG GMT+9 → terbuka");
    expect(memberRows([member({ organizationId: "x" })], [], NOW, MS)[0]?.label).toBe("Juruteknik · luaran · organisasi");
    expect(memberErrors(draft({ userId: "", reason: "" }), { selfUserId: "me" }, t)).toEqual({ userId: "Pilih pengguna", reason: "Sebab perubahan diperlukan (1–1000 aksara)" });
  });
});

// IR622: a refused save focuses the first field to fix, so every field memberErrors names has a place in the order
describe("the form's field order", () => {
  it("holds every field a check can name, each with its element id", () => {
    const all = { ...memberErrors(draft({ userId: "", organizationId: "", scopes: [], validFrom: "", validUntil: "", reason: " ", permissions: [ALERT_READ, "billing.read"] }), { selfUserId: "me" }),
      ...memberErrors(draft({ userId: "", creatingUser: true, inviteEmail: "nope", inviteName: " " }), { selfUserId: "me" }) };
    expect(Object.keys(all).filter((k) => !(MEMBER_FIELDS as readonly string[]).includes(k))).toEqual([]);
    expect([memberFieldId("newUser.email"), memberFieldId("validFrom")]).toEqual(["member-newUser-email", "member-validFrom"]);
  });
});
