import { describe, expect, it } from "vitest";
import {
  changedFields, conditionText, coverageRows, standing as standingOf, unitErrors, type ApiContractLite, type ApiCoverage,
  contractMatch, coverageMatch, customerDraft, customerErrors, customerMatch, filterUnits, importMessage, locationTree, nameError, propertyDraft, propertyErrors, propertyInput, reasonError,
  selection, selectionId, spacePath, unitInput, unitsAt, unitDraft, relocates,
  type ApiImportRow, type ApiPropertyRow, type ApiSpaceRow, type ApiUnitRow, type CoverageRow, type CustomerRow, type Standing,
} from "@ac/web/lib/assets";

// The forms, filters and location tree of HQ customers & units (FR-A02, FR-A18, FR-A19): what each filter keeps, the
// drafts and checks of a customer, property and unit, the location tree with its selection, and the words of an import
// row's result.

const row = (over: Partial<CustomerRow> = {}): CustomerRow => ({
  id: "cust-a", version: 1, name: "Tan household", billingName: "Tan Sdn Bhd", status: "active", orgId: "org-a", orgVersion: 1, orgStatus: "active", profile: "rto", since: "Jan 2026",
  properties: 1, propertyNames: ["Home A"], units: 2, operation: "50%", running: 1, known: 2, alerts: 0, standing: null, marks: [], ...over,
});
const standing = (over: Partial<Standing> = {}): Standing => ({ plans: ["rto"], contracts: 1, contractUnits: 2, overdue: 0, overdueAmount: null, restriction: null, ...over });

const coverageRow = (over: Partial<CoverageRow>): CoverageRow => ({
      unitId: "u-1", unit: "Bedroom AC", sub: "", customerId: "cust-a", customer: "Tan", model: "m", ends: "", endsDate: "", endsAt: null, days: 20, contractIds: [], contracts: "—", status: "expiring",
      statusText: "", ...over,
    });
const importRow = (messageKey: string | null, over: Partial<ApiImportRow> = {}): ApiImportRow => ({
    rowNumber: 2, propertyName: "Office A", floorName: "2F", roomName: "Room 1", unitName: "AC 1", modelCode: "SPL-100", serial: "AC-9", installedOn: null, warrantyEnd: null, result: "error", messageKey, ...over,
  });
const contract = (id: string, over: Partial<ApiContractLite> = {}): ApiContractLite => ({ id, customerId: "cust-a", unitIds: ["u-1"], planType: "rto", startAt: "2026-01-01T00:00:00Z", endAt: "2027-01-01T00:00:00Z", activeRestrictionIds: [], ...over });
const coverageOf = (unitId: string, over: Partial<ApiCoverage>): ApiCoverage => ({ unitId, customerId: "cust-a", modelId: "m-1", warrantyEndsAt: null, contractIds: [], status: "no_coverage", claimableJobIds: [], ...over });

describe("the register's filters", () => {
  it("keeps the customers each contract filter names", () => {
    const good = row({ standing: standing() });
    const overdue = row({ standing: standing({ overdue: 2 }) });
    const restricted = row({ standing: standing({ restriction: { id: "r-1", state: "applied", contractId: "k-1" } }) });
    const none = row({ standing: standing({ contracts: 0 }) });
    const alerting = row({ alerts: 3 });
    const inactive = row({ status: "inactive", standing: standing({ overdue: 1 }), alerts: 2 });
    const unknown = row();
    const keep = (f: Parameters<typeof contractMatch>[0]) => [good, overdue, restricted, none, alerting, inactive, unknown].map((r) => contractMatch(f, r));
    expect(keep("all")).toStrictEqual([true, true, true, true, true, true, true]);
    expect(keep("attention")).toStrictEqual([false, true, true, false, true, false, false]); // an inactive customer never needs attention
    expect(keep("overdue")).toStrictEqual([false, true, false, false, false, false, false]);
    expect(keep("restricted")).toStrictEqual([false, false, true, false, false, false, false]);
    expect(keep("good")).toStrictEqual([true, false, false, false, false, false, false]);
    expect(keep("none")).toStrictEqual([false, false, false, true, false, false, false]); // the standing unknown counts as no contract only when read
  });

  it("searches the name, billing name, ID and property names, ignoring case and spaces", () => {
    const r = row();
    expect(["", "  ", "tan HOUSE", " sdn ", "cust-a", "home a"].map((q) => customerMatch(r, q))).toStrictEqual([true, true, true, true, true, true]);
    expect(customerMatch(r, "Lim")).toBe(false);
  });

  it("keeps the coverage rows of a customer, a status and an end within some days", () => {
    const rows = [coverageRow({}), coverageRow({ unitId: "u-2", customerId: "cust-b", days: 200, status: "under_warranty" }), coverageRow({ unitId: "u-3", days: null, status: "no_coverage" }), coverageRow({ unitId: "u-4", days: -5, status: "no_coverage" })];
    const ids = (f: Parameters<typeof coverageMatch>[1]) => rows.filter((r) => coverageMatch(r, f)).map((r) => r.unitId);
    expect(ids({ customerId: "", coverage: "", within: "" })).toStrictEqual(["u-1", "u-2", "u-3", "u-4"]);
    expect(ids({ customerId: "cust-a", coverage: "", within: "" })).toStrictEqual(["u-1", "u-3", "u-4"]);
    expect(ids({ customerId: "", coverage: "no_coverage", within: "" })).toStrictEqual(["u-3", "u-4"]);
    expect(ids({ customerId: "", coverage: "", within: "30" })).toStrictEqual(["u-1"]); // an ended or unknown end is not within
    expect(ids({ customerId: "", coverage: "", within: "365" })).toStrictEqual(["u-1", "u-2"]);
  });
});

describe("the customer and property forms", () => {
  it("starts a new customer as active general care, an existing one from its row, and checks both names", () => {
    expect(customerDraft()).toStrictEqual({ billingName: "", name: "", profile: "general", status: "active" });
    expect(customerDraft(row({ status: "inactive" }))).toStrictEqual({ billingName: "Tan Sdn Bhd", name: "Tan household", profile: "rto", status: "inactive" });
    expect(customerErrors(customerDraft())).toStrictEqual({ billingName: "1–120 characters", name: "1–120 characters" });
    expect(customerErrors({ billingName: "x".repeat(121), name: " Tan ", profile: "rto", status: "active" })).toStrictEqual({ billingName: "1–120 characters" });
    expect(customerErrors(customerDraft(row()))).toStrictEqual({});
  });

  it("drafts a property, checks its lengths, and sends empty texts as none", () => {
    expect(propertyDraft()).toStrictEqual({ kind: "home", name: "", address: "", accessInstructions: "", location: null, geofenceRadiusMeters: null });
    const d = { kind: "office" as const, name: " HQ ", address: "a".repeat(501), accessInstructions: "b".repeat(1001), location: { latitude: 3.1, longitude: 101.6 }, geofenceRadiusMeters: 300 };
    expect(propertyErrors({ ...d, name: "" })).toStrictEqual({ name: "1–120 characters", address: "At most 500 characters", accessInstructions: "At most 1000 characters" });
    expect(propertyErrors({ ...d, address: " A ", accessInstructions: "" })).toStrictEqual({});
    expect(propertyInput({ ...d, address: "  ", accessInstructions: " Gate 2 " }, "org-a", "p-1")).toStrictEqual({
      id: "p-1", customerOrgId: "org-a", kind: "office", name: "HQ", address: null, accessInstructions: "Gate 2", location: { latitude: 3.1, longitude: 101.6 }, geofenceRadiusMeters: 300,
    });
    expect(Object.keys(propertyInput(d, "org-a"))).not.toContain("id");
  });

  it("checks a location name and a reason", () => {
    expect([nameError(""), nameError("Bedroom"), nameError("x".repeat(121))]).toStrictEqual(["1–120 characters", undefined, "1–120 characters"]);
    expect([reasonError(" "), reasonError("Moved"), reasonError("x".repeat(1001))]).toStrictEqual(["A reason is required (1–1000 characters)", undefined, "A reason is required (1–1000 characters)"]);
  });
});

const property = (id: string, over: Partial<ApiPropertyRow> = {}): ApiPropertyRow => ({
  id, version: 1, customerOrgId: "org-a", kind: "home", name: `Home ${id}`, address: null, accessInstructions: null, location: null, geofenceRadiusMeters: null, archived: false, updatedAt: "2026-09-01T00:00:00Z", ...over,
});
const space = (id: string, propertyId: string, kind: ApiSpaceRow["kind"], parentSpaceId: string | null = null, over: Partial<ApiSpaceRow> = {}): ApiSpaceRow => ({ id, propertyId, kind, parentSpaceId, version: 1, name: `${kind} ${id}`, archived: false, ...over });
const unit = (id: string, propertyId: string, spaceId: string | null, over: Partial<ApiUnitRow> = {}): ApiUnitRow => ({
  id, propertyId, spaceId, version: 3, customerOrgId: "org-a", displayName: `Unit ${id}`, modelId: "m-1", type: "split", installState: "installed", installedAt: null, serviceScope: ["indoor"], alertPolicyIds: [],
  archived: false, capabilityVersion: 1, connection: "online", lastSeenAt: null, warrantyEndsAt: null, effectivePowerState: "on", activeAlertCount: 0, ...over,
});
const props = [property("p-b"), property("p-a"), property("p-gone", { archived: true }), property("p-other", { customerOrgId: "org-z" })];
const spaces = [space("s-floor", "p-a", "floor"), space("s-room", "p-a", "room", "s-floor"), space("s-old", "p-a", "room", "s-floor", { archived: true }), space("s-area", "p-b", "area")];
const units = [unit("u-1", "p-a", "s-room"), unit("u-2", "p-a", null, { effectivePowerState: "off", connection: "offline" }), unit("u-3", "p-b", "s-area")];
const tree = locationTree(props, spaces, units, "org-a");

const at = (id: string) => {
      const sel = selection(tree, id, spaces);
      return sel ? unitsAt(sel, units).map((u) => u.id) : [];
    };
const unitsKept = (f: Parameters<typeof filterUnits>[1]) => filterUnits(units, f).map((u) => u.id);

describe("the location tree", () => {
  it("lists the customer's live properties by name with their spaces, counts and unassigned units", () => {
    expect(tree.map((p) => p.id)).toStrictEqual(["p-a", "p-b"]); // by name; archived and other customers' left out
    const [a] = tree;
    expect([a?.units, a?.unassigned, a?.floors, a?.rooms]).toStrictEqual([2, 1, 1, 1]);
    expect(a?.spaces.map((s) => [s.id, s.units, s.direct, s.children.map((c) => c.id)])).toStrictEqual([["s-floor", 1, 0, ["s-room"]]]); // the archived room left out
    expect(spacePath("s-room", spaces)).toBe("floor s-floor › room s-room");
    expect(spacePath(null, spaces)).toBe("");
  });

  it("selects a property, a space or the unassigned units from the URL, the first property otherwise", () => {
    const pick = (id: string | undefined) => selection(tree, id, spaces);
    expect([pick("p-b")?.kind, selectionId(pick("p-b"))]).toStrictEqual(["property", "p-b"]);
    const room = pick("s-room");
    expect([room?.kind, selectionId(room), room?.kind === "space" ? room.path : null]).toStrictEqual(["space", "s-room", "Home p-a › floor s-floor › room s-room"]);
    expect([pick("unassigned:p-a")?.kind, selectionId(pick("unassigned:p-a"))]).toStrictEqual(["unassigned", "unassigned:p-a"]);
    expect(selectionId(pick("unassigned:p-gone"))).toBe("p-a"); // an unknown property: the first
    expect(selectionId(pick(undefined))).toBe("p-a");
    expect([selection([], "p-a", spaces), selectionId(null)]).toStrictEqual([null, ""]);
  });

  it("lists the units at a selection, a space with its descendants, and filters them by power, connection and search", () => {
    expect([at("p-a"), at("s-floor"), at("s-room"), at("unassigned:p-a"), at("p-b")]).toStrictEqual([["u-1", "u-2"], ["u-1"], ["u-1"], ["u-2"], ["u-3"]]);
    expect(unitsKept({ connections: [], search: "" })).toStrictEqual(["u-1", "u-2", "u-3"]);
    expect(unitsKept({ powerState: "off", connections: [], search: "" })).toStrictEqual(["u-2"]);
    expect(unitsKept({ connections: ["online"], search: "" })).toStrictEqual(["u-1", "u-3"]);
    expect(unitsKept({ connections: [], search: " UNIT U-3 " })).toStrictEqual(["u-3"]);
    expect(unitsKept({ connections: [], search: "u-1" })).toStrictEqual(["u-1"]); // its ID
  });
});

describe("the unit form", () => {
  it("starts a new unit at the chosen place and an existing one from the unit, its dates as Kuala Lumpur days", () => {
    expect(unitDraft(undefined, "p-a", "s-room")).toMatchObject({ propertyId: "p-a", spaceId: "s-room", installState: "installed", serviceScope: ["indoor", "outdoor"], installedAt: "" });
    const saved = unit("u-1", "p-a", null, { installedAt: "2026-01-09T16:30:00Z", warrantyEndsAt: "2027-01-09T16:00:00Z", iotModelId: "i-1" });
    expect(unitDraft(saved, "p-x", "s-x")).toMatchObject({ propertyId: "p-a", spaceId: "", installedAt: "2026-01-10", warrantyEnd: "2027-01-10", iotModelId: "i-1" });
  });

  it("sends the dates at Kuala Lumpur midnight, empty fields as none, and the reason of a move", () => {
    const d = { displayName: " Bedroom AC ", propertyId: "p-a", spaceId: "", modelId: "m-1", iotModelId: "", installState: "installed" as const, installedAt: "2026-01-10", warrantyEnd: "", serviceScope: ["indoor" as const] };
    expect(unitInput(d, "org-a")).toStrictEqual({
      customerOrgId: "org-a", propertyId: "p-a", spaceId: null, displayName: "Bedroom AC", modelId: "m-1", iotModelId: null, type: "split", installState: "installed",
      installedAt: "2026-01-10T00:00:00+08:00", warrantyEndsAt: null, serviceScope: ["indoor"],
    });
    expect(unitInput({ ...d, spaceId: "s-room", iotModelId: "i-1", installedAt: "", warrantyEnd: "2027-01-10" }, "org-a", "u-1", "Moved")).toMatchObject({
      id: "u-1", spaceId: "s-room", iotModelId: "i-1", installedAt: null, warrantyEndsAt: "2027-01-10T00:00:00+08:00", changeReason: "Moved",
    });
    expect([relocates({ propertyId: "p-a", spaceId: null }, d), relocates({ propertyId: "p-a", spaceId: "s-room" }, d), relocates({ propertyId: "p-b", spaceId: null }, d)]).toStrictEqual([false, true, true]);
  });
});

describe("an import row's result", () => {

  it("words every result the preview gives, and keeps a key it does not know", () => {
    expect([
      importMessage(importRow(null)), importMessage(importRow("warning.importCreatesProperty")), importMessage(importRow("warning.importCreatesSpace")), importMessage(importRow("warning.importCreatesSpace", { roomName: null })),
      importMessage(importRow("warning.importCreatesSpace", { roomName: null, floorName: null })), importMessage(importRow("error.unknownModel")), importMessage(importRow("error.serialBound")),
      importMessage(importRow("error.unknownSerial", { serial: null })), importMessage(importRow("error.duplicateSerial")), importMessage(importRow("error.tamperUnresolved")), importMessage(importRow("error.duplicateSiblingName")),
      importMessage(importRow("error.future")), importMessage(importRow("error.importInvalidDate")), importMessage(importRow("error.importMissingField")), importMessage(importRow("error.length")), importMessage(importRow("error.somethingNew")),
    ]).toStrictEqual([
      "Ready", "Property “Office A” does not exist — it will be created (office; add its address afterwards).", "“Room 1” does not exist — it will be created.", "“2F” does not exist — it will be created.",
      "“” does not exist — it will be created.", "Model SPL-100 is not in the model register — add it under Devices & models first.", "Serial AC-9 is already bound to another unit.",
      "Serial  is not a registered device.", "Serial AC-9 appears more than once in this file.", "Device AC-9 has an unresolved tamper.", "A unit named “AC 1” already exists there.",
      "The installation date is in the future.", "A date is not a valid YYYY-MM-DD date.", "Property, unit name and model are required.", "The unit name is longer than 120 characters.", "error.somethingNew",
    ]);
  });
});

describe("the contract standing", () => {
  const now = new Date("2026-09-14T01:00:00Z");

  it("names the most pressing restriction of the customer's contracts, the state known", () => {
    const restrictions = [
      { id: "r-sched", contractId: "k-1", state: "scheduled", unitIds: [] }, { id: "r-req", contractId: "k-1", state: "requested", unitIds: [] }, { id: "r-done", contractId: "k-1", state: "released", unitIds: [] },
      { id: "r-other", contractId: "k-other", state: "applied", unitIds: [] }, { id: "r-none", state: "applied", unitIds: [] },
    ];
    expect(standingOf("cust-a", [contract("k-1")], [], restrictions, now).restriction).toStrictEqual({ id: "r-req", state: "requested", contractId: "k-1" });
    expect(standingOf("cust-a", [contract("k-1")], [], [], now).restriction).toBeNull();
  });

  it("without restriction.read, says a contract's active restriction is there, its state unknown", () => {
    expect(standingOf("cust-a", [contract("k-0"), contract("k-1", { activeRestrictionIds: ["r-9"] })], [], null, now).restriction).toStrictEqual({ id: "r-9", state: "active", contractId: "k-1" });
    expect(standingOf("cust-a", [contract("k-1")], [], null, now).restriction).toBeNull();
    const ended = standingOf("cust-a", [contract("k-old", { endAt: "2026-06-01T00:00:00Z" })], [], [], now);
    expect([ended.contracts, ended.plans, ended.contractUnits]).toStrictEqual([0, [], 0]); // an ended contract does not count
  });
});

describe("the unit form's checks and changes", () => {
  const draft = unitDraft(undefined, "p-a");

  it("checks a planned unit has no dates yet, no installation in the future, and a warranty after it", () => {
    const base = { ...draft, displayName: "AC", modelId: "m-1" };
    expect(unitErrors(base, "2026-09-14")).toStrictEqual({});
    expect(unitErrors({ ...base, installState: "planned", installedAt: "2026-09-01", warrantyEnd: "2027-09-01" }, "2026-09-14")).toStrictEqual({
      installedAt: "A planned unit has no installation date yet", warrantyEnd: "A planned unit has no warranty end yet",
    });
    expect(unitErrors({ ...base, installedAt: "2026-09-15", warrantyEnd: "2026-09-10" }, "2026-09-14")).toStrictEqual({
      installedAt: "An installation date cannot be in the future", warrantyEnd: "A warranty cannot end before the installation",
    });
    expect(unitErrors({ ...draft, propertyId: "", serviceScope: [] }, "2026-09-14")).toStrictEqual({
      displayName: "1–120 characters", propertyId: "Choose a location", modelId: "Choose a model", serviceScope: "Choose at least one inspection group",
    });
  });

  it("lists the fields a draft changes against the saved unit, the scope in any order", () => {
    const saved = unit("u-1", "p-a", "s-room", { serviceScope: ["outdoor", "indoor"], iotModelId: "i-1" });
    const same = unitDraft(saved);
    expect(changedFields(saved, { ...same, serviceScope: ["indoor", "outdoor"] })).toStrictEqual([]);
    expect(changedFields(saved, { ...same, displayName: " Unit u-1 " })).toStrictEqual([]); // only spaces around it
    expect(changedFields(saved, { ...same, displayName: "Bedroom", spaceId: "", modelId: "m-2", iotModelId: "", installedAt: "2026-01-01", warrantyEnd: "2027-01-01", serviceScope: ["indoor"] })).toStrictEqual([
      "Name", "Location", "Model", "IoT model", "Installed at", "Warranty end", "Service scope",
    ]);
  });
});

describe("a policy's condition and the warranty rows", () => {
  it("words a condition with its window, duration and severity, or the heartbeat gap", () => {
    expect(conditionText({ metric: "temperature", operator: "gte", threshold: 30, durationSeconds: 7200, activeWindow: { weekdays: [1, 2, 3, 4, 5], startLocal: "08:00", endLocal: "19:00" }, severity: "warning" }))
      .toBe("Temperature ≥ 30 °C for 2 h, weekdays 08:00–19:00 → Warning");
    expect(conditionText({ metric: "humidity", operator: "lt", threshold: 30, durationSeconds: 600, activeWindow: { weekdays: [6, 7], startLocal: "00:00", endLocal: "06:00" } })).toMatch(/^Humidity < 30 % for 10 min, weekends 00:00–06:00$/);
    expect(conditionText({ metric: "co2", threshold: 1200, durationSeconds: 45, activeWindow: { weekdays: [1, 2, 3, 4, 5, 6, 7], startLocal: "08:00", endLocal: "09:00" }, severity: "critical" })).toMatch(/≥ 1200 ppm for 45 s, 08:00–09:00 → Critical$/);
    expect(conditionText({ metric: "temperature", threshold: 30, durationSeconds: 60, activeWindow: { weekdays: [3, 1], startLocal: "08:00", endLocal: "09:00" } })).toMatch(/for 60 s, Mon, Wed 08:00–09:00$/);
    expect(conditionText({ metric: "heartbeat_gap", threshold: 10, severity: "normal" })).toBe("No heartbeat for 10 min → Info");
    expect(conditionText({})).toBe(" ≥ 0  for 0 s"); // nothing known: an empty sentence rather than a failure
  });

  it("orders the coverage by the soonest warranty end, units without one last, and words each status", () => {
    const rows = coverageRows([
      coverageOf("u-none-b", {}), coverageOf("u-none-a", {}), coverageOf("u-ended", { warrantyEndsAt: "2026-08-01T00:00:00Z" }), coverageOf("u-soon", { warrantyEndsAt: "2026-10-01T00:00:00Z", status: "expiring" }),
      coverageOf("u-ok", { warrantyEndsAt: "2027-06-01T00:00:00Z", status: "under_warranty" }), coverageOf("u-contract", { status: "contract", contractIds: ["k-1"] }),
    ], { unit: (id) => (id.startsWith("u-none") ? { name: id, place: "Home" } : undefined), customer: () => "Tan", model: () => "AC-Co", contract: (id) => `RTO ${id}` }, new Date("2026-09-14T01:00:00Z"));
    expect(rows.map((r) => [r.unitId, r.statusText])).toStrictEqual([
      ["u-ended", "Warranty ended · no contract"], ["u-soon", "Ends in 17 d · no contract"], ["u-ok", "Under warranty"],
      ["u-contract", "Covered by contract"], ["u-none-a", "No warranty · no contract"], ["u-none-b", "No warranty · no contract"],
    ]);
    expect(rows.find((r) => r.unitId === "u-none-a")?.sub).toBe("u-none-a · Home");
    expect([rows[0]?.unit, rows[0]?.contracts, rows[3]?.contracts]).toStrictEqual(["u-ended", "—", "RTO k-1"]); // an unknown unit: its ID
  });
});
