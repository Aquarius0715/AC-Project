"use client";

import Link from "next/link";
import { useState } from "react";
import { useRouter } from "next/navigation";
import { Badge, Banner, Btn, Card, Check, ConnBadge, Modal, Page, PowerBadge, Stat, SummaryList, Toggle, cx, useToast, Choice } from "@ac/web/components/ui";
import { units, unitRowFromApi, type ApiUnit, type UnitRow } from "@ac/web/lib/client";
import { OpError } from "@ac/web/lib/ops";
import { historyRow, klTime, latest, type ApiCommand, type ApiUnitDetail, type UnitAction } from "@ac/web/lib/units";
import { waitForCommand } from "@ac/web/lib/unitCommands";
import { createCommand, setUnitAlertPolicies } from "../actions";

export type PolicyOption = { id: string; name: string; rule: string };

const siblings = [units[0], units[1]];
type Cmd = { id: string; text: string; when: string; bad?: boolean };

/** Unit detail and remote control. In API mode the Server Component passes the unit, its room and the policies. */
export function UnitView({ id, detail, room: roomRows, policies }: { id: string; detail?: ApiUnitDetail; room?: UnitRow[]; policies?: PolicyOption[] }) {
  const toast = useToast();
  const router = useRouter();
  const d = detail ?? null;
  const api = detail !== undefined;
  const roomUnits = { data: roomRows ?? units };
  const apiPolicies = { data: policies ?? null };
  const mockUnit = units.find((u) => u.id === id) ?? units[0];
  const unit = d ? { ...unitRowFromApi(d as unknown as ApiUnit), loc: d.location.pathLabels.join(" › "), obs: klTime(d.lastSeenAt) } : mockUnit;
  const offline = unit.conn === "offline";
  const policy = d?.effectiveControlPolicy.state === "restricted" ? d.effectiveControlPolicy.policy : null;
  const restricted = d ? policy?.kind === "temperature_limit" : id === "unit-limited";
  const blocked = d?.controlAvailability.state === "blocked";
  const [temp, setTemp] = useState(24);
  const [mode, setMode] = useState<"cool" | "dry" | "fan">("cool");
  const [fan, setFan] = useState<"low" | "mid" | "high">("mid");
  const [confirm, setConfirm] = useState(false);
  // api mode: start the controls from the device-reported setting when another unit or a newer report arrives
  // (adjust state during render, React "You might not need an effect")
  const reported = d ? `${d.id}|${d.observedState.observedAt}` : null;
  const [seenReport, setSeenReport] = useState<string | null>(null);
  if (d && reported !== seenReport) {
    setSeenReport(reported);
    if (d.observedState.celsius !== null) setTemp(d.observedState.celsius);
    if (d.observedState.mode) setMode(d.observedState.mode);
    if (d.observedState.fanLevel) setFan(d.observedState.fanLevel);
  }
  // Alert policies on this AC (FR-C15, Figma 02e / 02l): customer policies attached per unit; the default policy is implicit.
  const mockPolicies = [
    { id: "policy-temp-a", name: "Bedroom too hot", rule: "Room temperature ≥ 30 °C for 60 s → Warning" },
    { id: "policy-co2-a", name: "Stuffy office", rule: "CO₂ ≥ 1200 ppm for 15 min · weekdays → Warning" },
    { id: "policy-humidity-a", name: "Night humidity", rule: "Humidity ≥ 70 % for 30 min · 22:00–06:00 → Info" },
  ];
  const customerPolicies = apiPolicies.data ?? mockPolicies;
  const [mockAttached, setAttached] = useState<string[]>(["policy-temp-a"]);
  const attached = d ? d.alertPolicyIds : mockAttached;
  const [attachOpen, setAttachOpen] = useState(false);
  const [picked, setPicked] = useState<string[]>([]);
  const [waiting, setWaiting] = useState(false);
  const [mockOn, setOn] = useState(unit.state === "running");
  const on = d ? d.observedState.power === true : mockOn;
  const [sent, setSent] = useState<ApiCommand[]>([]);
  const [mockHist, setHist] = useState<Cmd[]>([
    { id: "cmd-0230", text: "Set power ON", when: "09:15" },
    { id: "cmd-0229", text: "Set mode COOL", when: "Yesterday 21:02" },
    { id: "cmd-0228", text: "Set fan MID", when: "Yesterday 21:02" },
    { id: "cmd-0227", text: "Set temperature 15°C — rejected: below model minimum 16°C", when: "Yesterday 19:40", bad: true },
  ]);
  const hist: Cmd[] = d ? [...sent, ...d.pendingCommands.filter((p) => !sent.some((x) => x.id === p.id))].map(historyRow) : mockHist;
  const cap = d?.capabilities.temperature ?? { min: 16, max: 30, step: 1 };
  const min = policy?.kind === "temperature_limit" ? Math.max(cap.min, policy.minimumCoolingSetpoint) : restricted ? 24 : cap.min;
  const max = cap.max;
  const confirmed = d?.observedState.celsius ?? 26;
  const room = d ? latest(d, "temperature") : { text: "28.0 °C", at: "09:12:30" };
  const hum = d ? latest(d, "humidity") : { text: "60 %", at: "09:12:30" };
  const pow = d ? latest(d, "power") : { text: "742 W", at: "09:12:30" };
  const disabled = offline || blocked || (d !== null && !d.capabilities.control);
  const track = (c: ApiCommand) => setSent((xs) => [c, ...xs.filter((x) => x.id !== c.id)]);
  const reasons: Record<string, string> = { "errors.unit_busy": "another command is still waiting for this AC", "error.versionConflict": "this AC changed — reload and try again", "control.restriction_active": "a service restriction limits this setting", "control.reconciliation_required": "HQ is checking this AC" };
  const fail = (e: unknown) => toast(e instanceof OpError ? `Not sent: ${reasons[e.error.messageKey] ?? e.error.code}` : "Not sent", "warn");
  const changes = (): UnitAction[] => {
    if (!d) return [];
    const a: UnitAction[] = [];
    if (temp !== d.observedState.celsius) a.push({ kind: "set_temperature", celsius: temp });
    if (d.capabilities.modeControl && mode !== d.observedState.mode) a.push({ kind: "set_mode", mode });
    if (d.capabilities.fanControl && fan !== d.observedState.fanLevel) a.push({ kind: "set_fan", fanLevel: fan });
    return a;
  };
  const sendApi = async (actions: UnitAction[]) => {
    setWaiting(true);
    try {
      for (const a of actions) {
        const created = await createCommand(id, a); // Server Action
        if (!created.ok) {
          toast(`Not sent: ${reasons[created.messageKey] ?? created.code}`, "warn");
          break;
        }
        const c = await waitForCommand(created.value, track); // polls commands.get through the BFF

        toast(`${unit.name}: ${historyRow(c).text}`, c.status === "acknowledged" ? undefined : "warn");
        if (c.status !== "acknowledged") break;
      }
    } catch (e) {
      fail(e);
    } finally {
      setWaiting(false);
      router.refresh(); // re-read the unit on the server (observed setting, pending commands)
    }
  };
  const togglePower = (v: boolean) => (d ? void sendApi([{ kind: "set_power", power: v }]) : (setOn(v), toast(`Power ${v ? "ON" : "OFF"} sent`)));
  const savePolicies = (ids: string[], msg: string, tone?: "warn") => {
    if (!d) return setAttached(ids), toast(msg, tone);
    setUnitAlertPolicies(id, ids).then((res) => (res.ok ? toast(msg, tone) : toast(`Not saved: ${reasons[res.messageKey] ?? res.code}`, "warn")), fail);
  };
  const send = () => {
    setConfirm(false);
    if (d) return void sendApi(changes());
    setWaiting(true);
    setTimeout(() => { setWaiting(false); setHist((h) => [{ id: "cmd-0231", text: `Set temperature ${temp}°C`, when: "now" }, ...h]); toast(`${unit.name}: ${temp}°C acknowledged by device`); }, 1500);
  };
  return (
    <Page>
      <div className="flex flex-wrap items-center gap-2 text-[13px] text-muted"><Link href="/customer/properties" className="font-semibold text-primary">← Back to {d ? d.location.pathLabels.at(-1) ?? "Units" : "Bedroom"}</Link><span>{unit.loc} › <b className="text-ink">{unit.name}</b></span></div>
      <div className="split-rev">
        <Card title="DEVICES IN THIS ROOM" sub={d ? `${d.location.pathLabels.at(-1) ?? "Room"} · ${roomUnits.data.length} AC${roomUnits.data.length === 1 ? "" : "s"}` : "Bedroom · 2 ACs · Home A › 1F"} className="self-start">
          <div className="flex flex-col gap-2">
            {(api ? roomUnits.data : siblings).map((s) => (
              <Link key={s.id} href={`/customer/units/${s.id}`} className={cx("rounded-xl border p-3", s.id === id ? "border-primary bg-primary-soft/60" : "border-line hover:bg-surface2/60")}>
                <div className="flex items-center justify-between font-bold">{s.name}{s.id === id && <Badge tone="primary">Selected</Badge>}</div>
                <div className="text-xs text-muted">{s.temp ?? "—"}°C room{s.power !== null ? ` · ${s.power} W` : ""}</div>
              </Link>
            ))}
          </div>
          <p className="mt-3 text-[11px] text-muted">Choose an AC to control it on its own, or use Group control in Units &amp; locations to send one setting to several ACs in this space.</p>
        </Card>

        <div className="flex min-w-0 flex-col gap-4">
          <Card title={unit.name} sub={api ? unit.loc : `${unit.loc} · ${unit.id}`} action={<span className="flex items-center gap-2"><PowerBadge s={unit.state} /><ConnBadge s={unit.conn} /></span>}>
            {offline ? <Banner tone="warn">This AC is offline. Remote actions are disabled until the connection returns. Last seen {unit.obs}.</Banner> : (
              <div className="grid-fluid" style={{ ["--min" as string]: "160px" }}>
                <Stat label="Room temperature (measured)" value={room?.text ?? "—"} sub={room ? `observed ${room.at}` : "no recent measurement"} />
                <Stat label="Confirmed setting" value={d ? `${d.observedState.celsius ?? "—"}°C · ${d.observedState.mode ?? "—"} · ${d.observedState.fanLevel ?? "—"}` : "26°C · Cool · Mid"} sub={d ? `reported ${klTime(d.observedState.observedAt)}` : "acknowledged by device"} />
                <Stat label="Humidity" value={hum?.text ?? "—"} sub={hum ? `observed ${hum.at}` : "no recent measurement"} />
              </div>
            )}
          </Card>
          {blocked && <Banner tone="warn">Remote control is paused while HQ checks this AC’s restriction state. Try again later.</Banner>}
          {policy?.kind === "power_off" && <Banner tone="warn">Service restriction active — this AC is kept off. <Link className="font-semibold text-primary" href="/customer/payments">View details →</Link></Banner>}
          {restricted && <Banner tone="warn">Cooling restriction active — minimum {min} °C. <Link className="font-semibold text-primary" href="/customer/payments/invoice-overdue-a?tab=restriction">View details →</Link></Banner>}
          <Card title="REMOTE CONTROL" sub="Controls apply to this AC only; you confirm before sending">
            <div className="flex flex-col gap-4">
              <div className="flex flex-wrap items-center justify-between gap-2"><div><div className="font-semibold">Power</div><div className="text-xs text-muted">Sent as its own command</div></div><Toggle on={on} onChange={(v) => (!disabled && !waiting && policy?.kind !== "power_off" ? togglePower(v) : undefined)} label="Power" /></div>
              <div className="flex flex-wrap items-center justify-between gap-2">
                <div><div className="font-semibold">Set temperature</div><div className="text-xs text-muted">{min}–{max}°C · step {cap.step}°C · confirmed {confirmed}°C</div></div>
                <div className="flex items-center gap-2"><Btn disabled={disabled || temp - cap.step < min} onClick={() => setTemp((t) => t - cap.step)}>–</Btn><span className="w-14 text-center text-lg font-bold">{temp}°C</span><Btn disabled={disabled || temp + cap.step > max} onClick={() => setTemp((t) => t + cap.step)}>+</Btn></div>
              </div>
              <div><div className="mb-1 font-semibold">Mode</div><Choice value={mode} onChange={setMode} options={[{ id: "cool", label: "Cool" }, { id: "dry", label: "Dry" }, { id: "fan", label: "Fan" }]} /></div>
              <div><div className="mb-1 font-semibold">Fan speed</div><Choice value={fan} onChange={setFan} options={[{ id: "low", label: "Low" }, { id: "mid", label: "Mid" }, { id: "high", label: "High" }]} /></div>
              <div className="flex flex-wrap items-center justify-between gap-2 rounded-xl bg-surface2 p-3 text-xs"><span><b>{d ? `${changes().length} change${changes().length === 1 ? "" : "s"}:` : "1 change:"}</b> {d ? changes().map((a) => a.kind === "set_temperature" ? `Set temperature ${confirmed}°C → ${temp}°C` : a.kind === "set_mode" ? `Mode → ${mode}` : a.kind === "set_fan" ? `Fan → ${fan}` : "").join(" · ") || "No change yet" : `Set temperature 26°C → ${temp}°C`}<br /><span className="text-muted">Target: {unit.name} only · you will confirm before sending</span></span><Btn variant="primary" disabled={disabled || waiting || (d !== null && changes().length === 0)} onClick={() => setConfirm(true)}>{waiting ? "Waiting for device…" : "Review & send"}</Btn></div>
              {waiting && <Banner>Waiting for device response… The result is shown only after the device acknowledges (fails after 30 s).</Banner>}
            </div>
          </Card>
          <div className="split-even">
            <Card title="Live telemetry"><SummaryList items={d ? [["Power draw", pow ? `${pow.text} · ${pow.at}` : "—"], ["Connection", d.connection], ["Last seen", klTime(d.lastSeenAt, true)], ["Room temp", room ? `${room.text} · ${room.at}` : "—"], ["Humidity", hum ? `${hum.text} · ${hum.at}` : "—"]] : [["Power draw", "742 W · 09:12:30"], ["Wi-Fi", "RSSI −52 dBm (strong)"], ["Last heartbeat", "2 s ago"], ["Room temp", "28.0 °C · 09:12:30"], ["Humidity", "60 % · 09:12:30"]]} /></Card>
            <Card title="Device information"><SummaryList items={d ? [["Model", `${d.capabilities.manufacturer} ${d.capabilities.model}`], ["Remote control", d.capabilities.control ? "supported" : "not supported"], ["Capabilities", `Modes ${d.capabilities.modes.join("/") || "—"} · fan ${d.capabilities.fanLevels.join("/") || "—"} · ${cap.min}–${cap.max}°C step ${cap.step}`]] : [["Model", "Split 1.5HP (demo)"], ["Firmware", "v2.4.1"], ["Protocol", "MQTT/TLS"], ["Uptime", "14d 8h"], ["Capabilities", "Modes cool/dry/fan · fan low/mid/high · 16–30°C step 1"]]} /></Card>
          </div>
          <Card title="Alert policies on this AC" sub="Limits that raise alerts for this AC. Policies are made in Alerts › Alert policies.">
            <div className="flex flex-col gap-2 text-[13px]"><div className="rounded-xl border border-line p-3"><b>Default policy</b><div className="text-xs text-muted">6 rules · ventilation (CO₂, PM2.5) + fault causes · Always attached · 5 of 6 rules on</div></div>{customerPolicies.filter((pl) => attached.includes(pl.id)).map((pl) => <div key={pl.id} className="rounded-xl border border-line p-3"><b>{pl.name}</b><div className="text-xs text-muted">{pl.rule}</div><Btn size="sm" className="mt-2" onClick={() => savePolicies(attached.filter((y) => y !== pl.id), "Detached from this AC only", "warn")}>Detach</Btn></div>)}<Btn size="sm" variant="ghost" onClick={() => { setPicked([]); setAttachOpen(true); }}>+ Attach policy</Btn></div>
            <p className="mt-2 text-[11px] text-muted">Detaching only removes it from this AC — the policy stays in your list.</p>
          </Card>
          <Card title="Command history" sub="This unit only · newest first">
            <ul className="flex flex-col divide-y divide-line">{hist.length === 0 && <li className="py-2 text-[13px] text-muted">No commands sent from this screen yet</li>}{hist.map((h) => <li key={h.id} className="flex flex-wrap justify-between gap-2 py-2 text-[13px]"><span className={h.bad ? "text-crit" : ""}>{h.text}</span><span className="text-xs text-muted">{h.id} · {h.when}</span></li>)}</ul>
          </Card>
        </div>
      </div>
      <Modal open={confirm} onClose={() => setConfirm(false)} title="Confirm command" footer={<><Btn onClick={() => setConfirm(false)}>Cancel</Btn><Btn variant="primary" onClick={send}>Send to {unit.name}</Btn></>}>
        <SummaryList items={[["Target", `${unit.loc} › ${unit.name}`], ["Change", d ? changes().map((a) => a.kind === "set_temperature" ? `Set temperature ${confirmed}°C → ${temp}°C` : a.kind === "set_mode" ? `Mode → ${mode}` : a.kind === "set_fan" ? `Fan → ${fan}` : "").join(" · ") : `Set temperature 26°C → ${temp}°C`], ["Mode / fan", `${mode} / ${fan}`]]} />
        <p className="text-xs text-muted">Only {unit.name} changes. The result shows after the device acknowledges.</p>
      </Modal>
      <Modal open={attachOpen} onClose={() => setAttachOpen(false)} title={`Attach a policy to ${unit.name}`} footer={<><Btn onClick={() => setAttachOpen(false)}>Cancel</Btn><Btn variant="primary" disabled={!picked.length} onClick={() => { savePolicies([...attached, ...picked], `${picked.length} polic${picked.length > 1 ? "ies" : "y"} attached to this AC`); setAttachOpen(false); }}>Attach</Btn></>}>
        <p className="mb-2 text-[13px] text-muted">Only your account’s policies are listed. The default policy is always attached.</p>
        <div className="flex flex-col gap-2">{customerPolicies.filter((pl) => !attached.includes(pl.id)).map((pl) => <Check key={pl.id} label={<span><b>{pl.name}</b> <span className="text-xs text-muted">{pl.rule}</span></span>} checked={picked.includes(pl.id)} onChange={(v) => setPicked((x) => (v ? [...x, pl.id] : x.filter((y) => y !== pl.id)))} />)}</div>
      </Modal>
    </Page>
  );
}
