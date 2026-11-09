// The command round trip (phase 3): a customer asks the air conditioner of one of their units to change its setpoint, and
// the time is taken until the command is acknowledged by the device. The path is the real one inside the load stack: the
// Core API stores the command, the bridge sends it to the device's topic, the simulated module answers, and the bridge
// reports the answer back. Meanwhile the telemetry generator (cmd/iotload) keeps the broker and the bridge busy.
//
// The time is taken from just before the request to the first poll that sees the command acknowledged; the poll every
// 200 ms makes the figure up to 200 ms too long. The bridge asks the API for requested commands every 2 seconds, which
// is most of what a command waits.
import { sleep } from "k6";
import { call } from "./api.ts";
import { record } from "./metrics.ts";
import type { User } from "./manifest.ts";

const POLL_MS = 200;
const GIVE_UP_MS = 20_000;
const SETPOINTS = [22, 23, 24, 25];
const NAME = "commands.roundtrip";

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function field(value: unknown, name: string): unknown {
  return isRecord(value) ? value[name] : undefined;
}

/** One command from request to acknowledgement; the outcome is recorded as kind `command`. */
export function commandRoundTrip(user: User, seed: number): void {
  const unit = user.unitIds[seed % Math.max(1, user.unitIds.length)];
  if (!unit) return;
  const current = call(user, { method: "GET", name: "units.get", path: `/v1/units/${unit}` });
  const version = field(current.data, "version");
  if (typeof version !== "number") {
    record("command", NAME, current.status, 0, false);
    return;
  }
  const started = Date.now();
  const created = call(user, {
    body: { action: { celsius: SETPOINTS[seed % SETPOINTS.length], kind: "set_temperature" }, expectedUnitVersion: version, unitId: unit },
    method: "POST",
    name: "commands.create",
    path: "/v1/commands",
  });
  const id = field(created.data, "id");
  if (!created.ok || typeof id !== "string") {
    record("command", NAME, created.status, Date.now() - started, false);
    return;
  }
  for (;;) {
    sleep(POLL_MS / 1000);
    const polled = call(user, { method: "GET", name: "commands.get", path: `/v1/commands/${id}` });
    const status = field(polled.data, "status");
    const elapsed = Date.now() - started;
    if (status === "acknowledged") {
      record("command", NAME, polled.status, elapsed, true);
      return;
    }
    // any other end (failed, expired, cancelled), a failed read, or no answer in time, is an error
    const ended = polled.ok && status !== "requested" && status !== "sent";
    if (ended || !polled.ok || elapsed > GIVE_UP_MS) {
      record("command", NAME, polled.status, elapsed, false);
      return;
    }
  }
}
