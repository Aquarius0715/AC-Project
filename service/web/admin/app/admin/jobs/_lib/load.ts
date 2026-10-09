// The reads of the HQ Jobs tab (FR-A06, DD-A06, SCR-A06) through the DAL: organizations.list (HQ, customers and
// contractors), customers.list / properties.list / units.list for the scope and the names, members.list for the
// people, one jobs.list total per pipeline stage within the scope, jobs.list with the filters and the sort (IR34),
// and for the selected job jobs.get, jobs.events, units.get and — for a requested job — members.eligible per
// preferred time (the HQ technicians free and qualified then, IR113).
import "server-only";
import { coreAll, coreNow, coreOp, CoreError } from "@ac/web/lib/dal";
import { jobEventTitle, type ApiJobEvent } from "@ac/web/lib/partnerOverview";
import { controls, delivery, facts, filtersOf, hqRow, preferredRows, SORTS, sortOf, STAGES, stageOf, stepper, type ApiHqJob, type ApiHqRow, type Names, type Query } from "@ac/web/lib/adminJobs";
import { klTime } from "@ac/web/lib/devices";

type Page<T> = { items: T[]; total: number };
type Org = { id: string; name: string; kind: string; status: string };
type Member = { id: string; userId: string; displayName: string; role: string; employment: string | null; organizationId: string };
const optional = <T,>(p: Promise<T>, fallback: T): Promise<T> => p.catch((e) => {
  if (e instanceof CoreError && e.error.code !== "UNAVAILABLE" && e.error.code !== "TIMEOUT") return fallback;
  throw e;
});
const one = (v: string | string[] | undefined) => (Array.isArray(v) ? v[0] : v);

export async function loadJobs(sp: Record<string, string | string[] | undefined>) {
  const now = await coreNow();
  const nowMs = now.getTime();
  const q: Query = { customerId: one(sp.customerId), propertyId: one(sp.propertyId), unitId: one(sp.unitId), origin: one(sp.origin), delivery: one(sp.delivery), assignee: one(sp.assignee), overdue: one(sp.overdue), type: one(sp.type), stage: stageOf(one(sp.stage)) ?? undefined };
  const sort = sortOf(one(sp.sort));
  const spec = SORTS.find((s) => s.id === sort)!;
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
  const scope = filtersOf({ customerId: q.customerId, propertyId: q.propertyId, unitId: q.unitId }, hq?.id ?? null, false);
  const [counts, list] = await Promise.all([
    Promise.all(STAGES.map((s) => coreOp<Page<unknown>>("jobs.list", { filters: { ...scope, ...s.filter }, limit: 1 }).then((r) => r.total))),
    coreOp<Page<ApiHqRow>>("jobs.list", { filters: filtersOf(q, hq?.id ?? null), sort: { field: spec.field, direction: spec.direction }, limit: 100 }),
  ]);
  const rows = list.items.filter((j) => j.status !== "cancelled" && (!q.type || j.type === q.type)).map((j) => hqRow(j, nowMs, names, unitOrg));
  const custOrg = customers.find((c) => c.id === q.customerId)?.organizationId;
  const base = {
    now: now.toISOString(), q, sort, sorts: SORTS, total: list.total,
    stages: STAGES.map((s, i) => ({ id: s.id, label: s.label, count: counts[i] })),
    scope: {
      customers: customers.map((c) => ({ id: c.id, name: c.name })),
      properties: properties.filter((p) => !custOrg || p.customerOrgId === custOrg).map((p) => ({ id: p.id, name: p.name })),
      units: units.filter((u) => (!custOrg || u.customerOrgId === custOrg) && (!q.propertyId || u.propertyId === q.propertyId)).map((u) => ({ id: u.id, name: u.displayName })),
    },
    deliveries: [{ id: "internal", name: "Internal" }, ...contractors.map((c) => ({ id: c.id, name: c.name }))],
    assignees: members.filter((m) => m.role === "technician").map((m) => ({ id: m.id, name: m.displayName })),
    contractors: contractors.map((c) => ({ id: c.id, name: c.name, status: c.status })),
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
  const [events, unit, eligible] = await Promise.all([
    optional(coreOp<Page<ApiJobEvent>>("jobs.events", { jobId: job.id, query: { limit: 100 } }).then((r) => r.items), [] as ApiJobEvent[]),
    optional(coreOp<{ displayName: string; location: { pathLabels: string[] } }>("units.get", { id: job.unitId }), null),
    job.status === "requested" && job.slotProposal?.status !== "pending"
      ? Promise.all(job.preferredSlots.map((s) => Date.parse(s.startAt) <= nowMs ? Promise.resolve([] as { id: string; displayName: string }[])
        : optional(coreOp<Page<{ id: string; displayName: string }>>("members.eligible", { jobId: job.id, startAt: s.startAt, endAt: s.endAt, query: { limit: 20 } }).then((r) => r.items), [] as { id: string; displayName: string }[])))
      : Promise.resolve([] as { id: string; displayName: string }[][]),
  ]);
  const byUser = new Map(members.map((m) => [m.userId, m.displayName]));
  return {
    ...base,
    detail: {
      job, title: names.units.get(job.unitId) ?? unit?.displayName ?? "Unit", customer: names.customers.get(unitOrg.get(job.unitId) ?? "") ?? "customer", location: unit?.location.pathLabels.join(" › ") ?? "",
      steps: stepper(job.status, !job.contractorOrgId), facts: facts(job, names), delivery: delivery(job, names, nowMs), controls: controls(job.status),
      preferred: preferredRows(job, eligible, nowMs),
      history: [...events].sort((a, b) => Date.parse(b.occurredAt) - Date.parse(a.occurredAt)).slice(0, 12).map((e) => ({ id: e.id, at: klTime(e.occurredAt).slice(5), text: `${jobEventTitle[e.action] ?? e.action.replace(/[._]/g, " ")}${e.actorUserId ? ` · ${byUser.get(e.actorUserId) ?? "client"}` : ""}` })),
      partnerTech: job.partnerSlotProposal ? names.people.get(job.partnerSlotProposal.technicianMembershipId) ?? "technician" : null,
      holdWho: job.slotProposal ? (job.slotProposal.hold.kind === "internal" ? names.people.get(job.slotProposal.hold.membershipId) ?? "HQ technician" : names.orgs.get(job.slotProposal.hold.contractorOrgId) ?? "contractor") : null,
    },
  };
}

export type JobsLive = Awaited<ReturnType<typeof loadJobs>>;
