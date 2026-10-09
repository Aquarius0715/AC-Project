import { describe, expect, it } from "vitest";
import { coreRequest } from "@ac/web/lib/rest";
import { coreRoutes } from "@ac/web/lib/routes.gen";

/** The query of a request path as name → all values. */
const query = (path: string) => {
  const u = new URL(`http://core${path}`);
  const out: Record<string, string[]> = {};
  for (const k of new Set(u.searchParams.keys())) out[k] = u.searchParams.getAll(k);
  return { pathname: u.pathname, out };
};

describe("coreRequest", () => {
  it("covers every operation of the catalog", () => {
    expect(Object.keys(coreRoutes)).toHaveLength(200); // IR238 filterCare.getSettings
    expect(Object.values(coreRoutes).flat()).toHaveLength(220);
    expect(coreRequest("filterCare.getSettings", {})).toEqual({ method: "GET", path: "/v1/filter-care/settings" });
    for (const routes of Object.values(coreRoutes)) for (const r of routes) expect(r.path).toMatch(/^\/v1(\/([a-z0-9-]+|\{[A-Za-z]\w*(\.[A-Za-z]\w*)?\}))+$/);
  });

  it("sends a list read's paging query, filters, sort and lists in the query string", () => {
    const r = coreRequest("jobs.list", { filters: { status: "open", unitIds: ["u1", "u2"], overdueOnly: false, statuses: [], origin: null }, sort: { field: "dueAt", direction: "asc" }, limit: 50, cursor: undefined })!;
    expect(r.method).toBe("GET");
    expect(r.body).toBeUndefined();
    expect(query(r.path)).toEqual({ pathname: "/v1/jobs", out: { status: ["open"], unitIds: ["u1", "u2"], overdueOnly: ["false"], statuses: [""], sort: ["dueAt:asc"], limit: ["50"] } });
  });

  it("fills and encodes path parameters and keeps the other fields in the query", () => {
    expect(coreRequest("units.get", { id: "a/b c", jobId: "j1" })).toEqual({ method: "GET", path: "/v1/units/a%2Fb%20c?jobId=j1" });
    expect(coreRequest("session.get", undefined)).toEqual({ method: "GET", path: "/v1/session" });
    const r = coreRequest("commands.list", { unitId: "u1", query: { limit: 20, sort: { field: "requestedAt", direction: "desc" } } })!;
    expect(query(r.path)).toEqual({ pathname: "/v1/commands", out: { unitId: ["u1"], limit: ["20"], sort: ["requestedAt:desc"] } });
    expect(query(coreRequest("sla.scorecard", { period: { from: "2026-08-01T00:00:00Z", to: "2026-09-01T00:00:00Z" } })!.path).out).toEqual({ "period.from": ["2026-08-01T00:00:00Z"], "period.to": ["2026-09-01T00:00:00Z"] });
    expect(query(coreRequest("devices.events", { id: "d1", query: { limit: 50, filters: { from: "x" } } })!.path)).toEqual({ pathname: "/v1/devices/d1/events", out: { limit: ["50"], from: ["x"] } });
  });

  it("creates on the collection and updates on the item", () => {
    expect(coreRequest("units.save", { displayName: "A", spaceId: null })).toEqual({ method: "POST", path: "/v1/units", body: '{"displayName":"A","spaceId":null}' });
    expect(coreRequest("units.save", { id: "u1", displayName: "A" })).toEqual({ method: "PUT", path: "/v1/units/u1", body: '{"displayName":"A"}' });
    expect(coreRequest("units.delete", { id: "u1", reason: "duplicate entry" })).toEqual({ method: "DELETE", path: "/v1/units/u1?reason=duplicate+entry" });
    expect(coreRequest("jobs.checkIn", { jobId: "j1", method: "manual", reason: "no signal" })).toEqual({ method: "POST", path: "/v1/jobs/j1/check-in", body: '{"method":"manual","reason":"no signal"}' });
  });

  it("chooses the route of the fixed field and takes nested path parameters out of the body", () => {
    expect(coreRequest("payouts.transition", { statementId: "s1", action: "mark_paid", reason: "paid" })).toEqual({ method: "POST", path: "/v1/payouts/s1/mark-paid", body: '{"reason":"paid"}' });
    expect(coreRequest("payouts.transition", { statementId: "s1", action: "approve" })).toEqual({ method: "POST", path: "/v1/payouts/s1/approve", body: "{}" });
    // an action no route fixes stays in the body, so the API refuses it (error.pathMismatch)
    expect(coreRequest("payouts.transition", { statementId: "s1", action: "nope" })).toEqual({ method: "POST", path: "/v1/payouts/s1/approve", body: '{"action":"nope"}' });
    expect(coreRequest("firmwareCampaigns.control", { campaignId: "c1", action: "retry_device", deviceId: "d1" })).toEqual({ method: "POST", path: "/v1/firmware-campaigns/c1/devices/d1/retry", body: "{}" });
    expect(coreRequest("locations.rename", { target: { kind: "space", id: "s1" }, name: "Lobby" })).toEqual({ method: "POST", path: "/v1/locations/space/s1/rename", body: '{"target":{},"name":"Lobby"}' });
  });

  it("does not change the caller's input and knows only catalogued operations", () => {
    const input = { id: "u1", displayName: "A" };
    coreRequest("units.save", input);
    expect(input).toEqual({ id: "u1", displayName: "A" });
    expect(coreRequest("nope.nope", {})).toBeNull();
  });
});
