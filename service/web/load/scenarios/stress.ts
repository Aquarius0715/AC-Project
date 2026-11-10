// stress: from 100 to 1,500 virtual users in steps, to find where the latency and the error rate start to rise. It only
// records: the thresholds do not fail the run. It reads only (WRITES=0 is set by deploy/load/run.sh): with more virtual
// users than accounts the accounts are shared, and one membership may write 60 times a minute, so a refusal could not be
// told from a slowdown.
import { iteration, watched } from "../lib/run.ts";

export const options = {
  scenarios: {
    stress: {
      executor: "ramping-vus",
      startVUs: 0,
      stages: [
        { duration: "1m", target: 100 },
        { duration: "3m", target: 100 },
        { duration: "1m", target: 300 },
        { duration: "3m", target: 300 },
        { duration: "1m", target: 600 },
        { duration: "3m", target: 600 },
        { duration: "1m", target: 1000 },
        { duration: "3m", target: 1000 },
        { duration: "1m", target: 1500 },
        { duration: "3m", target: 1500 },
        { duration: "1m", target: 0 },
      ],
      gracefulRampDown: "30s",
    },
  },
  thresholds: watched(),
};

export default function stress(): void {
  iteration(false);
}

export { handleSummary } from "../lib/run.ts";
