"use client";

import { useState } from "react";
import { Banner, Btn, Card, DemoBadge, Page, SummaryList, useToast } from "@/components/ui";
import { jobActions } from "@/lib/jobs";
import { clientUserActions } from "@/lib/clientUsers";

export default function Demo() {
  const toast = useToast();
  const [sim, setSim] = useState(false);
  return (
    <Page className="max-w-3xl">
      <Banner tone="warn" action={<DemoBadge />}>Visually separated from business screens. Nothing here reaches real devices, payments, notifications, or IoT connections (FR-X05).</Banner>
      <Card title="Demo clock" sub="Real-time speed · advancing jumps do not consume session lifetime"><div className="flex flex-wrap items-center gap-3"><span className="font-mono text-sm">2026-09-14 09:41 UTC</span><Btn size="sm" onClick={() => toast("Clock advanced +1 min (demo)")}>+1 min</Btn><Btn size="sm" onClick={() => toast("Clock advanced +1 hour (demo)")}>+1 hour</Btn></div></Card>
      <Card title="Scenario" sub="fixture-contract.json demoSeed is the source of truth"><div className="flex flex-wrap items-center gap-3"><span className="font-mono text-sm">simulator = {String(sim)} (baseline seed)</span><Btn size="sm" onClick={() => setSim((s) => !s)}>Toggle simulator</Btn></div></Card>
      <Card title="Reset" sub="Returns clock, subscriptions, image URLs, caches, and business data to their initial seed"><Btn variant="danger" onClick={() => { setSim(false); jobActions.reset(); clientUserActions.reset(); toast("Demo data reset to seed"); }}>Reset demo data</Btn></Card>
      <Card title="Trigger failures">
        <SummaryList items={[["Device offline", "communication_lost on unit-online-rto"], ["Delay transport", "3000ms on commands.create"], ["Expire session", "session_expired for current Membership"]]} />
        <div className="mt-3 flex flex-wrap gap-2"><Btn size="sm" onClick={() => toast("Device offline triggered", "warn")}>Device offline</Btn><Btn size="sm" onClick={() => toast("Transport delay 3000 ms set", "warn")}>Delay transport</Btn><Btn size="sm" onClick={() => toast("Session expired (demo)", "crit")}>Expire session</Btn></div>
      </Card>
    </Page>
  );
}
