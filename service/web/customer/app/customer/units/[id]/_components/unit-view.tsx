"use client";
const SET_TEMPERATURE_FROM_C_TO_C = "Set temperature {from}°C → {to}°C";

import Link from "next/link";
import { useState } from "react";
import { useRouter } from "next/navigation";
import { Badge, Banner, Btn, Card, Check, ConnBadge, Modal, Page, PowerBadge, Stat, SummaryList, Toggle, cx, useToast, Choice } from "@ac/web/components/ui";
import { units, unitRowFromApi, type UnitRow } from "@ac/web/lib/client";
import { OpError } from "@ac/web/lib/ops";
import { controlLayout, FAN_LABEL, historyRow, latest, MODE_LABEL, type ApiCommand, type ApiUnitDetail, type UnitAction } from "@ac/web/lib/units";
import { waitForCommand } from "@ac/web/lib/unitCommands";
import { byCount, showRecent, showTime, textOf } from "@ac/web/lib/i18n";
import { useDisplay, useT } from "@ac/web/components/I18n";
import { createCommand, setUnitAlertPolicies } from "../actions";
import { RenameModal } from "../../../_components/rename-modal";
import { Icon } from "@ac/web/components/Icon";

export interface PolicyOption { id: string; name: string; rule: string }

const siblings = units.slice(0, 2); // the demo room holds the first two units

const CONNECTION: Record<string, string> = { online: "Online", offline: "Offline", unknown: "Unknown", connecting: "Connecting", error: "Error" };
const REASONS: Record<string, string> = { "errors.unit_busy": "another command is still waiting for this AC", "error.versionConflict": "this AC changed — reload and try again", "control.restriction_active": "a service restriction limits this setting", "control.reconciliation_required": "HQ is checking this AC" };

type Reading = { text: string; at: string } | null;
interface CommandRow { id: string; text: string; when: string; bad?: boolean }

/** The readings card: the measured values of a unit (API mode), or the demo room's. */
function TelemetryCard({ d, pow, room, hum }: Readonly<{ d: ApiUnitDetail | null; pow: Reading; room: Reading; hum: Reading }>) {
  const t = useT();
  const display = useDisplay();
  return (
    <Card title={t("Live telemetry")}><SummaryList items={d ? [[t("Power draw"), pow ? `${pow.text} · ${pow.at}` : "—"], [t("Connection"), textOf(CONNECTION, d.connection, t) ?? d.connection], [t("Last seen"), showTime(d.lastSeenAt, display)], [t("Room temp"), room ? `${room.text} · ${room.at}` : "—"], [t("Humidity"), hum ? `${hum.text} · ${hum.at}` : "—"]] : [[t("Power draw"), "742 W · 09:12:30"], ["Wi-Fi", "RSSI −52 dBm (strong)"], [t("Last heartbeat"), "2 s ago"], [t("Room temp"), "28.0 °C · 09:12:30"], [t("Humidity"), "60 % · 09:12:30"]]} /></Card>
  );
}

/** The model card: what the unit is and what it can be set to (API mode), or the demo unit's. */
function DeviceInfoCard({ d, temperatureRange }: Readonly<{ d: ApiUnitDetail | null; temperatureRange: { min: number; max: number; step: number } | null }>) {
  const t = useT();
  const modeWord = (m: string | null) => (m === null ? "—" : textOf(MODE_LABEL, m, t) ?? m);
  const fanWord = (f: string | null) => (f === null ? "—" : textOf(FAN_LABEL, f, t) ?? f);
  return (
    <Card title={t("Device information")}><SummaryList items={d ? [[t("Model"), `${d.capabilities.manufacturer} ${d.capabilities.model}`], [t("Remote control"), t(d.capabilities.control ? "supported" : "not supported")], [t("Capabilities"), temperatureRange ? t("Modes {modes} · fan {fans} · {min}–{max}°C step {step}", { modes: d.capabilities.modes.map((item) => modeWord(item)).join("/") || "—", fans: d.capabilities.fanLevels.map((item) => fanWord(item)).join("/") || "—", min: temperatureRange.min, max: temperatureRange.max, step: temperatureRange.step }) : t("Modes {modes} · fan {fans} · no temperature setting", { modes: d.capabilities.modes.map((item) => modeWord(item)).join("/") || "—", fans: d.capabilities.fanLevels.map((item) => fanWord(item)).join("/") || "—" })]] : [[t("Model"), "Split 1.5HP (demo)"], [t("Firmware"), "v2.4.1"], [t("Protocol"), "MQTT/TLS"], [t("Uptime"), "14d 8h"], [t("Capabilities"), "Modes cool/dry/fan · fan low/mid/high · 16–30°C step 1"]]} /></Card>
  );
}

/** The unit's commands, newest first. */
function CommandHistoryCard({ hist }: Readonly<{ hist: CommandRow[] }>) {
  const t = useT();
  return (
    <Card title={t("Command history")} sub={t("This unit only · newest first")}>
      <ul className="flex flex-col divide-y divide-line">{hist.length === 0 && <li className="py-2 text-[13px] text-muted-foreground">{t("No commands for this AC yet")}</li>}{hist.map((h) => <li key={h.id} className="flex flex-wrap justify-between gap-2 py-2 text-[13px]"><span className={h.bad ? "text-crit" : ""}>{h.text}</span><span className="text-xs text-muted-foreground">{h.id} · {h.when}</span></li>)}</ul>
    </Card>
  );
}

/** The units of the room, the shown one marked. */
function RoomDevicesCard({ id, d, rows }: Readonly<{ id: string; d: ApiUnitDetail | null; rows: UnitRow[] }>) {
  const t = useT();
  return (
      <Card title={t("DEVICES IN THIS ROOM")} sub={d ? byCount(rows.length, { one: t("{room} · {n} AC", { room: d.location.pathLabels.at(-1) ?? t("Room"), n: 1 }), many: t("{room} · {n} ACs", { room: d.location.pathLabels.at(-1) ?? t("Room"), n: rows.length }) }) : "Bedroom · 2 ACs · Home A › 1F"} className="self-start">
        <div className="flex flex-col gap-2">
          {rows.map((s) => (
            <Link key={s.id} href={`/customer/units/${s.id}`} className={cx("rounded-xl border p-3", s.id === id ? "border-primary bg-primary-soft/60" : "border-line hover:bg-surface2/60")}>
              <div className="flex items-center justify-between font-bold">{s.name}{s.id === id && <Badge tone="primary">{t("Selected")}</Badge>}</div>
              <div className="text-xs text-muted-foreground">{t("{temp}°C room", { temp: s.temp ?? "—" })}{s.power === null ? "" : ` · ${String(s.power)} W`}</div>
            </Link>
          ))}
        </div>
        <p className="mt-3 text-[11px] text-muted-foreground">{t("Choose an AC to control it on its own, or use Group control in Units & locations to send one setting to several ACs in this space.")}</p>
      </Card>
  );
}

/** The alert policies on the unit: the always-attached default and the attached customer policies. */
function AlertPoliciesCard({ defaults, policies, attached, onDetach, onAttach }: Readonly<{ defaults: { n: number; on: number } | null; policies: PolicyOption[]; attached: string[]; onDetach: (id: string) => void; onAttach: () => void }>) {
  const t = useT();
  return (
      <Card title={t("Alert policies on this AC")} sub={t("Limits that raise alerts for this AC. Policies are made in Alerts › Alert policies.")}>
        <div className="flex flex-col gap-2 text-[13px]"><div className="rounded-xl border border-line p-3"><b>{t("Default policy")}</b><div className="text-xs text-muted-foreground">{t("{n} rules · ventilation (CO₂, PM2.5) + fault causes · Always attached · {on} of {n} rules on", { n: defaults?.n ?? 6, on: defaults?.on ?? 5 })}</div></div>{policies.filter((pl) => attached.includes(pl.id)).map((pl) => <div key={pl.id} className="rounded-xl border border-line p-3"><b>{pl.name}</b><div className="text-xs text-muted-foreground">{pl.rule}</div><Btn size="sm" className="mt-2" onClick={() => { onDetach(pl.id); }}>{t("Detach")}</Btn></div>)}<Btn size="sm" variant="ghost" onClick={onAttach}><Icon name="add" /> {t("Attach policy")}</Btn></div>
        <p className="mt-2 text-[11px] text-muted-foreground">{t("Detaching only removes it from this AC — the policy stays in your list.")}</p>
      </Card>
  );
}

/** What blocks or limits remote control: HQ's check in progress, a power-off restriction, a cooling limit. */
function RestrictionBanners({ isBlocked, policy, isRestricted, min, href, offHref }: Readonly<{ isBlocked: boolean; policy: { kind: string } | null; isRestricted: boolean; min: number; href: string; offHref: string }>) {
  const t = useT();
  return (
    <>
      {isBlocked && <Banner tone="warn">{t("Remote control is paused while HQ checks this AC’s restriction state. Try again later.")}</Banner>}
      {policy?.kind === "power_off" && <Banner tone="warn">{t("Service restriction active — payment-related: this AC is kept off until the release is confirmed.")} <Link className="font-semibold text-primary" href={offHref}>{t("View restriction details")} <Icon name="arrowRight" /></Link></Banner>}
      {isRestricted && <Banner tone="warn">{t("Cooling restriction active — payment-related: setpoints below {min} °C are unavailable until the release is confirmed.", { min })} <Link className="font-semibold text-primary" href={href}>{t("View restriction details")} <Icon name="arrowRight" /></Link></Banner>}
    </>
  );
}

type ControlMode = "cool" | "dry" | "fan";
type FanLevel = "low" | "mid" | "high";
interface Edits { temp?: boolean; mode?: boolean; fan?: boolean }

/** What the user changed since the last report and the unit does not already have: only those settings are sent. */
function plannedChanges({ d, edited, setpoint, mode, fan, isShowMode, isShowFan }: Readonly<{ d: ApiUnitDetail | null; edited: Edits; setpoint: number; mode: ControlMode; fan: FanLevel; isShowMode: boolean; isShowFan: boolean }>): UnitAction[] {
  if (!d?.capabilities.control) return [];
  const actions: UnitAction[] = [];
  // a setting the AC has not reported starts from a default that is no change
  if (d.capabilities.temperature && edited.temp && setpoint !== d.observedState.celsius) actions.push({ kind: "set_temperature", celsius: setpoint });
  if (isShowMode && edited.mode && mode !== d.observedState.mode) actions.push({ kind: "set_mode", mode });
  if (isShowFan && edited.fan && fan !== d.observedState.fanLevel) actions.push({ kind: "set_fan", fanLevel: fan });
  return actions;
}

interface RemoteControlProps {
  isControllable: boolean; temperatureRange: { min: number; max: number; step: number } | null; cap: { min: number; max: number; step: number };
  setpoint: number; min: number; max: number; confirmed: number | string;
  isOn: boolean; isDisabled: boolean; isOffline: boolean; waiting: boolean; isPowerOff: boolean;
  /** the modes and fan levels the model offers; an empty list hides the choice */
  modes: readonly (ControlMode)[]; mode: ControlMode; fans: readonly (FanLevel)[]; fan: FanLevel;
  /** the changes to review before sending; null for a model that sends the power at once (Figma 02o) */
  review: { count: number; summary: string; target: string; canSend: boolean } | null;
  repairHref: string;
  onPower: (on: boolean) => void; onStep: (delta: number) => void; onMode: (mode: ControlMode) => void; onFan: (fan: FanLevel) => void; onReview: () => void;
}

/** The remote control card: the power, the setpoint, the mode and the fan, and the review of what will be sent. */
function RemoteControlCard(p: Readonly<RemoteControlProps>) {
  const t = useT();
  const { isControllable, temperatureRange, cap, setpoint, min, max, confirmed, isOn, isDisabled, isOffline, waiting, isPowerOff, modes, mode, fans, fan, review } = p;
  const isPowerLocked = isDisabled || waiting || isPowerOff;
  return (
    <Card title={t("REMOTE CONTROL")} sub={t(isControllable ? "Controls apply to this AC only; you confirm before sending" : "No remote control on this model")}>
      {isControllable ? (
      <div className="flex flex-col gap-4">
        <div className="flex flex-wrap items-center justify-between gap-2"><div><div className="font-semibold">{t("Power")}</div><div className="text-xs text-muted-foreground">{t("Sent as its own command")}</div></div><Toggle on={isOn} disabled={isPowerLocked} onChange={(v) => { if (!isPowerLocked) p.onPower(v); }} label={t("Power")} /></div>
        {temperatureRange && <div className="flex flex-wrap items-center justify-between gap-2">
          <div><div className="font-semibold">{t("Set temperature")}</div><div className="text-xs text-muted-foreground">{t("{min}–{max}°C · step {step}°C · confirmed {confirmed}°C", { min, max, confirmed, step: cap.step })}</div></div>
          <div className="flex items-center gap-2"><Btn disabled={isDisabled || setpoint - cap.step < min} onClick={() => { p.onStep(-cap.step); }} aria-label={t("Lower the temperature")}><Icon name="remove" /></Btn><span className="w-14 text-center text-lg font-bold">{setpoint}°C</span><Btn disabled={isDisabled || setpoint + cap.step > max} onClick={() => { p.onStep(cap.step); }} aria-label={t("Raise the temperature")}><Icon name="add" /></Btn></div>
        </div>}
        {modes.length > 0 && <div><div className="mb-1 font-semibold">{t("Mode")}</div><Choice value={mode} disabled={isDisabled} onChange={p.onMode} options={modes.map((m) => ({ id: m, label: t(MODE_LABEL[m]) }))} /></div>}
        {fans.length > 0 && <div><div className="mb-1 font-semibold">{t("Fan speed")}</div><Choice value={fan} disabled={isDisabled} onChange={p.onFan} options={fans.map((f) => ({ id: f, label: t(FAN_LABEL[f]) }))} /></div>}
        {review && ( // a model with the power only sends it at once: nothing to review (Figma 02o)
        <div className="flex flex-wrap items-center justify-between gap-2 rounded-xl bg-surface2 p-3 text-xs"><span><b>{t(review.count === 1 ? "{n} change:" : "{n} changes:", { n: review.count })}</b> {review.summary}<br /><span className="text-muted-foreground">{t("Target: {name} only · you will confirm before sending", { name: review.target })}</span></span><Btn variant="primary" disabled={isDisabled || waiting || !review.canSend} onClick={p.onReview}>{t(waiting ? "Waiting for device…" : "Review & send")}</Btn></div>
        )}
        {waiting && <Banner>{t("Waiting for device response… The result is shown only after the device acknowledges (fails after 30 s).")}</Banner>}
        {isOffline && ( // Figma 02h: nothing is sent to an offline AC; a repair request is the way forward (IR243, IR315)
          <div className="flex flex-wrap items-center justify-between gap-2 rounded-xl bg-surface2 p-3 text-xs">
            <span><b>{t("Controls disabled while offline")}</b><br /><span className="text-muted-foreground">{t("Try again when the unit reconnects, or request a repair.")}</span></span>
            <Link href={p.repairHref} className="rounded-control bg-primary-soft px-2.5 py-1 font-semibold text-primary">{t("Request repair")}</Link>
          </div>
        )}
      </div>
      ) : <p className="text-[13px] text-muted-foreground">{t("This AC’s module only measures: its readings are shown below, and settings are changed on the AC itself.")}</p>}
    </Card>
  );
}

const NO_COMMANDS: ApiCommand[] = [];

/** Unit detail and remote control. In API mode the Server Component passes the unit, its room, the policies and the
scenario's now; the texts and times follow the user's display language and time zone (FR-X01, IR44, IR259), and a
time of another day keeps its date (IR613). */
export function UnitView({ id, detail, room: roomRows, policies, defaults = null, history = NO_COMMANDS, restrictionHref = null, nowMs = 0 }: Readonly<{ id: string; detail?: ApiUnitDetail; room?: UnitRow[]; policies?: PolicyOption[]; defaults?: { on: number; n: number } | null; history?: ApiCommand[]; restrictionHref?: string | null; nowMs?: number }>) {
  const t = useT();
  const display = useDisplay();
  const toast = useToast();
  const router = useRouter();
  const d = detail ?? null;
  const isApi = detail !== undefined;
  const roomUnits = { data: roomRows ?? units };
  const apiPolicies = { data: policies ?? null };
  const mockUnit = units.find((u) => u.id === id) ?? units[0];
  const unit = d ? { ...unitRowFromApi(d), loc: d.location.pathLabels.join(" › "), obs: showRecent(d.lastSeenAt, nowMs, display) } : mockUnit;
  const isOffline = unit.conn === "offline";
  const policy = d?.effectiveControlPolicy.state === "restricted" ? d.effectiveControlPolicy.policy : null;
  const isRestricted = d ? policy?.kind === "temperature_limit" : id === "unit-limited";
  const isBlocked = d?.controlAvailability.state === "blocked";
  const [setpoint, setSetpoint] = useState(24);
  const [mode, setMode] = useState<ControlMode>("cool");
  const [fan, setFan] = useState<FanLevel>("mid");
  const [edited, setEdited] = useState<Edits>({}); // settings the user changed since the last report
  const [confirm, setConfirm] = useState(false);
  const [renaming, setRenaming] = useState(false); // Rename of the AC (Figma 02e, DD-C03, IR315)
  // API mode: start the controls from the device-reported setting when another unit or a newer report arrives
  // (adjust state during render, React "You might not need an effect")
  const reported = d ? `${d.id}|${d.observedState.observedAt ?? ""}` : null;
  const [seenReport, setSeenReport] = useState<string | null>(null);
  if (d && reported !== seenReport) {
    setSeenReport(reported);
    setEdited({});
    if (d.observedState.celsius !== null) setSetpoint(d.observedState.celsius);
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
  const [mockAttached, setMockAttached] = useState<string[]>(["policy-temp-a"]);
  const attached = d ? d.alertPolicyIds : mockAttached;
  const [attachOpen, setAttachOpen] = useState(false);
  const [picked, setPicked] = useState<string[]>([]);
  const [waiting, setWaiting] = useState(false);
  const [mockOn, setMockOn] = useState(unit.state === "running");
  const isOn = d ? d.observedState.power === true : mockOn;
  const [sent, setSent] = useState<ApiCommand[]>([]);
  const [mockHist, setMockHist] = useState<CommandRow[]>([
    { id: "cmd-0230", text: "Set power ON", when: "09:15" },
    { id: "cmd-0229", text: "Set mode COOL", when: "Yesterday 21:02" },
    { id: "cmd-0228", text: "Set fan MID", when: "Yesterday 21:02" },
    { id: "cmd-0227", text: "Set temperature 15°C — rejected: below model minimum 16°C", when: "Yesterday 19:40", bad: true },
  ]);
  // this screen's sends (live status) over the stored history (commands.list, IR216), newest first
  const hist: CommandRow[] = d ? [...sent, ...[...history, ...d.pendingCommands].filter((p, i, all) => sent.every((x) => x.id !== p.id) && all.findIndex((x) => x.id === p.id) === i)]
    .toSorted((a, b) => b.requestedAt.localeCompare(a.requestedAt)).map((c) => historyRow(c, t, display)) : mockHist;
  // the model's capabilities decide what can be set (deterministic contracts: control=false forbids ordinary actions,
  // temperature=null temperature changes; AT-X06); the demo screen shows a full model
  const layout = controlLayout(d?.capabilities ?? null);
  const isControllable = layout.control;
  const temperatureRange = layout.temperature;
  const cap = temperatureRange ?? { min: 16, max: 30, step: 1 };
  const { modes, fans } = layout;
  const isShowMode = modes.length > 0;
  const isShowFan = fans.length > 0;
  const capMin = isRestricted ? 24 : cap.min;
  const min = policy?.kind === "temperature_limit" ? Math.max(cap.min, policy.minimumCoolingSetpoint) : capMin;
  const max = cap.max;
  const confirmed = d ? d.observedState.celsius ?? "—" : 26;
  const room = d ? latest(d, "temperature", t, display, nowMs) : { text: "28.0 °C", at: "09:12:30" };
  const hum = d ? latest(d, "humidity", t, display, nowMs) : { text: "60 %", at: "09:12:30" };
  const pow = d ? latest(d, "power", t, display, nowMs) : { text: "742 W", at: "09:12:30" };
  const isDisabled = isOffline || isBlocked || (d !== null && !d.capabilities.control);
  const track = (c: ApiCommand) => { setSent((xs) => [c, ...xs.filter((x) => x.id !== c.id)]); };
  const reason = (key: string, code: string) => (textOf(REASONS, key, t) ?? code);
  const fail = (e: unknown) => { toast(e instanceof OpError ? t("Not sent: {reason}", { reason: reason(e.error.messageKey, e.error.code) }) : t("Not sent"), "warn"); };
  const modeWord = (m: string | null) => (m === null ? "—" : textOf(MODE_LABEL, m, t) ?? m);
  const fanWord = (f: string | null) => (f === null ? "—" : textOf(FAN_LABEL, f, t) ?? f);
  const changeText = (a: UnitAction) => {
    if (a.kind === "set_temperature") return t(SET_TEMPERATURE_FROM_C_TO_C, { from: confirmed, to: setpoint });
    if (a.kind === "set_mode") return t("Mode → {mode}", { mode: modeWord(mode) });
    if (a.kind === "set_fan") return t("Fan → {fan}", { fan: fanWord(fan) });
    return "";
  };
  const changes = () => plannedChanges({ d, edited, setpoint, mode, fan, isShowMode, isShowFan });
  const changeCount = d ? changes().length : 1; // the demo shows its one temperature change
  // one command: created, then polled through the BFF until the device answers; false stops the rest
  const sendOne = async (a: UnitAction): Promise<boolean> => {
    const created = await createCommand(id, a); // Server Action
    if (!created.ok) {
      toast(t("Not sent: {reason}", { reason: reason(created.messageKey, created.code) }), "warn");
      return false;
    }
    const c = await waitForCommand(created.value, track); // polls commands.get through the BFF
    toast(`${unit.name}: ${historyRow(c, t, display).text}`, c.status === "acknowledged" ? undefined : "warn");
    return c.status === "acknowledged";
  };
  const sendApi = async (actions: UnitAction[]) => {
    setWaiting(true);
    try {
      for (const a of actions) {
        if (!(await sendOne(a))) break;
      }
    } catch (error) {
      fail(error);
    } finally {
      setWaiting(false);
      router.refresh(); // re-read the unit on the server (observed setting, pending commands)
    }
  };
  const togglePower = (v: boolean) => {
    if (d) { void sendApi([{ kind: "set_power", power: v }]); return; }
    setMockOn(v);
    toast(t(v ? "Power ON sent" : "Power OFF sent"));
  };
  const savePolicies = (ids: string[], message: string, tone?: "warn") => {
    if (!d) {
      setMockAttached(ids);
      toast(message, tone);
      return;
    }
    const report = (res: Awaited<ReturnType<typeof setUnitAlertPolicies>>) => {
      if (res.ok) toast(message, tone); else toast(t("Not saved: {reason}", { reason: reason(res.messageKey, res.code) }), "warn");
    };
    void (async () => {
      try {
        report(await setUnitAlertPolicies(id, ids));
      } catch (error: unknown) {
        fail(error);
      }
    })();
  };
  const send = () => {
    setConfirm(false);
    if (d) {
      void sendApi(changes());
      return;
    }
    setWaiting(true);
    setTimeout(() => { setWaiting(false); setMockHist((h) => [{ id: "cmd-0231", text: t("Set temperature {celsius}°C", { celsius: setpoint }), when: t("now") }, ...h]); toast(t("{name}: {celsius}°C acknowledged by device", { name: unit.name, celsius: setpoint })); }, 1500);
  };
  return (
    <Page>
      <div className="flex flex-wrap items-center gap-2 text-[13px] text-muted-foreground"><Link href="/customer/properties" className="font-semibold text-primary"><Icon name="arrowLeft" /> {t("Back to {place}", { place: d ? d.location.pathLabels.at(-1) ?? t("Units") : "Bedroom" })}</Link><span>{unit.loc} › <b className="text-ink">{unit.name}</b></span></div>
      <div className="split-rev">
        <RoomDevicesCard id={id} d={d} rows={isApi ? roomUnits.data : siblings} />

        <div className="flex min-w-0 flex-col gap-4">
          <Card title={unit.name} sub={isApi ? unit.loc : `${unit.loc} · ${unit.id}`} action={<span className="flex flex-wrap items-center justify-end gap-2"><Btn size="sm" variant="ghost" onClick={() => { setRenaming(true); }}><Icon name="edit" />{t("Rename")}</Btn><PowerBadge s={unit.state} /><ConnBadge s={unit.conn} /></span>}>
            {isOffline ? <Banner tone="warn">{t("This AC is offline. Remote actions are disabled until the connection returns. Last seen {when}.", { when: unit.obs })}</Banner> : (
              <div className="grid-fluid" style={{ ["--min" as string]: "160px" }}>
                <Stat label={t("Room temperature (measured)")} value={room?.text ?? "—"} sub={room ? t("observed {at}", { at: room.at }) : t("no recent measurement")} />
                <Stat label={t("Confirmed setting")} value={d ? `${String(d.observedState.celsius ?? "—")}°C · ${modeWord(d.observedState.mode)} · ${fanWord(d.observedState.fanLevel)}` : `26°C · ${t("Cool")} · ${t("Mid")}`} sub={d ? t("reported {at}", { at: showRecent(d.observedState.observedAt, nowMs, display) }) : t("acknowledged by device")} />
                <Stat label={t("Humidity")} value={hum?.text ?? "—"} sub={hum ? t("observed {at}", { at: hum.at }) : t("no recent measurement")} />
              </div>
            )}
          </Card>
          <RestrictionBanners isBlocked={isBlocked} policy={policy} isRestricted={isRestricted} min={min} href={d ? restrictionHref ?? "/customer/payments" : "/customer/payments/invoice-overdue-a?tab=restriction"} offHref={restrictionHref ?? "/customer/payments"} />
          <RemoteControlCard
            isControllable={isControllable} temperatureRange={temperatureRange} cap={cap} setpoint={setpoint} min={min} max={max} confirmed={confirmed}
            isOn={isOn} isDisabled={isDisabled} isOffline={isOffline} waiting={waiting} isPowerOff={policy?.kind === "power_off"}
            modes={isShowMode ? modes : []} mode={mode} fans={isShowFan ? fans : []} fan={fan}
            review={layout.review ? { count: changeCount, summary: d ? changes().map((item) => changeText(item)).join(" · ") || t("No change yet") : t(SET_TEMPERATURE_FROM_C_TO_C, { from: 26, to: setpoint }), target: unit.name, canSend: d === null || changes().length > 0 } : null}
            repairHref={`/customer/maintenance?new=${d?.id ?? unit.id}`}
            onPower={togglePower} onStep={(delta) => { setSetpoint((x) => x + delta); setEdited((e) => ({ ...e, temp: true })); }}
            onMode={(m) => { setMode(m); setEdited((e) => ({ ...e, mode: true })); }} onFan={(f) => { setFan(f); setEdited((e) => ({ ...e, fan: true })); }} onReview={() => { setConfirm(true); }}
          />
          <div className="split-even">
            <TelemetryCard d={d} pow={pow} room={room} hum={hum} />
            <DeviceInfoCard d={d} temperatureRange={temperatureRange} />
          </div>
          <AlertPoliciesCard defaults={defaults} policies={customerPolicies} attached={attached} onDetach={(policyId) => { savePolicies(attached.filter((y) => y !== policyId), t("Detached from this AC only"), "warn"); }} onAttach={() => { setPicked([]); setAttachOpen(true); }} />
          <CommandHistoryCard hist={hist} />
        </div>
      </div>
      <Modal open={confirm} onClose={() => { setConfirm(false); }} title={t("Confirm command")} footer={<><Btn onClick={() => { setConfirm(false); }}>{t("Cancel")}</Btn><Btn variant="primary" onClick={send}>{t("Send to {name}", { name: unit.name })}</Btn></>}>
        <SummaryList items={[[t("Target"), `${unit.loc} › ${unit.name}`], [t("Change"), d ? changes().map((item) => changeText(item)).join(" · ") : t(SET_TEMPERATURE_FROM_C_TO_C, { from: 26, to: setpoint })], [t("Mode / fan"), `${modeWord(mode)} / ${fanWord(fan)}`]]} />
        <p className="text-xs text-muted-foreground">{t("Only {name} changes. The result shows after the device acknowledges.", { name: unit.name })}</p>
      </Modal>
      <Modal open={attachOpen} onClose={() => { setAttachOpen(false); }} title={t("Attach a policy to {name}", { name: unit.name })} footer={<><Btn onClick={() => { setAttachOpen(false); }}>{t("Cancel")}</Btn><Btn variant="primary" disabled={picked.length === 0} onClick={() => { savePolicies([...attached, ...picked], t(picked.length > 1 ? "{n} policies attached to this AC" : "{n} policy attached to this AC", { n: picked.length })); setAttachOpen(false); }}>{t("Attach")}</Btn></>}>
        <p className="mb-2 text-[13px] text-muted-foreground">{t("Only your account’s policies are listed. The default policy is always attached.")}</p>
        <div className="flex flex-col gap-2">{customerPolicies.filter((pl) => !attached.includes(pl.id)).map((pl) => <Check key={pl.id} label={<span><b>{pl.name}</b> <span className="text-xs text-muted-foreground">{pl.rule}</span></span>} checked={picked.includes(pl.id)} onChange={(v) => { setPicked((x) => (v ? [...x, pl.id] : x.filter((y) => y !== pl.id))); }} />)}</div>
      </Modal>
      {renaming && <RenameModal demo={!isApi} r={{ kind: "unit", id: d?.id ?? unit.id, version: d?.version ?? 1, name: unit.name, where: `${unit.loc} › ${unit.name}` }} onClose={() => { setRenaming(false); }} />}
    </Page>
  );
}
