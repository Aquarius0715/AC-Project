import { beforeEach, describe, expect, it, vi } from "vitest";

// the team screen reads the own company's technicians through the DAL (FR-P06); another company's id in the URL reads
// as not found before anything else is read (AT-P06-E item 1)
const dal = vi.hoisted(() => ({
  coreNow: vi.fn(), coreDisplay: vi.fn(), coreIdentity: vi.fn(), corePrincipal: vi.fn(), coreOp: vi.fn(), coreAll: vi.fn(), coreItems: vi.fn(),
  orWhenRefused: async <T, F>(read: Promise<T>, fallback: F): Promise<T | F> => read.catch(() => fallback),
}));
vi.mock("server-only", () => ({}));
vi.mock("@ac/web/lib/dal", () => dal);

import { loadTeam } from "../app/partner/team/_lib/load";
import type { ApiTeamMember } from "@ac/web/lib/partnerTeam";

const technician: ApiTeamMember = { id: "m-1", userId: "u-1", displayName: "Aiman", role: "technician", organizationId: "org-a", employment: null, validFrom: "2026-01-01T00:00:00Z", validUntil: null, revokedAt: null, qualifications: [] };

describe("partner team loader", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    dal.coreNow.mockResolvedValue(new Date("2026-11-18T02:00:00Z"));
    dal.coreDisplay.mockResolvedValue({ locale: "en", timeZone: "Asia/Kuala_Lumpur" });
    dal.coreIdentity.mockResolvedValue({ displayName: "Owner", organizationName: "Contractor A" });
    dal.corePrincipal.mockResolvedValue({ organizationId: "org-a" });
    dal.coreItems.mockResolvedValue([]);
    dal.coreOp.mockResolvedValue({ items: [], total: 0 });
    dal.coreAll.mockImplementation((operation: string) => Promise.resolve(operation === "members.list" ? [technician] : []));
  });

  it("refuses another company's id in the URL before reading the team", async () => {
    const live = await loadTeam({ orgId: "org-b" });
    expect(live).toStrictEqual({ notFound: true, company: "Contractor A" });
    expect(dal.coreAll).not.toHaveBeenCalled();
    expect(dal.coreOp).not.toHaveBeenCalled();
  });

  it("reads the team for the own company's id and for no id", async () => {
    for (const orgId of ["org-a", undefined]) {
      const live = await loadTeam({ orgId, date: "2026-11-18" });
      expect(live.notFound).toBe(false);
      expect(live.notFound ? [] : live.technicians).toStrictEqual([{ id: "m-1", name: "Aiman" }]);
    }
  });

  it("names the company by default when the session has no organization name", async () => {
    dal.coreIdentity.mockResolvedValue({ displayName: "Owner", organizationName: "" });
    await expect(loadTeam({ orgId: "org-b" })).resolves.toStrictEqual({ notFound: true, company: "your company" });
  });
});
