"use client";

import { useState } from "react";
import { Btn, Select, useToast } from "@ac/web/components/ui";
import { jobActions } from "@ac/web/lib/jobs";
import { clientUserActions } from "@ac/web/lib/clientUsers";
import { useT } from "@ac/web/components/I18n";
import { ClockFace, DemoFrame, TriggerTile } from "./demo-frame";

/** Demo controls of the Phase 1A browser demo (FR-X05): the fixture clock, the simulator switch, the reset of the
 * browser stores and the failure triggers as notices; API mode renders DemoView from the Core API. */
export function DemoMock() {
  const t = useT();
  const toast = useToast();
  const [sim, setSim] = useState(false);
  return (
    <DemoFrame
      clock={<ClockFace time="2026-09-14 09:41 UTC" onMinute={() => toast(t("Clock advanced +1 min (demo)"))} onHour={() => toast(t("Clock advanced +1 hour (demo)"))} />}
      scenario={<><Select aria-label={t("Scenario")} value={sim ? "on" : "off"} onChange={(e) => setSim(e.target.value === "on")}><option value="off">{t("simulator = false (baseline seed)")}</option><option value="on">simulator = true</option></Select><p className="mt-2 text-[11px] text-muted">{t("fixture-contract.json demoSeed is the source of truth")}</p></>}
      reset={<><p className="text-[11px] text-muted">{t("Returns clock, subscriptions, image URLs, caches, and business data to their initial seed")}</p><Btn variant="danger" className="mt-3" onClick={() => { setSim(false); jobActions.reset(); clientUserActions.reset(); toast(t("Demo data reset to seed")); }}>{t("Reset demo data")}</Btn></>}
      triggers={
        <div className="grid-fluid" style={{ ["--min" as string]: "240px" }}>
          <TriggerTile icon="⌁" title={t("Device offline")} sub={t("communication_lost on {unit}", { unit: "unit-online-rto" })} onClick={() => toast(t("Device offline triggered"), "warn")} />
          <TriggerTile icon="◷" title={t("Delay transport")} sub={t("3000ms on commands.create")} onClick={() => toast(t("Transport delay 3000 ms set"), "warn")} />
          <TriggerTile icon="☻" title={t("Expire session")} sub={t("session_expired for current Membership")} onClick={() => toast(t("Session expired (demo)"), "crit")} />
        </div>
      }
    />
  );
}
