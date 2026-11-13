import { describe, expect, it } from "vitest";
import { EN } from "@ac/web/lib/i18n";
import { declineError, quantityError, requestRows, type InstallationRequest } from "@ac/web/lib/installationRequests";

const slot = { startAt: "2026-09-15T02:00:00Z", endAt: "2026-09-15T04:00:00Z" };
const request: InstallationRequest = {
  id: "11111111-aaaa-bbbb-cccc-000000000001", tenantId: "t", version: 1, createdAt: "2026-09-14T01:00:00Z", updatedAt: "2026-09-14T01:00:00Z",
  customerOrgId: "org", propertyId: "p1", spaceId: "s2", quantity: 2, modelId: null, withIot: true, preferredSlots: [slot, slot, slot], contactWindow: null, note: null,
  status: "submitted", declineReason: null, decidedAt: null, jobIds: [],
};
const names = {
  properties: [{ id: "p1", version: 1, customerOrgId: "org", kind: "office" as const, name: "HQ tower", address: null, accessInstructions: null, archived: false, updatedAt: "" }],
  spaces: [
    { id: "s1", version: 1, propertyId: "p1", parentSpaceId: null, kind: "floor" as const, name: "Level 2", archived: false },
    { id: "s2", version: 1, propertyId: "p1", parentSpaceId: "s1", kind: "room" as const, name: "Room 2.1", archived: false },
  ],
  models: [],
};

describe("installation requests", () => {
  it("names the place, the model not decided and the state", () => {
    const [row] = requestRows([request], names, EN);
    expect(row).toMatchObject({ number: "11111111", place: "HQ tower › Level 2 › Room 2.1", model: "Not decided", statusLabel: "Submitted", tone: "warn", quantity: 2, withIot: true });
  });
  it("keeps a declined request's reason and the linked jobs", () => {
    const [row] = requestRows([{ ...request, status: "declined", declineReason: "No capacity", jobIds: ["j1"] }], names, EN);
    expect(row).toMatchObject({ statusLabel: "Declined", tone: "crit", declineReason: "No capacity", jobIds: ["j1"] });
  });
  it("takes 1–10 whole units", () => {
    expect(["1", "10", "0", "11", "1.5", "", "x"].map((q) => quantityError(q, EN.t) === null)).toStrictEqual([true, true, false, false, false, false, false]);
  });
  it("takes a decline reason of 5–500 characters", () => {
    expect([declineError("no", EN.t), declineError("No capacity", EN.t), declineError("x".repeat(501), EN.t)].map((e) => e === null)).toStrictEqual([false, true, false]);
  });
});
