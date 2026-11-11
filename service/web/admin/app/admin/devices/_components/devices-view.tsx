"use client";

import { Page, Tabs } from "@ac/web/components/ui";
import { useT } from "@ac/web/components/I18n";
import { useUrlPatch } from "@ac/web/lib/useUrlPatch";
import type { ApiDeviceDetail, CampaignRow, DeviceRow, IoTModelRow, ModelRow } from "@ac/web/lib/devices";
import type { deviceTiles, EventRow, firmwareCard, OperationRow, sensorRows, TechDeviceRow } from "@ac/web/lib/techDevices";
import { DeviceTab } from "./device-tab";
import { FirmwareTab } from "./firmware-tab";
import { IoTModelsTab } from "./iot-models-tab";
import { ModelsTab } from "./models-tab";

export interface DevicesLive {
  tab: "models" | "iot" | "devices" | "firmware"; now: string; canAudit: boolean; canWrite: boolean;
  /** the air conditioner models and the IoT models are two registers (DEC-105) */
  models: ModelRow[]; iotModels: IoTModelRow[]; devices: DeviceRow[]; units: { id: string; label: string; modelId: string; iotModelId: string | null }[];
  /** how many devices the registry holds, and on the devices tab the page of it (server search, cursor, IR723) */
  deviceTotal: number; page?: { q: string; next: string | null; cursor: string | null };
  /** the devices tab's list (Figma Admin 246:2): sorted by serial, with the state of each */
  rows?: TechDeviceRow[];
  device?: {
    detail: ApiDeviceDetail; row: TechDeviceRow;
    tiles: ReturnType<typeof deviceTiles>; sensors: ReturnType<typeof sensorRows>;
    unit: { name: string; place: string; model: string } | null; firmware: string[]; firmwareCard: ReturnType<typeof firmwareCard>;
    operations: OperationRow[]; calibrations: string[]; events: EventRow[];
  };
  campaigns?: CampaignRow[]; campaign?: CampaignRow;
}

/** The device registry in API mode: the tab, the selected device and campaign live in the URL; each tab reads its data
on the server and writes through Server Actions. Texts in the display language (IR295). */
export function DevicesView({ live }: Readonly<{ live: DevicesLive }>) {
  const t = useT();
  const nav = useUrlPatch();
  return (
    <Page>
      <Tabs value={live.tab} onChange={(v) => { nav({ tab: v === "models" ? null : v, deviceId: null, campaignId: null, q: null, cursor: null }); }} tabs={[
        { id: "models", label: t("Aircon models"), count: live.models.length }, { id: "iot", label: t("IoT models"), count: live.iotModels.length },
        { id: "devices", label: t("IoT devices"), count: live.deviceTotal },
        { id: "firmware", label: t("Firmware campaigns"), count: live.campaigns?.length },
      ]} />
      {live.tab === "models" && <ModelsTab live={live} />}{live.tab === "iot" && <IoTModelsTab live={live} />}{live.tab === "devices" && <DeviceTab live={live} />}{live.tab === "firmware" && <FirmwareTab live={live} />}
    </Page>
  );
}
