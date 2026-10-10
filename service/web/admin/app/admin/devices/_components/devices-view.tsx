"use client";

import { Page, Tabs } from "@ac/web/components/ui";
import { useT } from "@ac/web/components/I18n";
import { useUrlPatch } from "@ac/web/lib/useUrlPatch";
import type { ApiDeviceDetail, CampaignRow, DeviceRow, ModelRow } from "@ac/web/lib/devices";
import { DeviceTab } from "./device-tab";
import { FirmwareTab } from "./firmware-tab";
import { ModelsTab } from "./models-tab";

type Item = { time: string; title: string; detail?: string; tone?: "ok" | "crit" | "warn" };
export type DevicesLive = {
  tab: "models" | "devices" | "firmware"; now: string; canAudit: boolean; canWrite: boolean;
  models: ModelRow[]; devices: DeviceRow[]; units: { id: string; label: string; modelId: string }[];
  device?: { detail: ApiDeviceDetail; row: DeviceRow; firmware: string[]; operations: Item[]; calibrations: Item[]; events: Item[]; texts: { lastSeen: string | null; calibrated: Record<string, string | null> } };
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
