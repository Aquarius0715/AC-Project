"use client";

import Link from "next/link";
import { use, useState } from "react";
import { Badge, Banner, Btn, Card, ConnBadge, Modal, Page, PowerBadge, Stat, SummaryList, LinkBtn, Toggle, cx, useToast, Choice } from "@/components/ui";
import { units } from "@/lib/client";

const siblings = [units[0], units[1]];
type Cmd = { id: string; text: string; when: string; bad?: boolean };

export default function UnitControl({ params }: { params: Promise<{ id: string }> }) {
  const { id } = use(params);
  const toast = useToast();
  const unit = units.find((u) => u.id === id) ?? units[0];
  const offline = unit.conn === "offline";
  const restricted = id === "unit-lobby";
  const [temp, setTemp] = useState(24);
  const [mode, setMode] = useState<"cool" | "dry" | "fan">("cool");
  const [fan, setFan] = useState<"low" | "mid" | "high">("mid");
  const [confirm, setConfirm] = useState(false);
  const [waiting, setWaiting] = useState(false);
  const [on, setOn] = useState(unit.state === "running");
  const [hist, setHist] = useState<Cmd[]>([
    { id: "cmd-0230", text: "Set power ON", when: "09:15" },
    { id: "cmd-0229", text: "Set mode COOL", when: "Yesterday 21:02" },
    { id: "cmd-0228", text: "Set fan MID", when: "Yesterday 21:02" },
    { id: "cmd-0227", text: "Set temperature 15°C — rejected: below model minimum 16°C", when: "Yesterday 19:40", bad: true },
  ]);
  const min = restricted ? 24 : 16;
  const send = () => {
    setConfirm(false); setWaiting(true);
    setTimeout(() => { setWaiting(false); setHist((h) => [{ id: "cmd-0231", text: `Set temperature ${temp}°C`, when: "now" }, ...h]); toast(`${unit.name}: ${temp}°C acknowledged by device`); }, 1500);
  };
  return (
    <Page>
      <div className="flex flex-wrap items-center gap-2 text-[13px] text-muted"><Link href="/customer/properties" className="font-semibold text-primary">← Back to Bedroom</Link><span>{unit.loc} › <b className="text-ink">{unit.name}</b></span></div>
      <div className="split-rev">
        <Card title="DEVICES IN THIS ROOM" sub="Bedroom · 2 ACs · Home A › 1F" className="self-start">
          <div className="flex flex-col gap-2">
            {siblings.map((s) => (
              <Link key={s.id} href={`/customer/units/${s.id}`} className={cx("rounded-xl border p-3", s.id === id ? "border-primary bg-primary-soft/60" : "border-line hover:bg-surface2/60")}>
                <div className="flex items-center justify-between font-bold">{s.name}{s.id === id && <Badge tone="primary">Selected</Badge>}</div>
                <div className="text-xs text-muted">{s.temp}°C room · {s.power} W</div>
              </Link>
            ))}
          </div>
          <p className="mt-3 text-[11px] text-muted">Choose one AC to control. Controls apply only to the selected AC — no room-wide bulk control.</p>
        </Card>

        <div className="flex min-w-0 flex-col gap-4">
          <Card title={unit.name} sub={`${unit.loc} · ${unit.id}`} action={<span className="flex items-center gap-2"><PowerBadge s={unit.state} /><ConnBadge s={unit.conn} /></span>}>
            {offline ? <Banner tone="warn">This AC is offline. Remote actions are disabled until the connection returns. Last seen {unit.obs}.</Banner> : (
              <div className="grid-fluid" style={{ ["--min" as string]: "160px" }}>
                <Stat label="Room temperature (measured)" value="28.0 °C" sub="observed 09:12:30" />
                <Stat label="Confirmed setting" value="26°C · Cool · Mid" sub="acknowledged by device" />
                <Stat label="Humidity" value="60 %" sub="observed 09:12:30" />
              </div>
            )}
          </Card>
          {restricted && <Banner tone="warn">Cooling restriction active — minimum 24 °C. <Link className="font-semibold text-primary" href="/customer/payments/invoice-overdue-a?view=restriction">View details →</Link></Banner>}
          <Card title="REMOTE CONTROL" sub="Controls apply to this AC only; you confirm before sending">
            <div className="flex flex-col gap-4">
              <div className="flex flex-wrap items-center justify-between gap-2"><div><div className="font-semibold">Power</div><div className="text-xs text-muted">Sent as its own command</div></div><Toggle on={on} onChange={(v) => { setOn(v); toast(`Power ${v ? "ON" : "OFF"} sent`); }} label="Power" /></div>
              <div className="flex flex-wrap items-center justify-between gap-2">
                <div><div className="font-semibold">Set temperature</div><div className="text-xs text-muted">{min}–30°C · step 1°C · confirmed 26°C</div></div>
                <div className="flex items-center gap-2"><Btn disabled={offline || temp <= min} onClick={() => setTemp((t) => t - 1)}>–</Btn><span className="w-14 text-center text-lg font-bold">{temp}°C</span><Btn disabled={offline || temp >= 30} onClick={() => setTemp((t) => t + 1)}>+</Btn></div>
              </div>
              <div><div className="mb-1 font-semibold">Mode</div><Choice value={mode} onChange={setMode} options={[{ id: "cool", label: "Cool" }, { id: "dry", label: "Dry" }, { id: "fan", label: "Fan" }]} /></div>
              <div><div className="mb-1 font-semibold">Fan speed</div><Choice value={fan} onChange={setFan} options={[{ id: "low", label: "Low" }, { id: "mid", label: "Mid" }, { id: "high", label: "High" }]} /></div>
              <div className="flex flex-wrap items-center justify-between gap-2 rounded-xl bg-surface2 p-3 text-xs"><span><b>1 change:</b> Set temperature 26°C → {temp}°C<br /><span className="text-muted">Target: {unit.name} only · you will confirm before sending</span></span><Btn variant="primary" disabled={offline || waiting} onClick={() => setConfirm(true)}>{waiting ? "Waiting for device…" : "Review & send"}</Btn></div>
              {waiting && <Banner>Waiting for device response… The result is shown only after the device acknowledges (fails after 30 s).</Banner>}
            </div>
          </Card>
          <div className="split-even">
            <Card title="Live telemetry"><SummaryList items={[["Power draw", "742 W · 09:12:30"], ["Wi-Fi", "RSSI −52 dBm (strong)"], ["Last heartbeat", "2 s ago"], ["Room temp", "28.0 °C · 09:12:30"], ["Humidity", "60 % · 09:12:30"]]} /></Card>
            <Card title="Device information"><SummaryList items={[["Model", "Split 1.5HP (demo)"], ["Firmware", "v2.4.1"], ["Protocol", "MQTT/TLS"], ["Uptime", "14d 8h"], ["Capabilities", "Modes cool/dry/fan · fan low/mid/high · 16–30°C step 1"]]} /></Card>
          </div>
          <Card title="Alert policies on this AC" sub="Limits that raise alerts for this AC. Policies are made in Alerts › Alert policies.">
            <div className="flex flex-col gap-2 text-[13px]"><div className="rounded-xl border border-line p-3"><b>Default policy</b><div className="text-xs text-muted">6 rules · ventilation (CO₂, PM2.5) + fault causes · Always attached · 5 of 6 rules on</div></div><div className="rounded-xl border border-line p-3"><b>Bedroom too hot</b><div className="text-xs text-muted">Room temperature ≥ 30 °C for 60 s → Warning</div><Btn size="sm" className="mt-2" onClick={() => toast("Detached from this AC only", "warn")}>Detach</Btn></div></div>
            <p className="mt-2 text-[11px] text-muted">Detaching only removes it from this AC — the policy stays in your list.</p>
          </Card>
          <Card title="Command history" sub="This unit only · newest first">
            <ul className="flex flex-col divide-y divide-line">{hist.map((h) => <li key={h.id} className="flex flex-wrap justify-between gap-2 py-2 text-[13px]"><span className={h.bad ? "text-crit" : ""}>{h.text}</span><span className="text-xs text-muted">{h.id} · {h.when}</span></li>)}</ul>
          </Card>
        </div>
      </div>
      <Modal open={confirm} onClose={() => setConfirm(false)} title="Confirm command" footer={<><Btn onClick={() => setConfirm(false)}>Cancel</Btn><Btn variant="primary" onClick={send}>Send to {unit.name}</Btn></>}>
        <SummaryList items={[["Target", `${unit.loc} › ${unit.name}`], ["Change", `Set temperature 26°C → ${temp}°C`], ["Mode / fan", `${mode} / ${fan}`]]} />
        <p className="text-xs text-muted">Only {unit.name} changes. The result shows after the device acknowledges.</p>
      </Modal>
    </Page>
  );
}
