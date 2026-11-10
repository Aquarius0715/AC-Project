// spike: the start of a working day. 60 people are already working; at minute 3, 225 more sign in within seconds (all
// the accounts that have a web app), then keep working. The windows show how pages and sign-ins fare before the spike,
// during it, and for the minutes after: the time it takes to come back to the earlier speed is read from them. It only
// records; the error rate is the one threshold.
import { configurePhases } from "../lib/phase.ts";
import { minutes, minutesMs, vus } from "../lib/scale.ts";
import { watched, webIteration, type Setup } from "../lib/run.ts";

configurePhases([
  { name: "before", untilMs: minutesMs(3) },
  { name: "spike", untilMs: minutesMs(5) },
  { name: "recovery", untilMs: minutesMs(8) },
  { name: "after", untilMs: minutesMs(11) },
]);

export const options = {
  noCookiesReset: true, // a virtual user is one browser: its session cookie survives the iterations
  scenarios: {
    working: { executor: "constant-vus", vus: vus(60), duration: minutes(11), exec: "visit" },
    arriving: { executor: "constant-vus", vus: vus(225), duration: minutes(8), startTime: minutes(3), exec: "arrive" },
  },
  thresholds: { ...watched(), ac_errors: ["rate<0.05"] },
};

export { handleSummary, setup } from "../lib/run.ts";

/** Someone already at work: their pages again and again. */
export function visit(data: Setup): void {
  webIteration(data, false);
}

/** Someone arriving: their first visit signs in, all of them at the same moment; then they work like the others. */
export function arrive(data: Setup): void {
  webIteration(data, false);
}
