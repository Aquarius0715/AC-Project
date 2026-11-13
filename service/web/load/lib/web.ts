// What a person does in a browser, as far as the server can tell (phase 2): sign in through the app's own sign-in step and
// the identity provider's page (OIDC authorization code with PKCE, the BFF's session cookie), then open the app's main
// pages. Only the HTML document of each page is fetched: the scripts and styles a browser also loads are static files that
// a CDN serves in production, and no script runs, so what is measured is the BFF and the server-side rendering with the
// Core API calls behind them.
import http from "k6/http";
import { sleep } from "k6";
import { env } from "./env.ts";
import { record } from "./metrics.ts";
import type { User } from "./manifest.ts";

type WebGroup = "customer" | "partner" | "technician";

const HOME: Record<WebGroup, string> = { customer: "/customer", partner: "/partner", technician: "/technician" };

// the pages of each app that its menu leads to (docs/03-uiux/screen-catalog.csv)
const PAGES: Record<WebGroup, string[]> = {
  customer: ["/customer", "/customer/properties", "/customer/alerts", "/customer/maintenance", "/customer/payments", "/customer/energy"],
  partner: ["/partner", "/partner/jobs", "/partner/schedule", "/partner/team", "/partner/payouts"],
  technician: ["/technician", "/technician/devices"],
};

let signedIn = false;
let complaints = 0;
const MAX_LOGGED = 5; // failed calls a virtual user writes to the log: enough to see what fails, not a flood

function complain(message: string): void {
  if (complaints >= MAX_LOGGED) return;
  complaints += 1;
  console.error(`failed ${message}`);
}

/** Whether the user has a web app at all: HQ's sign-in needs a one-time code, which a load script does not have. */
export function webGroup(user: User): WebGroup | null {
  return user.group === "hq" ? null : user.group;
}

function baseOf(group: WebGroup): string {
  switch (group) {
    case "customer": {
      return env.customerUrl;
    }
    case "partner": {
      return env.partnerUrl;
    }
    case "technician": {
      return env.technicianUrl;
    }
  }
}

function think(): void {
  sleep(2 + Math.random() * 3);
}

/** Signs in as the user through the app and the identity provider; true when the app's home page came back. */
function signIn(user: User, group: WebGroup): boolean {
  const base = baseOf(group);
  const email = `${user.username}@load.local`;
  const started = Date.now();
  let status = 0;
  let ok: boolean;
  try {
    const login = http.get(`${base}/bff/auth/login?returnTo=${encodeURIComponent(HOME[group])}`, {
      responseType: "text",
      tags: { name: "sign-in start", kind: "signin" },
    });
    const back = login.submitForm({
      formSelector: "#kc-form-login",
      fields: { username: email, password: user.password },
      params: { responseType: "text", tags: { name: "sign-in password", kind: "signin" } },
    });
    status = back.status;
    ok = back.status === 200 && back.url.startsWith(`${base}${HOME[group]}`);
  } catch {
    ok = false; // the identity provider's form was not there
  }
  record("signin", "sign-in", status, Date.now() - started, ok);
  if (!ok) complain(`sign-in as ${user.group} ${user.username} -> ${String(status)}`);
  signedIn = ok;
  return ok;
}

/** Opens the app's pages one after the other, the way a person moves through the menu. */
function openPages(user: User, group: WebGroup, seed: number): void {
  const base = baseOf(group);
  const pages = [...PAGES[group]];
  const unit = user.unitIds[seed % Math.max(1, user.unitIds.length)];
  const job = user.jobIds[seed % Math.max(1, user.jobIds.length)];
  if (group === "customer" && unit) pages.push(`/customer/units/${unit}`);
  if (group === "partner" && job) pages.push(`/partner/jobs/${job}`);
  for (const path of pages) {
    const res = http.get(`${base}${path}`, { responseType: "text", tags: { name: path.replaceAll(/[0-9a-f-]{36}/g, ":id"), kind: "page" } });
    const name = path.replaceAll(/[0-9a-f-]{36}/g, ":id");
    // a page that sends the person back to sign in is a failure: the session is gone
    const ok = res.status === 200 && !res.url.includes("/login");
    record("page", name, res.status, res.timings.duration, ok);
    if (!ok) {
      complain(`page ${path} as ${user.group} -> ${String(res.status)} at ${res.url}`);
      signedIn = false;
      return;
    }
    think();
  }
}

/** One visit: sign in when there is no session, then open the pages. */
export function webVisit(user: User, seed: number): void {
  const group = webGroup(user);
  if (group === null) return;
  if (!signedIn && !signIn(user, group)) {
    sleep(5); // not signed in: wait a little before the next try instead of hammering the identity provider
    return;
  }
  openPages(user, group, seed);
}
