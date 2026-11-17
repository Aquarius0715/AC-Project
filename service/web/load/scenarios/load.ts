// load: the launch size of backend-architecture §13, 300 concurrent users for 15 minutes (one account each, in the
// manifest's proportions). The thresholds are §13's: read p95 ≤ 300 ms, write p95 ≤ 600 ms, errors < 1%.
import { iteration, thresholds } from "../lib/run.ts";
import { minutes, vus } from "../lib/scale.ts";

export const options = {
  scenarios: {
    load: {
      executor: "ramping-vus",
      startVUs: 0,
      stages: [
        { duration: minutes(2), target: vus(300) },
        { duration: minutes(13), target: vus(300) },
      ],
      gracefulRampDown: "30s",
    },
  },
  thresholds,
};

export default function load(): void {
  iteration(false);
}

export { handleSummary } from "../lib/run.ts";
