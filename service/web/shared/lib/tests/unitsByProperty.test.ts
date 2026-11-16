import { describe, expect, it } from "vitest";
import { unitsByProperty } from "@ac/web/lib/clientBilling";

describe("unitsByProperty", () => {
  it("groups the units by property, sorted, each named with the rest of its path", () => {
    expect(unitsByProperty([
      { id: "u1", name: "Meeting room AC", place: "Office A › Office 1F › Meeting room" },
      { id: "u2", name: "Bedroom AC", place: "Home A › 1F › Bedroom" },
      { id: "u3", name: "Lobby AC", place: "Office A" },
      { id: "u4", name: "Living room AC", place: "Home A › 1F › Living room" },
    ])).toStrictEqual([
      { property: "Home A", units: [{ id: "u2", label: "Bedroom AC · 1F › Bedroom" }, { id: "u4", label: "Living room AC · 1F › Living room" }] },
      { property: "Office A", units: [{ id: "u3", label: "Lobby AC" }, { id: "u1", label: "Meeting room AC · Office 1F › Meeting room" }] },
    ]);
  });
});
