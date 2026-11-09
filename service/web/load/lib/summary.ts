// The report at the end of a run: per kind of call (read, write, sign-in) the count, the error rate and the latency of the
// successful answers, then the status classes that were seen, so a good percentile can be checked against what was
// refused. k6 hands the data over as an object of its own; it is read here with checks, not assumed.
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

const KINDS = ["read", "write", "auth"] as const;
const TRENDS = { read: "ac_read_duration", write: "ac_write_duration", auth: "ac_auth_duration" } as const;
export const CLASSES = ["2xx", "3xx", "4xx", "5xx", "network"] as const;
export const WRITE_NAMES = ["jobs.create", "preferences.update"] as const;

/** The text report of a run. */
export function report(data: unknown): string {
  const lines = ["", "kind    requests  errors%   p50 ms   p95 ms   p99 ms   max ms"];
  for (const kind of KINDS) {
    const trend = TRENDS[kind];
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
  for (const name of WRITE_NAMES) {
    const t = `ac_write_duration{name:${name}}`;
    lines.push(`  ${name.padEnd(20)} p50 ${show(metricValue(data, t, "med"), 1)} ms   p95 ${show(metricValue(data, t, "p(95)"), 1)} ms`);
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
