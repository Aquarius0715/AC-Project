// The four web apps under test (one per entry point, IR178), their demo identities and where each project keeps its
// signed-in browser state. E2E_<APP>_URL points a project at another host (default: the local ports).
import path from "node:path";

export type App = {
  id: "customer" | "partner" | "technician" | "admin";
  url: string;
  home: string;
  user: string; // the demo identity of the app's sign-in page (Figma Login frames)
  otp: boolean; // HQ accounts also enter a one-time code
  state: string;
};

const url = (name: string, port: number) => (process.env[name] ?? `http://localhost:${port}`).replace(/\/$/, "");
const state = (id: string) => path.resolve(__dirname, "../.auth", `${id}.json`);

export const APPS: App[] = [
  { id: "customer", url: url("E2E_CUSTOMER_URL", 3000), home: "/customer", user: "customer-a", otp: false, state: state("customer") },
  { id: "partner", url: url("E2E_PARTNER_URL", 3001), home: "/partner", user: "contractor-a", otp: false, state: state("partner") },
  { id: "technician", url: url("E2E_TECHNICIAN_URL", 3002), home: "/technician", user: "tech-internal-a", otp: false, state: state("technician") },
  { id: "admin", url: url("E2E_ADMIN_URL", 3003), home: "/admin", user: "hq-operator", otp: true, state: state("admin") },
];
