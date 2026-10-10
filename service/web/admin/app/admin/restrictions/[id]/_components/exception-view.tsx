"use client";

import Link from "next/link";
import { useState } from "react";
import { Badge, Banner, Btn, Card, Field, Input, Page, SummaryList, Textarea, Timeline } from "@ac/web/components/ui";
import { useI18n } from "@ac/web/components/I18n";
import { useAction } from "@ac/web/lib/useAction";
import { zonedInstant, zonedParts } from "@ac/web/lib/i18n";
import { actionOutcome, policyText, stateTone, stateWord, type ActionKind, type ApiRestriction, type UnitRow } from "@ac/web/lib/restrictions";
import { cancelRestriction, deferRestriction, exemptRestriction, overrideRestriction, reconcileUnits, retryUnits } from "../../actions";

type Live = {
  now: string; r: ApiRestriction; units: UnitRow[]; canWrite: boolean; canOverride: boolean; canAudit: boolean;
  /** formatted on the server in the display language and time zone (IR282) */
  period: string | null; intent: string; audit: { time: string; title: string; detail: string }[];
};
const ops: Record<ActionKind, string> = { defer: "restrictions.defer", exempt: "restrictions.exempt", cancel: "restrictions.cancel", override: "restrictions.override · restriction.override" };
const kinds: ActionKind[] = ["defer", "exempt", "cancel", "override"];

/** Grace periods, exceptions, cancellation and manual release (FR-A10) in API mode. Each action card states the
 * transition for the current state before saving (IR96 / IR35); every action needs a reason and the version read.
 * Texts in the display language; the grace or exception end is typed in the display time zone (IR301). */
export function ExceptionView({ live }: { live: Live }) {
  const i = useI18n(), { t } = i, zone = i.display.timeZone;
  const [pending, run] = useAction();
  const { r } = live;
  const now = new Date(live.now);
  const label: Record<ActionKind, string> = { defer: t("Grace period"), exempt: t("Exception"), cancel: t("Cancel"), override: t("Override release") };
  const button: Record<ActionKind, string> = { defer: t("Apply grace period"), exempt: t("Apply exception"), cancel: t("Cancel restriction"), override: t("Override release") };
  const recorded: Record<ActionKind, string> = { defer: t("Grace period recorded"), exempt: t("Exception recorded"), cancel: t("Cancellation recorded"), override: t("Override recorded") };
  const allowedFor = (k: ActionKind) => (k === "override" ? live.canOverride : live.canWrite);
  const first = kinds.find((k) => allowedFor(k) && actionOutcome(k, r.state).allowed) ?? "cancel";
  const [act, setAct] = useState<ActionKind>(first);
  const [until, setUntil] = useState(() => {
    const p = zonedParts(new Date(now.getTime() + 7 * 24 * 3600 * 1000).toISOString(), zone); // a week ahead in the display zone
    return `${p.date}T${p.time}`;
  });
  const [reason, setReason] = useState("");
  const [tried, setTried] = useState(false);
  const needsDate = act === "defer" || act === "exempt";
  const outcome = actionOutcome(act, r.state, t);
  const untilAt = until ? zonedInstant(until.slice(0, 10), until.slice(11, 16), zone) : "";
  const untilMs = untilAt ? Date.parse(untilAt) : NaN;
  const dateErr = needsDate && !(untilMs > now.getTime() && untilMs <= now.getTime() + 90 * 24 * 3600 * 1000) ? t("A future date within 90 days") : undefined;
  const reasonErr = reason.trim().length < 1 || reason.length > 1000 ? t("A reason is required (1–1000 characters)") : undefined;
  const isRelease = r.projection === "release";
  const save = () => {
    setTried(true);
    if (!outcome.allowed || reasonErr || (needsDate && dateErr)) return;
    const why = reason.trim();
    const fn = act === "defer" ? () => deferRestriction(r.id, r.version, untilAt, why)
      : act === "exempt" ? () => exemptRestriction(r.id, r.version, untilAt, why)
      : act === "cancel" ? () => cancelRestriction(r.id, r.version, why) : () => overrideRestriction(r.id, r.version, why);
    run(fn, recorded[act], () => { setReason(""); setTried(false); });
  };
  // IR96: override-only callers see reconcile / release retry only for an override release intent or unresolved recovery
  const followUp = live.canWrite || (live.canOverride && (r.releaseIntent?.source === "override" || r.recoveryCases.some((c) => c.state !== "resolved")));
  return (
    <Page>
      <div className="flex flex-wrap items-center gap-2 text-[13px] text-muted"><Link href={`/admin/restrictions?restrictionId=${r.id}`} className="font-semibold text-primary">{t("← Restrictions")}</Link>/ <b className="text-ink">{r.id.slice(0, 8)}</b> <Badge tone={stateTone(r.state)}>{stateWord(r.state, t)}</Badge></div>
      <div className="split">
        <div className="flex min-w-0 flex-col gap-4">
          <Card title={t("Choose an action")} sub={t("Version {v} · the result for the current state is shown before saving", { v: r.version })}>
            <div className="grid-fluid" style={{ ["--min" as string]: "220px" }}>{kinds.map((k) => {
              const o = actionOutcome(k, r.state, t);
              const permitted = allowedFor(k);
              return (
                <button key={k} disabled={!permitted || !o.allowed} onClick={() => setAct(k)} aria-pressed={act === k} className={`rounded-xl border p-3 text-left disabled:opacity-50 ${act === k ? "border-primary bg-primary-soft/60" : "border-line hover:bg-surface2"}`}>
                  <b className="text-[13px]">{label[k]}</b><div className="font-mono text-[10px] text-muted">{ops[k]}</div><div className="mt-1 text-xs text-muted">{permitted ? o.result : k === "override" ? t("Needs restriction.override") : t("Needs restriction.write")}</div>
                </button>
              );
            })}</div>
            {needsDate && <div className="mt-4 max-w-xs"><Field label={t("Until · future, at most 90 days")} error={tried ? dateErr : undefined} hint={t("Expiry never reapplies the restriction — conditions are checked again · times in {zone}", { zone })}><Input type="datetime-local" value={until} onChange={(e) => setUntil(e.target.value)} /></Field></div>}
            <div className="mt-3"><Field label={t("Reason · required")} error={tried ? reasonErr : undefined} hint={`${reason.length} / 1000`}><Textarea value={reason} maxLength={1000} onChange={(e) => setReason(e.target.value)} /></Field></div>
            <div className="mt-3 flex justify-end"><Btn variant={act === "override" ? "danger" : "primary"} disabled={pending || !outcome.allowed || !allowedFor(act)} onClick={save}>{button[act]}</Btn></div>
          </Card>
          <Card title={t("What happens")}><ul className="list-disc pl-5 text-[13px]"><li>{outcome.result}</li><li>{t("A release still needs per-unit evidence (released or not required)")}</li><li>{t("Cause invoices stay unpaid — no action here settles billing")}</li><li>{t("Before/after, expiry, reason and your name are recorded in the audit log")}</li></ul></Card>
          {followUp && live.units.some((u) => u.reconcile || u.retryRelease) && (
            <Card title={t("Units needing follow-up")}>{live.units.filter((u) => u.reconcile || u.retryRelease).map((u) => (
              <div key={u.unitId} className="flex items-center justify-between gap-2 border-t border-line py-1.5 text-[13px] first:border-0"><span>{u.name} <span className="text-xs text-muted">{t("apply {apply} · release {release}", { apply: u.apply, release: u.release })}{u.pending ? ` · ${u.pending}` : ""}</span></span><span className="flex gap-1">{u.reconcile && <Btn size="sm" disabled={pending} onClick={() => run(() => reconcileUnits(r.id, r.version, [u.unitId]), t("Reconciled"))}>{t("Reconcile")}</Btn>}{u.retryRelease && <Btn size="sm" disabled={pending} onClick={() => run(() => retryUnits(r.id, r.version, [u.unitId], "release", r.rulesVersion, "Retry release from the exception screen"), t("Release retried"))}>{t("Retry release")}</Btn>}</span></div>
            ))}</Card>
          )}
        </div>
        <div className="flex min-w-0 flex-col gap-4">
          <Card title={t("Summary")}>
            {isRelease && <Banner>{t("Release view (restriction.override only, IR03): billing fields are not shown.")}</Banner>}
            <SummaryList items={[
              [t("Policy"), policyText(r.policy, t)], [t("Units"), live.units.map((u) => `${u.name} · ${u.apply}/${u.release}`).join(", ") || "—"],
              ...(isRelease ? [] : [[t("Cause invoices"), `${r.causeInvoiceIds?.length ?? 0}`] as [string, string]]),
              [t("Grace / exception"), live.period ?? t("none")], [t("Release intent"), live.intent],
              [t("Your permissions"), [live.canWrite && "restriction.write", live.canOverride && "restriction.override", live.canAudit && "audit.read"].filter(Boolean).join(" · ") || t("read only")],
            ]} />
          </Card>
          {live.canAudit && <Card title={t("Audit")} sub="audit.read">{live.audit.length === 0 ? <p className="text-xs text-muted">{t("No audit entries yet.")}</p> : <Timeline items={live.audit} />}</Card>}
        </div>
      </div>
    </Page>
  );
}
