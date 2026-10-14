// The customer unit row from the Core API's UnitSummary (DATA_SOURCE=api): online units show the reading's time in
// Malaysia time, offline ones when they were last seen; the power state and connection map onto the row's words.
import { describe, expect, it } from "vitest";
import { unitRowFromApi, type ApiUnit } from "@ac/web/lib/client";

const unit = (over: Partial<ApiUnit>): ApiUnit => ({
  id: "u-1",
  displayName: "Bedroom AC",
  connection: "online",
  effectivePowerState: "on",
  observedState: { power: true, celsius: 24.5, observedAt: "2026-09-20T01:12:00Z" },
  lastSeenAt: "2026-09-20T01:12:00Z",
  ...over,
});

describe("unitRowFromApi", () => {
  it("shows an online unit's reading time in MYT and its temperature", () => {
    expect(unitRowFromApi(unit({}))).toMatchObject({ id: "u-1", name: "Bedroom AC", obs: "09:12", temp: 24.5, state: "running", conn: "online" });
  });

  it("shows when an offline unit was last seen, and folds the other connection states into offline", () => {
    for (const connection of ["offline", "unknown", "connecting", "error"] as const) {
      const row = unitRowFromApi(unit({ connection, lastSeenAt: "2026-09-19T10:40:00Z" }));
      expect(row).toMatchObject({ obs: "last seen 18:40", conn: "offline" });
    }
  });

  it("maps the power state and shows a dash without a time", () => {
    expect(unitRowFromApi(unit({ effectivePowerState: "off" })).state).toBe("stopped");
    expect(unitRowFromApi(unit({ effectivePowerState: "unknown" })).state).toBe("unknown");
    expect(unitRowFromApi(unit({ observedState: { power: null, celsius: null, observedAt: null } }))).toMatchObject({ obs: "—", temp: null });
  });
});
