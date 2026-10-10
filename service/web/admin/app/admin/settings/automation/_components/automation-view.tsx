"use client";

import { useState } from "react";
import { Badge, Banner, Btn, Card, Check, Choice, DataTable, DemoBadge, EmptyState, Field, Input, ListRow, Page, Select } from "@ac/web/components/ui";
import { useT } from "@ac/web/components/I18n";
import { useAction } from "@ac/web/lib/useAction";
import { useUrlPatch } from "@ac/web/lib/useUrlPatch";
import {
  actionText, autoDraft, autoErrors, autoInput, conditionText, decisionWord, disabledReasonWord, evaluationInput, factOf, reasonWord, subjectText,
  type ApiAutoPolicy, type AutoCondition, type AutoDraft, type Compare, type FactRow, type PolicyGroup,
} from "@ac/web/lib/automation";
import { fireAuto, saveAutoPolicy, simulateAuto } from "../actions";

type UnitOption = { id: string; name: string; customerId: string; propertyId: string; temperature: { min: number; max: number; step: number } | null; ventilation: boolean };
type Live = {
  now: string; canWrite: boolean; scope: { customerId?: string; propertyId?: string; unitId?: string }; groups: PolicyGroup[]; selected: ApiAutoPolicy | null; isNew: boolean;
  customers: { id: string; name: string }[]; properties: { id: string; name: string; customerId: string }[]; units: UnitOption[];
};
type Result = { unitId: string; decision: string; ruleId: string | null; reason: string | null; commandId?: string | null };
const compares: { id: Compare; label: string }[] = [{ id: "gt", label: ">" }, { id: "gte", label: "≥" }, { id: "lt", label: "<" }, { id: "lte", label: "≤" }];

/** HQ automation policies (FR-A11) in API mode: the scope and the selected policy live in the URL; saving, simulating
 * and firing (demo) are Server Actions. Tier order: capabilities / restrictions → HQ policies → customer rules. Texts
 * in the display language (IR303). */
export function AutomationView({ live }: { live: Live }) {
  const t = useT();
  const nav = useUrlPatch();
  const [pending, run] = useAction();
  const sel = live.isNew ? null : live.selected;
  const key = sel ? `${sel.id}:${sel.version}` : "new";
  const [source, setSource] = useState(key);
  const [d, setD] = useState<AutoDraft>(autoDraft(sel ?? undefined));
  const [tried, setTried] = useState(false);
  const [facts, setFacts] = useState<FactRow[]>((sel?.unitIds ?? []).map((u) => ({ unitId: u, value: "", quality: "valid" })));
  const [results, setResults] = useState<{ fired: boolean; rows: Result[] } | null>(null);
  const [eventId, setEventId] = useState("");
  const [showOff, setShowOff] = useState(true);
  if (source !== key) { // restart from each saved policy version (adjust state during render)
    setSource(key);
    setD(autoDraft(sel ?? undefined));
    setFacts((sel?.unitIds ?? []).map((u) => ({ unitId: u, value: "", quality: "valid" })));
    setResults(null);
    setTried(false);
  }
  const set = (patch: Partial<AutoDraft>) => setD((x) => ({ ...x, ...patch }));
  const targets = live.units.filter((u) => d.unitIds.includes(u.id));
  const range = targets.reduce<{ min: number; max: number } | null>((acc, u) => (u.temperature ? { min: Math.max(acc?.min ?? -Infinity, u.temperature.min), max: Math.min(acc?.max ?? Infinity, u.temperature.max) } : acc), null);
  const errors = autoErrors(d, range, t);
  const name = (id: string) => live.units.find((u) => u.id === id)?.name ?? id.slice(0, 8);
  const scopeProps = live.properties.filter((p) => !live.scope.customerId || p.customerId === live.scope.customerId);
  const scopeUnits = live.units.filter((u) => (!live.scope.customerId || u.customerId === live.scope.customerId) && (!live.scope.propertyId || u.propertyId === live.scope.propertyId));
  const kinds: { id: AutoCondition["type"]; label: string }[] = [{ id: "occupancy", label: t("Occupancy") }, { id: "tariff", label: t("Tariff") }, { id: "peak", label: t("Peak") }, { id: "solar", label: t("Solar") }, { id: "battery", label: t("Battery") }];
  // a changed fact makes a new evaluation: the next fire gets a new one-time event ID
  const editFact = (i: number, patch: Partial<FactRow>) => { setEventId(""); setFacts((s) => s.map((x, j) => (j === i ? { ...x, ...patch } : x))); };
  const addFact = () => { setEventId(""); setFacts((s) => [...s, { unitId: sel?.unitIds[0] ?? "", kind: d.type, value: "", quality: "valid" }]); };
  const removeFact = (i: number) => { setEventId(""); setFacts((s) => s.filter((_, j) => j !== i)); };
  const clean = JSON.stringify(d) === JSON.stringify(autoDraft(sel ?? undefined));
  const save = () => {
    setTried(true);
    if (Object.keys(errors).length > 0) return;
    run(() => saveAutoPolicy(autoInput(d, sel?.id), sel?.version), (p) => (sel ? t("Policy saved as v{v}", { v: p.version }) : t("Policy created")), (p) => nav({ policyId: p.id }));
  };
  const evaluate = (fire: boolean) => {
    if (facts.length === 0) return;
    const id = fire ? eventId || crypto.randomUUID() : crypto.randomUUID();
    if (fire) setEventId(id);
    const input = evaluationInput(d.type, facts, new Date(live.now), id);
    run(() => (fire ? fireAuto(input) : simulateAuto(input)), fire ? t("Fired (demo) — commands go through the shared command rules") : t("Simulated — no commands created"), (rows) => setResults({ fired: fire, rows }));
  };
  const err = (k: string) => (tried ? errors[k] : undefined);
  const preview = autoInput(d);
  return (
    <Page>
      <div className="flex flex-wrap items-end gap-3">
        <Field label={t("Customer")}><Select value={live.scope.customerId ?? ""} onChange={(e) => nav({ customerId: e.target.value || null, propertyId: null, unitId: null, policyId: null })}><option value="">{t("All customers")}</option>{live.customers.map((c) => <option key={c.id} value={c.id}>{c.name}</option>)}</Select></Field>
        <Field label={t("Property")}><Select value={live.scope.propertyId ?? ""} onChange={(e) => nav({ propertyId: e.target.value || null, unitId: null, policyId: null })}><option value="">{t("All properties")}</option>{scopeProps.map((p) => <option key={p.id} value={p.id}>{p.name}</option>)}</Select></Field>
        <Field label={t("Unit")}><Select value={live.scope.unitId ?? ""} onChange={(e) => nav({ unitId: e.target.value || null, policyId: null })}><option value="">{t("All units")}</option>{scopeUnits.map((u) => <option key={u.id} value={u.id}>{u.name}</option>)}</Select></Field>
        <div className="pb-2"><Check label={t("Show disabled")} checked={showOff} onChange={setShowOff} /></div>
      </div>
      <div className="split-rev">
        <Card title={t("HQ automation policies")} sub={t("priority ↓")} action={live.canWrite && <Btn size="sm" variant="primary" onClick={() => nav({ policyId: "new" })}>{t("+ New")}</Btn>} className="self-start">
          {live.groups.length === 0 ? <EmptyState title={t("No automation policies")}>{t("No HQ automation policy targets this scope.")}</EmptyState> : live.groups.map((g) => ({ ...g, rows: g.rows.filter((p) => showOff || p.enabled || p.id === sel?.id) })).filter((g) => g.rows.length > 0).map((g) => (
            <div key={g.label} className="mb-3"><div className="mb-1 text-[10px] font-bold uppercase tracking-wide text-muted">{g.label}</div><div className="flex flex-col gap-2">{g.rows.map((p) => <ListRow key={p.id} selected={sel?.id === p.id} onClick={() => nav({ policyId: p.id })}><div className="min-w-0 flex-1"><b className="text-[13px]">{p.name}</b><div className="text-[11px] text-muted">{p.sentence}</div><div className="text-[11px] text-muted">{t("Priority {n}", { n: p.priority })}{!p.enabled && ` · ${t("Off")}`}{p.disabledReason && ` · ${p.disabledReason}`}</div></div></ListRow>)}</div></div>
          ))}
        </Card>
        <div className="flex min-w-0 flex-col gap-4">
          <Card title={sel ? sel.name : t("New automation policy")} sub={sel ? t("HQ tier · version {v}", { v: sel.version }) : t("Saving creates version 1 (disabled by default)")} action={sel && <Badge tone={sel.enabled ? "ok" : "muted"}>{sel.enabled ? t("Enabled") : t("Disabled")}</Badge>}>
            <div className="flex flex-col gap-5">
              {sel?.disabledReason && <Banner tone="warn">{t("Disabled automatically: {reason}. Review the policy before enabling it again.", { reason: disabledReasonWord(sel.disabledReason, t) })}</Banner>}
              <h3 className="-mb-3 text-[13px] font-bold">{t("Basics")}</h3>
              <section className="grid-fluid" style={{ ["--min" as string]: "160px" }}>
                <Field label={t("Name")} error={err("name")}><Input value={d.name} maxLength={120} onChange={(e) => set({ name: e.target.value })} /></Field>
                <Field label={t("Priority")} hint={t("0–100 · higher wins in its tier")} error={err("priority")}><Input type="number" value={d.priority} onChange={(e) => set({ priority: e.target.value })} /></Field>
                <Field label={t("Timezone")} error={err("timezone")}><Input value={d.timezone} onChange={(e) => set({ timezone: e.target.value })} /></Field>
                <div className="self-end pb-2"><Check label={t("Enabled")} checked={d.enabled} onChange={(v) => set({ enabled: v })} /></div>
              </section>
              <section>
                <h3 className="mb-1 text-[13px] font-bold">{t("Units")}</h3>
                <div className="flex flex-wrap gap-x-5 gap-y-1.5 text-[13px]">{live.units.map((u) => <Check key={u.id} label={u.name} checked={d.unitIds.includes(u.id)} onChange={(on) => { set({ unitIds: on ? [...d.unitIds, u.id] : d.unitIds.filter((x) => x !== u.id) }); setFacts((f) => (on ? [...f, { unitId: u.id, value: "", quality: "valid" }] : f.filter((x) => x.unitId !== u.id))); }} />)}</div>
                {err("unitIds") && <p className="mt-1 text-xs text-crit">{err("unitIds")}</p>}
              </section>
              <section>
                <h3 className="mb-1 text-[13px] font-bold">{t("When")}</h3>
                <p className="mb-2 text-xs text-muted">{t("Missing or stale data never triggers the action — the unit is skipped with a reason.")}</p>
                <Choice value={d.type} onChange={(type) => set({ type })} options={[{ id: "occupancy", label: t("Occupancy") }, { id: "tariff", label: t("Tariff") }, { id: "peak", label: t("Peak") }, { id: "solar", label: t("Solar") }, { id: "battery", label: t("Battery") }]} />
                <div className="mt-2 flex flex-wrap items-end gap-2 text-[13px]">
                  {d.type === "occupancy" || d.type === "peak" ? <Select className="w-auto" value={String(d.flag)} onChange={(e) => set({ flag: e.target.value === "true" })}><option value="true">{d.type === "occupancy" ? t("When the room is occupied") : t("When a peak period is active")}</option><option value="false">{d.type === "occupancy" ? t("When the room is not occupied") : t("When no peak period is active")}</option></Select>
                    : <><span className="pb-2">{subjectText(d.type, t)}</span><Select className="w-auto" value={d.operator} onChange={(e) => set({ operator: e.target.value as Compare })}>{compares.map((c) => <option key={c.id} value={c.id}>{c.label}</option>)}</Select><div className="w-24"><Field label="" error={err("value")}><Input type="number" step="0.1" value={d.value} onChange={(e) => set({ value: e.target.value })} /></Field></div><span className="pb-2">{d.type === "tariff" ? "MYR / kWh" : "kW"}</span></>}
                </div>
              </section>
              <section>
                <h3 className="mb-1 text-[13px] font-bold">{t("Then")}</h3>
                <p className="mb-2 text-xs text-muted">{t("Only actions every target unit supports, and that no active restriction blocks, can run.")}</p>
                <div className="flex flex-wrap items-end gap-2 text-[13px]">
                  <Select className="w-auto" value={d.actionKind} onChange={(e) => set({ actionKind: e.target.value as AutoDraft["actionKind"] })}><option value="set_temperature">{t("Set the temperature")}</option><option value="set_power">{t("Set the power")}</option><option value="set_mode">{t("Set the mode")}</option><option value="set_fan">{t("Set the fan")}</option><option value="ventilate">{t("Ventilate")}</option></Select>
                  {d.actionKind === "set_temperature" && <><div className="w-24"><Field label="" error={err("celsius")}><Input type="number" value={d.celsius} onChange={(e) => set({ celsius: e.target.value })} /></Field></div><span className="pb-2">°C {range ? t("(targets allow {min}–{max} °C)", { min: range.min, max: range.max }) : ""}</span></>}
                  {d.actionKind === "set_power" && <Select className="w-auto" value={String(d.power)} onChange={(e) => set({ power: e.target.value === "true" })}><option value="false">{t("off")}</option><option value="true">{t("on")}</option></Select>}
                  {d.actionKind === "set_mode" && <Select className="w-auto" value={d.mode} onChange={(e) => set({ mode: e.target.value as AutoDraft["mode"] })}><option value="cool">{t("cool")}</option><option value="dry">{t("dry")}</option><option value="fan">{t("fan")}</option></Select>}
                  {(d.actionKind === "set_fan" || d.actionKind === "ventilate") && <Select className="w-auto" value={d.level} onChange={(e) => set({ level: e.target.value as AutoDraft["level"] })}><option value="low">{t("low")}</option><option value="mid">{t("mid")}</option><option value="high">{t("high")}</option></Select>}
                </div>
                <p className="mt-2 text-xs text-muted">{conditionText(preview.condition as AutoCondition, t)} → {actionText(preview.action, t)}.</p>
              </section>
              <section className="rounded-xl bg-surface2 p-3"><h3 className="mb-2 text-[13px] font-bold">{t("How conflicts are resolved")}</h3><p className="mb-2 text-xs text-muted">{t("Evaluated per unit — at most one action per unit per evaluation.")}</p><ol className="grid-fluid text-xs" style={{ ["--min" as string]: "180px" }}><li><b>{t("1 Capabilities & active restrictions")}</b><br />{t("always first")}</li><li><b>{t("2 HQ policies")}</b><br />{t("higher priority wins · ties: ascending ID")}</li><li><b>{t("3 Customer rules")}</b><br />{t("only when no HQ policy applies")}</li></ol></section>
              {live.canWrite && (
                <div className="flex flex-wrap items-center justify-end gap-2">
                  {sel && <span className="mr-auto text-xs text-muted">{clean ? t("No unsaved changes · form loaded from policy v{v}", { v: sel.version }) : t("Unsaved changes — saving creates version {n}", { n: sel.version + 1 })}</span>}
                  {sel && <Btn disabled={pending || clean} onClick={() => { setD(autoDraft(sel)); setTried(false); }}>{t("Discard")}</Btn>}
                  <Btn variant="primary" disabled={pending} onClick={save}>{sel ? t("Save policy") : t("Create policy")}</Btn>
                </div>
              )}
            </div>
          </Card>
          {sel && (
            <Card title={t("Simulate")} sub={t("Evaluates all enabled HQ policies and customer rules for these units with synthetic facts at the current minute.")}>
              <div className="scroll-x"><table className="w-full min-w-[560px] text-[13px]"><thead className="text-left text-[11px] uppercase text-muted"><tr><th>{t("Unit")}</th><th>{t("Fact")}</th><th>{t("Value")}</th><th>{t("Quality")}</th><th /></tr></thead><tbody>{facts.map((f, i) => {
                const fact = factOf[f.kind ?? d.type];
                return (
                  <tr key={i} className="border-t border-line">
                    <td className="py-2 pr-2"><Select aria-label={t("Unit")} value={f.unitId} onChange={(e) => editFact(i, { unitId: e.target.value })}>{sel.unitIds.map((u) => <option key={u} value={u}>{name(u)}</option>)}</Select></td>
                    <td className="pr-2"><Select aria-label={t("Fact")} value={f.kind ?? d.type} onChange={(e) => editFact(i, { kind: e.target.value as AutoCondition["type"], value: "" })}>{kinds.map((k) => <option key={k.id} value={k.id}>{k.label} · {factOf[k.id].metric}</option>)}</Select></td>
                    <td className="pr-2">{fact.boolean ? <Select aria-label={t("Value")} value={f.value} onChange={(e) => editFact(i, { value: e.target.value })}><option value="">{t("— no reading")}</option><option value="true">{t("true")}</option><option value="false">{t("false")}</option></Select> : <Input aria-label={t("Value")} type="number" step="0.1" value={f.value} placeholder={t("— no reading")} onChange={(e) => editFact(i, { value: e.target.value })} />}</td>
                    <td className="pr-2"><Select aria-label={t("Quality")} value={f.quality} onChange={(e) => editFact(i, { quality: e.target.value as FactRow["quality"] })}><option value="valid">{t("valid")}</option><option value="stale">{t("stale")}</option><option value="suspect">{t("suspect")}</option><option value="missing">{t("missing")}</option></Select></td>
                    <td><button aria-label={t("Remove the fact")} className="text-muted hover:text-crit" onClick={() => removeFact(i)}>✕</button></td>
                  </tr>
                );
              })}</tbody></table></div>
              <div className="mt-3 flex flex-wrap gap-2"><Btn disabled={pending || sel.unitIds.length === 0} onClick={addFact}>{t("+ Add fact")}</Btn><Btn variant="primary" disabled={pending || facts.length === 0} onClick={() => evaluate(false)}>{t("Run simulation")}</Btn></div>
              {results && <div className="mt-3"><div className="mb-1 text-[11px] font-bold uppercase tracking-wide text-muted">{results.fired ? t("Fire result per unit") : t("Result per unit")}</div><DataTable rows={results.rows} rowKey={(r) => r.unitId} cols={[{ key: "u", label: t("Unit"), render: (r) => <b>{name(r.unitId)}</b> }, { key: "d", label: t("Decision"), render: (r) => <Badge tone={r.decision === "selected" || r.decision === "requested" ? "ok" : r.decision === "failed" ? "crit" : "warn"}>{decisionWord(r.decision, t)}</Badge> }, { key: "r", label: t("Rule / reason"), render: (r) => <span className="text-xs">{r.ruleId ? (r.ruleId === sel.id ? t("this policy") : t("rule {id}", { id: r.ruleId.slice(0, 8) })) : r.reason ? reasonWord(r.reason, t) : "—"}{r.commandId ? ` · ${t("command {id}", { id: r.commandId.slice(0, 8) })}` : ""}</span> }]} /></div>}
              {live.canWrite && <div className="mt-4 flex flex-wrap items-center justify-between gap-2 border-t border-line pt-3"><p className="max-w-xl text-xs text-muted"><DemoBadge /> {t("Fire runs the same evaluation for real: selected units get a command through the shared command rules. The event ID is a one-time key, so repeating it returns the same result without new commands.")}</p><Btn variant="danger" disabled={pending || !results || facts.length === 0} onClick={() => evaluate(true)}>{t("Fire (demo)")}</Btn></div>}
            </Card>
          )}
        </div>
      </div>
    </Page>
  );
}
