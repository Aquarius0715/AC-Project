"use client";

import { Page, Tabs } from "@ac/web/components/ui";
import { useT } from "@ac/web/components/I18n";
import { useUrlPatch } from "@ac/web/lib/useUrlPatch";
import type { ApiDeviceDetail, CampaignRow, DeviceRow, ModelRow } from "@ac/web/lib/devices";
import type { deviceTiles, EventRow, firmwareCard, OperationRow, sensorRows, TechDeviceRow } from "@ac/web/lib/techDevices";
import { DeviceTab } from "./device-tab";
import { FirmwareTab } from "./firmware-tab";
import { ModelsTab } from "./models-tab";

export type DevicesLive = {
  tab: "models" | "devices" | "firmware"; now: string; canAudit: boolean; canWrite: boolean;
  models: ModelRow[]; devices: DeviceRow[]; units: { id: string; label: string; modelId: string }[];
  /** the devices tab's list (Figma Admin 246:2): sorted by serial, with the state of each */
  rows?: TechDeviceRow[];
  device?: {
    detail: ApiDeviceDetail; row: TechDeviceRow;
    tiles: ReturnType<typeof deviceTiles>; sensors: ReturnType<typeof sensorRows>;
    unit: { name: string; place: string; model: string } | null; firmware: string[]; firmwareCard: ReturnType<typeof firmwareCard>;
    operations: OperationRow[]; calibrations: string[]; events: EventRow[];
  };
  campaigns?: CampaignRow[]; campaign?: CampaignRow;
};

/** The device registry in API mode: the tab, the selected device and campaign live in the URL; each tab reads its data
 * on the server and writes through Server Actions. Texts in the display language (IR295). */
export function DevicesView({ live }: { live: DevicesLive }) {
  const t = useT();
  const nav = useUrlPatch();
  return (
    <Page>
      <Tabs value={live.tab} onChange={(v) => nav({ tab: v === "models" ? null : v, deviceId: null, campaignId: null })} tabs={[
        { id: "models", label: t("Models"), count: live.models.length }, { id: "devices", label: t("IoT devices"), count: live.devices.length },
        { id: "firmware", label: t("Firmware campaigns"), count: live.campaigns?.length },
      ]} />
      {live.tab === "models" ? <ModelsTab live={live} /> : live.tab === "devices" ? <DeviceTab live={live} /> : <FirmwareTab live={live} />}
    </Page>
  );
}
