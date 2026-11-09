// iot-command: customers send setpoint commands to the air conditioners of their own units, one after the other, for as long
// as the telemetry generator runs (IOT_SECONDS), and the time from request to the device's acknowledgement is measured. The
// threshold is the PROPOSED target of backend-architecture §13: the round trip's p95 within 5 seconds for online devices.
// 20 virtual users make a few commands a second at most (each waits for its own, and the write limit is 60 a minute each).
import exec from "k6/execution";
import { env } from "../lib/env.ts";
import { commandRoundTrip } from "../lib/iot.ts";
import { loadUsers } from "../lib/manifest.ts";
import { vus } from "../lib/scale.ts";
import { watched } from "../lib/run.ts";
import { sleep } from "k6";

export { handleSummary } from "../lib/run.ts";

const customers = loadUsers(env.manifest).filter((u) => u.group === "customer");

export const options = {
  scenarios: { commands: { executor: "constant-vus", vus: vus(20), duration: `${String(env.iotSeconds)}s` } },
  thresholds: { ...watched(), ac_command_duration: ["p(95)<5000"], "ac_errors{kind:command}": ["rate<0.01"] },
};

export default function iotCommand(): void {
  const user = customers[(exec.vu.idInTest - 1) % customers.length];
  if (!user) throw new Error("the manifest has no customer account");
  commandRoundTrip(user, exec.vu.iterationInScenario);
  sleep(1 + Math.random() * 2);
}
