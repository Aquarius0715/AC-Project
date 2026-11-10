// @vitest-environment node
import { beforeEach, describe, expect, it, vi } from "vitest";

const calls: string[] = [];
const dal = vi.hoisted(() => ({ coreAll: vi.fn(), coreOp: vi.fn() }));
vi.mock("@ac/web/lib/dal", () => dal);

const { deleteAccounts } = await import("@ac/web/lib/forceDelete");

beforeEach(() => {
  calls.length = 0;
  dal.coreAll.mockReset();
  dal.coreOp.mockReset();
  dal.coreAll.mockResolvedValue([{ id: "m1", version: 3 }, { id: "m2", version: 1 }, { id: "m3", version: 2 }]);
  dal.coreOp.mockImplementation((op: string, input: { id: string }) => { calls.push(`${op} ${input.id}`); return Promise.resolve(null); });
});

// data.forceDelete: the accounts of the organization go one after the other, each with its own version
describe("deleteAccounts", () => {
  it("lists the organization's memberships and deletes each with force and its version, in order", async () => {
    await deleteAccounts("org-1", "closed");
    expect(dal.coreAll).toHaveBeenCalledWith("members.list", { filters: { organizationId: "org-1" } });
    expect(calls).toStrictEqual(["members.delete m1", "members.delete m2", "members.delete m3"]);
    expect(dal.coreOp.mock.calls.map((c: unknown[]) => [c[1], c[2]])).toStrictEqual([
      [{ id: "m1", force: true, reason: "closed" }, { write: true, expectedVersion: 3 }],
      [{ id: "m2", force: true, reason: "closed" }, { write: true, expectedVersion: 1 }],
      [{ id: "m3", force: true, reason: "closed" }, { write: true, expectedVersion: 2 }],
    ]);
  });
  it("stops at the first account the Core API refuses and leaves the rest", async () => {
    dal.coreOp.mockImplementation((op: string, input: { id: string }) => {
      calls.push(`${op} ${input.id}`);
      return input.id === "m2" ? Promise.reject(new Error("memberUsed")) : Promise.resolve(null);
    });
    await expect(deleteAccounts("org-1", "closed")).rejects.toThrow("memberUsed");
    expect(calls).toStrictEqual(["members.delete m1", "members.delete m2"]);
  });
  it("does nothing for an organization without accounts", async () => {
    dal.coreAll.mockResolvedValue([]);
    await deleteAccounts("org-1", "closed");
    expect(dal.coreOp).not.toHaveBeenCalled();
  });
});
