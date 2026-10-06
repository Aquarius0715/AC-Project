"use client";

/**
 * Shared maintenance-job mock store (IR113). All four roles read and write the same jobs, so a
 * request made on /customer/maintenance shows up in /admin/jobs, then /partner/jobs and
 * /technician. State lives in localStorage (demo only) and is reset from /demo.
 */
import { useSyncExternalStore } from "react";

export type Slot = { date: string; win: string };
export type Origin = "request" | "plan";
export type JobStatus = "requested" | "time_proposed" | "offered" | "accepted" | "assigned" | "in_progress" | "submitted" | "rework_requested" | "completed" | "cancelled" | "on_hold";
export type Proposal = { by: "hq" | "contractor"; slot: Slot; who: string; message: string; replyBy: string; status: "pending" | "accepted" | "declined" | "withdrawn" };
export type PartnerProposal = { slot: Slot; tech: string; reason: string; sentAt: string; status: "pending" | "sent_to_client" | "rejected" | "approved" | "withdrawn" };
export type TechAck = { status: "pending" | "accepted" | "cant_make"; reason?: string; alt?: string; at?: string };
export type Job = {
  id: string;
  unitId: string;
  unit: string;
  loc: string;
  customer: string;
  origin: Origin;
  type: "Reactive" | "Preventive" | "Periodic";
  planId?: string;
  planVisit?: string;
  symptom: string;
  status: JobStatus;
  /** Client's preferred times for the current round (1st–3rd). Empty for plan visits. */
  preferred: Slot[];
  round: number;
  scheduled: Slot | null;
  delivery: "internal" | "contractor" | null;
  contractor: string | null;
  technician: string | null;
  techAck: TechAck | null;
  proposal: Proposal | null;
  partnerProposal: PartnerProposal | null;
  declined: { reason: string; comment: string; at: string } | null;
  history: { at: string; text: string }[];
};
export type Note = { id: string; role: "client" | "admin" | "contractor" | "technician"; title: string; detail: string; href: string; at: string; read: boolean };

const KEY = "ac-jobs-v2";
const NKEY = "ac-job-notes-v2";
export const NOW = "09-22 11:05";

const req = (id: string, p: Partial<Job> & Pick<Job, "unitId" | "unit" | "loc" | "type" | "symptom" | "status">): Job => ({
  id, customer: "customer-a", origin: "request", preferred: [], round: 1, scheduled: null, delivery: null, contractor: null, technician: null, techAck: null, proposal: null, partnerProposal: null, declined: null, history: [], ...p,
});

export const SEED: Job[] = [
  req("job-c07", { unitId: "unit-bedroom-2", unit: "Bedroom AC #2", loc: "Home A › 1F › Bedroom", type: "Reactive", symptom: "Cold air is weak and there is water dripping from the indoor unit.", status: "requested",
    preferred: [{ date: "2026-09-28", win: "14:00–16:00" }, { date: "2026-09-29", win: "10:00–12:00" }, { date: "2026-09-30", win: "09:00–11:00" }],
    history: [{ at: "09-22 09:14", text: "customer-a submitted the request with 3 preferred times" }] }),
  req("job-c04", { unitId: "unit-online-rto", unit: "Bedroom AC", loc: "Home A › 1F › Bedroom", type: "Reactive", symptom: "AC is making a rattling noise and airflow feels weaker than usual since yesterday evening.", status: "in_progress",
    preferred: [{ date: "2026-09-15", win: "10:00–12:00" }, { date: "2026-09-16", win: "14:00–16:00" }, { date: "2026-09-17", win: "09:00–11:00" }], scheduled: { date: "2026-09-15", win: "10:00–12:00" }, delivery: "internal", technician: "tech-internal-a", techAck: { status: "accepted", at: "09-14 21:05" },
    history: [{ at: "09-14 20:10", text: "Requested with 3 preferred times" }, { at: "09-14 21:00", text: "HQ booked the 1st preferred time · tech-internal-a" }, { at: "09-15 10:05", text: "Technician on site" }] }),
  req("job-c05", { unitId: "unit-kitchen", unit: "Kitchen AC", loc: "Home A › 1F › Kitchen", type: "Preventive", symptom: "Yearly preventive check before the hot season.", status: "assigned",
    preferred: [{ date: "2026-09-25", win: "09:00–11:00" }, { date: "2026-09-26", win: "09:00–11:00" }, { date: "2026-09-28", win: "09:00–11:00" }], scheduled: { date: "2026-09-25", win: "09:00–11:00" }, delivery: "contractor", contractor: "contractor-a", technician: "tech-external-a2", techAck: { status: "accepted", at: "09-21 08:30" },
    history: [{ at: "09-20 10:00", text: "Requested with 3 preferred times" }, { at: "09-20 11:00", text: "Offered to contractor-a at the 1st preferred time" }, { at: "09-21 08:30", text: "tech-external-a2 accepted the assignment" }] }),
  req("job-c03", { unitId: "unit-living", unit: "Living room AC", loc: "Home A › 1F › Living room", type: "Reactive", symptom: "Unit trips the breaker after 10 minutes.", status: "rework_requested", scheduled: { date: "2026-08-22", win: "14:00–16:00" }, delivery: "internal", technician: "tech-internal-a", techAck: { status: "accepted" },
    history: [{ at: "08-22 16:30", text: "Report returned by HQ for rework" }] }),
  req("job-c02", { unitId: "unit-living", unit: "Living room AC", loc: "Home A › 1F › Living room", origin: "plan", planId: "plan-living-a", planVisit: "3 of 4", type: "Periodic", symptom: "Periodic inspection & filter cleaning", status: "completed", scheduled: { date: "2026-09-08", win: "09:00–11:00" }, delivery: "internal", technician: "tech-internal-b", techAck: { status: "accepted" },
    history: [{ at: "09-08 15:00", text: "Report accepted by HQ" }] }),
  req("job-c08", { unitId: "unit-living", unit: "Living room AC", loc: "Home A › 1F › Living room", origin: "plan", planId: "plan-living-a", planVisit: "4 of 4", type: "Periodic", symptom: "Periodic inspection & filter cleaning", status: "assigned", scheduled: { date: "2026-12-08", win: "09:00–11:00" }, delivery: "internal", technician: "tech-internal-b", techAck: { status: "accepted", at: "11-09 09:00" },
    history: [{ at: "11-08 09:00", text: "Generated from plan-living-a · client notified" }] }),
  req("job-c01", { unitId: "unit-offline-rto", unit: "Study AC", loc: "Home A › 2F › Study", type: "Reactive", symptom: "Unit shows offline.", status: "cancelled", history: [{ at: "09-02 10:00", text: "Cancelled by client · resolved itself" }] }),
  req("job-internal-a", { unitId: "unit-online-rto", unit: "Bedroom AC", loc: "Home A › 1F › Bedroom", type: "Reactive", symptom: "Unit shows offline since 08:12.", status: "requested",
    preferred: [{ date: "2026-09-24", win: "10:00–12:00" }, { date: "2026-10-05", win: "14:00–16:00" }, { date: "2026-10-06", win: "10:00–12:00" }], history: [{ at: "09-14 08:20", text: "Requested by HQ on behalf of customer-a (phone)" }] }),
  req("job-a11", { unitId: "unit-p-rooftop", unit: "Rooftop unit", loc: "Office B › Roof", customer: "customer-b", type: "Reactive", symptom: "Loud compressor noise.", status: "offered", scheduled: { date: "2026-09-24", win: "09:00–12:00" }, delivery: "contractor", contractor: "contractor-a",
    history: [{ at: "09-21 09:00", text: "Re-offered to contractor-a after decline" }] }),
  req("job-a06", { unitId: "unit-living", unit: "Living room AC", loc: "Home A › 1F › Living room", origin: "plan", planId: "plan-living-a", planVisit: "Sep occurrence", type: "Periodic", symptom: "Periodic filter cleaning", status: "submitted", scheduled: { date: "2026-09-15", win: "10:00–12:00" }, delivery: "internal", technician: "tech-internal-a", techAck: { status: "accepted" },
    history: [{ at: "09-15 11:48", text: "Report v1 submitted" }] }),
  req("job-p09", { unitId: "unit-limited", unit: "Lobby AC", loc: "Office A › Lobby", customer: "customer-b", origin: "plan", planId: "plan-lobby-b", planVisit: "quarterly", type: "Periodic", symptom: "Quarterly inspection", status: "submitted", scheduled: { date: "2026-09-16", win: "10:00–12:00" }, delivery: "contractor", contractor: "contractor-a", technician: "tech-external-a", techAck: { status: "accepted" },
    history: [{ at: "09-19 16:00", text: "Report v1 submitted · awaiting contractor review" }] }),
];

/* ---------- availability (demo rule set) ---------- */
export type Fit = { hq: string; partner: string; fits: "none" | "internal" | "contractor" | "both" };
export function fitFor(s: Slot): Fit {
  const d = s.date;
  if (d < "2026-10-01") return { hq: "tech-internal-a: booked · tech-internal-b: not qualified (refrigerant)", partner: "contractor-a: no free hours · contractor-b: outside service area", fits: "none" };
  if (d === "2026-10-02") return { hq: "tech-internal-a: overlaps job-c04 follow-up", partner: "contractor-a: tech-external-a fully booked", fits: "none" };
  const internal = s.win.startsWith("14") && d >= "2026-10-01";
  return { hq: internal ? "tech-internal-a: free ✓ (refrigerant ✓)" : "tech-internal-b: not qualified", partner: "contractor-a: tech-external-a free ✓", fits: internal ? "both" : "contractor" };
}

/* ---------- store ---------- */
type State = { jobs: Job[]; notes: Note[] };
let state: State | null = null;
const listeners = new Set<() => void>();
const load = (): State => {
  if (state) return state;
  let jobs = SEED, notes: Note[] = [];
  try {
    const j = localStorage.getItem(KEY);
    const n = localStorage.getItem(NKEY);
    if (j) jobs = JSON.parse(j);
    if (n) notes = JSON.parse(n);
  } catch {}
  state = { jobs, notes };
  return state;
};
const save = () => {
  try { localStorage.setItem(KEY, JSON.stringify(state!.jobs)); localStorage.setItem(NKEY, JSON.stringify(state!.notes)); } catch {}
  listeners.forEach((l) => l());
};
const SERVER: State = { jobs: SEED, notes: [] };
const subscribe = (l: () => void) => { listeners.add(l); return () => listeners.delete(l); };
export function useJobStore() {
  return useSyncExternalStore(subscribe, load, () => SERVER);
}
export const useJobs = () => useJobStore().jobs;
/** Non-React read (tests, scripts). */
export const getJobState = () => load();
export const useNotes = (role: Note["role"]) => useJobStore().notes.filter((n) => n.role === role);

function update(id: string, fn: (j: Job) => Partial<Job>, log?: string) {
  const s = load();
  state = { ...s, jobs: s.jobs.map((j) => (j.id === id ? { ...j, ...fn(j), history: log ? [...j.history, { at: NOW, text: log }] : j.history } : j)) };
  save();
}
function notify(role: Note["role"], title: string, detail: string, href: string) {
  const s = load();
  state = { ...s, notes: [{ id: `${Date.now()}-${Math.random()}`, role, title, detail, href, at: "just now", read: false }, ...s.notes] };
  save();
}
export const fmt = (s: Slot | null | undefined) => (s ? `${s.date.slice(5)} · ${s.win}` : "—");
export const longDate = (s: Slot) => {
  const d = new Date(`${s.date}T00:00:00`);
  return `${d.toLocaleDateString("en-GB", { weekday: "short" })} ${s.date.slice(5)} · ${s.win}`;
};

export const jobActions = {
  reset() { state = { jobs: SEED, notes: [] }; save(); },
  markRead(id: string) { const s = load(); state = { ...s, notes: s.notes.map((n) => (n.id === id ? { ...n, read: true } : n)) }; save(); },
  /** Client: new request with 3 preferred times (FR-C09 / IR113). */
  create(p: { unitId: string; unit: string; loc: string; type: "Reactive" | "Preventive"; symptom: string; preferred: Slot[] }) {
    const s = load();
    const n = s.jobs.filter((j) => /^job-c\d+$/.test(j.id)).map((j) => +j.id.slice(5)).reduce((a, b) => Math.max(a, b), 0) + 1;
    const id = `job-c${String(n).padStart(2, "0")}`;
    state = { ...s, jobs: [req(id, { ...p, status: "requested", history: [{ at: NOW, text: "customer-a submitted the request with 3 preferred times" }] }), ...s.jobs] };
    save();
    notify("admin", `New client request ${id}`, `${p.unit} · 3 preferred times — book one or propose another time`, `/admin/jobs?jobId=${id}`);
    return id;
  },
  cancel(id: string, reason: string) { update(id, () => ({ status: "cancelled", proposal: null }), `Cancelled by client · ${reason}`); notify("admin", `${id} cancelled by the client`, reason, `/admin/jobs?jobId=${id}`); },
  /** HQ: book one of the client's own times — no client approval needed. */
  book(id: string, slot: Slot, delivery: "internal" | "contractor", who: string) {
    if (delivery === "internal") {
      update(id, () => ({ status: "assigned", scheduled: slot, delivery, technician: who, contractor: null, techAck: { status: "pending" }, proposal: null }), `HQ booked ${fmt(slot)} · assigned ${who}`);
      notify("technician", `New assignment ${id}`, `${fmt(slot)} — accept or tell HQ you can’t make it`, `/technician/jobs/${id}`);
    } else {
      update(id, () => ({ status: "offered", scheduled: slot, delivery, contractor: who, technician: null, techAck: null, proposal: null, partnerProposal: null }), `HQ offered ${id} to ${who} at ${fmt(slot)}`);
      notify("contractor", `New offer ${id}`, `Visit time fixed: ${fmt(slot)} — accept, decline or propose another time`, `/partner/jobs/${id}`);
    }
    notify("client", `Booked: ${id}`, `${fmt(slot)} — one of your preferred times`, `/customer/maintenance?jobId=${id}`);
  },
  /** HQ: none of the preferred times fits → propose another time (client must accept). */
  propose(id: string, slot: Slot, who: string, message: string, replyBy: string) {
    update(id, () => ({ status: "time_proposed", proposal: { by: "hq", slot, who, message, replyBy, status: "pending" } }), `HQ proposed ${fmt(slot)} (${who} held) · reply by ${replyBy}`);
    notify("client", `HQ proposed a new time for ${id}`, `${longDate(slot)} — accept or decline by ${replyBy}`, `/customer/maintenance?jobId=${id}`);
  },
  withdraw(id: string) { update(id, (j) => ({ status: "requested", proposal: j.proposal ? { ...j.proposal, status: "withdrawn" } : null }), "HQ withdrew the proposal"); notify("client", `Proposal withdrawn — ${id}`, "HQ will contact you again", `/customer/maintenance?jobId=${id}`); },
  /** Client accepts a proposed time → booked automatically with the held partner. */
  accept(id: string) {
    const j = load().jobs.find((x) => x.id === id);
    if (!j?.proposal) return;
    const p = j.proposal;
    if (p.by === "contractor") {
      update(id, () => ({ status: "offered", scheduled: p.slot, proposal: { ...p, status: "accepted" }, partnerProposal: j.partnerProposal ? { ...j.partnerProposal, status: "approved" } : null }), `Client approved ${fmt(p.slot)} proposed by ${j.contractor}`);
      notify("contractor", `Client approved your time for ${id}`, `${fmt(p.slot)} — accept the offer`, `/partner/jobs/${id}`);
    } else if (p.who.startsWith("contractor")) {
      update(id, () => ({ status: "offered", scheduled: p.slot, delivery: "contractor", contractor: p.who.split(" ")[0], proposal: { ...p, status: "accepted" } }), `Client accepted ${fmt(p.slot)} → offered to ${p.who.split(" ")[0]}`);
      notify("contractor", `New offer ${id}`, `Visit time agreed with the client: ${fmt(p.slot)}`, `/partner/jobs/${id}`);
    } else {
      update(id, () => ({ status: "assigned", scheduled: p.slot, delivery: "internal", technician: p.who, techAck: { status: "pending" }, proposal: { ...p, status: "accepted" } }), `Client accepted ${fmt(p.slot)} → assigned ${p.who}`);
      notify("technician", `New assignment ${id}`, `${fmt(p.slot)} — accept or tell HQ you can’t make it`, `/technician/jobs/${id}`);
    }
    notify("admin", `Client accepted the proposed time — ${id}`, fmt(p.slot), `/admin/jobs?jobId=${id}`);
  },
  /** Client declines → back to requested with reason and (optionally) 3 new preferred times. */
  decline(id: string, reason: string, comment: string, preferred: Slot[]) {
    update(id, (j) => ({
      status: j.proposal?.by === "contractor" ? "offered" : "requested",
      round: preferred.length ? j.round + 1 : j.round,
      preferred: preferred.length ? preferred : j.preferred,
      declined: { reason, comment, at: NOW },
      proposal: j.proposal ? { ...j.proposal, status: "declined" } : null,
      partnerProposal: j.partnerProposal ? { ...j.partnerProposal, status: "rejected" } : null,
    }), `Client declined the proposed time · ${reason}${preferred.length ? " · sent 3 new times" : ""}`);
    notify("admin", `Client declined the proposal — ${id}`, `${reason}${comment ? ` — “${comment}”` : ""}`, `/admin/jobs?jobId=${id}`);
  },
  /** Client asks for another time for a periodic plan visit. */
  requestOther(id: string, preferred: Slot[], comment: string) {
    update(id, (j) => ({ status: "requested", preferred, round: j.round + 1, scheduled: null, technician: null, techAck: null }), `Client asked for another time${comment ? ` — “${comment}”` : ""}`);
    notify("admin", `Client asked to move ${id}`, "3 new preferred times", `/admin/jobs?jobId=${id}`);
  },
  /* ---- contractor ---- */
  partnerAccept(id: string) { update(id, () => ({ status: "accepted", partnerProposal: null }), "contractor-a accepted the offer"); notify("admin", `contractor-a accepted ${id}`, "assign a technician next", `/admin/jobs?jobId=${id}`); },
  partnerDecline(id: string, reason: string) { update(id, () => ({ status: "requested", contractor: null, delivery: null, scheduled: null }), `contractor-a declined · ${reason}`); notify("admin", `contractor-a declined ${id}`, reason, `/admin/jobs?jobId=${id}`); },
  partnerPropose(id: string, slot: Slot, tech: string, reason: string) {
    update(id, () => ({ partnerProposal: { slot, tech, reason, sentAt: NOW, status: "pending" } }), `contractor-a proposed ${fmt(slot)} instead`);
    notify("admin", `contractor-a proposes another time — ${id}`, `${fmt(slot)} · ${reason}`, `/admin/jobs?jobId=${id}`);
  },
  partnerWithdraw(id: string) { update(id, () => ({ partnerProposal: null }), "contractor-a withdrew its time proposal"); },
  partnerAssign(id: string, tech: string) {
    update(id, () => ({ status: "assigned", technician: tech, techAck: { status: "pending" } }), `contractor-a assigned ${tech}`);
    notify("technician", `New assignment ${id}`, "accept it or tell contractor-a you can’t make it", `/technician/jobs/${id}`);
  },
  /* ---- HQ on a partner proposal ---- */
  forwardPartner(id: string, replyBy: string) {
    const j = load().jobs.find((x) => x.id === id);
    if (!j?.partnerProposal) return;
    const pp = j.partnerProposal;
    update(id, () => ({ status: "time_proposed", proposal: { by: "contractor", slot: pp.slot, who: `${j.contractor} · ${pp.tech}`, message: pp.reason, replyBy, status: "pending" }, partnerProposal: { ...pp, status: "sent_to_client" } }), "HQ sent the partner’s time to the client");
    notify("client", `Your service partner asks for another time — ${id}`, `${longDate(pp.slot)} — accept or decline`, `/customer/maintenance?jobId=${id}`);
  },
  keepTime(id: string) { update(id, (j) => ({ partnerProposal: j.partnerProposal ? { ...j.partnerProposal, status: "rejected" } : null }), "HQ kept the agreed time"); notify("contractor", `HQ kept the agreed time — ${id}`, "accept or decline the offer", `/partner/jobs/${id}`); },
  /* ---- technician ---- */
  techAccept(id: string) {
    const j = load().jobs.find((x) => x.id === id);
    update(id, () => ({ techAck: { status: "accepted", at: NOW } }), `${j?.technician} accepted the assignment`);
    notify(j?.delivery === "contractor" ? "contractor" : "admin", `${j?.technician} accepted ${id}`, fmt(j?.scheduled), j?.delivery === "contractor" ? "/partner/schedule" : `/admin/jobs?jobId=${id}`);
    notify("client", `Technician confirmed — ${id}`, `${j?.technician} · ${fmt(j?.scheduled)}`, `/customer/maintenance?jobId=${id}`);
  },
  techCantMake(id: string, reason: string, alt: string) {
    const j = load().jobs.find((x) => x.id === id);
    update(id, () => ({ techAck: { status: "cant_make", reason, alt, at: NOW } }), `${j?.technician} can’t make the time · ${reason}`);
    notify(j?.delivery === "contractor" ? "contractor" : "admin", `${j?.technician} can’t make ${id}`, `${reason}${alt ? ` · could do ${alt}` : ""}`, j?.delivery === "contractor" ? "/partner/schedule" : `/admin/jobs?jobId=${id}`);
  },
};

export const statusLabel: Record<JobStatus, string> = {
  requested: "Requested", time_proposed: "Time proposed", offered: "Offered", accepted: "Accepted", assigned: "Assigned", in_progress: "In progress", submitted: "Submitted", rework_requested: "Rework requested", completed: "Completed", cancelled: "Cancelled", on_hold: "On hold",
};
