// The report at the end of a run: per kind of call (read, write, sign-in) the count, the error rate and the latency of the
// successful answers, then the status classes that were seen, so a good percentile can be checked against what was
// refused. k6 hands the data over as an object of its own; it is read here with checks, not assumed.
import { env } from "./env.ts";
import { phaseNames } from "./phase.ts";

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function metricValue(data: unknown, name: string, key: string): number | null {
  if (!isRecord(data)) return null;
  const metrics = data.metrics;
  if (!isRecord(metrics)) return null;
  const metric = metrics[name];
  if (!isRecord(metric)) return null;
  const values = metric.values;
  const value = isRecord(values) ? values[key] : metric[key];
  return typeof value === "number" ? value : null;
}

function show(value: number | null, digits = 0): string {
  return value === null ? "-" : value.toFixed(digits);
}

/** A window's figure, or a dash when nothing was measured in it (a sign-in happens once, at the start of a run). */
function inWindow(data: unknown, metric: string, key: string): string {
  return (metricValue(data, metric, "max") ?? 0) === 0 ? "-" : show(metricValue(data, metric, key), 1);
}

const KINDS = ["read", "write", "auth", "page", "signin"] as const;
const TRENDS = {
  read: "ac_read_duration",
  write: "ac_write_duration",
  auth: "ac_auth_duration",
  page: "ac_page_duration",
  signin: "ac_signin_duration",
} as const;
export const CLASSES = ["2xx", "3xx", "4xx", "5xx", "network"] as const;
export const WRITE_NAMES = ["jobs.create", "preferences.update"] as const;

/** One line per window of the run: how pages and sign-ins did in it. */
function windows(data: unknown): string[] {
  const names = phaseNames();
  if (names.length < 2) return [];
  const lines = ["", "window        pages p50 ms   p95 ms  sign-in p95 ms   errors%"];
  for (const name of names) {
    const errors = metricValue(data, `ac_errors{phase:${name}}`, "rate");
    lines.push(
      [
        name.padEnd(12),
        inWindow(data, `ac_page_duration{phase:${name}}`, "med").padStart(12),
        inWindow(data, `ac_page_duration{phase:${name}}`, "p(95)").padStart(8),
        inWindow(data, `ac_signin_duration{phase:${name}}`, "p(95)").padStart(15),
        show(errors === null ? null : errors * 100, 2).padStart(9),
      ].join(" "),
    );
  }
  return lines;
}

/** The text report of a run. */
export function report(data: unknown): string {
  const lines = ["", "kind    requests  errors%   p50 ms   p95 ms   p99 ms   max ms"];
  for (const kind of KINDS) {
    const trend = TRENDS[kind];
    if ((metricValue(data, `ac_requests{kind:${kind}}`, "count") ?? 0) === 0) continue; // this run made none of them
    const errors = metricValue(data, `ac_errors{kind:${kind}}`, "rate");
    lines.push(
      [
        kind.padEnd(7),
        show(metricValue(data, `ac_requests{kind:${kind}}`, "count")).padStart(8),
        show(errors === null ? null : errors * 100, 2).padStart(8),
        show(metricValue(data, trend, "med"), 1).padStart(8),
        show(metricValue(data, trend, "p(95)"), 1).padStart(8),
        show(metricValue(data, trend, "p(99)"), 1).padStart(8),
        show(metricValue(data, trend, "max"), 1).padStart(8),
      ].join(" "),
    );
  }
  const writeNames = (metricValue(data, "ac_requests{kind:write}", "count") ?? 0) === 0 ? [] : WRITE_NAMES;
  for (const name of writeNames) {
    const t = `ac_write_duration{name:${name}}`;
    lines.push(`  ${name.padEnd(20)} p50 ${show(metricValue(data, t, "med"), 1)} ms   p95 ${show(metricValue(data, t, "p(95)"), 1)} ms`);
  }
  lines.push(...windows(data));
  if (env.timeScale !== 1 || env.vuScale !== 1) {
    lines.push("", `SHORTENED RUN (TIME_SCALE ${String(env.timeScale)}, VU_SCALE ${String(env.vuScale)}): not a measurement of the stated scenario`);
  }
  lines.push(
    "",
    `status classes: ${CLASSES.map((c) => `${c} ${show(metricValue(data, `ac_status{class:${c}}`, "count"))}`).join("  ")}`,
    `rate limited (429): ${show(metricValue(data, "ac_rate_limited", "count"))}   error answers' median: ${show(metricValue(data, "ac_error_duration", "med"), 1)} ms`,
    `iterations: ${show(metricValue(data, "iterations", "count"))}   virtual users (max): ${show(metricValue(data, "vus_max", "max"))}`,
    "latency counts successful answers only; errors are in the error rate and are never averaged in.",
    "",
  );
  return lines.join("\n");
}
