"use client";

import { useState } from "react";
import { AsyncSearchSelect, Btn, Field, Modal } from "@ac/web/components/ui";
import { useT } from "@ac/web/components/I18n";
import { useAction } from "@ac/web/lib/useAction";
import { mqttNodeValue } from "@ac/web/lib/devices";
import type { PickerOption } from "@ac/web/lib/pickerOption";
import { NewDeviceFields, NO_NEW, unitsWithoutDevice, type IoTChoice } from "../../_components/device-fields";
import { registerAndBind } from "../actions";
import type { IoTModelRow } from "@ac/web/lib/devices";

/** Register device (DD-A04): the serial, the IoT model (its sensors are the device's start) and optionally the AC unit to
connect it to. Without a unit the device is registered on its own: a spare, or one not installed yet. With one it is
registered and bound in one go; if the bind is refused the device stays registered and is connected from its page. */
export function RegisterDeviceModal({ onClose, iotModels, onDone }: Readonly<{ onClose: () => void; iotModels: IoTModelRow[]; onDone: (deviceId: string) => void }>) {
  const t = useT();
  const [pending, run] = useAction();
  const [draft, setDraft] = useState(NO_NEW);
  const [unit, setUnit] = useState<PickerOption | null>(null);
  const [tried, setTried] = useState(false);
  const choices: IoTChoice[] = iotModels.map((m) => ({ id: m.id, label: m.name, sensors: m.model.sensors.map((x) => x.metric) }));
  const node = mqttNodeValue(draft.node);
  const message = (bindError: string | null) => {
    if (bindError) return t("Device registered, but not connected to the unit — connect it from its page");
    return unit ? t("Device registered and connected to the unit") : t("Device registered — connect it to a unit when it is installed");
  };
  const register = () => {
    setTried(true);
    if (!draft.serial.trim() || !draft.iotModelId || node === undefined) return;
    run(() => registerAndBind({ serial: draft.serial.trim(), sensorTypes: draft.sensors, mqttNode: node, iotModelId: draft.iotModelId, unitId: unit?.id ?? null }),
      (v) => message(v.bindError), (v) => { onClose(); onDone(v.id); });
  };
  return (
    <Modal open onClose={onClose} title={t("Register device")} footer={<><Btn onClick={onClose}>{t("Cancel")}</Btn><Btn variant="primary" disabled={pending} onClick={register}>{t("Register")}</Btn></>}>
      <NewDeviceFields iotModels={choices} value={draft} tried={tried} onChange={setDraft} />
      <Field label={t("Connect to AC unit · optional")} hint={t("Leave it empty to register the device on its own (a spare, or one not installed yet). Only units without a device are listed.")}>
        <AsyncSearchSelect value={unit} onChange={setUnit} search={unitsWithoutDevice} label={t("AC unit")} />
      </Field>
    </Modal>
  );
}
