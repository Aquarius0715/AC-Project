"use client";

import { useState } from "react";
import { Badge, Banner, Btn, Card, Check, Choice, Input, Modal, Page, Select, Toggle, useToast } from "@ac/web/components/ui";
import { useStoredRole } from "@ac/web/components/AppShell";

/** Preferences of the Phase 1A demo (browser state only); API mode renders PreferencesView from the Core API. */
export function PreferencesDemo() {
  const toast = useToast();
  const role = useStoredRole();
  const [lang, setLang] = useState<"en" | "ms">("en");
  const [loc, setLoc] = useState(true);
  const [mic, setMic] = useState(true);
  const [twoFa, setTwoFa] = useState(false);
  const [modal, setModal] = useState(false);
  const [code, setCode] = useState("");
  const [codes, setCodes] = useState<string[] | null>(null);
  return (
    <Page className="max-w-3xl">
      <Card title="Display language" sub="Affects key screens, notifications, and date/time display only. Stored data stays UTC (NFR-08).">
        <Choice value={lang} onChange={(v) => { setLang(v); toast("Language saved"); }} options={[{ id: "en", label: "English (default)" }, { id: "ms", label: "Bahasa Melayu" }]} />
      </Card>
      <Card title="Display timezone" sub="Booked times keep their meaning across timezone changes.">
        <Select defaultValue="kl" onChange={() => toast("Timezone saved")}><option value="kl">Asia/Kuala_Lumpur (UTC+8)</option><option value="sg">Asia/Singapore (UTC+8)</option><option value="tk">Asia/Tokyo (UTC+9)</option></Select>
      </Card>
      <Card title="Currency" sub="MYR — Malaysian Ringgit · read-only demo"><Select disabled><option>MYR — Malaysian Ringgit</option></Select></Card>
      <Card title="Security · Two-step verification" sub="Ask for a 6-digit code from an authenticator app when signing in on a new device" action={twoFa ? <span className="flex items-center gap-2"><Badge tone="ok">On</Badge><Btn size="sm" onClick={() => { setTwoFa(false); toast("Two-step verification turned off", "warn"); }}>Turn off</Btn></span> : <span className="flex items-center gap-2"><Badge>Off</Badge><Btn size="sm" variant="primary" onClick={() => { setModal(true); setCodes(null); setCode(""); }}>Turn on</Btn></span>}>
        <p className="text-xs text-muted">Demo only — demo sign-in is unchanged (FR-X08).</p>
      </Card>
      {role === "client" && (
        <Card title="Consent">
          <div className="flex flex-col gap-4">
            <div className="flex items-center justify-between gap-3"><div><div className="text-[13px] font-semibold">Location, for voice room disambiguation</div><div className="text-xs text-muted">Withdrawal discards pending voice requests only</div></div><Toggle on={loc} onChange={setLoc} label="Location consent" /></div>
            <div className="flex items-center justify-between gap-3"><div><div className="text-[13px] font-semibold">Microphone, for voice actions</div><div className="text-xs text-muted">Text input always available as a fallback</div></div><Toggle on={mic} onChange={setMic} label="Microphone consent" /></div>
            <Check label="Allow WhatsApp notifications" checked={false} />
            <Check label="Email me the monthly energy report (1st of each month)" checked={false} />
          </div>
        </Card>
      )}
      <Modal open={modal} onClose={() => setModal(false)} title="Turn on two-step verification" footer={codes ? <Btn variant="primary" onClick={() => { setModal(false); setTwoFa(true); toast("Two-step verification is on"); }}>I saved my codes</Btn> : <><Btn onClick={() => setModal(false)}>Cancel</Btn><Btn variant="primary" disabled={!/^\d{6}$/.test(code)} onClick={() => setCodes(["7HQ2-K9PD", "3MZX-41RT", "Q8W2-6TNB", "L5CE-2PXK", "9VAD-7MJF", "R2KN-8QWE", "XT4P-1GHS", "B6UY-3ZLM"])}>Verify & turn on</Btn></>}>
        {codes ? <><Banner tone="ok">Two-step verification is ready. Save these 8 recovery codes — they are shown only once.</Banner><div className="grid grid-cols-2 gap-2 font-mono text-sm">{codes.map((c) => <span key={c} className="rounded-lg bg-surface2 px-3 py-1.5">{c}</span>)}</div></> : <>
          <ol className="flex flex-col gap-3 text-[13px]">
            <li><b>1 · Scan with an authenticator app</b><div className="mt-2 grid h-28 w-28 place-items-center rounded-lg border border-line text-xs text-muted">[ QR ]</div><div className="mt-1 text-xs text-muted">or enter key <span className="font-mono">JBSW Y3DP EHPK 3PXP</span></div></li>
            <li><b>2 · Enter the 6-digit code</b><Input inputMode="numeric" maxLength={6} value={code} onChange={(e) => setCode(e.target.value.replace(/\D/g, ""))} placeholder="000000" className="mt-1 max-w-[160px] font-mono tracking-[0.3em]" /></li>
            <li><b>3 · Save your 8 recovery codes</b> <span className="text-xs text-muted">(shown after verifying)</span></li>
          </ol>
          <p className="text-[11px] text-muted">Demo: no real authenticator is called — any 6 digits are accepted.</p>
        </>}
      </Modal>
    </Page>
  );
}
