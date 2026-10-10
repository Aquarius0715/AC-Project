import { describe, expect, it } from "vitest";
import { i18nOf, translator } from "@ac/web/lib/i18n";
import {
  allowedFor, allPermissions, isActive, memberDraft, memberErrors, memberInput, memberRows, orgKind, resourceWord, roleDefaults, roleWord, validity, zonedLocal,
  type ApiMember, type MemberDraft,
} from "@ac/web/lib/members";

const MS = i18nOf({ locale: "ms", timeZone: "Asia/Tokyo" });
const NOW = new Date("2026-09-15T01:00:00Z");
const member = (over: Partial<ApiMember>): ApiMember => ({
  id: "m1", version: 2, updatedAt: "2026-09-01T00:00:00Z", userId: "u1", displayName: "Aina", organizationId: "org-p", role: "technician", employment: "external",
  permissions: ["alert.read"], scopes: [{ kind: "unit", id: "unit-1" }], scopeVersion: 3, validFrom: "2026-09-01T00:00:00Z", validUntil: "2026-12-31T16:00:00Z", ...over,
});
const draft = (over: Partial<MemberDraft>): MemberDraft => ({
  userId: "u1", organizationId: "org-p", role: "technician", employment: "external", permissions: ["alert.read"], scopes: [{ kind: "unit", id: "unit-1" }], validFrom: "2026-09-01T08:00", validUntil: "2027-01-01T00:00", reason: "Rotation", ...over,
});

describe("HQ access & roles (FR-A03, BR-A03, IR107)", () => {
  it("has the 38 permissions and the role limits of members.save", () => {
    expect([allPermissions.length, allowedFor.contractor, roleDefaults.admin.includes("identity.write"), roleDefaults.admin.includes("identity.read")]).toEqual([38, ["partner.accept", "partner.assign", "partner.review"], false, true]);
    expect([orgKind("admin", "internal"), orgKind("contractor", "internal"), orgKind("technician", "external"), orgKind("technician", "internal")]).toEqual(["operator", "contractor", "contractor", "operator"]);
  });

  it("rows memberships without clients, with their valid period in the display time zone", () => {
    const rows = memberRows([member({}), member({ id: "m2", role: "client", employment: null }), member({ id: "m3", role: "admin", employment: null, organizationId: "x", validUntil: null })], [{ id: "org-p", name: "CoolFix", kind: "contractor", status: "active" }], NOW);
    expect(rows.map((r) => [r.id, r.label, r.active, r.validity])).toEqual([
      ["m1", "Technician · external · CoolFix", true, "1 Sept 2026, 8:00 am MYT → 1 Jan 2027, 12:00 am MYT"], ["m3", "Admin · organization", true, "1 Sept 2026, 8:00 am MYT → open-ended"],
    ]);
    expect([isActive(member({ validUntil: "2026-09-15T00:00:00Z" }), NOW), isActive(member({ validFrom: "2026-09-16T00:00:00Z" }), NOW)]).toEqual([false, false]);
  });

  it("types the valid period in the display time zone and saves the same instants", () => {
    const tokyo = memberDraft(member({}), "", "Asia/Tokyo");
    expect([tokyo.validFrom, tokyo.validUntil, memberDraft(member({})).validFrom]).toEqual(["2026-09-01T09:00", "2027-01-01T01:00", "2026-09-01T08:00"]);
    expect(memberInput({ ...tokyo, reason: " Rotation " }, "m1", "Asia/Tokyo")).toMatchObject({ id: "m1", employment: "external", validFrom: "2026-09-01T00:00:00.000Z", validUntil: "2026-12-31T16:00:00.000Z", reason: "Rotation" });
    expect(memberInput(draft({ role: "admin", validUntil: "" })).employment).toBeNull();
    expect(zonedLocal("2026-09-15T01:00:00Z", "Asia/Tokyo")).toBe("2026-09-15T10:00"); // a revoke ends the access now
    expect(memberDraft().role).toBe("contractor");
  });

  it("checks a membership before members.save (no self-grant of identity.write or restriction.override)", () => {
    expect(memberErrors(draft({}), { selfUserId: "me" })).toEqual({});
    expect(memberErrors(draft({ userId: "", organizationId: "", scopes: [], validFrom: "", validUntil: "", reason: " ", permissions: ["alert.read", "billing.read"] }), { selfUserId: "me" })).toEqual({
      userId: "Choose a user", organizationId: "Choose the organization", permissions: "Not allowed for this role: billing.read", scopes: "Add at least one unit, property or customer organization",
      validFrom: "Required", validUntil: "External technicians need an end date (IR74)", reason: "A change reason is required (1–1000 characters)",
    });
    expect(memberErrors(draft({ role: "admin", permissions: ["asset.write"], validUntil: "2026-08-01T00:00" }), { selfUserId: "me" })).toEqual({ permissions: "asset.write needs asset.read", validUntil: "Must be after valid from" });
    expect(memberErrors(draft({ userId: "me", role: "admin", permissions: ["identity.read", "identity.write", "restriction.override"] }), { selfUserId: "me", current: member({ permissions: ["identity.read"] }) }).permissions)
      .toBe("Another identity administrator must grant identity.write, restriction.override to you");
  });
});

describe("HQ access & roles in Malay with the display time zone (IR302)", () => {
  it("words roles, resources, periods and checks", () => {
    const t = translator("ms");
    expect([roleWord("technician", "internal", t), roleWord("contractor", null, t), resourceWord("Users & roles", t), resourceWord("Something new", t)]).toEqual(["Juruteknik · dalaman", "Kontraktor", "Pengguna & peranan", "Something new"]);
    expect(validity(member({ validUntil: null }), MS)).toBe("1 Sep 2026, 9:00 PG GMT+9 → terbuka");
    expect(memberRows([member({ organizationId: "x" })], [], NOW, MS)[0].label).toBe("Juruteknik · luaran · organisasi");
    expect(memberErrors(draft({ userId: "", reason: "" }), { selfUserId: "me" }, t)).toEqual({ userId: "Pilih pengguna", reason: "Sebab perubahan diperlukan (1–1000 aksara)" });
  });
});
