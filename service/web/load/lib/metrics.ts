// What a run measures. The latency trends hold only the answers that succeeded: an error is usually fast (a refusal
// takes a few milliseconds), so counting it in would make the percentiles look better the more the system fails. Errors
// are counted on their own, by kind and status class, and the thresholds fail the run on any of them above the limit.
import { Counter, Rate, Trend } from "k6/metrics";
import { currentPhase } from "./phase.ts";

export type Kind = "read" | "write" | "auth" | "page" | "signin" | "command";

const duration: Record<Kind, Trend> = {
  read: new Trend("ac_read_duration", true),
  write: new Trend("ac_write_duration", true),
  auth: new Trend("ac_auth_duration", true),
  page: new Trend("ac_page_duration", true),
  signin: new Trend("ac_signin_duration", true),
  command: new Trend("ac_command_duration", true),
};
const requests = new Counter("ac_requests");
const errors = new Rate("ac_errors");
const rateLimited = new Counter("ac_rate_limited");
const failedDuration = new Trend("ac_error_duration", true);
const statusClass = new Counter("ac_status");

function classOf(status: number): string {
  return status === 0 ? "network" : `${String(Math.floor(status / 100))}xx`;
}

/** Records one answer: its kind and name, the status, the time it took, and whether the body was the expected one. */
export function record(kind: Kind, name: string, status: number, ms: number, valid: boolean): void {
  const ok = status >= 200 && status < 300 && valid;
  requests.add(1, { kind });
  statusClass.add(1, { kind, class: classOf(status) });
  const phase = currentPhase();
  errors.add(!ok, { kind, phase });
  if (ok) {
    duration[kind].add(ms, { name, phase });
    return;
  }
  failedDuration.add(ms, { kind });
  if (status === 429) rateLimited.add(1, { kind });
}
