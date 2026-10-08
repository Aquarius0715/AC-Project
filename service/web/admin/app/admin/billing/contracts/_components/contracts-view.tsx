"use client";

import Link from "next/link";
import { useState, useTransition } from "react";
import { usePathname, useRouter } from "next/navigation";
import { Badge, Banner, Btn, Card, Check, EmptyState, Field, Input, ListRow, Page, Select, Tabs, useToast } from "@ac/web/components/ui";
import { actionMessage } from "@ac/web/lib/actionMessage";
import { dayInstant, draftErrors, planLabel, priceMinor, type ContractDraft, type ContractRow, type PlanType, type UnitOption } from "@ac/web/lib/contracts";
import { saveContract } from "../actions";

type Scope = { customerId?: string; unitId?: string; kind?: PlanType };
type Live = { scope: Scope; rows: ContractRow[]; selectedId: string | null; customers: { id: string; name: string }[]; units: UnitOption[] };

const demoCustomers = [{ id: "customer-a", name: "customer-a" }, { id: "customer-b", name: "customer-b" }];
const demoRow = (id: string, cust: string, planType: PlanType, unitIds: string[], priceMinor: number, start: string, end: string, eligible: boolean, restrictionIds: string[]): ContractRow => ({
  id, version: 1, planType, plan: planLabel[planType], customerId: cust, cust, unitIds, units: `${unitIds.length} unit${unitIds.length === 1 ? "" : "s"}`, priceMinor, currency: "MYR",
  price: `${(priceMinor / 100).toFixed(2)} MYR`, startAt: `${start}T00:00:00+08:00`, endAt: `${end}T00:00:00+08:00`, start, end, restrictionEligible: eligible, rulesVersion: eligible ? "demo-v1" : null,
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
 * fixture rows. */
export function ContractsView({ live }: { live?: Live }) {
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
  const errors = draftErrors(d);
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
      if (creating) toast("Contract created");
      else if (sel) {
        const minor = priceMinor(d.price);
        setDemoRows((l) => l.map((r) => (r.id === sel.id ? {
          ...r, planType: d.planType, plan: planLabel[d.planType], unitIds: d.unitIds, units: `${d.unitIds.length} unit${d.unitIds.length === 1 ? "" : "s"}`,
          priceMinor: minor, price: `${(minor / 100).toFixed(2)} ${d.currency}`, currency: d.currency, end: d.end, version: r.version + 1,
          restrictionEligible: eligible, rulesVersion: eligible ? d.rulesVersion.trim() : null,
        } : r)));
        toast(`Saved as version ${sel.version + 1}`);
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
      if (!r.ok) return toast(actionMessage(r), "crit");
      toast(creating || !sel ? "Contract created" : `Saved as version ${r.value.version}`);
      setCreating(false);
      nav({ contractId: r.value.id });
    });
  };

  return (
    <Page>
      {live && (
        <div className="flex flex-wrap items-end gap-3">
          <Field label="Customer"><Select value={live.scope.customerId ?? ""} onChange={(e) => nav({ customerId: e.target.value || null, unitId: null, contractId: null })}><option value="">All customers</option>{customers.map((c) => <option key={c.id} value={c.id}>{c.name}</option>)}</Select></Field>
          <Field label="Unit"><Select value={live.scope.unitId ?? ""} onChange={(e) => nav({ unitId: e.target.value || null, contractId: null })}><option value="">All units</option>{scopeUnits.map((u) => <option key={u.id} value={u.id}>{u.label}</option>)}</Select></Field>
        </div>
      )}
      <Tabs value={kind} onChange={(k) => (live ? nav({ kind: k === "all" ? null : k, contractId: null }) : setDemoKind(k))} tabs={(["all", ...plans] as const).map((k) => ({ id: k, label: k === "all" ? "All" : planLabel[k], count: k === "all" ? (live ? rows.length : demoRows.length) : live ? undefined : demoRows.filter((r) => r.planType === k).length }))} />
      <div className="split-rev">
        <Card title="Contracts" sub="id ↑" action={<Btn size="sm" variant="primary" onClick={() => setCreating(true)}>+ New</Btn>} className="self-start">
          {rows.length === 0 ? <EmptyState title="No contracts">No contract matches this scope.</EmptyState> : <div className="flex flex-col gap-2">{rows.map((c) => <ListRow key={c.id} selected={sel?.id === c.id} onClick={() => select(c.id)}><div className="min-w-0 flex-1"><div className="flex items-center justify-between gap-2"><b className="truncate text-[13px]">{live ? c.id.slice(0, 8) : c.id}</b><b className="text-[13px]">{c.price}</b></div><div className="flex items-center gap-1.5 text-[11px] text-muted"><Badge tone={c.planType === "rto" ? "primary" : "muted"}>{c.plan}</Badge>{c.cust} · {c.units}</div><div className="text-[11px] text-muted">{c.start} → {c.end}{c.restrictionEligible ? ` · restriction eligible (${c.rulesVersion})` : ""}</div></div></ListRow>)}</div>}
          <p className="mt-3 text-[11px] text-muted">Units without a contract can still be monitored and maintained.</p>
        </Card>
        {creating || sel ? (
          <Card title={creating || !sel ? "New contract" : live ? `Contract ${sel.id.slice(0, 8)}` : sel.id} sub={creating || !sel ? "Saving creates version 1" : `Version ${sel.version} · Editing → v${sel.version + 1}`} action={<Badge tone="primary">{planLabel[d.planType]}</Badge>}>
            <div className="flex flex-col gap-5">
              <section className="grid-fluid" style={{ ["--min" as string]: "180px" }}>
                <Field label="Customer" error={tried ? errors.customerId : undefined} hint={!creating ? "Fixed after creation" : undefined}><Select value={d.customerId} disabled={!creating || blocked} onChange={(e) => set({ customerId: e.target.value, unitIds: [] })}><option value="">Select…</option>{customers.map((c) => <option key={c.id} value={c.id}>{c.name}</option>)}</Select></Field>
                <Field label="Plan type"><Select value={d.planType} disabled={blocked} onChange={(e) => set({ planType: e.target.value as PlanType })}>{plans.map((p) => <option key={p} value={p}>{planLabel[p]}</option>)}</Select></Field>
              </section>
              <section>
                <h3 className="mb-1 text-[13px] font-bold">Units</h3>
                <p className="mb-2 text-xs text-muted">Only this customer’s active units can be included. A unit on another contract is labelled, not hidden.</p>
                {!d.customerId ? <p className="text-xs text-muted">Choose a customer first.</p> : customerUnits.length === 0 ? <p className="text-xs text-muted">This customer has no active units.</p> : (
                  <div className="flex flex-col gap-1.5">{customerUnits.map((u) => {
                    const others = u.otherContracts.filter((k) => k !== sel?.id || creating);
                    return <Check key={u.id} checked={d.unitIds.includes(u.id)} disabled={blocked} onChange={(on) => set({ unitIds: on ? [...d.unitIds, u.id] : d.unitIds.filter((x) => x !== u.id) })} label={<span>{u.label} <span className="text-xs text-muted">{u.where}{others.length ? ` · also on ${others.map((k) => (live ? k.slice(0, 8) : k)).join(", ")}` : ""}</span></span>} />;
                  })}</div>
                )}
                {tried && errors.unitIds && <p className="mt-1 text-xs text-crit">{errors.unitIds}</p>}
              </section>
              <section className="grid-fluid" style={{ ["--min" as string]: "150px" }}>
                <Field label="Start" error={tried ? errors.start : undefined}><Input type="date" value={d.start} disabled={blocked} onChange={(e) => set({ start: e.target.value })} /></Field>
                <Field label="End" error={tried ? errors.end : undefined}><Input type="date" value={d.end} disabled={blocked} onChange={(e) => set({ end: e.target.value })} /></Field>
                <Field label="Price" error={tried ? errors.price : undefined}><Input type="number" min="0" step="0.01" value={d.price} disabled={blocked} onChange={(e) => set({ price: e.target.value })} /></Field>
                <Field label="Currency"><Select value={d.currency} disabled={blocked} onChange={(e) => set({ currency: e.target.value as "MYR" | "USD" })}><option>MYR</option><option>USD</option></Select></Field>
              </section>
              <section>
                <h3 className="mb-1 text-[13px] font-bold">Restriction eligibility</h3>
                <p className="mb-2 text-xs text-muted">{d.planType === "rto" ? "Only RTO contracts can be restriction eligible, and only when set explicitly here." : `Not available for ${planLabel[d.planType]} contracts — only RTO contracts can be restriction eligible.`}</p>
                <Check label="Allow cooling restrictions for unpaid invoices" checked={d.restrictionEligible} disabled={blocked || d.planType !== "rto"} onChange={(on) => set({ restrictionEligible: on })} />
                <div className="mt-2 max-w-xs"><Field label="Rules version · required when eligible" hint="Demo rules — not approved production rules" error={tried ? errors.rulesVersion : undefined}><Input value={d.rulesVersion} maxLength={64} disabled={blocked || !d.restrictionEligible} onChange={(e) => set({ rulesVersion: e.target.value })} /></Field></div>
              </section>
              {blocked && sel && <Banner tone="warn" action={sel.restrictionIds[0] && <Link className="text-xs font-semibold text-primary" href={live ? `/admin/restrictions/${sel.restrictionIds[0]}` : "/admin/restrictions"}>Open restriction →</Link>}>Save is disabled (SR19): {sel.restrictionIds.length > 0 ? `${sel.restrictionIds.length} active restriction(s) on this contract` : "an unresolved recovery case exists"}. Cancel the notice or release the restriction first — contract changes never cancel or release restrictions.</Banner>}
              <section className="rounded-xl bg-surface2 p-3 text-xs"><b className="text-[13px]">What saving does</b><ul className="mt-1 list-disc pl-5">{creating || !sel ? <><li>Creates version 1 of a new contract for this customer</li><li>New invoices can be issued against it</li></> : <><li>Creates version {sel.version + 1} — version {sel.version} stays in history</li><li>Invoices already issued keep version {sel.version} and their original amounts</li><li>New invoices will use version {sel.version + 1} ({/^\d+(\.\d{1,2})?$/.test(d.price.trim()) ? (priceMinor(d.price) / 100).toFixed(2) : "—"} {d.currency})</li></>}</ul></section>
              <div className="flex justify-end gap-2">{creating && <Btn onClick={() => setCreating(false)}>Cancel</Btn>}{blocked && <Badge tone="muted">Read-only while a restriction is active</Badge>}<Btn variant="primary" disabled={pending || blocked} onClick={save}>{creating || !sel ? "Create contract" : `Save as v${sel.version + 1}`}</Btn></div>
            </div>
          </Card>
        ) : <Card title="Contract"><EmptyState title="Nothing selected">Choose a contract or create a new one.</EmptyState></Card>}
      </div>
    </Page>
  );
}
