// web: the launch size through the web apps. 285 virtual users, one account each (the 300 accounts less HQ's 15, whose
// sign-in needs a one-time code), sign in once and then move through their pages for 15 minutes. Section 13 names no target
// for the web layer, so only the error rate is held (below 1%); page and sign-in times are recorded to be read.
import { configurePhases } from "../lib/phase.ts";
import { minutes, minutesMs, vus } from "../lib/scale.ts";
import { watched, webIteration, type Setup } from "../lib/run.ts";

configurePhases([
  { name: "ramp", untilMs: minutesMs(2) },
  { name: "steady", untilMs: minutesMs(15) },
]);

export const options = {
  noCookiesReset: true, // a virtual user is one browser: its session cookie survives the iterations
  scenarios: {
    web: {
      executor: "ramping-vus",
      startVUs: 0,
      stages: [
        { duration: minutes(2), target: vus(285) },
        { duration: minutes(13), target: vus(285) },
      ],
      gracefulRampDown: "30s",
    },
  },
  thresholds: { ...watched(), ac_errors: ["rate<0.01"] },
};

export { handleSummary, setup } from "../lib/run.ts";

export default function web(data: Setup): void {
  webIteration(data, false);
}
