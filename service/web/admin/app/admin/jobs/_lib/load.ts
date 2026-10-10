// The reads of the HQ Jobs tab (FR-A06, DD-A06, SCR-A06) through the DAL: organizations.list (HQ, customers and
// contractors), customers.list / properties.list / units.list for the scope and the names, members.list for the
// people, one jobs.list total per pipeline stage within the scope (and the requested-time period from / to of the HQ
// overview's links, IR245), jobs.list with the filters and the sort (IR34),
// and for the selected job jobs.get, jobs.events, units.get, — for a requested job — members.eligible per preferred
// time (the HQ technicians free and qualified then, IR113), and the latest report (reports.get with up to four ready
// photos through attachments.getContent). The Plans tab (loadPlans) reads plans.list in the same scope, plans.get for
// the selected plan and jobs.list (the unit's periodic jobs) for the status of each generated occurrence.
import "server-only";
import { coreAll, coreDisplay, coreNow, coreOp, CoreError } from "@ac/web/lib/dal";
import { i18nOf, showDate, showTime } from "@ac/web/lib/i18n";
import { jobEventTitle, type ApiJobEvent } from "@ac/web/lib/partnerOverview";
import type { ApiWorkReport } from "@ac/web/lib/partnerReview";
import { agreedSlots, classifyBy, controls, costTotals, delivery, facts, filtersOf, hqRow, LISTED, longSlot, periodChip, periodOfQuery, preferredRows, reportCard, slotText, SORTS, sortOf, STAGES, stageOf, stepper, type ApiHqJob, type ApiHqRow, type Names, type Query } from "@ac/web/lib/adminJobs";
import { planRows, type ApiPlan } from "@ac/web/lib/adminPlans";
import { breachRows, PERIODS, periodOf, slaRows, slaTiles, targetsByPlan, type ApiScorecard } from "@ac/web/lib/adminSla";
import { contractorRows, kpiTiles, pendingCertificates, profileFacts, rateCardView, technicianRows, type ApiCertificate, type ApiProfile, type ApiRateCard, type ApiTechnician } from "@ac/web/lib/adminContractors";

type Page<T> = { items: T[]; total: number };
type Org = { id: string; name: string; kind: string; status: string };
type Member = { id: string; userId: string; displayName: string; role: string; employment: string | null; organizationId: string; validUntil: string | null; qualifications: ApiTechnician["qualifications"] };
const optional = <T,>(p: Promise<T>, fallback: T): Promise<T> => p.catch((e) => {
  if (e instanceof CoreError && e.error.code !== "UNAVAILABLE" && e.error.code !== "TIMEOUT") return fallback;
  throw e;
});
const one = (v: string | string[] | undefined) => (Array.isArray(v) ? v[0] : v);

type SP = Record<string, string | string[] | undefined>;

/** The reads both tabs share: the clock, organizations, customers, properties, units and members with their names, the
 * customer → property → unit scope from the URL, and the tab counts (jobs that are not cancelled, plans, contractors). */
async function shared(sp: SP) {
  const [now, display] = await Promise.all([coreNow(), coreDisplay()]);
  const i = i18nOf(display);
  const [orgs, customers, properties, units, members] = await Promise.all([
    optional(coreAll<Org>("organizations.list"), []), optional(coreAll<{ id: string; name: string; organizationId: string }>("customers.list"), []),
    optional(coreAll<{ id: string; name: string; customerOrgId: string }>("properties.list"), []), optional(coreAll<{ id: string; displayName: string; customerOrgId: string; propertyId: string }>("units.list"), []),
    optional(coreAll<Member>("members.list"), []),
  ]);
  const hq = orgs.find((o) => o.kind === "operator") ?? null;
  const contractors = orgs.filter((o) => o.kind === "contractor");
  const names: Names = {
    units: new Map(units.map((u) => [u.id, u.displayName])), customers: new Map(orgs.filter((o) => o.kind === "customer").map((o) => [o.id, o.name])),
    people: new Map(members.map((m) => [m.id, m.displayName])), orgs: new Map(orgs.map((o) => [o.id, o.name])),
  };
  const unitOrg = new Map(units.map((u) => [u.id, u.customerOrgId]));
  const sq = { customerId: one(sp.customerId), propertyId: one(sp.propertyId), unitId: one(sp.unitId) };
  const scope = filtersOf(sq, hq?.id ?? null, false);
  const custOrg = customers.find((c) => c.id === sq.customerId)?.organizationId;
  const [jobsInScope, plansInScope] = await Promise.all([
    coreOp<Page<unknown>>("jobs.list", { filters: { ...scope, statuses: LISTED }, limit: 1 }).then((r) => r.total),
    optional(coreOp<Page<unknown>>("plans.list", { filters: scope, limit: 1 }).then((r) => r.total), 0),
  ]);
  return {
    now, i, orgs, customers, properties, units, members, hq, contractors, names, unitOrg, scope,
    head: {
      now: now.toISOString(), counts: { jobs: jobsInScope, plans: plansInScope, contractors: contractors.length },
      scope: {
        customers: customers.map((c) => ({ id: c.id, name: c.name })),
        properties: properties.filter((p) => !custOrg || p.customerOrgId === custOrg).map((p) => ({ id: p.id, name: p.name })),
        units: units.filter((u) => (!custOrg || u.customerOrgId === custOrg) && (!sq.propertyId || u.propertyId === sq.propertyId)).map((u) => ({ id: u.id, name: u.displayName })),
      },
      units: units.map((u) => ({ id: u.id, name: u.displayName, customer: names.customers.get(u.customerOrgId) ?? i.t("customer") })).sort((a, b) => a.customer.localeCompare(b.customer) || a.name.localeCompare(b.name)),
    },
  };
}

export async function loadJobs(sp: SP) {
  const { now, i, members, hq, contractors, names, unitOrg, scope, head } = await shared(sp);
  const { t } = i;
  const nowMs = now.getTime();
  const q: Query = { customerId: one(sp.customerId), propertyId: one(sp.propertyId), unitId: one(sp.unitId), origin: one(sp.origin), delivery: one(sp.delivery), assignee: one(sp.assignee), overdue: one(sp.overdue), type: one(sp.type), stage: stageOf(one(sp.stage)) ?? undefined, from: one(sp.from), to: one(sp.to) };
  const period = periodOfQuery(q);
  const sort = sortOf(one(sp.sort));
  const spec = SORTS.find((s) => s.id === sort)!;
  const [counts, list, profiles] = await Promise.all([
    Promise.all(STAGES.map((s) => coreOp<Page<unknown>>("jobs.list", { filters: { ...scope, ...(period ?? {}), ...s.filter }, limit: 1 }).then((r) => r.total))),
    coreOp<Page<ApiHqRow>>("jobs.list", { filters: filtersOf(q, hq?.id ?? null), sort: { field: spec.field, direction: spec.direction }, limit: 100 }),
    optional(coreAll<{ organizationId: string; status: string }>("contractors.list"), []), // offers suspended per profile (DD-A21)
  ]);
  const rows = list.items.map((j) => hqRow(j, nowMs, names, unitOrg, i));
  const base = {
    ...head, q, period, periodChip: period ? periodChip(period, i) : null, sort, sorts: SORTS.map((x) => ({ id: x.id, text: t(x.text) })), total: list.total,
    stages: STAGES.map((x, k) => ({ id: x.id, label: t(x.label), count: counts[k] })),
    deliveries: [{ id: "internal", name: t("Internal") }, ...contractors.map((c) => ({ id: c.id, name: c.name }))],
    assignees: members.filter((m) => m.role === "technician").map((m) => ({ id: m.id, name: m.displayName })),
    contractors: contractors.map((c) => ({ id: c.id, name: c.name, status: profiles.find((p) => p.organizationId === c.id)?.status === "suspended" ? "suspended" : c.status })),
    internalTechs: members.filter((m) => m.role === "technician" && m.employment === "internal").map((m) => ({ id: m.id, name: m.displayName })),
    rows,
  };
  const pick = one(sp.jobId) ?? rows[0]?.id;
  if (!pick) return { ...base, detail: null };
  const job = await coreOp<ApiHqJob>("jobs.get", { jobId: pick }).catch((e) => {
    if (e instanceof CoreError && e.error.code === "NOT_FOUND") return null;
    throw e;
  });
  if (!job) return { ...base, detail: null, missing: pick };
  const ref = job.reportRefs?.[job.reportRefs.length - 1];
  const [events, unit, eligible, report] = await Promise.all([
    optional(coreOp<Page<ApiJobEvent>>("jobs.events", { jobId: job.id, query: { limit: 100 } }).then((r) => r.items), [] as ApiJobEvent[]),
    optional(coreOp<{ displayName: string; location: { pathLabels: string[] } }>("units.get", { id: job.unitId }), null),
    job.status === "requested" && job.slotProposal?.status !== "pending"
      ? Promise.all(agreedSlots(job).map((s) => Date.parse(s.startAt) <= nowMs ? Promise.resolve([] as { id: string; displayName: string }[])
        : optional(coreOp<Page<{ id: string; displayName: string }>>("members.eligible", { jobId: job.id, startAt: s.startAt, endAt: s.endAt, query: { limit: 20 } }).then((r) => r.items), [] as { id: string; displayName: string }[])))
      : Promise.resolve([] as { id: string; displayName: string }[][]),
    ref ? optional(coreOp<ApiWorkReport>("reports.get", { jobId: job.id, reportId: ref.reportId, reportVersion: ref.reportVersion }), null) : Promise.resolve(null),
  ]);
  const byUser = new Map(members.map((m) => [m.userId, m.displayName]));
  const sp2 = job.slotProposal, pp = job.partnerSlotProposal;
  const photos = report ? (await Promise.all(report.attachmentRefs.filter((a) => a.status === "ready" && a.size <= 2_000_000).slice(0, 4).map(async (a) => {
    const b = await optional(coreOp<{ mime: string; bytes: string }>("attachments.getContent", { jobId: job.id, reportId: report.id, reportVersion: report.version, attachmentId: a.id }), null);
    return b ? { id: a.id, name: a.name, url: `data:${b.mime};base64,${b.bytes}` } : null;
  }))).filter((p): p is { id: string; name: string; url: string } => !!p) : [];
  const costs = job.costs ?? [];
  return {
    ...base,
    detail: {
      job, title: names.units.get(job.unitId) ?? unit?.displayName ?? t("Unit"), customer: names.customers.get(unitOrg.get(job.unitId) ?? "") ?? t("customer"), location: unit?.location.pathLabels.join(" › ") ?? "",
      steps: stepper(job.status, !job.contractorOrgId, t), facts: facts(job, names, i), delivery: delivery(job, names, nowMs, i), controls: controls(job.status, t),
      preferred: preferredRows(job, eligible, nowMs, i),
      history: [...events].sort((a, b) => Date.parse(b.occurredAt) - Date.parse(a.occurredAt)).slice(0, 12).map((e) => ({
        id: e.id, at: showTime(e.occurredAt, i.display), text: `${jobEventTitle[e.action] ? t(jobEventTitle[e.action]) : e.action.replace(/[._]/g, " ")}${e.actorUserId ? ` · ${byUser.get(e.actorUserId) ?? t("client")}` : ""}`,
      })),
      partnerTech: pp ? names.people.get(pp.technicianMembershipId) ?? t("technician") : null,
      holdWho: sp2 ? (sp2.hold.kind === "internal" ? names.people.get(sp2.hold.membershipId) ?? t("HQ technician") : names.orgs.get(sp2.hold.contractorOrgId) ?? t("contractor")) : null,
      // the times the detail shows on its first render, formatted here in the display time zone (IR290)
      texts: {
        occurrence: job.occurrenceAt ? showDate(job.occurrenceAt, { ...i.display, timeZone: "Asia/Kuala_Lumpur" }) : null, classifyBy: classifyBy(job.createdAt, i),
        declinedAt: sp2?.decidedAt ? showTime(sp2.decidedAt, i.display) : null, replyBy: sp2 ? showTime(sp2.replyBy, i.display) : null, proposed: sp2 ? longSlot(sp2.slot, i) : null,
        agreed: job.offer ? slotText(job.offer.visitSlot, i) : null, partnerSlot: pp ? longSlot(pp.slot, i) : null, partnerSent: pp ? showTime(pp.sentAt, i.display) : null,
        accessUntil: job.offer ? showTime(job.offer.accessValidUntil, i.display) : null,
      },
      report: report ? { ...reportCard(job, report, byUser, i), photos } : null, draft: !!job.draftReportRef,
      costs, totals: costTotals(costs),
      extend: job.offer?.decision === "accept" && !["completed", "cancelled"].includes(job.status) ? { until: job.offer.accessValidUntil } : null,
    },
  };
}

export type JobsLive = Awaited<ReturnType<typeof loadJobs>>;

/** The Plans tab: plans.list in the scope (next due first), plans.get for planId (default the first), the unit's place
 * (units.get) and the status of each generated occurrence (the unit's periodic jobs, jobs.list). */
export async function loadPlans(sp: SP) {
  const { names, unitOrg, scope, head } = await shared(sp);
  const plans = await optional(coreAll<ApiPlan>("plans.list", { filters: scope }), [] as ApiPlan[]);
  const customerOfUnit = new Map([...unitOrg].map(([u, org]) => [u, names.customers.get(org) ?? "customer"]));
  const rows = planRows(plans, names.units, customerOfUnit);
  const q = { customerId: one(sp.customerId), propertyId: one(sp.propertyId), unitId: one(sp.unitId) };
  const base = { ...head, q, rows, plansTotal: plans.length };
  const pick = one(sp.planId) ?? rows[0]?.id;
  if (!pick) return { ...base, detail: null };
  const plan = await coreOp<ApiPlan>("plans.get", { id: pick }).catch((e) => {
    if (e instanceof CoreError && e.error.code === "NOT_FOUND") return null;
    throw e;
  });
  if (!plan) return { ...base, detail: null, missing: pick };
  const [unit, jobs] = await Promise.all([
    optional(coreOp<{ displayName: string; location: { pathLabels: string[] } }>("units.get", { id: plan.unitId }), null),
    optional(coreAll<{ id: string; status: string; displayStatus: string }>("jobs.list", { filters: { unitId: plan.unitId, origin: "periodic_plan" } }), []),
  ]);
  const status = new Map(jobs.map((j) => [j.id, j.displayStatus === "time_proposed" ? "time_proposed" : j.status]));
  return {
    ...base,
    detail: {
      plan, unit: names.units.get(plan.unitId) ?? unit?.displayName ?? "Unit", customer: customerOfUnit.get(plan.unitId) ?? "customer", location: unit?.location.pathLabels.join(" › ") ?? "",
      occurrences: [...plan.generatedOccurrences].sort((a, b) => Date.parse(b.occurrenceAt) - Date.parse(a.occurrenceAt)).map((o) => ({ ...o, status: status.get(o.jobId) ?? null })),
    },
  };
}

export type PlansLive = Awaited<ReturnType<typeof loadPlans>>;

/** The Contractors tab (DD-A21, IR131): every contractor organization with its profile (contractors.list with the
 * 90-day KPIs, the delegation and the rate card in effect) and for contractorId its rate cards (rateCards.list, newest
 * first), technicians (members.list) and uploaded certificates (certificates.list). */
export async function loadContractors(sp: SP) {
  const { now, members, contractors: orgs, names, head } = await shared(sp);
  const nowMs = now.getTime();
  const profiles = await optional(coreAll<ApiProfile>("contractors.list"), [] as ApiProfile[]);
  const technicians: ApiTechnician[] = members.filter((m) => m.role === "technician" && orgs.some((o) => o.id === m.organizationId))
    .map((m) => ({ id: m.id, displayName: m.displayName, organizationId: m.organizationId, role: m.role, validUntil: m.validUntil ?? null, qualifications: m.qualifications ?? [] }));
  const rows = contractorRows(orgs.map((o) => ({ id: o.id, name: o.name })), profiles, technicians);
  const base = { ...head, q: {}, rows, withoutProfile: orgs.filter((o) => !profiles.some((p) => p.organizationId === o.id)).map((o) => ({ id: o.id, name: o.name })) };
  const pick = one(sp.contractorId) ?? rows[0]?.id;
  if (!pick) return { ...base, detail: null };
  const org = orgs.find((o) => o.id === pick);
  if (!org) return { ...base, detail: null, missing: pick };
  const profile = profiles.find((p) => p.organizationId === pick) ?? null;
  const [cards, certs] = await Promise.all([
    optional(coreAll<ApiRateCard>("rateCards.list", { filters: { contractorOrgId: pick } }), [] as ApiRateCard[]),
    optional(coreAll<ApiCertificate>("certificates.list", { filters: { organizationId: pick } }), [] as ApiCertificate[]),
  ]);
  // without a profile there is no rateCardId: the card in effect is the latest that has started
  const inEffect = profile?.rateCardId ?? [...cards].filter((c) => Date.parse(c.effectiveFrom) <= nowMs).sort((a, b) => Date.parse(b.effectiveFrom) - Date.parse(a.effectiveFrom))[0]?.id ?? null;
  const mine = technicians.filter((t) => t.organizationId === pick);
  return {
    ...base,
    detail: {
      org: { id: org.id, name: org.name }, profile, kpis: profile ? kpiTiles(profile.kpis) : null, facts: profile ? profileFacts(profile, nowMs) : null,
      rates: rateCardView(cards, inEffect, nowMs), techs: technicianRows(mine, certs, nowMs),
      pending: pendingCertificates(certs).map((c) => ({ id: c.id, version: c.version, technician: names.people.get(c.membershipId) ?? "technician", name: c.name, code: c.code, number: c.number, issuedAt: c.issuedAt, expiresAt: c.expiresAt, fileName: c.fileName, renewal: !!c.renewalOf })),
    },
  };
}

export type ContractorsLive = Awaited<ReturnType<typeof loadContractors>>;

/** The SLA tab (DD-A22, IR236): sla.scorecard for the period (URL key period, default 90 days) and contractor
 * (contractorId) — totals with the targets of the customers' plans, one row per customer, the recent breaches and the
 * targets per plan type for the edit dialog. */
export async function loadSla(sp: SP) {
  const { now, customers, properties, units, contractors: orgs, head } = await shared(sp);
  const period = periodOf(one(sp.period), now.getTime());
  const contractorId = orgs.find((o) => o.id === one(sp.contractorId))?.id ?? "";
  const sc = await coreOp<ApiScorecard>("sla.scorecard", { period: { from: period.from, to: period.to }, ...(contractorId ? { contractorOrgId: contractorId } : {}) });
  return {
    ...head, q: {}, period, periods: PERIODS.map((p) => ({ id: p.id, label: p.label })), contractorId, contractors: orgs.map((o) => ({ id: o.id, name: o.name })),
    tiles: slaTiles(sc), rows: slaRows(sc, customers, properties, units), breaches: breachRows(sc, new Map(customers.map((c) => [c.id, c.name]))),
    targets: targetsByPlan(sc.targets), scorecardPeriod: sc.period,
  };
}

export type SlaLive = Awaited<ReturnType<typeof loadSla>>;

