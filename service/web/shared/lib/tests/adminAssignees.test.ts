import { describe, expect, it } from "vitest";
import { assigneeGroups } from "@ac/web/lib/adminJobs";
import { translator } from "@ac/web/lib/i18n";

const NOW = Date.parse("2026-09-20T00:00:00Z");
const orgs = new Map([["org-b", "Demo Contractor B"], ["org-a", "Demo Contractor A"], ["org-hq", "Demo HQ"]]);

describe("assigneeGroups", () => {
  it("groups technicians by company and puts ended access last, marked", () => {
    const groups = assigneeGroups([
      { id: "m1", displayName: "Siti", organizationId: "org-b", validUntil: null },
      { id: "m2", displayName: "Ahmad", organizationId: "org-a", validUntil: "2026-09-01T00:00:00Z" },
      { id: "m3", displayName: "Siti", organizationId: "org-a", validUntil: "2026-12-01T00:00:00Z" },
      { id: "m4", displayName: "Badrul", organizationId: "org-a", validUntil: null, revokedAt: "2026-09-10T00:00:00Z" },
      { id: "m5", displayName: "Lee", organizationId: "org-hq", validUntil: null },
    ], orgs, NOW);
    expect(groups).toStrictEqual([
      { org: "Demo Contractor A", people: [{ id: "m3", name: "Siti" }, { id: "m2", name: "Ahmad (access ended)" }, { id: "m4", name: "Badrul (access ended)" }] },
      { org: "Demo Contractor B", people: [{ id: "m1", name: "Siti" }] },
      { org: "Demo HQ", people: [{ id: "m5", name: "Lee" }] },
    ]);
  });

  it("marks ended access in the display language", () => {
    const [group] = assigneeGroups([{ id: "m2", displayName: "Ahmad", organizationId: "org-a", validUntil: "2026-09-01T00:00:00Z" }], orgs, NOW, translator("ja"));
    expect(group?.people).toStrictEqual([{ id: "m2", name: "Ahmad（アクセス終了）" }]);
  });
});
