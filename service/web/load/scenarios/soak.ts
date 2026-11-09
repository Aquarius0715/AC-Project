// soak: 285 virtual users (one account each) work through the web apps for 2 hours. Each quarter of an hour is a window,
// so a rise in page time, in errors, or in what the run samples beside it (connections, memory and CPU of each container,
// postgres.csv) shows as a drift from the first window to the last. The session of each user is renewed by the BFF as the
// 5-minute access token ends. Only the error rate is a threshold.
import { configurePhases } from "../lib/phase.ts";
import { minutes, minutesMs, vus } from "../lib/scale.ts";
import { watched, webIteration, type Setup } from "../lib/run.ts";

const WINDOW_MIN = 15;
const WINDOWS = 8;

configurePhases(Array.from({ length: WINDOWS }, (_, i) => ({ name: `min ${String(i * WINDOW_MIN)}-${String((i + 1) * WINDOW_MIN)}`, untilMs: minutesMs((i + 1) * WINDOW_MIN) })));

export const options = {
  noCookiesReset: true, // a virtual user is one browser: its session cookie survives the iterations
  scenarios: {
    soak: {
      executor: "ramping-vus",
      startVUs: 0,
      stages: [
        { duration: minutes(5), target: vus(285) },
        { duration: minutes(115), target: vus(285) },
      ],
      gracefulRampDown: "30s",
    },
  },
  thresholds: { ...watched(), ac_errors: ["rate<0.01"] },
};

export { handleSummary, setup } from "../lib/run.ts";

export default function soak(data: Setup): void {
  webIteration(data, false);
}
