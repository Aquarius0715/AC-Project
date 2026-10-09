"use client";

import { useState, useTransition } from "react";
import { Banner, Btn, Select } from "@ac/web/components/ui";
import { useAction } from "@ac/web/lib/useAction";
import { demoRefusal, type DeviceOption } from "@ac/web/lib/demoPanel";
import type { ActionFailure } from "@ac/web/lib/actionMessage";
import { useT } from "@ac/web/components/I18n";
import { advanceClock, deviceSignal, expireSession } from "../actions";
import { ClockFace, DemoFrame, TriggerTile } from "./demo-frame";

export type DemoLive = { clock: string; devices: DeviceOption[] | null; hidden: number }; // hidden: listed devices whose history needs a current assignment

/** Demo controls in API mode (FR-X05, IR154, IR249; Figma Client 10e): the Core API scenario clock moves forward, a
 * device the signed-in role can see loses its connection or gets it back, and the session can be expired; the reset and
 * the transport delay exist only in the browser demo and say so. */
export function DemoView({ live }: { live: DemoLive }) {
  const t = useT();
  const [pending, run] = useAction();
  const [expiring, startExpire] = useTransition();
  const [failed, setFailed] = useState<string | null>(null);
  const [picked, setPicked] = useState(live.devices?.[0]?.id ?? "");
  const device = live.devices?.find((d) => d.id === picked) ?? live.devices?.[0] ?? null;
  const ok = () => setFailed(null);
  const fail = (f: ActionFailure) => setFailed(demoRefusal(f));
  const busy = pending || expiring;
  return (
    <DemoFrame
      above={failed && <Banner tone="crit">{t("Not done: {reason}", { reason: t(failed) })}</Banner>}
      clock={<ClockFace time={live.clock} disabled={busy} onMinute={() => run(() => advanceClock(1), t("Clock advanced 1 minute"), ok, fail)} onHour={() => run(() => advanceClock(60), t("Clock advanced 1 hour"), ok, fail)} />}
      scenario={<><Select aria-label={t("Scenario")} disabled value="seed"><option value="seed">{t("Core API seed · generation 1")}</option></Select><p className="mt-2 text-[11px] text-muted">{t("fixture-contract.json demoSeed is the source of truth. The Core API demo runs this one scenario; the simulator switch is in the browser demo (IR154).")}</p></>}
      reset={<><p className="text-[11px] text-muted">{t("Returns clock, subscriptions, image URLs, caches, and business data to their initial seed")}</p><Btn variant="danger" className="mt-3" disabled>{t("Reset demo data")}</Btn><p className="mt-2 text-[11px] text-muted">{t("The running Core API keeps shared data: rebuild it offline with make resetdb (IR154).")}</p></>}
      triggers={
        <>
          {device && <Select aria-label={t("Device")} className="mb-3" value={device.id} onChange={(e) => setPicked(e.target.value)}>{live.devices!.map((d) => <option key={d.id} value={d.id}>{d.label}{d.fault ? ` · ${t("fault open")}` : ""}</option>)}</Select>}
          {device && live.hidden > 0 && <p className="-mt-2 mb-3 text-[11px] text-muted">{t(live.hidden === 1 ? "{n} more device needs a current assignment before their history can be read (SR24)." : "{n} more devices need a current assignment before their history can be read (SR24).", { n: live.hidden })}</p>}
          <div className="grid-fluid" style={{ ["--min" as string]: "240px" }}>
            {device?.fault
              ? <TriggerTile icon="⌁" tone="ok" title={t("Restore connection")} sub={t("restored on {unit} · the open communication_lost as its source", { unit: device.unit })} disabled={busy} onClick={() => run(() => deviceSignal(device.id, true), t("{unit}: connection restored", { unit: device.unit }), ok, fail)} />
              : <TriggerTile icon="⌁" title={t("Device offline")} sub={device ? t("communication_lost on {unit}", { unit: device.unit }) : t(live.hidden ? "Your devices need a current assignment here (SR24)" : live.devices ? "No device is bound to a unit you can see" : "Devices are not visible to this role")} disabled={busy} onClick={device ? () => run(() => deviceSignal(device.id, false), t("{unit}: connection lost", { unit: device.unit }), ok, fail) : undefined} />}
            <TriggerTile icon="◷" title={t("Delay transport")} sub={t("3000ms on commands.create · browser demo only (IR37)")} disabled />
            <TriggerTile icon="☻" title={t("Expire session")} sub={t("session_expired for current Membership")} disabled={busy} onClick={() => startExpire(() => expireSession())} />
          </div>
        </>
      }
    />
  );
}
