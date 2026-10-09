"use client";

import { useState } from "react";
import { Badge, Banner, Btn, Card, Choice, CodeInput, Field, Modal, Page, QrCode, Select, Toggle } from "@ac/web/components/ui";
import { useAction } from "@ac/web/lib/useAction";
import { actionMessage } from "@ac/web/lib/actionMessage";
import { setStoredValue, useStoredValue } from "@ac/web/lib/urlState";
import { consentNote, type Consent } from "@ac/web/lib/preferences";
import { codeComplete, groupKey, type QrPath } from "@ac/web/lib/twoFactor";
import { disableTwoFactor, enableTwoFactor, savePreferences } from "../actions";

export type PreferencesLive = {
  prefs: { locale: "en" | "ms"; timezone: string; currency: "MYR"; monthlyReportEmail: boolean; twoFactorEnabled: boolean };
  twoFactor: { enabled: boolean; enabledAt: string | null; recoveryCodesLeft: number | null; setupKey: string | null };
  consent: Consent | null;
  client: boolean;
  zones: { id: string; label: string }[];
  qr: QrPath | null; // the setup key as an authenticator QR code while two-step verification is off
};
const MIC = "ac-voice-microphone"; // a device permission: kept in this browser, not on the account

/** Preferences (FR-X01, FR-X08, DDC-07, Figma Client 10d/10g) from the Core API: language and time zone of the user,
 * for a client the location consent and the monthly energy report e-mail (BR-C16), two-step verification; the
 * microphone switch stays on this device. */
export function PreferencesView({ live }: { live: PreferencesLive }) {
  const [pending, run] = useAction();
  const [locale, setLocale] = useState(live.prefs.locale);
  const [zone, setZone] = useState(live.prefs.timezone);
  const [location, setLocation] = useState(live.consent?.granted ?? false);
  const [monthly, setMonthly] = useState(live.prefs.monthlyReportEmail);
  const mic = useStoredValue(MIC) !== "off";
  const [modal, setModal] = useState<null | "on" | "off">(null);
  const [failed, setFailed] = useState<string | null>(null);
  const consentChanged = live.client && location !== (live.consent?.granted ?? false);
  const dirty = locale !== live.prefs.locale || zone !== live.prefs.timezone || consentChanged || (live.client && monthly !== live.prefs.monthlyReportEmail);
  // monthlyReportEmail only from a client session (preferences.update refuses it otherwise, IR142 item 5)
  const save = () => run(() => savePreferences({ locale, timezone: zone, ...(live.client ? { monthlyReportEmail: monthly } : {}) }, consentChanged ? { granted: location, version: live.consent?.version ?? 0 } : null), "Preferences saved",
    () => setFailed(null), (f) => setFailed(actionMessage(f)));
  const reset = () => { setLocale(live.prefs.locale); setZone(live.prefs.timezone); setLocation(live.consent?.granted ?? false); setMonthly(live.prefs.monthlyReportEmail); setFailed(null); };
  const tf = live.twoFactor;
  return (
    <Page className="max-w-3xl">
      {failed && <Banner tone="crit">Not saved: {failed}</Banner>}
      <Card>
        <Field label="Display language" hint="Affects key screens, notifications, and date/time display only. Stored data stays UTC (NFR-08). This build shows English only (i18n).">
          <Choice value={locale} onChange={setLocale} options={[{ id: "en", label: "English (default)" }, { id: "ms", label: "Bahasa Melayu" }]} />
        </Field>
        <div className="my-4 border-t border-line" />
        <Field label="Display timezone" hint="Booked times keep their meaning across timezone changes.">
          <Select value={zone} onChange={(e) => setZone(e.target.value)}>{live.zones.map((z) => <option key={z.id} value={z.id}>{z.label}</option>)}</Select>
        </Field>
        <div className="mt-4"><Field label="Currency" hint="Invoices keep their own currency — nothing is converted."><div className="flex items-center justify-between rounded-control border border-line bg-surface2 px-3 py-2 text-[13px] text-muted">MYR — Malaysian Ringgit <span className="text-[11px]">read-only demo</span></div></Field></div>
        <div className="my-4 border-t border-line" />
        {live.client && (
          <div className="flex flex-col gap-3">
            <div className="text-[13px] font-semibold">Consent</div>
            <div className="flex items-center justify-between gap-3"><div><div className="text-[13px]">Location, for voice room disambiguation and location automations</div><div className="text-xs text-muted">{consentNote(live.consent)} · withdrawal turns off location automations and discards pending voice requests</div></div><Toggle on={location} onChange={setLocation} label="Location consent" /></div>
            <div className="flex items-center justify-between gap-3"><div><div className="text-[13px]">Microphone, for voice actions (this device)</div><div className="text-xs text-muted">Text input always available as a fallback</div></div><Toggle on={mic} onChange={(v) => setStoredValue(MIC, v ? "on" : "off")} label="Microphone consent" /></div>
            <div className="mt-1 text-[13px] font-semibold">Reports</div>
            <div className="flex items-center justify-between gap-3"><div><div className="text-[13px]">Email me the monthly energy report (1st of each month)</div><div className="text-xs text-muted">Last month&apos;s report, to your account e-mail · also set from Energy &amp; cost › Export report</div></div><Toggle on={monthly} onChange={setMonthly} label="Monthly energy report e-mail" /></div>
          </div>
        )}
        <div className="mt-4 flex flex-wrap items-center justify-between gap-3">
          <div><div className="text-[13px] font-semibold">Security · Two-step verification</div><div className="text-xs text-muted">Ask for a 6-digit code from an authenticator app when signing in on a new device{tf.enabled && tf.recoveryCodesLeft !== null ? ` · ${tf.recoveryCodesLeft} recovery codes left` : ""}.</div></div>
          <span className="flex items-center gap-2">{tf.enabled ? <><Badge tone="ok">On</Badge><Btn size="sm" onClick={() => setModal("off")}>Turn off</Btn></> : <><Badge>Off</Badge><Btn size="sm" variant="primary" onClick={() => setModal("on")}>Turn on</Btn></>}</span>
        </div>
        <p className="mt-1 text-[11px] text-muted">Demo only — any 6 digits are accepted and the demo sign-in is unchanged (FR-X08).</p>
        <div className="mt-5 flex justify-end gap-2"><Btn disabled={!dirty || pending} onClick={reset}>Cancel</Btn><Btn variant="primary" disabled={!dirty || pending} onClick={save}>✓ Save preferences</Btn></div>
      </Card>
      {modal === "on" && <TurnOn setupKey={tf.setupKey} qr={live.qr} onClose={() => setModal(null)} />}
      {modal === "off" && <TurnOff onClose={() => setModal(null)} />}
    </Page>
  );
}

function TurnOn({ setupKey, qr, onClose }: { setupKey: string | null; qr: QrPath | null; onClose: () => void }) {
  const [pending, run] = useAction();
  const [code, setCode] = useState("");
  const [codes, setCodes] = useState<string[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  return (
    <Modal open onClose={onClose} title="Turn on two-step verification" footer={codes ? <Btn variant="primary" onClick={onClose}>I saved my codes</Btn> : <><Btn onClick={onClose}>Cancel</Btn><Btn variant="primary" disabled={!codeComplete(code) || pending} onClick={() => run(() => enableTwoFactor(code), "Two-step verification is on", (c) => setCodes(c), (f) => setError(actionMessage(f)))}>Verify & turn on</Btn></>}>
      {codes ? <><Banner tone="ok">Two-step verification is on. Save these {codes.length} recovery codes — they are shown only once.</Banner><div className="grid grid-cols-2 gap-2 font-mono text-sm">{codes.map((c) => <span key={c} className="rounded-lg bg-surface2 px-3 py-1.5">{c}</span>)}</div></> : <div className="flex flex-col gap-3.5">
        <div className="flex flex-col gap-5 sm:flex-row">
          <div className="grid h-[150px] w-[150px] shrink-0 place-items-center rounded-lg border border-line bg-white p-2">{qr ? <QrCode qr={qr} label="QR code with the setup key for an authenticator app" className="h-full w-full" /> : <span className="text-xs text-muted">No setup key</span>}</div>
          <div className="flex min-w-0 flex-col gap-2.5 text-[13px]">
            <b>1&nbsp;&nbsp;Scan with an authenticator app</b>
            <div className="text-xs text-muted">or enter key&nbsp;&nbsp;<span className="font-mono">{setupKey ? groupKey(setupKey) : "—"}</span></div>
            <b>2&nbsp;&nbsp;Enter the 6-digit code</b>
            <CodeInput label="6-digit code" value={code} onChange={setCode} autoFocus />
            <b>3&nbsp;&nbsp;Save your 8 recovery codes (shown after verifying)</b>
          </div>
        </div>
        {error && <p className="text-xs text-crit">✕ {error}</p>}
        <Banner>Demo: no real authenticator is called — any 6 digits are accepted, and sign-in is unchanged. Turning off later asks for a current code.</Banner>
      </div>}
    </Modal>
  );
}

function TurnOff({ onClose }: { onClose: () => void }) {
  const [pending, run] = useAction();
  const [code, setCode] = useState("");
  const [error, setError] = useState<string | null>(null);
  return (
    <Modal open onClose={onClose} title="Turn off two-step verification" footer={<><Btn onClick={onClose}>Keep it on</Btn><Btn variant="danger" disabled={!codeComplete(code) || pending} onClick={() => run(() => disableTwoFactor(code), "Two-step verification turned off", onClose, (f) => setError(actionMessage(f)))}>Turn off</Btn></>}>
      <Field label="Enter a 6-digit code from your authenticator app"><CodeInput label="6-digit code" value={code} onChange={setCode} autoFocus /></Field>
      {error && <p className="mt-2 text-xs text-crit">✕ {error}</p>}
    </Modal>
  );
}
