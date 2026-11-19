import type { ApiUnitDetail } from "@ac/web/lib/units";

/** A complete unit as units.get answers it, with the fields a test cares about replaced. */
export const unitDetail = (over: Partial<ApiUnitDetail> = {}): ApiUnitDetail => ({
  id: "u1", version: 1, spaceId: null, displayName: "Unit", installState: "installed", modelId: "model-1", alertPolicyIds: [], connection: "online", effectivePowerState: "on",
  observedState: { power: true, celsius: 24, mode: "cool", fanLevel: "mid", observedAt: null }, lastSeenAt: null, latestMeasurements: [],
  capabilities: {
    manufacturer: "AC-Co", model: "m1", control: true, modeControl: true, fanControl: true, temperature: { min: 18, max: 30, step: 1 }, modes: ["cool", "dry", "fan"],
    fanLevels: ["low", "mid", "high"], ventilation: false, ventilationLevels: [],
  },
  iotModel: null,
  effectiveControlPolicy: { state: "unrestricted" }, controlAvailability: { state: "available" }, pendingCommands: [],
  location: { pathLabels: [], address: null, accessInstructions: null }, installedAt: null, capabilityVersion: 1, serviceScope: ["indoor"], components: [], ...over,
});
