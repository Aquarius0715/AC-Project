"use client";

import { useState } from "react";
import { AsyncSearchSelect, Banner, Btn, Field, Modal, SummaryList, Textarea } from "@ac/web/components/ui";
import { useT } from "@ac/web/components/I18n";
import { useAction } from "@ac/web/lib/useAction";
import type { PickerOption } from "@ac/web/lib/pickerOption";
import { allUnits } from "../../_components/device-fields";
import { bindDevice, deviceOnUnit } from "../actions";

/** Bind (a device not on a unit yet) or Rebind (move it): the target unit is searched on the server, the old and the new
unit are shown side by side, and a unit that has a device is replaced in the same write (its device is unbound, keeping
its history). */
export function BindDeviceModal({ onClose, device, first }: Readonly<{ onClose: () => void; device: { id: string; version: number; from: string | null }; first: boolean }>) {
  const t = useT();
  const [pending, run] = useAction();
  const [unit, setUnit] = useState<PickerOption | null>(null);
  const [reason, setReason] = useState(first ? t("Installed on site") : "");
  const [taken, setTaken] = useState<{ id: string; serial: string } | null>(null); // the device now on the target unit: the bind replaces it
  const [tried, setTried] = useState(false);
  const pick = (o: PickerOption | null) => {
    setUnit(o);
    setTaken(null);
    if (!o) return;
    void deviceOnUnit(o.id).then((r) => { if (r.ok && r.value && r.value.id !== device.id) setTaken(r.value); });
  };
  const done = () => {
    if (taken) return t("Device connected — the previous device on the unit was unbound");
    return t(first ? "Device bound to the unit — it is connected now" : "Device bound — new sensor IDs issued");
  };
  const save = () => {
    setTried(true);
    if (!unit || !reason.trim()) return;
    run(() => bindDevice(device.id, device.version, unit.id, reason.trim(), taken?.id), done(), onClose);
  };
  return (
    <Modal open onClose={onClose} title={t(first ? "Bind device" : "Rebind device")} footer={<><Btn onClick={onClose}>{t("Cancel")}</Btn><Btn variant="primary" disabled={pending} onClick={save}>{t(first ? "Bind" : "Rebind")}</Btn></>}>
      <Field label={t(first ? "Unit" : "New unit")} hint={t("Units of every customer in your scope; the customer, place and aircon model are shown beside the name.")} error={tried && !unit ? t("Choose a unit") : undefined}>
        <AsyncSearchSelect value={unit} label={t("AC unit")} search={allUnits} onChange={pick} />
      </Field>
      <SummaryList items={[[t("From"), device.from ?? t("Not connected")], [t("To"), unit ? [unit.label, unit.detail].filter(Boolean).join(" · ") : "—"]]} />
      {taken && <Banner tone="warn">{t("{serial} is on this unit now. Connecting replaces it: it is unbound, and its history stays with it.", { serial: taken.serial })}</Banner>}
      <Field label={t("Reason")} error={tried && !reason.trim() ? t("A reason is required") : undefined}><Textarea value={reason} maxLength={1000} onChange={(e) => { setReason(e.target.value); }} /></Field>
      <p className="text-[11px] text-muted-foreground">{t(first ? "The device's sensors start reporting for this unit." : "History stays with the previous binding; the device gets new sensor IDs on the new unit.")}</p>
    </Modal>
  );
}
