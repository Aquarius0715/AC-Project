"use client";

import { useState } from "react";
import { Badge, Banner, Btn, Card, Check, Choice, CodeInput, Modal, Page, QrCode, Select, Toggle, useToast } from "@ac/web/components/ui";
import { useStoredRole } from "@ac/web/components/AppShell";
import { useT } from "@ac/web/components/I18n";
import { isLocale, LOCALE_KEY, translate } from "@ac/web/lib/i18n";
import { setStoredValue, useStoredValue } from "@ac/web/lib/urlState";
import { codeComplete, groupKey, otpauthUri, qrPath } from "@ac/web/lib/twoFactor";

const DEMO_KEY = "JBSWY3DPEHPK3PXP"; // the fixed demo setup key (Figma 10g)
const DEMO_QR = qrPath(otpauthUri(DEMO_KEY, "demo"));

/** Preferences of the Phase 1A demo (browser state only; the language is kept in this browser and read by the shell);
 * API mode renders PreferencesView from the Core API. */
export function PreferencesDemo() {
  const toast = useToast();
  const t = useT();
  const role = useStoredRole();
  const stored = useStoredValue(LOCALE_KEY);
  const lang = isLocale(stored) ? stored : "en";
  const [loc, setLoc] = useState(true);
  const [mic, setMic] = useState(true);
  const [twoFa, setTwoFa] = useState(false);
  const [modal, setModal] = useState(false);
  const [code, setCode] = useState("");
  const [codes, setCodes] = useState<string[] | null>(null);
  return (
    <Page className="max-w-3xl">
      <Card title={t("Display language")} sub={t("Affects the screens, notifications and date/time display. Stored data stays UTC (NFR-08). Malay text is a draft under review; text not translated yet stays in English.")}>
        <Choice value={lang} onChange={(v) => { setStoredValue(LOCALE_KEY, v); toast(translate(v, "Language saved")); }} options={[{ id: "en", label: t("English (default)") }, { id: "ms", label: "Bahasa Melayu" }]} />
      </Card>
      <Card title={t("Display timezone")} sub={t("Booked times keep their meaning across timezone changes.")}>
        <Select defaultValue="kl" onChange={() => toast(t("Timezone saved"))}><option value="kl">Asia/Kuala_Lumpur (UTC+8)</option><option value="sg">Asia/Singapore (UTC+8)</option><option value="tk">Asia/Tokyo (UTC+9)</option></Select>
      </Card>
      <Card title={t("Currency")} sub={`${t("MYR — Malaysian Ringgit")} · ${t("read-only demo")}`}><Select disabled><option>{t("MYR — Malaysian Ringgit")}</option></Select></Card>
      <Card title={t("Security · Two-step verification")} sub={t("Ask for a 6-digit code from an authenticator app when signing in on a new device")} action={twoFa ? <span className="flex items-center gap-2"><Badge tone="ok">{t("On")}</Badge><Btn size="sm" onClick={() => { setTwoFa(false); toast(t("Two-step verification turned off"), "warn"); }}>{t("Turn off")}</Btn></span> : <span className="flex items-center gap-2"><Badge>{t("Off")}</Badge><Btn size="sm" variant="primary" onClick={() => { setModal(true); setCodes(null); setCode(""); }}>{t("Turn on")}</Btn></span>}>
        <p className="text-xs text-muted">{t("Demo only — demo sign-in is unchanged (FR-X08).")}</p>
      </Card>
      {role === "client" && (
        <Card title={t("Consent")}>
          <div className="flex flex-col gap-4">
            <div className="flex items-center justify-between gap-3"><div><div className="text-[13px] font-semibold">{t("Location, for voice room disambiguation")}</div><div className="text-xs text-muted">{t("Withdrawal discards pending voice requests only")}</div></div><Toggle on={loc} onChange={setLoc} label={t("Location consent")} /></div>
            <div className="flex items-center justify-between gap-3"><div><div className="text-[13px] font-semibold">{t("Microphone, for voice actions")}</div><div className="text-xs text-muted">{t("Text input always available as a fallback")}</div></div><Toggle on={mic} onChange={setMic} label={t("Microphone consent")} /></div>
            <Check label={t("Allow WhatsApp notifications")} checked={false} />
            <Check label={t("Email me the monthly energy report (1st of each month)")} checked={false} />
          </div>
        </Card>
      )}
      <Modal open={modal} onClose={() => setModal(false)} title={t("Turn on two-step verification")} footer={codes ? <Btn variant="primary" onClick={() => { setModal(false); setTwoFa(true); toast(t("Two-step verification is on")); }}>{t("I saved my codes")}</Btn> : <><Btn onClick={() => setModal(false)}>{t("Cancel")}</Btn><Btn variant="primary" disabled={!codeComplete(code)} onClick={() => setCodes(["7HQ2-K9PD", "3MZX-41RT", "Q8W2-6TNB", "L5CE-2PXK", "9VAD-7MJF", "R2KN-8QWE", "XT4P-1GHS", "B6UY-3ZLM"])}>{t("Verify & turn on")}</Btn></>}>
        {codes ? <><Banner tone="ok">{t("Two-step verification is ready. Save these {n} recovery codes — they are shown only once.", { n: codes.length })}</Banner><div className="grid grid-cols-2 gap-2 font-mono text-sm">{codes.map((c) => <span key={c} className="rounded-lg bg-surface2 px-3 py-1.5">{c}</span>)}</div></> : <div className="flex flex-col gap-3.5">
          <div className="flex flex-col gap-5 sm:flex-row">
            <div className="grid h-[150px] w-[150px] shrink-0 place-items-center rounded-lg border border-line bg-white p-2"><QrCode qr={DEMO_QR} label={t("QR code with the setup key for an authenticator app")} className="h-full w-full" /></div>
            <div className="flex min-w-0 flex-col gap-2.5 text-[13px]">
              <b>1&nbsp;&nbsp;{t("Scan with an authenticator app")}</b>
              <div className="text-xs text-muted">{t("or enter key")}&nbsp;&nbsp;<span className="font-mono">{groupKey(DEMO_KEY)}</span></div>
              <b>2&nbsp;&nbsp;{t("Enter the 6-digit code")}</b>
              <CodeInput label={t("6-digit code")} value={code} onChange={setCode} autoFocus />
              <b>3&nbsp;&nbsp;{t("Save your 8 recovery codes (shown after verifying)")}</b>
            </div>
          </div>
          <Banner>{t("Demo: no real authenticator is called — any 6 digits are accepted. Real sign-in stays out of scope (login is a demo role picker). Turning off later asks for a current code.")}</Banner>
        </div>}
      </Modal>
    </Page>
  );
}
