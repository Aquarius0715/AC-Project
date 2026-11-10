// web-smoke: three virtual users (a customer, a partner, a technician) sign in through the web apps and open their pages for
// a minute. It checks the script and the web stack, and has to finish with no error at all.
import { watched, webIteration, type Setup } from "../lib/run.ts";

export const options = {
  noCookiesReset: true, // a virtual user is one browser: its session cookie survives the iterations
  scenarios: { webSmoke: { executor: "constant-vus", vus: 3, duration: "1m" } },
  thresholds: { ...watched(), ac_errors: ["rate==0"] },
};

export { handleSummary, setup } from "../lib/run.ts";

export default function webSmoke(data: Setup): void {
  webIteration(data, true);
}
