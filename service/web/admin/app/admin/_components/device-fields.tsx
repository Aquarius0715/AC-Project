"use client";

import { Check, Field, Input, Select } from "@ac/web/components/ui";
import { useT } from "@ac/web/components/I18n";
import { metrics, metricUnit, mqttNodeValue, type Metric } from "@ac/web/lib/devices";
import type { PickerOption } from "@ac/web/lib/pickerOption";
import { searchDevices, searchUnits } from "../devices/actions";

/** The IoT models a device can be of, with the sensors each carries (the IoT model register, not the aircon model). */
export interface IoTChoice { id: string; label: string; sensors: Metric[] }

// the options of the pickers come from the server as the user types (every record in scope, not only a first page); the
// functions are module-level so a picker's effect does not restart on every render
async function found(read: Promise<{ ok: true; value: PickerOption[] } | { ok: false; messageKey: string }>): Promise<PickerOption[]> {
  const r = await read;
  if (!r.ok) throw new Error(r.messageKey);
  return r.value;
}
/** Devices registered and not on a unit. */
export const freeDevices = (q: string) => found(searchDevices(q, false, true));
/** Units that have no device. */
export const unitsWithoutDevice = (q: string) => found(searchUnits(q, true));
/** Every unit in scope. */
export const allUnits = (q: string) => found(searchUnits(q, false));

export interface NewDraft { serial: string; node: string; iotModelId: string; sensors: Metric[] }
export const NO_NEW: NewDraft = { serial: "", node: "", iotModelId: "", sensors: [] };

/** What a new device starts with: the IoT model's sensors, all of them, for the user to take off the ones it lacks. */
export function NewDeviceFields({ iotModels, value, onChange, tried }: Readonly<{ iotModels: IoTChoice[]; value: NewDraft; onChange: (v: NewDraft) => void; tried: boolean }>) {
  const t = useT();
  const model = iotModels.find((k) => k.id === value.iotModelId);
  return (
    <>
      <Field label={t("Device serial")} error={tried && !value.serial.trim() ? t("Serial is required") : undefined}><Input value={value.serial} onChange={(e) => { onChange({ ...value, serial: e.target.value }); }} placeholder="AC-DEMO-0006" /></Field>
      <Field label={t("MQTT node ID (optional)")} error={tried && mqttNodeValue(value.node) === undefined ? t("Use 1–64 letters, digits, hyphens or underscores") : undefined}><Input value={value.node} onChange={(e) => { onChange({ ...value, node: e.target.value }); }} placeholder="node_3" /></Field>
      <Field label={t("IoT model · sensors")} hint={t("What the device measures comes from this model; it is not the aircon model of the unit.")} error={tried && !value.iotModelId ? t("Choose an IoT model") : undefined}>
        <Select value={value.iotModelId} onChange={(e) => { onChange({ ...value, iotModelId: e.target.value, sensors: iotModels.find((k) => k.id === e.target.value)?.sensors ?? [] }); }}><option value="">{t("Select…")}</option>{iotModels.map((k) => <option key={k.id} value={k.id}>{k.label}</option>)}</Select>
      </Field>
      {model && (
        <div className="sm:col-span-2"><p className="mb-1 text-[13px] font-semibold">{t("Sensors")}</p><p className="mb-1 text-xs text-muted-foreground">{t("All sensors of the IoT model are selected; take off the ones this device does not carry.")}</p>
          <div className="flex flex-wrap gap-x-5 gap-y-1.5">{metrics.filter((m) => model.sensors.includes(m)).map((m) => <Check key={m} label={`${m} (${metricUnit[m]})`} checked={value.sensors.includes(m)} onChange={(on) => { onChange({ ...value, sensors: on ? [...value.sensors, m] : value.sensors.filter((x) => x !== m) }); }} />)}</div>
          {!value.sensors.includes("temperature") && <p role="status" className="mt-2 text-xs font-semibold text-warn">{t("No temperature sensor: temperature cannot be measured for this device.")}</p>}</div>
      )}
    </>
  );
}

