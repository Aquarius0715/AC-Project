// Demo controls in API mode (FR-X05, DDC-07, SCR-X-demo, IR154, IR249; Figma Client 10e): the scenario clock of the Core
// API, the devices the signed-in role can see for the device fault and its recovery, and the refusals of demo.*.
import type { ActionFailure } from "@ac/web/lib/actionMessage";

/** “2026-09-14 09:41 UTC”, the clock as Figma 10e shows it. */
export const clockText = (iso: string) => `${iso.slice(0, 10)} ${iso.slice(11, 16)} UTC`;

/** demo.advanceClock moves forward only: the target is the clock now plus the jump. */
export const advanceTarget = (now: Date, minutes: number) => new Date(now.getTime() + minutes * 60_000).toISOString();

export type ApiDevice = { id: string; serial: string; unitId: string | null; bindingId: string | null; connection: string };
export type ApiDeviceEvent = { id: string; eventType: string; sequence: number; occurredAt: string; restoredAt: string | null };

/** The latest connection fault that has not been restored: the source a recovery must name (SR20). */
export function openFault(events: ApiDeviceEvent[]): ApiDeviceEvent | null {
  return events.filter((e) => e.eventType === "communication_lost" && !e.restoredAt).sort((a, b) => b.sequence - a.sequence)[0] ?? null;
}

/** The next device sequence: above every event of the device, so the signal is never taken for an old one. */
export const nextSequence = (events: ApiDeviceEvent[]) => Math.max(0, ...events.map((e) => e.sequence)) + 1;

export type DeviceOption = { id: string; unit: string; label: string; connection: string; fault: string | null };
/** One option per bound device: unit name, serial and connection; `fault` is the open connection fault to restore. */
export function deviceOptions(devices: ApiDevice[], unitNames: Map<string, string>, faults: Map<string, string>): DeviceOption[] {
  return devices.filter((d) => d.unitId && d.bindingId).map((d) => {
    const unit = unitNames.get(d.unitId!) ?? "Unit";
    return { id: d.id, unit, label: `${unit} · ${d.serial} · ${d.connection}`, connection: d.connection, fault: faults.get(d.id) ?? null };
  });
}

/** What a refused demo operation means for the person at the panel. */
export function demoRefusal(f: ActionFailure): string {
  const keys = [f.messageKey, ...Object.values(f.fieldErrors ?? {})];
  if (keys.includes("errors.demo_only")) return "Demo operations are off in this environment";
  if (keys.includes("errors.clock_backwards")) return "The clock only moves forward — reload and try again";
  if (keys.includes("errors.binding_mismatch")) return "The device was rebound — reload to pick it again";
  if (keys.includes("errors.recovery_source_not_current")) return "The fault was already restored or replaced — reload";
  if (f.code === "FORBIDDEN" || f.code === "NOT_FOUND") return "This device is not visible to you any more";
  return "Not done — try again";
}
