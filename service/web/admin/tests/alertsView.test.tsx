// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import type { ComponentProps, ReactNode } from "react";
import { adminAlertRows, policyRows, type AdminPolicy, type ApiAdminAlert } from "@ac/web/lib/adminAlerts";
import type { ApiEvidenceCandidate } from "@ac/web/lib/alertEvidence";
import { scopeLists } from "@ac/web/lib/scopeLists";

// The HQ alerts tab (FR-A05, DD-A05 items 5–6): the status tiles, severity and selection in the URL; acknowledge and
// resolve with a reason and the evidence the alert needs (IR327); the tabs keep the scope; "+ New policy" asks for the
// owner unless one customer is in scope. The policies tab itself is its own test.
const actions = vi.hoisted(() => ({ acknowledgeAlert: vi.fn<(...args: unknown[]) => Promise<unknown>>(), resolveAlert: vi.fn<(...args: unknown[]) => Promise<unknown>>() }));
const nav = vi.hoisted(() => ({ patch: vi.fn<(p: Record<string, string | null>) => void>(), replace: vi.fn<(href: string, options?: unknown) => void>(), toasts: [] as string[] }));
const op = vi.hoisted(() => {
  interface Op { data: ApiEvidenceCandidate[]; loading: boolean; error: Error | null; enabled: boolean[]; invalidate: ReturnType<typeof vi.fn>; noClock: boolean }
  const state: Op = { data: [], loading: false, error: null, enabled: [], invalidate: vi.fn(), noClock: false };
  return state;
});
const tabs = vi.hoisted(() => {
  const state: { props: unknown } = { props: null };
  return state;
});
vi.mock("../app/admin/alerts/actions", () => actions);
vi.mock("@ac/web/lib/useUrlPatch", () => ({ useUrlPatch: () => nav.patch }));
vi.mock("next/navigation", async (importOriginal) => ({ ...(await importOriginal<Record<string, unknown>>()), useRouter: () => ({ replace: nav.replace, refresh: vi.fn() }), usePathname: () => "/admin/alerts" }));
vi.mock("@ac/web/lib/useRefreshThen", () => ({ useRefreshThen: () => (then: () => void) => { then(); } }));
vi.mock("@ac/web/lib/useOp", () => ({
  useNow: () => (op.noClock ? null : new Date("2026-09-14T01:30:00Z")),
  invalidate: op.invalidate,
  useOp: (_operation: string, _input: unknown, mock: ApiEvidenceCandidate[], map: (d: { items: ApiEvidenceCandidate[] }) => ApiEvidenceCandidate[], enabled: boolean) => {
    op.enabled.push(enabled);
    return enabled && op.data.length > 0 ? { data: map({ items: op.data }), loading: op.loading, error: op.error } : { data: mock, loading: enabled && op.loading, error: enabled ? op.error : null };
  },
}));
vi.mock("@ac/web/components/ui", async (importOriginal) => ({ ...(await importOriginal<Record<string, unknown>>()), useToast: () => (m: string) => { nav.toasts.push(m); } }));
vi.mock("../app/admin/jobs/_components/jobs-header", () => ({ ScopeBar: ({ text, children }: Readonly<{ text: string; children?: ReactNode }>) => <div>{text}{children}</div> }));
vi.mock("../app/admin/alerts/_components/policies-tab", () => ({
  seedPolicies: [{ id: "pol-demo", name: "Demo policy" }], demoCustomers: [{ id: "customer-a", name: "customer-a" }],
  PoliciesTab: (props: Readonly<{ sel: unknown; onCopy: (ruleKey: string) => void; select: (id: string | null) => void; showOff: boolean }>) => {
    tabs.props = props;
    return <div><span>policies tab</span><button type="button" onClick={() => { props.onCopy("high_temperature"); }}>copy rule</button><button type="button" onClick={() => { props.select("pol-1"); }}>pick policy</button></div>;
  },
  NewPolicyModal: ({ onStart, onClose }: Readonly<{ onStart: (id: string) => void; onClose: () => void }>) => <div role="dialog" aria-label="owner"><button type="button" onClick={() => { onStart("cust-a"); }}>start for A</button><button type="button" onClick={onClose}>close owner</button></div>,
}));

import { AdminAlertsView } from "../app/admin/alerts/_components/alerts-view";

type Live = NonNullable<ComponentProps<typeof AdminAlertsView>["live"]>;
const NOW = Date.parse("2026-09-14T01:30:00Z");
const units = [{ id: "u-1", displayName: "Bedroom AC", customerOrgId: "org-a", propertyId: "p-1" }, { id: "u-9", displayName: "Stray AC", customerOrgId: "org-z", propertyId: "p-9" }];
const customers = [{ id: "cust-a", name: "Demo Customer A", organizationId: "org-a" }];
const alert = (id: string, over: Partial<ApiAdminAlert> = {}): ApiAdminAlert => ({
  id, version: 2, unitId: "u-1", policyId: null, type: "sensor", severity: "warning", status: "open", causeCode: "window_open", evidenceKind: "inferred", evidenceText: "cooling load rose",
  observedAt: "2026-09-14T00:50:00Z", evidenceIds: [], detectedAt: "2026-09-14T00:55:00Z", acknowledgedAt: null, resolvedAt: null, resolutionReason: null, ...over,
});
function liveOf(alerts: ApiAdminAlert[], over: Partial<Live> = {}, q: Partial<Live["q"]> = {}): Live {
  const query = { severity: null, status: "open" as const, alertId: null, ...q };
  return {
    tab: "alerts", q: query, scope: scopeLists(customers, [{ id: "p-1", name: "Home A", customerOrgId: "org-a" }], units, query), scopeName: null,
    alerts: adminAlertRows(alerts, units, customers, [], NOW), policies: [] satisfies AdminPolicy[],
    pol: { policyId: null, creatingFor: null, from: null, canWrite: true, timezone: "Asia/Kuala_Lumpur", customers: [{ id: "cust-a", name: "Demo Customer A" }], units: [] }, ...over,
  };
}
const candidate = (id: string): ApiEvidenceCandidate => ({ id, kind: "remeasurement", observedAt: "2026-09-14T01:20:00Z", metric: "temperature", value: 26.4, unit: "°C", origin: "measured", quality: "valid", eventType: null });
const button = (name: string | RegExp) => screen.getByRole<HTMLButtonElement>("button", { name });
const flush = () => new Promise((r) => { setTimeout(r, 0); });

beforeEach(() => {
  nav.patch.mockReset(); nav.replace.mockReset(); nav.toasts = [];
  op.data = []; op.loading = false; op.error = null; op.enabled = []; op.invalidate.mockReset(); op.noClock = false;
  actions.acknowledgeAlert.mockReset(); actions.resolveAlert.mockReset();
  history.replaceState(null, "", "/admin/alerts");
});
afterEach(cleanup);

describe("the HQ alerts tab in API mode", () => {
  it("counts each status, puts the status, severity and selection in the URL, and opens the first alert", () => {
    render(<AdminAlertsView live={liveOf([alert("al-1"), alert("al-2", { severity: "critical", causeCode: "unknown" }), alert("al-ack", { status: "acknowledged", acknowledgedAt: "2026-09-14T01:00:00Z" })])} />);
    expect(screen.getByRole("button", { name: /^Open\s*2/ }).getAttribute("aria-pressed")).toBe("true");
    expect(screen.getByText("3 alerts in scope")).toBeDefined();
    expect(screen.getByText("Open alerts · all customers")).toBeDefined();
    fireEvent.click(button(/^Acknowledged\s*1/));
    expect(nav.patch).toHaveBeenLastCalledWith({ status: "acknowledged", alertId: null });
    fireEvent.click(button(/^Open\s*2/));
    expect(nav.patch).toHaveBeenLastCalledWith({ status: null, alertId: null }); // open is no key
    fireEvent.change(screen.getByLabelText("Severity"), { target: { value: "attention" } });
    expect(nav.patch).toHaveBeenLastCalledWith({ severity: "attention", alertId: null });
    fireEvent.change(screen.getByLabelText("Severity"), { target: { value: "" } });
    expect(nav.patch).toHaveBeenLastCalledWith({ severity: null, alertId: null });
    const list = screen.getAllByRole("button").filter((b) => b.getAttribute("aria-pressed") !== null && b.textContent.includes("AC"));
    fireEvent.click(list.at(-1) ?? document.body);
    expect(nav.patch).toHaveBeenLastCalledWith({ alertId: "al-1" }); // the critical one sorts first, al-1 last
    expect(screen.getByRole("link", { name: "Request maintenance" }).getAttribute("href")).toBe(`/admin/jobs?new=u-1&alertId=al-2`);
    expect(screen.getByRole("link", { name: /Open unit/ }).getAttribute("href")).toBe("/admin/units?customerId=cust-a&unitId=u-1");
    expect(screen.getByText(/No policy escalation/)).toBeDefined();
  });

  it("opens the URL's alert, says when the scope has none, and keeps the scope when switching tabs", () => {
    const { rerender } = render(<AdminAlertsView live={liveOf([alert("al-1"), alert("al-2")], { scopeName: "Demo Customer A" }, { alertId: "al-2", customerId: "cust-a" })} />);
    expect(screen.getAllByText("Possible open window")).toHaveLength(3);
    expect(screen.getByText(/^al-2 · detected/)).toBeDefined();
    fireEvent.click(screen.getByRole("tab", { name: /Policies/ }));
    expect(nav.replace).toHaveBeenLastCalledWith("/admin/alerts?customerId=cust-a&tab=policies", { scroll: false });
    rerender(<AdminAlertsView live={liveOf([alert("al-r", { status: "resolved", resolvedAt: "2026-09-14T01:10:00Z", resolutionReason: "closed" })], {}, { status: "open" })} />);
    expect(screen.getByText("No alerts in this scope")).toBeDefined();
    expect(screen.getByText("No alert selected")).toBeDefined();
    rerender(<AdminAlertsView live={liveOf([alert("al-1")], { tab: "policies" })} />);
    fireEvent.click(screen.getByRole("tab", { name: /Alerts/ }));
    expect(nav.replace).toHaveBeenLastCalledWith("/admin/alerts", { scroll: false });
  });

  it("acknowledges an open alert at its version, and says why a refusal failed", async () => {
    actions.acknowledgeAlert.mockResolvedValueOnce({ ok: true }).mockResolvedValueOnce({ ok: false, code: "CONFLICT", messageKey: "error.conflict", fieldErrors: {} });
    render(<AdminAlertsView live={liveOf([alert("al-1")])} />);
    fireEvent.click(button("Acknowledge"));
    await flush();
    expect(actions.acknowledgeAlert).toHaveBeenCalledWith("al-1", 2);
    expect(nav.toasts).toStrictEqual(["Acknowledged"]);
    fireEvent.click(button("Acknowledge"));
    // the refusal's message: it takes more than one macrotask on a loaded CI runner (PR #684's run failed here)
    await waitFor(() => { expect(nav.toasts).toHaveLength(2); });
    expect(nav.toasts[1]).not.toBe("Acknowledged");
  });

  it("resolves only with a reason and, for an alert without a policy, at least one evidence record", async () => {
    op.data = [candidate("ev-1"), candidate("ev-2")];
    actions.resolveAlert.mockResolvedValue({ ok: true });
    render(<AdminAlertsView live={liveOf([alert("al-1")])} />);
    fireEvent.click(button("Resolve…"));
    const dialog = within(screen.getByRole("dialog"));
    fireEvent.click(dialog.getByRole("button", { name: "Resolve alert" }));
    expect(dialog.getByText("A reason is required")).toBeDefined();
    expect(dialog.getByText("Pick at least one evidence record — this alert has no policy")).toBeDefined();
    fireEvent.change(dialog.getByLabelText(/^Resolution reason/), { target: { value: "x".repeat(1001) } });
    fireEvent.click(dialog.getByRole("button", { name: "Resolve alert" }));
    expect(dialog.getByText("At most 1000 characters")).toBeDefined();
    fireEvent.change(dialog.getByLabelText(/^Resolution reason/), { target: { value: " Window closed " } });
    const [first] = dialog.getAllByRole("checkbox");
    fireEvent.click(first ?? document.body);
    fireEvent.click(dialog.getByRole("button", { name: "Resolve alert" }));
    await flush();
    expect(actions.resolveAlert).toHaveBeenCalledWith("al-1", 2, "Window closed", ["ev-1"]);
    expect(nav.toasts).toStrictEqual(["Alert resolved"]);
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(op.enabled.at(-1)).toBe(false); // the candidates are read only while the dialog is open
  });

  it("re-reads the candidates when the evidence is refused, and says when they cannot load", async () => {
    op.data = [candidate("ev-1")];
    actions.resolveAlert.mockResolvedValue({ ok: false, code: "VALIDATION", messageKey: "error.validation", fieldErrors: { resolutionEvidenceIds: "evidence.not_candidate" } });
    render(<AdminAlertsView live={liveOf([alert("al-1", { policyId: "pol-1" })])} />);
    fireEvent.click(button("Resolve…"));
    const dialog = within(screen.getByRole("dialog"));
    fireEvent.change(dialog.getByLabelText(/^Resolution reason/), { target: { value: "Back to normal" } });
    fireEvent.click(dialog.getByRole("button", { name: "Resolve alert" })); // a policy alert needs no evidence
    await flush();
    expect(actions.resolveAlert).toHaveBeenCalledWith("al-1", 2, "Back to normal", []);
    expect(op.invalidate).toHaveBeenCalledOnce();
    expect(screen.getByRole("dialog")).toBeDefined(); // stays open to pick again
    fireEvent.click(dialog.getByRole("button", { name: "Cancel" }));
    cleanup();
    op.error = new Error("down"); op.data = [];
    render(<AdminAlertsView live={liveOf([alert("al-1")])} />);
    fireEvent.click(button("Resolve…"));
    expect(screen.getByText("The evidence could not be loaded — close the dialog and try again.")).toBeDefined();
    fireEvent.keyDown(screen.getByRole("dialog"), { key: "Escape" });
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("offers nothing to change on a resolved alert, and links a unit without a known customer by its ID", () => {
    render(<AdminAlertsView live={liveOf([alert("al-r", { unitId: "u-9", status: "resolved", resolvedAt: "2026-09-14T01:10:00Z", resolutionReason: "closed" })], {}, { status: "resolved" })} />);
    expect(screen.getByRole("link", { name: /Open unit/ }).getAttribute("href")).toBe("/admin/units?unitId=u-9");
    expect(button("Acknowledge").disabled).toBe(true);
    expect(button("Resolve…").disabled).toBe(true);
    expect(screen.queryByRole("link", { name: "Request maintenance" })).toBeNull();
  });
});

describe("new policies from the alerts screen", () => {
  it("asks for the owner, or opens the scoped customer's draft at once", () => {
    op.noClock = true; // before the session answers, the evidence times use the browser's clock
    const { rerender } = render(<AdminAlertsView live={liveOf([])} />);
    fireEvent.click(button(/New policy/));
    fireEvent.click(button("close owner"));
    expect(screen.queryByRole("dialog", { name: "owner" })).toBeNull();
    fireEvent.click(button(/New policy/));
    fireEvent.click(button("start for A"));
    expect(nav.replace).toHaveBeenLastCalledWith("/admin/alerts?tab=policies&new=cust-a", { scroll: false });
    rerender(<AdminAlertsView live={liveOf([], {}, { customerId: "cust-a" })} />);
    fireEvent.click(button("New policy for Demo Customer A"));
    expect(nav.replace).toHaveBeenLastCalledWith("/admin/alerts?customerId=cust-a&tab=policies&new=cust-a", { scroll: false });
    rerender(<AdminAlertsView live={liveOf([], {}, { customerId: "cust-inactive" })} />);
    expect(button("New policy")).toBeDefined(); // a customer that takes no new policy is asked for again
    rerender(<AdminAlertsView live={liveOf([], { pol: { ...liveOf([]).pol, canWrite: false } })} />);
    expect(screen.queryByRole("button", { name: /New policy/ })).toBeNull();
  });

  it("copies a default rule into a new policy for the scoped customer or the one asked for, and names the policies of a scoped unit", () => {
    const pol = policyRows([{
      id: "pol-1", version: 2, kind: "alert", name: "Bedroom too hot", unitIds: ["u-1"], timezone: "Asia/Kuala_Lumpur", enabled: true, priority: 50, customerId: "cust-a", metric: "temperature",
      operator: "gte", threshold: 30, recoveryThreshold: 28, durationSeconds: 60, severity: "warning",
    }], customers, []);
    const { rerender } = render(<AdminAlertsView live={liveOf([], { tab: "policies", policies: pol }, { unitId: "u-1", customerId: "cust-a" })} />);
    expect(screen.getByText("Showing the policies attached to Bedroom AC: Bedroom too hot.")).toBeDefined();
    expect(screen.getByText("1 policy in scope")).toBeDefined();
    fireEvent.click(button("copy rule"));
    expect(nav.replace).toHaveBeenLastCalledWith("/admin/alerts?customerId=cust-a&unitId=u-1&tab=policies&new=cust-a&from=high_temperature", { scroll: false });
    fireEvent.click(button("pick policy"));
    expect(nav.patch).toHaveBeenLastCalledWith({ policyId: "pol-1", new: null, from: null });
    fireEvent.click(screen.getByRole("checkbox", { name: "Show disabled" }));
    expect(tabs.props).toMatchObject({ showOff: false });
    rerender(<AdminAlertsView live={liveOf([], { tab: "policies" }, { unitId: "u-gone" })} />);
    expect(screen.getByText("Showing the policies attached to u-gone: .")).toBeDefined(); // an unknown unit by its ID
    fireEvent.click(button("copy rule"));
    fireEvent.click(button("start for A"));
    expect(nav.replace).toHaveBeenLastCalledWith("/admin/alerts?unitId=u-gone&tab=policies&new=cust-a&from=high_temperature", { scroll: false });
  });
});

describe("the HQ alerts demo", () => {
  it("acknowledges and resolves its fixture alerts locally, and switches tabs and status in place", () => {
    op.data = [];
    render(<AdminAlertsView />);
    expect(screen.getByText("High temperature observed", { selector: "b" })).toBeDefined();
    fireEvent.click(button("Acknowledge"));
    expect(nav.toasts).toStrictEqual(["Acknowledged"]);
    fireEvent.click(button(/^Acknowledged\s*1/));
    expect(screen.getByText("Acknowledged — waiting for resolution")).toBeDefined();
    fireEvent.click(button("Resolve…"));
    const dialog = within(screen.getByRole("dialog"));
    fireEvent.change(dialog.getByLabelText(/^Resolution reason/), { target: { value: "Checked on site" } });
    const [first] = dialog.getAllByRole("checkbox");
    fireEvent.click(first ?? document.body);
    fireEvent.click(dialog.getByRole("button", { name: "Resolve alert" }));
    expect(nav.toasts.at(-1)).toBe("Alert resolved");
    fireEvent.click(button(/^Resolved\s*1/));
    expect(screen.getAllByText("Resolved").length).toBeGreaterThan(0);
    fireEvent.click(button(/^Open\s*4/));
    fireEvent.click(button(/Poor insulation suspected/));
    expect(screen.getByText(/^alert-insulation-a · detected/)).toBeDefined();
    expect(nav.patch).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("tab", { name: /Policies/ }));
    expect(screen.getByText("policies tab")).toBeDefined();
    fireEvent.click(button("pick policy"));
    expect(tabs.props).toMatchObject({ sel: { policyId: "pol-1", creatingFor: null, from: null } });
    fireEvent.click(button(/New policy/));
    fireEvent.click(button("start for A"));
    expect(tabs.props).toMatchObject({ sel: { policyId: null, creatingFor: "cust-a", from: null } });
    expect(nav.replace).not.toHaveBeenCalled();
  });
});
