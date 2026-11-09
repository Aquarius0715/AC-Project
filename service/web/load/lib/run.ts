// What every scenario shares: the accounts, which one a virtual user is, the thresholds of backend-architecture §13, and
// the summary written at the end. The thresholds are the PROPOSED values of §13 (read p95 ≤ 300 ms, write p95 ≤ 600 ms,
// errors < 1%), measured here on the lab server, which is no guarantee of production performance.
import exec from "k6/execution";
import { env } from "./env.ts";
import { visit } from "./flows.ts";
import { markStart, phaseNames } from "./phase.ts";
import { webGroup, webVisit } from "./web.ts";
import { loadUsers, type User } from "./manifest.ts";
import { CLASSES, report, WRITE_NAMES } from "./summary.ts";

const users = loadUsers(env.manifest);

/** The account of the virtual user: one each, in the manifest's order (so the groups meet in its proportions); with more
virtual users than accounts the accounts are shared, and `balanced` takes one account of each group first (smoke). */
function accountOf(vu: number, balanced: boolean): User {
  if (balanced) {
    const groups = ["customer", "partner", "technician", "hq"];
    const group = groups[(vu - 1) % groups.length];
    const ofGroup = users.filter((u) => u.group === group);
    const found = ofGroup[Math.floor((vu - 1) / groups.length) % ofGroup.length];
    if (found) return found;
  }
  const found = users[(vu - 1) % users.length];
  if (!found) throw new Error("the manifest has no accounts");
  return found;
}

/** The accounts that have a web app: everyone but HQ, whose sign-in needs a one-time code. */
const webUsers = users.filter((u) => webGroup(u) !== null);

/** What setup() hands every scenario: when the test started, which the time windows are counted from. */
export interface Setup { startedAtMs: number }

/** Called once before the scenarios start. */
export function setup(): Setup {
  return { startedAtMs: Date.now() };
}

/** One iteration of a virtual user of a web scenario (the BFF and the server-rendered pages); `balanced` takes one account
of each kind first (smoke). */
export function webIteration(data: Setup, balanced: boolean): void {
  markStart(data.startedAtMs);
  const vu = exec.vu.idInTest;
  const kinds = ["customer", "partner", "technician"];
  const user = balanced
    ? webUsers.filter((u) => u.group === kinds[(vu - 1) % kinds.length])[Math.floor((vu - 1) / kinds.length)]
    : webUsers[(vu - 1) % webUsers.length];
  if (!user) throw new Error("the manifest has no account for this virtual user");
  webVisit(user, exec.vu.iterationInScenario);
}

/** One iteration of a virtual user. */
export function iteration(balanced: boolean): void {
  const user = accountOf(exec.vu.idInTest, balanced);
  visit(user, env.writes, exec.vu.iterationInScenario);
}

/** The §13 thresholds: any breach makes k6 exit non-zero. `rate_limited` and the status classes are there to show that
a good percentile is not made of refusals. */
export const thresholds: Record<string, string[]> = {
  ac_read_duration: ["p(95)<300"],
  ac_write_duration: ["p(95)<600"],
  ac_errors: ["rate<0.01"],
  ac_rate_limited: ["count==0"],
  ...watched(),
};

/** Sub-metrics with no limit: naming them in the thresholds makes k6 keep them, so the report can split the counts. */
export function watched(): Record<string, string[]> {
  const out: Record<string, string[]> = {};
  for (const kind of ["read", "write", "auth"]) {
    out[`ac_requests{kind:${kind}}`] = [];
    out[`ac_errors{kind:${kind}}`] = [];
  }
  for (const c of CLASSES) out[`ac_status{class:${c}}`] = [];
  for (const kind of ["page", "signin"]) {
    out[`ac_requests{kind:${kind}}`] = [];
    out[`ac_errors{kind:${kind}}`] = [];
  }
  for (const phase of phaseNames()) {
    out[`ac_page_duration{phase:${phase}}`] = [];
    out[`ac_signin_duration{phase:${phase}}`] = [];
    out[`ac_errors{phase:${phase}}`] = [];
  }
  for (const name of WRITE_NAMES) out[`ac_write_duration{name:${name}}`] = [];
  return out;
}

/** Writes the report to the console and the whole data to /out/summary.json (the workflow keeps it). */
export function handleSummary(data: unknown): Record<string, string> {
  return { stdout: report(data), "/out/summary.json": JSON.stringify(data, null, 2) };
}
