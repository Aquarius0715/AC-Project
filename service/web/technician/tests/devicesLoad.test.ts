import { describe, expect, it, vi } from "vitest";

// the technician device screen (FR-T11) through the DAL; a device registered on site is reserved for its planned unit
// and stays unbound until devices.bind (D05, IR467)
const dal = vi.hoisted(() => ({
  coreNow: vi.fn(), coreDisplay: vi.fn(), corePermissions: vi.fn(), coreAll: vi.fn(), coreOp: vi.fn(),
  orAbsent: async <T,>(read: Promise<T>): Promise<T | null> => read.catch(() => null),
}));
vi.mock("server-only", () => ({}));
vi.mock("@ac/web/lib/dal", () => dal);

import { loadDevices } from "../app/technician/devices/_lib/load";
import type { ApiDeviceDetail } from "@ac/web/lib/devices";

const planned: ApiDeviceDetail = {
  id: "device-1", version: 1, unitId: null, targetUnitId: "unit-1", serial: "AC-NEW-1", connection: "unknown", lastSeenAt: null, firmwareVersion: "1.0.0",
  mqttNode: null, powerSignal: "unknown", tamper: "clear", sensors: [], calibrationRefs: [], activeOperation: null,
};

describe("technician devices", () => {
  it("runs a device not bound yet under the job of its planned unit, so it can be bound (IR722)", async () => {
    dal.coreNow.mockResolvedValue(new Date("2026-11-10T02:00:00Z"));
    dal.coreDisplay.mockResolvedValue({ locale: "en", timeZone: "Asia/Kuala_Lumpur" });
    dal.corePermissions.mockResolvedValue(new Set(["device.maintain", "alert.read"]));
    const lists = new Map<string, unknown[]>([
      ["devices.list", [planned]],
      ["units.list", [{ id: "unit-1", displayName: "Bedroom AC", archived: false }]],
      ["jobs.list", [{ projection: "summary", id: "job-1", unitId: "unit-1", status: "assigned" }]],
    ]);
    dal.coreAll.mockImplementation((operation: string) => Promise.resolve(lists.get(operation) ?? []));
    dal.coreOp.mockImplementation((operation: string) => {
      if (operation === "devices.get") return Promise.resolve(planned);
      if (operation === "units.get") return Promise.reject(new Error("not read")); // the unit reads as absent
      return Promise.resolve({ items: [] });
    });
    const live = await loadDevices({ selected: "device-1", events: false });
    expect(live.device?.jobId).toBe("job-1");
  });
});
