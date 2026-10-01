"use client";

import { useState } from "react";
import { Card, Check, Choice, Page, Select, Toggle, useToast } from "@/components/ui";

export default function Preferences() {
  const toast = useToast();
  const [lang, setLang] = useState<"en" | "ms">("en");
  const [loc, setLoc] = useState(true);
  const [mic, setMic] = useState(true);
  return (
    <Page className="max-w-3xl">
      <Card title="Display language" sub="Affects key screens, notifications, and date/time display only. Stored data stays UTC (NFR-08).">
        <Choice value={lang} onChange={(v) => { setLang(v); toast("Language saved"); }} options={[{ id: "en", label: "English (default)" }, { id: "ms", label: "Bahasa Melayu" }]} />
      </Card>
      <Card title="Display timezone" sub="Booked times keep their meaning across timezone changes.">
        <Select defaultValue="kl" onChange={() => toast("Timezone saved")}><option value="kl">Asia/Kuala_Lumpur (UTC+8)</option><option value="sg">Asia/Singapore (UTC+8)</option><option value="tk">Asia/Tokyo (UTC+9)</option></Select>
      </Card>
      <Card title="Currency" sub="MYR — Malaysian Ringgit · read-only demo"><Select disabled><option>MYR — Malaysian Ringgit</option></Select></Card>
      <Card title="Consent">
        <div className="flex flex-col gap-4">
          <div className="flex items-center justify-between gap-3"><div><div className="text-[13px] font-semibold">Location, for voice room disambiguation</div><div className="text-xs text-muted">Withdrawal discards pending voice requests only</div></div><Toggle on={loc} onChange={setLoc} label="Location consent" /></div>
          <div className="flex items-center justify-between gap-3"><div><div className="text-[13px] font-semibold">Microphone, for voice actions</div><div className="text-xs text-muted">Text input always available as a fallback</div></div><Toggle on={mic} onChange={setMic} label="Microphone consent" /></div>
          <Check label="Allow WhatsApp notifications" checked={false} />
        </div>
      </Card>
    </Page>
  );
}
