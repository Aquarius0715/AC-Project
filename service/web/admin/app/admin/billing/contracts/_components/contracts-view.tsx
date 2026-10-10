"use client";

import Link from "next/link";
import { useState, useTransition } from "react";
import { usePathname, useRouter } from "next/navigation";
import { Badge, Banner, Btn, Card, Check, EmptyState, Field, Input, ListRow, Page, Select, Tabs, useToast } from "@ac/web/components/ui";
import { useT } from "@ac/web/components/I18n";
import { actionMessage } from "@ac/web/lib/actionMessage";
import { KL } from "@ac/web/lib/billing";
import { dayInstant, draftErrors, planLabel, planWord, priceMinor, type ContractDraft, type ContractRow, type PlanType, type UnitOption } from "@ac/web/lib/contracts";
import { saveContract } from "../actions";

type Scope = { customerId?: string; unitId?: string; kind?: PlanType };
type Live = { scope: Scope; rows: ContractRow[]; counts: Record<PlanType | "all", number>; selectedId: string | null; customers: { id: string; name: string }[]; units: UnitOption[] };

const demoCustomers = [{ id: "customer-a", name: "customer-a" }, { id: "customer-b", name: "customer-b" }];
const demoRow = (id: string, cust: string, planType: PlanType, unitIds: string[], priceMinor: number, start: string, end: string, eligible: boolean, restrictionIds: string[]): ContractRow => ({
  id, version: 1, planType, plan: planLabel[planType], customerId: cust, cust, unitIds, units: `${unitIds.length} unit${unitIds.length === 1 ? "" : "s"}`, priceMinor, currency: "MYR",
  price: `${(priceMinor / 100).toFixed(2)} MYR`, startAt: `${start}T00:00:00+08:00`, endAt: `${end}T00:00:00+08:00`, start, end, period: `${start} → ${end}`, restrictionEligible: eligible, rulesVersion: eligible ? "demo-v1" : null,
  restrictionIds, recovery: false,
});
const seedRows = [
  demoRow("contract-general-a", "customer-a", "general", ["unit-non-rto"], 5000, "2026-01-01", "2027-01-01", false, []),
  demoRow("contract-rto-a", "customer-a", "rto", ["unit-online-rto", "unit-offline-rto", "unit-limited"], 12000, "2026-01-01", "2027-01-01", true, ["restriction-limited-a"]),
  demoRow("contract-rto-b", "customer-b", "rto", ["unit-other-customer"], 8500, "2026-03-01", "2027-03-01", true, []),
];
const seedUnits: UnitOption[] = [
  { id: "unit-online-rto", customerId: "customer-a", label: "Bedroom AC · unit-online-rto", where: "Home A › 1F › Bedroom", otherContracts: ["contract-rto-a"] },
  { id: "unit-offline-rto", customerId: "customer-a", label: "unit-offline-rto", where: "Home A › 2F › Study", otherContracts: ["contract-rto-a"] },
  { id: "unit-limited", customerId: "customer-a", label: "unit-limited", where: "Home A › 1F › Living room", otherContracts: ["contract-rto-a"] },
  { id: "unit-non-rto", customerId: "customer-a", label: "unit-non-rto", where: "Home B", otherContracts: ["contract-general-a"] },
  { id: "unit-other-customer", customerId: "customer-b", label: "unit-other-customer", where: "Home B", otherContracts: ["contract-rto-b"] },
];
const plans: PlanType[] = ["rto", "general", "energy", "environment"];
const draftOf = (r: ContractRow): ContractDraft => ({
  customerId: r.customerId, unitIds: r.unitIds, planType: r.planType, start: r.start, end: r.end, price: (r.priceMinor / 100).toFixed(2), currency: r.currency,
  restrictionEligible: r.restrictionEligible, rulesVersion: r.rulesVersion ?? "demo-v1",
});
const blankDraft = (customerId = ""): ContractDraft => ({ customerId, unitIds: [], planType: "general", start: "", end: "", price: "", currency: "MYR", restrictionEligible: false, rulesVersion: "demo-v1" });

/** Contract plans (FR-A07). `live` comes from the Server Component in API mode: the scope and the selected contract
 * live in the URL, saving is a Server Action that writes a new version and re-renders the route; the demo keeps the
 * fixture rows. Texts in the display language; contract dates are Kuala Lumpur days, typed as such (IR300). */
export function ContractsView({ live }: { live?: Live }) {
  const t = useT();
  const toast = useToast();
  const router = useRouter();
  const pathname = usePathname();
  const [pending, start] = useTransition();
  const nav = (patch: Record<string, string | null>) => {
    const q = new URLSearchParams(window.location.search);
    for (const [k, v] of Object.entries(patch)) {
      if (v) q.set(k, v);
      else q.delete(k);
    }
    router.replace(q.size ? `${pathname}?${q}` : pathname, { scroll: false });
  };
  const [demoRows, setDemoRows] = useState(seedRows);
  const [demoKind, setDemoKind] = useState<"all" | PlanType>("all");
  const [demoSel, setDemoSel] = useState(seedRows[1].id);
  const rows = live ? live.rows : demoRows.filter((r) => demoKind === "all" || r.planType === demoKind);
  const kind: "all" | PlanType = live ? live.scope.kind ?? "all" : demoKind;
  const customers = live ? live.customers : demoCustomers;
  const units = live ? live.units : seedUnits;
  const [creating, setCreating] = useState(false);
  const sel = creating ? null : rows.find((r) => r.id === (live ? live.selectedId : demoSel)) ?? rows[0] ?? null;

  // the form restarts from each selected contract version (adjust state during render, not in an effect)
  const key = creating ? "new" : sel ? `${sel.id}:${sel.version}` : "none";
  const [source, setSource] = useState(key);
  const [d, setD] = useState<ContractDraft>(sel ? draftOf(sel) : blankDraft());
  const [tried, setTried] = useState(false);
  if (source !== key) {
    setSource(key);
    setD(creating ? blankDraft(live?.scope.customerId) : sel ? draftOf(sel) : blankDraft());
    setTried(false);
  }
  const errors = draftErrors(d, t);
  const blocked = !creating && sel ? sel.restrictionIds.length > 0 || sel.recovery : false;
  const customerUnits = units.filter((u) => u.customerId === d.customerId);
  const scopeUnits = live?.scope.customerId ? units.filter((u) => u.customerId === live.scope.customerId) : units;
  const set = (patch: Partial<ContractDraft>) => setD((x) => {
    const next = { ...x, ...patch };
    if (next.planType !== "rto") next.restrictionEligible = false; // eligibility only for rto (DD-A07)
    return next;
  });
  const select = (id: string) => {
    setCreating(false);
    if (live) nav({ contractId: id });
    else setDemoSel(id);
  };

  const save = () => {
    setTried(true);
    if (Object.keys(errors).length > 0 || blocked) return;
    const eligible = d.planType === "rto" && d.restrictionEligible;
    if (!live) {
      if (creating) toast(t("Contract created"));
      else if (sel) {
        const minor = priceMinor(d.price);
        setDemoRows((l) => l.map((r) => (r.id === sel.id ? {
          ...r, planType: d.planType, plan: planLabel[d.planType], unitIds: d.unitIds, units: `${d.unitIds.length} unit${d.unitIds.length === 1 ? "" : "s"}`,
          priceMinor: minor, price: `${(minor / 100).toFixed(2)} ${d.currency}`, currency: d.currency, end: d.end, period: `${r.start} → ${d.end}`, version: r.version + 1,
          restrictionEligible: eligible, rulesVersion: eligible ? d.rulesVersion.trim() : null,
        } : r)));
        toast(t("Saved as version {v}", { v: sel.version + 1 }));
      }
      setCreating(false);
      return;
    }
    start(async () => {
      const r = await saveContract({
        ...(sel && !creating ? { id: sel.id } : {}), customerId: d.customerId, unitIds: d.unitIds, planType: d.planType,
        startAt: dayInstant(d.start, sel?.startAt), endAt: dayInstant(d.end, sel?.endAt), priceMinor: priceMinor(d.price), currency: d.currency,
        restrictionEligible: eligible, rulesVersion: eligible ? d.rulesVersion.trim() : null,
      }, sel?.version);
      if (!r.ok) return toast(actionMessage(r, t), "crit");
      toast(creating || !sel ? t("Contract created") : t("Saved as version {v}", { v: r.value.version }));
      setCreating(false);
      nav({ contractId: r.value.id });
    });
  };

  return (
    <Page>
      {live && (
        <div className="flex flex-wrap items-end gap-3">
          <span className="pb-2 text-xs font-semibold text-muted">{t("Scope")}</span>
          <Field label={t("Customer")}><Select value={live.scope.customerId ?? ""} onChange={(e) => nav({ customerId: e.target.value || null, unitId: null, contractId: null })}><option value="">{t("All customers")}</option>{customers.map((c) => <option key={c.id} value={c.id}>{c.name}</option>)}</Select></Field>
          <Field label={t("Unit")}><Select value={live.scope.unitId ?? ""} onChange={(e) => nav({ unitId: e.target.value || null, contractId: null })}><option value="">{t("All units")}</option>{scopeUnits.map((u) => <option key={u.id} value={u.id}>{u.label}</option>)}</Select></Field>
          <span className="ml-auto pb-2 text-xs text-muted">{t(live.counts.all === 1 ? "1 contract in scope" : "{n} contracts in scope", { n: live.counts.all })}</span>
        </div>
      )}
      <Tabs value={kind} onChange={(k) => (live ? nav({ kind: k === "all" ? null : k, contractId: null }) : setDemoKind(k))} tabs={(["all", ...plans] as const).map((k) => ({ id: k, label: k === "all" ? t("All") : planWord(k, t), count: live ? live.counts[k] : k === "all" ? demoRows.length : demoRows.filter((r) => r.planType === k).length }))} />
      <div className="split-rev">
        <Card title={t("Contracts")} sub="id ↑" action={<Btn size="sm" variant="primary" onClick={() => setCreating(true)}>{t("+ New")}</Btn>} className="self-start">
          {rows.length === 0 ? <EmptyState title={t("No contracts")}>{t("No contract matches this scope.")}</EmptyState> : <div className="flex flex-col gap-2">{rows.map((c) => <ListRow key={c.id} selected={sel?.id === c.id} onClick={() => select(c.id)}><div className="min-w-0 flex-1"><div className="flex items-center justify-between gap-2"><b className="truncate text-[13px]">{live ? c.id.slice(0, 8) : c.id}</b><b className="text-[13px]">{c.price}</b></div><div className="flex items-center gap-1.5 text-[11px] text-muted"><Badge tone={c.planType === "rto" ? "primary" : "muted"}>{planWord(c.planType, t)}</Badge>{c.cust} · {c.units}</div><div className="text-[11px] text-muted">{c.period}</div><div className="mt-1">{c.restrictionEligible ? <Badge tone="ok">{t("Eligible · {rules}", { rules: c.rulesVersion ?? "—" })}{c.restrictionIds.length > 0 ? ` · ${t("restriction active")}` : ""}</Badge> : <Badge tone="muted">{t("Not restriction eligible")}</Badge>}</div></div></ListRow>)}</div>}
          <p className="mt-3 text-[11px] text-muted">{t("Units without a contract can still be monitored and maintained.")}</p>
        </Card>
        {creating || sel ? (
          <Card title={creating || !sel ? t("New contract") : live ? t("Contract {id}", { id: sel.id.slice(0, 8) }) : sel.id} sub={creating || !sel ? t("Saving creates version 1") : `${sel.cust} · ${t("Version {v} · Editing → v{next}", { v: sel.version, next: sel.version + 1 })}${sel.restrictionIds.length ? ` · ${t(sel.restrictionIds.length === 1 ? "1 active restriction" : "{n} active restrictions", { n: sel.restrictionIds.length })}` : ""}`} action={<Badge tone="primary">{planWord(d.planType, t)}</Badge>}>
            <div className="flex flex-col gap-5">
              <section>
                <h3 className="mb-1 text-[13px] font-bold">{t("Customer & units")}</h3>
                <p className="mb-2 text-xs text-muted">{t("Only this customer’s active units can be included. A unit on another contract is labelled, not hidden.")}</p>
                <div className="mb-3 max-w-sm"><Field label={t("Customer")} error={tried ? errors.customerId : undefined} hint={!creating ? t("Fixed after creation — the units below must belong to this customer") : t("The units below must belong to this customer")}><Select value={d.customerId} disabled={!creating || blocked} onChange={(e) => set({ customerId: e.target.value, unitIds: [] })}><option value="">{t("Select…")}</option>{customers.map((c) => <option key={c.id} value={c.id}>{c.name}</option>)}</Select></Field></div>
                {!d.customerId ? <p className="text-xs text-muted">{t("Choose a customer first.")}</p> : customerUnits.length === 0 ? <p className="text-xs text-muted">{t("This customer has no active units.")}</p> : (
                  <div className="flex flex-col gap-1.5">{customerUnits.map((u) => {
                    const others = u.otherContracts.filter((k) => k !== sel?.id || creating);
                    return <Check key={u.id} checked={d.unitIds.includes(u.id)} disabled={blocked} onChange={(on) => set({ unitIds: on ? [...d.unitIds, u.id] : d.unitIds.filter((x) => x !== u.id) })} label={<span>{u.label} <span className="text-xs text-muted">{u.where}{others.length ? ` · ${t("also on {list}", { list: others.map((k) => (live ? k.slice(0, 8) : k)).join(", ") })}` : ""}</span></span>} />;
                  })}</div>
                )}
                {tried && errors.unitIds && <p className="mt-1 text-xs text-crit">{errors.unitIds}</p>}
              </section>
              <section>
                <h3 className="mb-2 text-[13px] font-bold">{t("Plan")}</h3>
                <div className="max-w-sm"><Field label={t("Plan type")}><Select value={d.planType} disabled={blocked} onChange={(e) => set({ planType: e.target.value as PlanType })}>{plans.map((p) => <option key={p} value={p}>{p === "rto" ? t("RTO — Rent to Own") : p === "general" ? t("General — maintenance") : planWord(p, t)}</option>)}</Select></Field></div>
              </section>
              <section className="grid-fluid" style={{ ["--min" as string]: "150px" }}>
                <Field label={t("Start")} error={tried ? errors.start : undefined} hint={t("Dates are Kuala Lumpur days ({zone}).", { zone: KL })}><Input type="date" value={d.start} disabled={blocked} onChange={(e) => set({ start: e.target.value })} /></Field>
                <Field label={t("End")} error={tried ? errors.end : undefined} hint={t("Must be after the start")}><Input type="date" value={d.end} disabled={blocked} onChange={(e) => set({ end: e.target.value })} /></Field>
                <Field label={t("Price")} error={tried ? errors.price : undefined} hint="≥ 0"><Input type="number" min="0" step="0.01" value={d.price} disabled={blocked} onChange={(e) => set({ price: e.target.value })} /></Field>
                <Field label={t("Currency")}><Select value={d.currency} disabled={blocked} onChange={(e) => set({ currency: e.target.value as "MYR" | "USD" })}><option>MYR</option><option>USD</option></Select></Field>
              </section>
              <section>
                <h3 className="mb-1 text-[13px] font-bold">{t("Restriction eligibility")}</h3>
                <p className="mb-2 text-xs text-muted">{d.planType === "rto" ? t("Only RTO contracts can be restriction eligible, and only when set explicitly here.") : t("Not available for {plan} contracts — only RTO contracts can be restriction eligible.", { plan: planWord(d.planType, t) })}</p>
                <Check label={<span>{t("Allow cooling restrictions for unpaid invoices")}<span className="block text-[11px] text-muted">{t("Advance notice → restriction → release after payment (Restrictions screen)")}</span></span>} checked={d.restrictionEligible} disabled={blocked || d.planType !== "rto"} onChange={(on) => set({ restrictionEligible: on })} />
                <div className="mt-2 max-w-xs"><Field label={t("Rules version · required when eligible")} hint={t("Demo rules — not approved production rules")} error={tried ? errors.rulesVersion : undefined}><Input value={d.rulesVersion} maxLength={64} disabled={blocked || !d.restrictionEligible} onChange={(e) => set({ rulesVersion: e.target.value })} /></Field></div>
              </section>
              {blocked && sel && <Banner tone="warn" action={sel.restrictionIds[0] && <Link className="text-xs font-semibold text-primary" href={live ? `/admin/restrictions?restrictionId=${sel.restrictionIds[0]}` : "/admin/restrictions"}>{t("Open restriction →")}</Link>}>{t("Save is disabled (SR19): {reason}. Cancel the notice or release the restriction first — contract changes never cancel or release restrictions.", { reason: sel.restrictionIds.length > 0 ? t(sel.restrictionIds.length === 1 ? "1 active restriction on this contract" : "{n} active restrictions on this contract", { n: sel.restrictionIds.length }) : t("an unresolved recovery case exists") })}</Banner>}
              <section className="rounded-xl bg-surface2 p-3 text-xs"><b className="text-[13px]">{t("What saving does")}</b><ul className="mt-1 list-disc pl-5">{creating || !sel ? <><li>{t("Creates version 1 of a new contract for this customer")}</li><li>{t("New invoices can be issued against it")}</li></> : <><li>{t("Creates version {next} — version {v} stays in history", { next: sel.version + 1, v: sel.version })}</li><li>{t("Invoices already issued keep version {v} and their original amounts", { v: sel.version })}</li><li>{t("New invoices will use version {next} ({amount})", { next: sel.version + 1, amount: `${/^\d+(\.\d{1,2})?$/.test(d.price.trim()) ? (priceMinor(d.price) / 100).toFixed(2) : "—"} ${d.currency}` })}</li></>}</ul></section>
              <div className="flex justify-end gap-2">{creating && <Btn onClick={() => setCreating(false)}>{t("Cancel")}</Btn>}{blocked && <Badge tone="muted">{t("Read-only while a restriction is active")}</Badge>}<Btn variant="primary" disabled={pending || blocked} onClick={save}>{creating || !sel ? t("Create contract") : t("Save as version {n}", { n: sel.version + 1 })}</Btn></div>
            </div>
          </Card>
        ) : <Card title={t("Contract")}><EmptyState title={t("Nothing selected")}>{t("Choose a contract or create a new one.")}</EmptyState></Card>}
      </div>
    </Page>
  );
}
