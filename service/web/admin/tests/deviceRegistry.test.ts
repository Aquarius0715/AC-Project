import { describe, expect, it, vi } from "vitest";

// HQ's device registry (FR-A04) pages devices.list on the server (IR723): sorted by serial, the search on the server,
// a URL device that is not on the page read on its own
const dal = vi.hoisted(() => ({
  coreOp: vi.fn(),
  orAbsent: async <T,>(read: Promise<T>): Promise<T | null> => read.catch(() => null),
  CoreError: class extends Error {},
}));
vi.mock("server-only", () => ({}));
vi.mock("@ac/web/lib/dal", () => dal);

import { registryPage } from "../app/admin/devices/_lib/registry";
import type { ApiDevice } from "@ac/web/lib/devices";

const device = (n: number): ApiDevice => ({
  id: `device-${String(n)}`, version: 1, unitId: `unit-${String(n)}`, targetUnitId: null, serial: `AC-${String(n).padStart(4, "0")}`, connection: "online",
  lastSeenAt: null, firmwareVersion: "1.0.0", mqttNode: null, powerSignal: "on", tamper: "clear", sensors: [],
});
interface Query { limit?: number; cursor?: string; filters?: { search?: string }; sort?: { field: string; direction: string } }

describe("HQ device registry", () => {
  it("reads one page by serial with the search on the server, and the URL device past it", async () => {
    const calls: Query[] = [];
    dal.coreOp.mockImplementation((operation: string, input: Query & { id?: string }) => {
      if (operation === "devices.get") return Promise.resolve(device(2500));
      calls.push(input);
      return Promise.resolve({ items: Array.from({ length: 100 }, (_, n) => device(n)), total: 3000, nextCursor: "page-2" });
    });
    const page = await registryPage("ac-", null, "device-2500");
    expect(calls).toStrictEqual([{ limit: 100, sort: { field: "serial", direction: "asc" }, filters: { search: "ac-" } }]);
    expect([page.items[0]?.id, page.items.length, page.total, page.next]).toStrictEqual(["device-2500", 101, 3000, "page-2"]);
  });

  it("reads the first page again when the cursor no longer holds", async () => {
    const calls: Query[] = [];
    dal.coreOp.mockImplementation((_: string, input: Query) => {
      calls.push(input);
      return input.cursor ? Promise.reject(new dal.CoreError("cursor expired")) : Promise.resolve({ items: [device(1)], total: 1, nextCursor: null });
    });
    const page = await registryPage("", "stale", null);
    expect([calls.length, calls[1]?.cursor, page.items.length]).toStrictEqual([2, undefined, 1]);
  });
});
