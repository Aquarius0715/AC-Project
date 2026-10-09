"use client";

import { useState, useTransition } from "react";
import { Banner, Btn, Select } from "@ac/web/components/ui";
import { useAction } from "@ac/web/lib/useAction";
import { demoRefusal, type DeviceOption } from "@ac/web/lib/demoPanel";
import type { ActionFailure } from "@ac/web/lib/actionMessage";
import { advanceClock, deviceSignal, expireSession } from "../actions";
import { ClockFace, DemoFrame, TriggerTile } from "./demo-frame";

export type DemoLive = { clock: string; devices: DeviceOption[] | null; hidden: number }; // hidden: listed devices whose history needs a current assignment

/** Demo controls in API mode (FR-X05, IR154, IR249; Figma Client 10e): the Core API scenario clock moves forward, a
 * device the signed-in role can see loses its connection or gets it back, and the session can be expired; the reset and
 * the transport delay exist only in the browser demo and say so. */
export function DemoView({ live }: { live: DemoLive }) {
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
      above={failed && <Banner tone="crit">Not done: {failed}</Banner>}
      clock={<ClockFace time={live.clock} disabled={busy} onMinute={() => run(() => advanceClock(1), "Clock advanced 1 minute", ok, fail)} onHour={() => run(() => advanceClock(60), "Clock advanced 1 hour", ok, fail)} />}
      scenario={<><Select aria-label="Scenario" disabled value="seed"><option value="seed">Core API seed · generation 1</option></Select><p className="mt-2 text-[11px] text-muted">fixture-contract.json demoSeed is the source of truth. The Core API demo runs this one scenario; the simulator switch is in the browser demo (IR154).</p></>}
      reset={<><p className="text-[11px] text-muted">Returns clock, subscriptions, image URLs, caches, and business data to their initial seed</p><Btn variant="danger" className="mt-3" disabled>Reset demo data</Btn><p className="mt-2 text-[11px] text-muted">The running Core API keeps shared data: rebuild it offline with make resetdb (IR154).</p></>}
      triggers={
        <>
          {device && <Select aria-label="Device" className="mb-3" value={device.id} onChange={(e) => setPicked(e.target.value)}>{live.devices!.map((d) => <option key={d.id} value={d.id}>{d.label}{d.fault ? " · fault open" : ""}</option>)}</Select>}
          {device && live.hidden > 0 && <p className="-mt-2 mb-3 text-[11px] text-muted">{live.hidden} more {live.hidden === 1 ? "device needs" : "devices need"} a current assignment before their history can be read (SR24).</p>}
          <div className="grid-fluid" style={{ ["--min" as string]: "240px" }}>
            {device?.fault
              ? <TriggerTile icon="⌁" tone="ok" title="Restore connection" sub={`restored on ${device.unit} · the open communication_lost as its source`} disabled={busy} onClick={() => run(() => deviceSignal(device.id, true), `${device.unit}: connection restored`, ok, fail)} />
              : <TriggerTile icon="⌁" title="Device offline" sub={device ? `communication_lost on ${device.unit}` : live.hidden ? "Your devices need a current assignment here (SR24)" : live.devices ? "No device is bound to a unit you can see" : "Devices are not visible to this role"} disabled={busy} onClick={device ? () => run(() => deviceSignal(device.id, false), `${device.unit}: connection lost`, ok, fail) : undefined} />}
            <TriggerTile icon="◷" title="Delay transport" sub="3000ms on commands.create · browser demo only (IR37)" disabled />
            <TriggerTile icon="☻" title="Expire session" sub="session_expired for current Membership" disabled={busy} onClick={() => startExpire(() => expireSession())} />
          </div>
        </>
      }
    />
  );
}
