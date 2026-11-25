import { beforeEach, describe, expect, it, vi } from "vitest";

// The Server Actions of HQ customers & units (FR-A02, FR-A17–A19): a customer made from its billing organization (an
// unused one left by an earlier half-finished attempt reused), renamed and deleted (with its accounts when forced);
// properties, spaces and units saved at their versions; a new unit with its device (an existing one bound, or a new one
// registered and bound, the unit kept when the device is refused); the address search behind asset.write; client
// users; the CSV import; warranty claims.
const dal = vi.hoisted(() => {
  interface DomainError { code: string; messageKey: string; fieldErrors: Record<string, string>; retryAfterSeconds: number | null }
  class CoreError extends Error {
    readonly error: DomainError;
    constructor(error: DomainError, options?: ErrorOptions) {
      super(error.code, options);
      this.name = "CoreError";
      this.error = error;
    }
  }
  return {
    CoreError,
    coreOp: vi.fn<(op: string, input?: unknown, options?: unknown) => Promise<unknown>>(),
    coreAll: vi.fn<(op: string, input?: unknown) => Promise<unknown>>(),
    corePermissions: vi.fn(() => Promise.resolve(new Set(["asset.write"]))),
    verifySession: vi.fn(() => Promise.resolve({ role: "admin" })),
  };
});
const cache = vi.hoisted(() => ({ refresh: vi.fn() }));
const geocode = vi.hoisted(() => ({ searchPlaces: vi.fn(() => Promise.resolve({ ok: true, places: [] })) }));
const force = vi.hoisted(() => ({ deleteAccounts: vi.fn(() => Promise.resolve()) }));
vi.mock("server-only", () => ({}));
vi.mock("next/cache", () => cache);
vi.mock("@ac/web/lib/dal", () => dal);
vi.mock("@ac/web/lib/geocode", () => geocode);
vi.mock("@ac/web/lib/forceDelete", () => force);

import {
  archiveLocation, archiveUnit, createCustomer, deleteCustomer, deleteUnit, importCommit, importPreview, importUndo, inviteClientUser, previewPasswordReset, recordWarrantyClaim,
  removeClientUser, resendInvite, saveProperty, saveSpace, saveUnit, saveUnitWithDevice, searchAddress, setDefaultRule, setUnitPolicies, signOutEverywhere, updateClientUser, updateCustomer,
} from "../app/admin/units/actions";

const refused = (code: string, messageKey = `error.${code.toLowerCase()}`, fieldErrors: Record<string, string> = {}) => new dal.CoreError({ code, messageKey, fieldErrors, retryAfterSeconds: null });
const v = (n: number) => ({ write: true, expectedVersion: n });
const calls = () => dal.coreOp.mock.calls;

beforeEach(() => {
  dal.coreOp.mockReset(); dal.coreAll.mockReset(); cache.refresh.mockReset(); force.deleteAccounts.mockClear(); geocode.searchPlaces.mockClear();
  dal.verifySession.mockResolvedValue({ role: "admin" });
  dal.corePermissions.mockResolvedValue(new Set(["asset.write"]));
});

describe("customers", () => {
  const input = { billingName: " Alpha Sdn Bhd ", name: " Alpha ", serviceProfile: "general" as const, status: "active" as const };

  it("creates the billing organization and then the customer", async () => {
    dal.coreOp.mockResolvedValueOnce({ id: "org-1" }).mockResolvedValueOnce({ id: "cust-1" });
    expect(await createCustomer(input)).toStrictEqual({ ok: true, value: "cust-1" });
    expect(calls()).toStrictEqual([
      ["organizations.save", { name: "Alpha Sdn Bhd", kind: "customer", status: "active" }, { write: true }],
      ["customers.save", { name: "Alpha", organizationId: "org-1", serviceProfile: "general", status: "active" }, { write: true }],
    ]);
    expect(cache.refresh).toHaveBeenCalledOnce();
  });

  it("reuses an organization of the same name that never got its customer, and refuses one that has", async () => {
    dal.coreOp.mockRejectedValueOnce(refused("CONFLICT", "error.duplicateName")).mockResolvedValueOnce({ id: "cust-2" });
    dal.coreAll.mockImplementation((op) => Promise.resolve(op === "organizations.list" ? [{ id: "org-left", name: "alpha sdn bhd " }] : []));
    expect(await createCustomer(input)).toStrictEqual({ ok: true, value: "cust-2" });
    expect(calls()[1]?.[1]).toMatchObject({ organizationId: "org-left" });
    expect(dal.coreAll).toHaveBeenCalledWith("customers.list", { filters: { organizationId: "org-left", status: "inactive" } });
    dal.coreOp.mockReset();
    dal.coreOp.mockRejectedValueOnce(refused("CONFLICT", "error.duplicateName"));
    dal.coreAll.mockImplementation((op) => Promise.resolve(op === "organizations.list" ? [{ id: "org-used", name: "Alpha Sdn Bhd" }] : [{ id: "cust-old" }]));
    expect(await createCustomer(input)).toMatchObject({ ok: false, messageKey: "error.duplicateName" });
    dal.coreOp.mockRejectedValueOnce(refused("CONFLICT", "error.duplicateName"));
    dal.coreAll.mockResolvedValue([]); // the name is another kind's: no customer organization to reuse
    expect(await createCustomer(input)).toMatchObject({ ok: false, messageKey: "error.duplicateName" });
    dal.coreOp.mockRejectedValueOnce(refused("FORBIDDEN"));
    expect(await createCustomer(input)).toMatchObject({ ok: false, code: "FORBIDDEN" });
    expect(dal.coreOp).toHaveBeenCalledTimes(3); // no customer saved after a refusal
  });

  it("renames the billing organization only when it changed, then saves the customer at their versions", async () => {
    dal.coreOp.mockResolvedValue({});
    const org = { id: "org-1", version: 4, name: "Alpha Sdn Bhd", status: "active" as const };
    expect(await updateCustomer("cust-1", 2, org, { ...input, billingName: "Alpha Sdn Bhd" })).toStrictEqual({ ok: true, value: null });
    expect(calls()).toStrictEqual([["customers.save", { id: "cust-1", name: "Alpha", organizationId: "org-1", serviceProfile: "general", status: "active" }, v(2)]]);
    dal.coreOp.mockClear();
    await updateCustomer("cust-1", 2, org, { ...input, billingName: "Alpha Holdings" });
    expect(calls()[0]).toStrictEqual(["organizations.save", { id: "org-1", name: "Alpha Holdings", kind: "customer", status: "active" }, v(4)]);
  });

  it("deletes the customer and then its organization, with every account first when forced", async () => {
    dal.coreOp.mockResolvedValue({});
    await deleteCustomer("cust-1", 2, { id: "org-1", version: 4 }, " Mistake ");
    expect(calls()).toStrictEqual([
      ["customers.delete", { id: "cust-1", force: false, reason: "Mistake" }, v(2)],
      ["organizations.delete", { id: "org-1", reason: "Mistake" }, v(4)],
    ]);
    expect(force.deleteAccounts).not.toHaveBeenCalled();
    await deleteCustomer("cust-1", 2, { id: "org-1", version: 4 }, "Test data", true);
    expect(force.deleteAccounts).toHaveBeenCalledWith("org-1", "Test data");
    dal.coreOp.mockReset();
    dal.coreOp.mockRejectedValueOnce(refused("CONFLICT"));
    expect(await deleteCustomer("cust-1", 2, { id: "org-1", version: 4 }, "x", true)).toMatchObject({ ok: false, code: "CONFLICT" });
    expect(dal.coreOp).toHaveBeenCalledOnce(); // nothing else removed after a refused customer delete
  });
});

describe("places and units", () => {
  it("saves properties, spaces and units new or at their version, and archives or deletes them with a reason", async () => {
    dal.coreOp.mockResolvedValue({ id: "x-1" });
    const property = { customerOrgId: "org-1", kind: "home" as const, name: "Home", address: null, accessInstructions: null, location: null, geofenceRadiusMeters: null };
    expect(await saveProperty(property)).toStrictEqual({ ok: true, value: "x-1" });
    expect(calls().at(-1)).toStrictEqual(["properties.save", property, { write: true }]);
    await saveProperty({ ...property, id: "p-1" }, 3);
    expect(calls().at(-1)?.[2]).toStrictEqual(v(3));
    expect(await saveSpace({ propertyId: "p-1", parentSpaceId: null, kind: "room", name: "Bedroom" }, 1)).toStrictEqual({ ok: true, value: "x-1" });
    expect(calls().at(-1)?.[0]).toBe("spaces.save");
    const unit = { customerOrgId: "org-1", propertyId: "p-1", spaceId: null, displayName: "Lobby AC", modelId: "m-1", type: "split" as const, installedAt: null, serviceScope: ["indoor" as const] };
    expect(await saveUnit(unit)).toStrictEqual({ ok: true, value: "x-1" });
    const cases: [() => Promise<unknown>, string, unknown, unknown][] = [
      [() => archiveLocation("property", "p-1", 3, "Sold"), "properties.archive", { id: "p-1", reason: "Sold" }, v(3)],
      [() => archiveLocation("space", "s-1", 1, "Merged"), "spaces.archive", { id: "s-1", reason: "Merged" }, v(1)],
      [() => archiveUnit("u-1", 5, "Replaced"), "units.archive", { id: "u-1", reason: "Replaced" }, v(5)],
      [() => deleteUnit("u-1", 5, "Mistake"), "units.delete", { id: "u-1", reason: "Mistake" }, v(5)],
      [() => setUnitPolicies("u-1", 5, ["pol-1"]), "units.setAlertPolicies", { unitId: "u-1", alertPolicyIds: ["pol-1"] }, v(5)],
      [() => setDefaultRule("pol-default", "offline", "cust-1", false, 0), "policies.setDefaultRule", { policyId: "pol-default", ruleKey: "offline", customerId: "cust-1", enabled: false }, v(0)],
    ];
    for (const [call, op, body, options] of cases) {
      expect(await call()).toStrictEqual({ ok: true, value: null });
      expect(calls().at(-1)).toStrictEqual([op, body, options]);
    }
  });

  const unit = { customerOrgId: "org-1", propertyId: "p-1", spaceId: null, displayName: "Lobby AC", modelId: "m-1", type: "split" as const, installedAt: null, serviceScope: ["indoor" as const] };

  it("saves a new unit and binds a registered device to it at the version read", async () => {
    dal.coreOp.mockResolvedValueOnce({ id: "u-new" }).mockResolvedValueOnce({});
    expect(await saveUnitWithDevice(unit, { kind: "existing", deviceId: "d-1", version: 3 })).toStrictEqual({ ok: true, value: { id: "u-new", deviceError: null } });
    expect(calls()[1]).toStrictEqual(["devices.bind", { deviceId: "d-1", unitId: "u-new", reason: "Registered with the unit" }, v(3)]);
  });

  it("registers a new device for the unit and binds it, with the node ID only when given", async () => {
    dal.coreOp.mockResolvedValueOnce({ id: "u-new" }).mockResolvedValueOnce({ id: "d-new", version: 1 }).mockResolvedValueOnce({});
    await saveUnitWithDevice(unit, { kind: "new", serial: "AC-9", mqttNode: null, iotModelId: "i-1", sensorTypes: ["temperature"] });
    expect(calls()[1]).toStrictEqual(["devices.register", { serial: "AC-9", iotModelId: "i-1", sensorTypes: ["temperature"], unitId: "u-new" }, { write: true }]);
    expect(calls()[2]).toStrictEqual(["devices.bind", { deviceId: "d-new", unitId: "u-new", reason: "Registered with the unit" }, v(1)]);
    dal.coreOp.mockReset();
    dal.coreOp.mockResolvedValueOnce({ id: "u-2" }).mockResolvedValueOnce({ id: "d-2", version: 1 }).mockResolvedValueOnce({});
    await saveUnitWithDevice(unit, { kind: "new", serial: "AC-10", mqttNode: "node_3", iotModelId: "i-1", sensorTypes: [] });
    expect(calls()[1]?.[1]).toMatchObject({ mqttNode: "node_3" });
  });

  it("keeps the unit when the device is refused, saying why field by field, and fails on anything else", async () => {
    dal.coreOp.mockResolvedValueOnce({ id: "u-new" }).mockRejectedValueOnce(refused("VALIDATION", "error.validation", { serial: "taken", mqttNode: "invalid" }));
    expect(await saveUnitWithDevice(unit, { kind: "new", serial: "AC-1", mqttNode: null, iotModelId: "i-1", sensorTypes: [] })).toStrictEqual({ ok: true, value: { id: "u-new", deviceError: "serial: taken, mqttNode: invalid" } });
    dal.coreOp.mockResolvedValueOnce({ id: "u-2" }).mockRejectedValueOnce(refused("CONFLICT"));
    expect(await saveUnitWithDevice(unit, { kind: "existing", deviceId: "d-1", version: 3 })).toStrictEqual({ ok: true, value: { id: "u-2", deviceError: "error.conflict" } });
    dal.coreOp.mockResolvedValueOnce({ id: "u-3" }).mockRejectedValueOnce(new TypeError("fetch failed"));
    expect(await saveUnitWithDevice(unit, { kind: "existing", deviceId: "d-1", version: 3 })).toMatchObject({ ok: false, code: "UNAVAILABLE" });
  });

  it("searches addresses only for an Admin session with asset.write", async () => {
    expect(await searchAddress("Jalan Ampang")).toStrictEqual({ ok: true, places: [] });
    dal.corePermissions.mockResolvedValue(new Set(["asset.read"]));
    expect(await searchAddress("x")).toStrictEqual({ ok: false, reason: "off" });
    dal.corePermissions.mockResolvedValue(new Set(["asset.write"]));
    dal.verifySession.mockResolvedValue({ role: "contractor" });
    expect(await searchAddress("x")).toStrictEqual({ ok: false, reason: "off" });
    expect(geocode.searchPlaces).toHaveBeenCalledExactlyOnceWith("Jalan Ampang"); // only the permitted search reached the provider
  });
});

describe("client users, import and warranty", () => {
  it("adds, changes, removes and signs out client users, with trimmed reasons", async () => {
    dal.coreOp.mockResolvedValue({ id: "cu-1" });
    expect(await inviteClientUser("cust-1", " a@example.com ", "owner", " Aisha ")).toStrictEqual({ ok: true, value: "cu-1" });
    expect(calls().at(-1)).toStrictEqual(["clientUsers.save", { customerId: "cust-1", clientRole: "owner", email: "a@example.com", displayName: "Aisha" }, { write: true }]);
    await updateClientUser("cu-1", 2, { customerId: "cust-1", email: "a@example.com", clientRole: "member", status: "disabled", reason: " Left " });
    expect(calls().at(-1)).toStrictEqual(["clientUsers.save", { id: "cu-1", customerId: "cust-1", email: "a@example.com", clientRole: "member", status: "disabled", reason: "Left" }, v(2)]);
    await removeClientUser("cu-1", 3, " Gone ");
    expect(calls().at(-1)).toStrictEqual(["clientUsers.remove", { id: "cu-1", reason: "Gone" }, v(3)]);
    await signOutEverywhere("m-1", " Lost phone ");
    expect(calls().at(-1)).toStrictEqual(["members.revokeSessions", { membershipId: "m-1", reason: "Lost phone" }, { write: true }]);
    await resendInvite("cu-1");
    expect(calls().at(-1)).toStrictEqual(["clientUsers.resendInvite", { id: "cu-1" }, { write: true }]);
    await previewPasswordReset("a@example.com");
    expect(calls().at(-1)).toStrictEqual(["auth.previewPasswordReset", { demoEmail: "a@example.com" }]); // a read: no write option
  });

  it("previews a CSV import, commits it once, undoes it at its version, and records a warranty claim", async () => {
    dal.coreOp.mockResolvedValue({ id: "imp-1", version: 1 });
    expect(await importPreview("cust-1", "units.csv", "a,b", { name: "a" })).toStrictEqual({ ok: true, value: { id: "imp-1", version: 1 } });
    expect(calls().at(-1)).toStrictEqual(["units.importPreview", { customerId: "cust-1", fileName: "units.csv", csvText: "a,b", mapping: { name: "a" } }]);
    await importCommit("prev-1", "cust-1");
    expect(calls().at(-1)).toStrictEqual(["units.importCommit", { previewId: "prev-1", customerId: "cust-1" }, { write: true }]);
    await importUndo("imp-1", 1, " Wrong file ");
    expect(calls().at(-1)).toStrictEqual(["units.importUndo", { importId: "imp-1", reason: "Wrong file" }, v(1)]);
    expect(await recordWarrantyClaim("job-1", 4, " Compressor ", 12_000, "MYR", " Covered ")).toStrictEqual({ ok: true, value: null });
    expect(calls().at(-1)).toStrictEqual(["jobs.recordWarrantyClaim", { jobId: "job-1", amountMinor: 12_000, currency: "MYR", partLabel: "Compressor", reason: "Covered" }, v(4)]);
    dal.coreOp.mockRejectedValueOnce(refused("CONFLICT"));
    expect(await importCommit("prev-1", "cust-1")).toMatchObject({ ok: false, code: "CONFLICT" });
  });
});
