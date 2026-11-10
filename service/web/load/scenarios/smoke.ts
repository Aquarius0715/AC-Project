// smoke: a few virtual users for a minute, one account of each kind first. It checks the script and the stack, and has to
// finish with no error at all.
import { iteration, thresholds } from "../lib/run.ts";

export const options = {
  scenarios: { smoke: { executor: "constant-vus", vus: 4, duration: "1m" } },
  thresholds: { ...thresholds, ac_errors: ["rate==0"] },
};

export default function smoke(): void {
  iteration(true);
}

export { handleSummary } from "../lib/run.ts";
