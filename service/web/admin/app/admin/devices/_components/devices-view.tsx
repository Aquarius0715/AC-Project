"use client";

import { Page, Tabs } from "@ac/web/components/ui";
import { useUrlPatch } from "@ac/web/lib/useUrlPatch";
import type { ApiDeviceDetail, CampaignRow, DeviceRow, ModelRow } from "@ac/web/lib/devices";
import { DeviceTab } from "./device-tab";
import { FirmwareTab } from "./firmware-tab";
import { ModelsTab } from "./models-tab";

type Item = { time: string; title: string; detail?: string; tone?: "ok" | "crit" | "warn" };
export type DevicesLive = {
  tab: "models" | "devices" | "firmware"; now: string; canAudit: boolean; canWrite: boolean;
  models: ModelRow[]; devices: DeviceRow[]; units: { id: string; label: string; modelId: string }[];
  device?: { detail: ApiDeviceDetail; row: DeviceRow; firmware: string[]; operations: Item[]; calibrations: Item[]; events: Item[] };
  campaigns?: CampaignRow[]; campaign?: CampaignRow;
};

/** The device registry in API mode: the tab, the selected device and campaign live in the URL; each tab reads its data
 * on the server and writes through Server Actions. */
export function DevicesView({ live }: { live: DevicesLive }) {
  const nav = useUrlPatch();
  return (
    <Page>
      <Tabs value={live.tab} onChange={(t) => nav({ tab: t === "models" ? null : t, deviceId: null, campaignId: null })} tabs={[
        { id: "models", label: "Models", count: live.models.length }, { id: "devices", label: "IoT devices", count: live.devices.length },
        { id: "firmware", label: "Firmware campaigns", count: live.campaigns?.length },
      ]} />
      {live.tab === "models" ? <ModelsTab live={live} /> : live.tab === "devices" ? <DeviceTab live={live} /> : <FirmwareTab live={live} />}
    </Page>
  );
}
