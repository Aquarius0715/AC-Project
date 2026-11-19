import { describe, expect, it } from "vitest";
import { scopeLists } from "../scopeLists";

const customers = [{ id: "cb", name: "Customer B", organizationId: "ob" }, { id: "ca", name: "Customer A", organizationId: "oa" }];
const properties = [{ id: "p2", name: "Office", customerOrgId: "oa" }, { id: "p3", name: "Home", customerOrgId: "ob" }, { id: "p1", name: "Home", customerOrgId: "oa" }];
const units = [
  { id: "u1", displayName: "Lobby AC", customerOrgId: "oa", propertyId: "p2" },
  { id: "u2", displayName: "Bedroom AC", customerOrgId: "ob", propertyId: "p3" },
  { id: "u3", displayName: "Bedroom AC", customerOrgId: "oa", propertyId: "p1" },
];

describe("scopeLists (UI/UX loop R127)", () => {
  it("sorts customers and properties by name and groups units customer › property", () => {
    const s = scopeLists(customers, properties, units, {});
    expect(s.customers.map((c) => c.name)).toStrictEqual(["Customer A", "Customer B"]);
    expect(s.properties.map((p) => p.id)).toStrictEqual(["p3", "p1", "p2"]); // equal names keep their order
    expect(s.unitGroups.map((g) => [g.label, g.units.map((u) => u.id)])).toStrictEqual([
      ["Customer A › Home", ["u3"]], ["Customer A › Office", ["u1"]], ["Customer B › Home", ["u2"]],
    ]);
  });
  it("narrows to the chosen customer and property", () => {
    const s = scopeLists(customers, properties, units, { customerId: "ca", propertyId: "p2" });
    expect(s.properties.map((p) => p.id)).toStrictEqual(["p1", "p2"]);
    expect(s.unitGroups).toStrictEqual([{ key: "p2", label: "Customer A › Office", units: [{ id: "u1", name: "Lobby AC" }] }]);
  });
});
