"use client";

import { useState } from "react";
import { AsyncSearchSelect, Banner, Btn, ConnBadge, Field, Modal, Select, SummaryList, Textarea, TextLink } from "@ac/web/components/ui";
import { Icon } from "@ac/web/components/Icon";
import { useT } from "@ac/web/components/I18n";
import { useAction } from "@ac/web/lib/useAction";
import { mqttNodeValue, type Connection } from "@ac/web/lib/devices";
import type { PickerOption } from "@ac/web/lib/pickerOption";
import { bindDevice, registerAndBind, unbindDevice } from "../../devices/actions";
import { freeDevices, NewDeviceFields, NO_NEW, type IoTChoice, type NewDraft } from "../../_components/device-fields";
import type { NewUnitDevice } from "../actions";

export interface UnitDevice { id: string; version: number; serial: string; connection: Connection; iotModelId: string | null; mqttNode: string | null }

const SPAN = "sm:col-span-2";
const MODES = ["none", "existing", "new"] as const;
type DeviceMode = (typeof MODES)[number];
const modeOf = (value: string): DeviceMode => MODES.find((m) => m === value) ?? "none";

/** The device section of a new unit: none, a device already registered (not on a unit), or a new one. It is connected
when the unit is saved. */
export function NewUnitDeviceSection({ iotModels, onChange }: Readonly<{ iotModels: IoTChoice[]; onChange: (d: NewUnitDevice | null, error: boolean) => void }>) {
  const t = useT();
  const deviceText = t("IoT device");
  const existingText = t("A registered device");
  const freshText = t("Register a new device");
  const [mode, setMode] = useState<DeviceMode>("none");
  const [picked, setPicked] = useState<PickerOption | null>(null);
  const [draft, setDraft] = useState(NO_NEW);
  const emit = (m: typeof mode, p: PickerOption | null, d: NewDraft) => {
    const node = mqttNodeValue(d.node);
    if (m === "existing") onChange(p ? { kind: "existing", deviceId: p.id, version: p.version } : null, !p);
    else if (m === "new") onChange(d.serial.trim() && d.iotModelId && node !== undefined ? { kind: "new", serial: d.serial.trim(), mqttNode: node ?? null, iotModelId: d.iotModelId, sensorTypes: d.sensors } : null, true);
    else onChange(null, false);
  };
  return (
    <section className={`grid gap-x-4 gap-y-3 rounded-xl bg-surface2 p-3 sm:grid-cols-2 ${SPAN}`} aria-label={t("IoT device · optional")}>
      <p className={`text-[12px] text-muted-foreground ${SPAN}`}>{t("Optional: connect the unit's IoT device now — one already registered, or a new one. It can also be connected later from the unit's page.")}</p>
      <Field label={deviceText} className={SPAN}>
        <Select value={mode} onChange={(e) => { const m = modeOf(e.target.value); setMode(m); emit(m, picked, draft); }}>
          <option value="none">{t("No device yet")}</option><option value="existing">{existingText}</option><option value="new">{freshText}</option>
        </Select>
      </Field>
      {mode === "existing" && (
        <Field label={t("Registered device")} hint={t("Devices not on a unit. Its IoT model and node ID are kept as registered.")} className={SPAN}>
          <AsyncSearchSelect value={picked} label={deviceText} search={freeDevices} onChange={(o) => { setPicked(o); emit("existing", o, draft); }} />
        </Field>
      )}
      {mode === "new" && <NewDeviceFields iotModels={iotModels} value={draft} tried onChange={(v) => { setDraft(v); emit("new", picked, v); }} />}
    </section>
  );
}


/** The unit's IoT device (units screen): shows the connected device and connects, replaces or unbinds it. A replacement
unbinds the old device and binds the new one in one write, so the unit never has two devices or none half way; both
keep their history. */
export function UnitDevicePanel({ unitId, unitName, device, deviceKnown, canWrite, iotModels }: Readonly<{ unitId: string; unitName: string; device: UnitDevice | null; deviceKnown: boolean; canWrite: boolean; iotModels: IoTChoice[] }>) {
  const t = useT();
  const deviceText = t("IoT device");
  const existingText = t("A registered device");
  const freshText = t("Register a new device");
  const [pending, run] = useAction();
  const [modal, setModal] = useState<null | "connect" | "unbind">(null);
  const [mode, setMode] = useState<"existing" | "new">("existing");
  const [picked, setPicked] = useState<PickerOption | null>(null);
  const [draft, setDraft] = useState(NO_NEW);
  const [reason, setReason] = useState("");
  const [tried, setTried] = useState(false);
  const close = () => { setModal(null); setTried(false); setPicked(null); setDraft(NO_NEW); setReason(""); };
  const iotName = (id: string | null) => iotModels.find((k) => k.id === id)?.label ?? null;
  const replacing = device !== null;
  const node = mqttNodeValue(draft.node);
  const connect = () => {
    setTried(true);
    if (!reason.trim()) return;
    if (mode === "existing") {
      if (!picked) return;
      run(() => bindDevice(picked.id, picked.version, unitId, reason.trim(), device?.id), t(replacing ? "Device replaced — the previous device was unbound" : "Device connected"), close);
      return;
    }
    if (!draft.serial.trim() || !draft.iotModelId || node === undefined) return;
    run(() => registerAndBind({ serial: draft.serial.trim(), sensorTypes: draft.sensors, mqttNode: node, iotModelId: draft.iotModelId, unitId, reason: reason.trim(), ...(device && { replaceDeviceId: device.id }) }),
      (v) => registeredText(v.bindError), close);
  };
  const registeredText = (bindError: string | null) => {
    if (bindError) return t("Device registered, but not connected — connect it again from here");
    return t(replacing ? "New device registered and replaced the previous one" : "New device registered and connected");
  };
  const unbind = () => {
    setTried(true);
    if (!device || !reason.trim()) return;
    run(() => unbindDevice(device.id, device.version, reason.trim()), t("Device unbound — it stays registered"), close);
  };
  return (
    <>
      <div className="flex flex-col gap-1.5 text-[13px]">
        {device ? (
          <>
            <span className="flex flex-wrap items-center gap-2"><b>{device.serial}</b><ConnBadge s={device.connection} /><TextLink href={`/admin/devices?tab=devices&deviceId=${device.id}`}>{t("Devices")} <Icon name="chevronRight" /></TextLink></span>
            <span className="text-xs text-muted-foreground">{[iotName(device.iotModelId) ?? t("No IoT model"), device.mqttNode].filter(Boolean).join(" · ")}</span>
          </>
        ) : <span className="text-muted-foreground">{deviceKnown ? t("No device bound") : t("Needs device.read")}</span>}
        {canWrite && deviceKnown && (
          <span className="flex flex-wrap gap-2">
            <Btn size="sm" onClick={() => { setModal("connect"); }}>{device ? t("Replace…") : t("Connect device…")}</Btn>
            {device && <Btn size="sm" onClick={() => { setModal("unbind"); }}>{t("Unbind…")}</Btn>}
          </span>
        )}
      </div>
      <Modal open={modal === "connect"} onClose={close} title={t(replacing ? "Replace the device" : "Connect a device")} footer={<><Btn onClick={close}>{t("Cancel")}</Btn><Btn variant="primary" disabled={pending} onClick={connect}>{t(replacing ? "Replace" : "Connect")}</Btn></>}>
        <div className="flex flex-col gap-3">
          <SummaryList items={[[t("AC unit"), unitName], [t("Now"), device ? device.serial : t("No device")], [t("After"), mode === "new" ? (draft.serial.trim() || "—") : (picked?.label ?? "—")]]} />
          <Field label={t("Device")}><Select value={mode} onChange={(e) => { setMode(e.target.value === "new" ? "new" : "existing"); }}><option value="existing">{existingText}</option><option value="new">{freshText}</option></Select></Field>
          {mode === "existing"
            ? <Field label={t("Registered device")} hint={t("Devices not on a unit. Its IoT model and node ID are kept as registered.")} error={tried && !picked ? t("Choose a device") : undefined}><AsyncSearchSelect value={picked} label={deviceText} search={freeDevices} onChange={setPicked} /></Field>
            : <NewDeviceFields iotModels={iotModels} value={draft} tried={tried} onChange={setDraft} />}
          {picked && mode === "existing" && picked.detail && <p className="text-[12px] text-muted-foreground">{picked.detail}</p>}
          {replacing && <Banner tone="warn">{t("{serial} is on this unit now. It is unbound in the same step and keeps its history; the unit has the new device only if both steps succeed.", { serial: device.serial })}</Banner>}
          <Field label={t("Reason")} error={tried && !reason.trim() ? t("A reason is required") : undefined}><Textarea rows={2} value={reason} maxLength={1000} onChange={(e) => { setReason(e.target.value); }} /></Field>
        </div>
      </Modal>
      <Modal open={modal === "unbind"} onClose={close} title={t("Unbind device")} footer={<><Btn onClick={close}>{t("Cancel")}</Btn><Btn variant="primary" disabled={pending} onClick={unbind}>{t("Unbind")}</Btn></>}>
        <Field label={t("Reason")} error={tried && !reason.trim() ? t("A reason is required") : undefined}><Textarea rows={2} value={reason} maxLength={1000} onChange={(e) => { setReason(e.target.value); }} /></Field>
        <p className="text-[11px] text-muted-foreground">{t("The device leaves its unit and stays registered; history stays with the binding, and the unit has no device until you connect another.")}</p>
      </Modal>
    </>
  );
}
