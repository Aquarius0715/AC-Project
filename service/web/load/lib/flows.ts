// What each kind of user does in one visit: the reads of the screens they open most (docs/03-uiux/screen-catalog.csv:
// the primary queries of the customer overview, units, alerts, maintenance and payments pages; the partner's jobs; the
// technician's units and alerts; HQ's summary and lists) and one write. The mix of users (customer 70%, partner 15%,
// technician 10%, HQ 5%) is an assumption without usage data. Think time between calls stands for a person reading.
import { sleep } from "k6";
import { call, type Answer, type Call } from "./api.ts";
import type { User } from "./manifest.ts";

const LIST = "limit=20";

function pick<T>(items: readonly T[], seed: number): T | undefined {
  return items.length === 0 ? undefined : items[seed % items.length];
}

function think(): void {
  sleep(1 + Math.random() * 2);
}

function run(user: User, calls: Call[]): Answer[] {
  const answers: Answer[] = [];
  for (const c of calls) {
    answers.push(call(user, c));
    think();
  }
  return answers;
}

const get = (name: string, path: string): Call => ({ method: "GET", name, path });

const summary = (kind: string): Call => get("summaries.get", `/v1/summaries/${kind}`);

const SESSION = get("session.get", "/v1/session");
const UNITS = get("units.list", `/v1/units?${LIST}`);
const JOBS = get("jobs.list", `/v1/jobs?${LIST}`);
const ALERTS = get("alerts.list", `/v1/alerts?${LIST}`);
const CONTRACTS = get("contracts.list", `/v1/contracts?${LIST}`);
const INVOICES = get("invoices.list", `/v1/invoices?${LIST}`);

/** The language and time zone every account may set: the write all four kinds share. */
function preferences(): Call {
  return { body: { locale: "en", timezone: "Asia/Kuala_Lumpur" }, method: "PUT", name: "preferences.update", path: "/v1/preferences" };
}

function customer(user: User, writes: boolean, seed: number): void {
  const unit = pick(user.unitIds, seed);
  const calls = [
    SESSION,
    UNITS,
    get("properties.list", `/v1/properties?${LIST}`),
    summary("customer"),
    ALERTS,
    JOBS,
    CONTRACTS,
    INVOICES,
  ];
  if (unit) calls.push(get("units.get", `/v1/units/${unit}`));
  if (writes && unit && seed % 5 === 0) {
    // a request names three distinct slots of 1-4 hours, each on a later Kuala Lumpur day than today (IR113)
    const slot = (days: number): { startAt: string; endAt: string } => {
      const start = Date.now() + days * 86_400_000;
      return { startAt: new Date(start).toISOString(), endAt: new Date(start + 7_200_000).toISOString() };
    };
    const [first, second, third] = [slot(2), slot(3), slot(4)];
    const body = {
      alternativeSlots: [second, third],
      requestedEnd: first.endAt,
      requestedStart: first.startAt,
      symptom: "Load test: the filter needs a check",
      type: "reactive",
      unitId: unit,
    };
    calls.push({ body, method: "POST", name: "jobs.create", path: "/v1/jobs" });
  }
  if (writes) calls.push(preferences());
  run(user, calls);
}

function partner(user: User, writes: boolean, seed: number): void {
  const job = pick(user.jobIds, seed);
  const calls = [
    SESSION,
    JOBS,
    summary("partner"),
    get("members.list", `/v1/members?${LIST}`),
    UNITS,
    get("payouts.list", `/v1/payouts?${LIST}`),
  ];
  if (job) calls.push(get("jobs.get", `/v1/jobs/${job}`));
  if (writes) calls.push(preferences());
  run(user, calls);
}

function technician(user: User, writes: boolean): void {
  const calls = [SESSION, JOBS, ALERTS, summary("technician"), UNITS];
  if (writes) calls.push(preferences());
  run(user, calls);
}

function hq(user: User, writes: boolean): void {
  // HQ's dashboard and audit log ask for a period of whole minutes (SR17): the last 30 days
  const minute = 60_000;
  const to = Math.floor(Date.now() / minute) * minute;
  const period = `from=${encodeURIComponent(new Date(to - 30 * 86_400_000).toISOString())}&to=${encodeURIComponent(new Date(to).toISOString())}`;
  const calls = [
    SESSION,
    get("admin.summary", `/v1/admin/summary?${period}`),
    get("customers.list", `/v1/customers?${LIST}`),
    UNITS,
    JOBS,
    CONTRACTS,
    INVOICES,
    get("audit.list", `/v1/audit?${LIST}&${period}`),
  ];
  if (writes) calls.push(preferences());
  run(user, calls);
}

/** One visit of the user: the reads of their screens and, unless writes are off, their write. */
export function visit(user: User, writes: boolean, seed: number): void {
  switch (user.group) {
    case "customer": {
      customer(user, writes, seed);
      break;
    }
    case "partner": {
      partner(user, writes, seed);
      break;
    }
    case "technician": {
      technician(user, writes);
      break;
    }
    case "hq": {
      hq(user, writes);
      break;
    }
  }
}
