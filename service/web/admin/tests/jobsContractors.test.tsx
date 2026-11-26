// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import type { ActionFailure } from "@ac/web/lib/actionMessage";

// The HQ contractor register (FR-A21, DD-A21, Figma Admin 06-9), from the data the loader builds: the list, a
// contractor's KPIs, profile, rate cards and technicians; Add contractor (an existing company or a new one created
// first, the profile kept to fix when it is refused), editing the profile, suspending and resuming offers, a new rate
// card version, deleting one that has not started, verifying certificate uploads, and deleting a contractor.
const dal = vi.hoisted(() => {
  interface DomainError { code: string; messageKey: string; fieldErrors: Record<string, string>; retryAfterSeconds: number | null }
  class CoreError extends Error {
    readonly error: DomainError;
    constructor(error: DomainError, options?: ErrorOptions) {
      super(error.code, options);
      this.name = "CoreError";
      this.error = error;
    }
  }
  return {
    CoreError, coreNow: vi.fn(() => Promise.resolve(new Date("2026-09-14T01:00:00Z"))), coreDisplay: vi.fn(() => Promise.resolve({ locale: "en", timeZone: "Asia/Kuala_Lumpur" })),
    corePermissions: vi.fn(() => Promise.resolve(new Set(["data.forceDelete"]))),
    coreOp: vi.fn<(op: string, input?: unknown) => Promise<unknown>>(), coreAll: vi.fn<(op: string, input?: unknown) => Promise<unknown>>(), coreItems: vi.fn<(op: string, input?: unknown) => Promise<unknown>>(),
    orWhenRefused: async <T, F>(read: Promise<T>, fallback: F): Promise<T | F> => { try { return await read; } catch { return fallback; } },
  };
});
const actions = vi.hoisted(() => {
  const fn = () => vi.fn<(...args: unknown[]) => Promise<unknown>>();
  return { createContractorOrganization: fn(), deleteContractor: fn(), deleteRateCard: fn(), saveContractor: fn(), saveRateCard: fn(), setOfferStatus: fn(), verifyCertificate: fn(), createJob: fn() };
});
const nav = vi.hoisted(() => ({ patch: vi.fn<(p: Record<string, string | null>) => void>(), push: vi.fn<(href: string) => void>(), refresh: vi.fn(), toasts: [] as string[] }));
vi.mock("server-only", () => ({}));
vi.mock("@ac/web/lib/dal", () => dal);
vi.mock("next/navigation", () => ({ useRouter: () => ({ push: nav.push, replace: vi.fn(), refresh: nav.refresh }), usePathname: () => "/admin/jobs", useSearchParams: () => new URLSearchParams() }));
vi.mock("../app/admin/jobs/actions", () => actions);
vi.mock("@ac/web/lib/useUrlPatch", () => ({ useUrlPatch: () => nav.patch }));
type Result = { ok: true; value: unknown } | ({ ok: false } & ActionFailure);
vi.mock("@ac/web/lib/useAction", () => ({
  useAction: () => [false, (fn: () => Promise<Result>, ok: string | ((v: unknown) => string), after?: (v: unknown) => void, onFail?: (f: ActionFailure) => void) => {
    void fn().then((r) => {
      if (!r.ok) { onFail?.(r); return; }
      nav.toasts.push(typeof ok === "string" ? ok : ok(r.value));
      after?.(r.value);
    });
  }] as const,
}));

import { loadContractors } from "../app/admin/jobs/_lib/load";
import { ContractorsView } from "../app/admin/jobs/_components/contractors-view";

const kpis = { period: { from: "2026-06-16T01:00:00Z", to: "2026-09-14T01:00:00Z" }, offerAcceptance: 92, arrivalInWindow: 96.2, firstTimeAccepted: 84, averageRating: 4.6, ratingCount: 12, reworkRate: 8 };
const profile = (over: Record<string, unknown> = {}) => ({
  id: "cp-1", version: 2, organizationId: "org-c", name: "Cool Partners", status: "active", registrationNo: "SSM 1", serviceAreas: ["Kuala Lumpur", "Selangor"], contactEmail: "ops@cool.example",
  insuranceValidUntil: "2027-01-31T00:00:00Z", delegation: { from: "2026-04-01T00:00:00Z", to: "2027-01-31T00:00:00Z" }, rateCardId: "rc-now", suspendedReason: null, createdAt: "2026-04-01T00:00:00Z",
  updatedAt: "2026-09-01T00:00:00Z", kpis, ...over,
});
const card = (id: string, version: number, effectiveFrom: string) => ({
  id, version, effectiveFrom, contractorOrgId: "org-c", currency: "MYR", createdAt: effectiveFrom,
  lines: [{ workType: "periodic_inspection", amountMinor: 8000, note: null }, { workType: "repair_base", amountMinor: 15_000, note: "Labour only" }],
});
const cert = (id: string, over: Record<string, unknown> = {}) => ({
  id, version: 1, membershipId: "m-ext", organizationId: "org-c", code: "demo_refrigerant", name: "Refrigerant handling", number: "N-1", issuedAt: "2026-01-01T00:00:00Z", expiresAt: "2027-01-01T00:00:00Z",
  fileName: "cert.pdf", status: "pending_verification", renewalOf: null, createdAt: "2026-09-10T00:00:00Z", ...over,
});
async function liveOf(sp: Record<string, string> = {}, w: { profiles?: unknown[]; cards?: unknown[]; certs?: unknown[]; orgs?: unknown[] } = {}) {
  const all = new Map<string, unknown>([
    ["organizations.list", w.orgs ?? [
      { id: "org-hq", version: 1, name: "HQ", kind: "operator", status: "active" }, { id: "org-c", version: 3, name: "Cool Partners", kind: "contractor", status: "active" },
      { id: "org-z", version: 1, name: "Zeta Cool", kind: "contractor", status: "active" },
    ]],
    ["customers.list", []], ["properties.list", []], ["units.list", [{ id: "u-1", displayName: "Lobby AC", customerOrgId: "org-a", propertyId: "p-1", installState: "installed" }]],
    ["members.list", [{ id: "m-ext", userId: "user-ext", displayName: "Ben", role: "technician", employment: "external", organizationId: "org-c", validUntil: "2027-01-01T00:00:00Z", qualifications: [] }]],
    ["contractors.list", w.profiles ?? [profile()]], ["rateCards.list", w.cards ?? [card("rc-next", 3, "2026-10-01T00:00:00Z"), card("rc-now", 2, "2026-08-01T00:00:00Z")]], ["certificates.list", w.certs ?? [cert("cert-1")]],
  ]);
  dal.coreAll.mockImplementation((op) => Promise.resolve(all.get(op) ?? []));
  dal.coreOp.mockImplementation(() => Promise.resolve({ items: [], total: 0 }));
  return loadContractors({ contractorId: "org-c", ...sp });
}
const button = (name: string | RegExp) => screen.getByRole<HTMLButtonElement>("button", { name });
const dialog = () => within(screen.getByRole("dialog"));
const flush = () => act(() => new Promise<void>((r) => { setTimeout(r, 0); }));
const ok = { ok: true, value: null };

beforeEach(() => {
  nav.patch.mockReset(); nav.push.mockReset(); nav.refresh.mockReset(); nav.toasts = [];
  for (const f of Object.values(actions)) f.mockReset();
});
afterEach(cleanup);

describe("a contractor", () => {
  it("shows the KPIs, the profile, the rate card in effect and the next one, and its technicians", async () => {
    render(<ContractorsView live={await liveOf()} />);
    expect(screen.getAllByRole("link").some((a) => a.getAttribute("href") === "/admin/jobs?tab=contractors&contractorId=org-z")).toBe(true);
    expect(screen.getByText(/^Rate card v2/)).toBeDefined();
    expect(screen.getByText("Labour only")).toBeDefined();
    expect(screen.getByText(/^Next: v3 from .* · scheduled — Periodic inspection/)).toBeDefined();
    expect(screen.getByText("Ben")).toBeDefined();
    expect(screen.getByText("1 renewal upload is waiting for verification.", { exact: false })).toBeDefined();
  });

  it("suspends offers with a reason, and resumes them for a suspended contractor", async () => {
    actions.setOfferStatus.mockResolvedValue(ok);
    render(<ContractorsView live={await liveOf()} />);
    fireEvent.click(button("Suspend offers"));
    fireEvent.click(dialog().getByRole("button", { name: "Suspend offers" }));
    expect(dialog().getByText("A reason is required")).toBeDefined();
    fireEvent.change(dialog().getByLabelText(/^Reason/), { target: { value: "Insurance expired" } });
    fireEvent.click(dialog().getByRole("button", { name: "Suspend offers" }));
    await flush();
    expect(actions.setOfferStatus).toHaveBeenCalledWith("org-c", 2, "suspended", "Insurance expired");
    expect(screen.getByText("Offers to Cool Partners suspended — open offers and jobs continue.")).toBeDefined();
    cleanup();
    render(<ContractorsView live={await liveOf({}, { profiles: [profile({ status: "suspended", suspendedReason: "Complaints" })] })} />);
    expect(screen.getByText(/^Offers suspended — “Complaints”\./)).toBeDefined();
    fireEvent.click(button("Resume offers"));
    fireEvent.change(dialog().getByLabelText(/^Reason/), { target: { value: "Fixed" } });
    fireEvent.click(dialog().getByRole("button", { name: "Resume offers" }));
    await flush();
    expect(actions.setOfferStatus).toHaveBeenLastCalledWith("org-c", 2, "active", "Fixed");
  });

  it("saves a new rate card version from a future Kuala Lumpur day, checking the lines", async () => {
    actions.saveRateCard.mockResolvedValue(ok);
    render(<ContractorsView live={await liveOf()} />);
    fireEvent.click(button("Edit rate card"));
    expect(dialog().getByText(/^New rate card for Cool Partners · v4$/)).toBeDefined();
    fireEvent.change(dialog().getByLabelText(/^Effective from/), { target: { value: "2026-09-01" } });
    fireEvent.change(dialog().getByLabelText("Amount 1"), { target: { value: "80.123" } });
    fireEvent.click(dialog().getByRole("button", { name: "Save rate card" }));
    expect(dialog().getByText("Effective from a future date.")).toBeDefined();
    expect(dialog().getByText("Amounts are non-negative with at most two decimals.")).toBeDefined();
    fireEvent.change(dialog().getByLabelText(/^Effective from/), { target: { value: "2026-11-01" } });
    fireEvent.change(dialog().getByLabelText("Amount 1"), { target: { value: "85" } });
    const ticks = dialog().getAllByRole("checkbox");
    fireEvent.click(ticks[3] ?? document.body); // add the rework deduction
    fireEvent.change(dialog().getByLabelText("Amount 4"), { target: { value: "20" } });
    fireEvent.change(dialog().getByLabelText("Note 4"), { target: { value: " 2nd return " } });
    fireEvent.change(dialog().getByLabelText(/^Currency/), { target: { value: "USD" } });
    fireEvent.click(dialog().getByRole("button", { name: "Save rate card" }));
    await flush();
    expect(actions.saveRateCard).toHaveBeenCalledWith({
      contractorOrgId: "org-c", effectiveFrom: "2026-10-31T16:00:00.000Z", currency: "USD",
      lines: [{ workType: "periodic_inspection", amountMinor: 8500, note: null }, { workType: "repair_base", amountMinor: 15_000, note: "Labour only" }, { workType: "rework_deduction", amountMinor: 2000, note: "2nd return" }],
    });
    expect(screen.getByText(/^Rate card v4 saved — effective from/)).toBeDefined();
  });

  it("deletes a rate card that has not started, with a reason", async () => {
    actions.deleteRateCard.mockResolvedValue(ok);
    render(<ContractorsView live={await liveOf()} />);
    fireEvent.click(button("Delete rate card…"));
    fireEvent.click(dialog().getByRole("button", { name: "Delete" }));
    expect(dialog().getByText("A reason is required")).toBeDefined();
    fireEvent.change(dialog().getByLabelText(/^Reason/), { target: { value: "Typo in amount" } });
    fireEvent.click(dialog().getByRole("button", { name: "Delete" }));
    await flush();
    expect(actions.deleteRateCard).toHaveBeenCalledWith("rc-next", "Typo in amount");
  });

  it("approves and rejects certificate uploads, a rejection with a reason", async () => {
    actions.verifyCertificate.mockResolvedValue(ok);
    render(<ContractorsView live={await liveOf({}, { certs: [cert("cert-1"), cert("cert-2", { renewalOf: "cert-0", fileName: null, name: "Electrical safety" })] })} />);
    fireEvent.click(button("Verify uploads (2)"));
    expect(dialog().getByRole("link", { name: /Open the document/ }).getAttribute("href")).toBe("/admin/certificates/cert-1/document");
    expect(dialog().getByText(/· renewal$/)).toBeDefined();
    fireEvent.click(dialog().getAllByRole("button", { name: "Reject" })[1] ?? document.body);
    expect(dialog().getByText("A reason is required to reject")).toBeDefined();
    fireEvent.change(dialog().getAllByLabelText(/^Reason \(to reject\)/)[1] ?? document.body, { target: { value: "Blurred" } });
    fireEvent.click(dialog().getAllByRole("button", { name: "Reject" })[1] ?? document.body);
    await flush();
    expect(actions.verifyCertificate).toHaveBeenCalledWith("cert-2", 1, "reject", "Blurred");
    fireEvent.click(button("Verify uploads (2)"));
    fireEvent.click(dialog().getAllByRole("button", { name: "Approve" })[0] ?? document.body);
    await flush();
    expect(actions.verifyCertificate).toHaveBeenLastCalledWith("cert-1", 1, "approve", "");
    expect(screen.getByText("Approved Refrigerant handling for Ben — it now counts as the qualification.")).toBeDefined();
  });

  it("says when the action is refused, and offers no verification without uploads", async () => {
    actions.setOfferStatus.mockResolvedValue({ ok: false, code: "CONFLICT", messageKey: "error.conflict", fieldErrors: {} });
    render(<ContractorsView live={await liveOf({}, { certs: [] })} />);
    expect(button("Verify uploads (0)").disabled).toBe(true);
    fireEvent.click(button("Suspend offers"));
    fireEvent.change(dialog().getByLabelText(/^Reason/), { target: { value: "x" } });
    fireEvent.click(dialog().getByRole("button", { name: "Suspend offers" }));
    await flush();
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(screen.queryByText(/suspended — open offers and jobs continue/)).toBeNull();
  });
});

describe("profiles and deletion", () => {
  it("edits a profile at its version, checking each field", async () => {
    actions.saveContractor.mockResolvedValue({ ok: true, value: { id: "cp-1" } });
    render(<ContractorsView live={await liveOf()} />);
    fireEvent.click(button("Edit"));
    expect(dialog().getByLabelText<HTMLSelectElement>(/^Contractor organization/).disabled).toBe(true);
    fireEvent.change(dialog().getByLabelText(/^Registration number/), { target: { value: "" } });
    fireEvent.change(dialog().getByLabelText(/^Service areas/), { target: { value: "KL, kl" } });
    fireEvent.change(dialog().getByLabelText(/^Contact e-mail/), { target: { value: "nope" } });
    fireEvent.click(dialog().getByRole("button", { name: "Save" }));
    expect(dialog().getByText("1–64 characters.")).toBeDefined();
    expect(dialog().getByText("A valid e-mail address.")).toBeDefined();
    fireEvent.change(dialog().getByLabelText(/^Registration number/), { target: { value: "SSM 2" } });
    fireEvent.change(dialog().getByLabelText(/^Service areas/), { target: { value: "Penang, Johor" } });
    fireEvent.change(dialog().getByLabelText(/^Contact e-mail/), { target: { value: "ops@cool.example" } });
    fireEvent.change(dialog().getByLabelText(/^Insurance valid until/), { target: { value: "" } });
    fireEvent.click(dialog().getByRole("button", { name: "Save" }));
    await flush();
    expect(actions.saveContractor).toHaveBeenCalledWith({ id: "cp-1", version: 2, organizationId: "org-c", registrationNo: "SSM 2", serviceAreas: ["Penang", "Johor"], contactEmail: "ops@cool.example", insuranceValidUntil: null });
    expect(screen.getByText("Profile saved.")).toBeDefined();
    expect(nav.push).not.toHaveBeenCalled();
  });

  it("adds a profile to a company without one, and opens it", async () => {
    actions.saveContractor.mockResolvedValue({ ok: true, value: { id: "cp-z" } });
    render(<ContractorsView live={await liveOf({ contractorId: "org-z" })} />);
    expect(screen.getByText(/^No contractor profile yet/)).toBeDefined();
    fireEvent.click(button(/Add profile/));
    fireEvent.change(dialog().getByLabelText(/^Registration number/), { target: { value: "SSM 9" } });
    fireEvent.change(dialog().getByLabelText(/^Service areas/), { target: { value: "Penang" } });
    fireEvent.change(dialog().getByLabelText(/^Contact e-mail/), { target: { value: "z@zeta.example" } });
    fireEvent.change(dialog().getByLabelText(/^Insurance valid until/), { target: { value: "2027-03-31" } });
    fireEvent.click(dialog().getByRole("button", { name: "Add contractor" }));
    await flush();
    expect(actions.saveContractor).toHaveBeenCalledWith(expect.objectContaining({ id: null, version: null, organizationId: "org-z", insuranceValidUntil: "2027-03-30T16:00:00.000Z" }));
    expect(nav.push).toHaveBeenCalledWith("/admin/jobs?tab=contractors&contractorId=org-z");
  });

  it("creates a new company first, and keeps it chosen when its profile is refused", async () => {
    actions.createContractorOrganization.mockResolvedValue({ ok: true, value: { id: "org-new", name: "Lim Aircon" } });
    actions.saveContractor.mockResolvedValueOnce({ ok: false, code: "VALIDATION", messageKey: "error.validation", fieldErrors: {} }).mockResolvedValueOnce({ ok: true, value: { id: "cp-new" } });
    render(<ContractorsView live={await liveOf({}, { orgs: [{ id: "org-c", version: 3, name: "Cool Partners", kind: "contractor", status: "active" }] })} />);
    fireEvent.click(button(/^Add contractor/));
    expect(dialog().getByLabelText<HTMLSelectElement>(/^Contractor organization/).value).toBe("new"); // every company has a profile
    fireEvent.click(dialog().getByRole("button", { name: "Add contractor" }));
    expect(dialog().getByText("A company name of 1–120 characters")).toBeDefined();
    fireEvent.change(dialog().getByLabelText(/^Company name/), { target: { value: "Lim Aircon" } });
    fireEvent.change(dialog().getByLabelText(/^Registration number/), { target: { value: "SSM 7" } });
    fireEvent.change(dialog().getByLabelText(/^Service areas/), { target: { value: "KL" } });
    fireEvent.change(dialog().getByLabelText(/^Contact e-mail/), { target: { value: "lim@lim.example" } });
    fireEvent.click(dialog().getByRole("button", { name: "Add contractor" }));
    await flush();
    expect(nav.refresh).toHaveBeenCalledOnce(); // the company exists: the dialog stays to fix the profile
    expect(dialog().getByLabelText<HTMLSelectElement>(/^Contractor organization/).value).toBe("org-new");
    fireEvent.click(dialog().getByRole("button", { name: "Add contractor" }));
    await flush();
    expect(actions.createContractorOrganization).toHaveBeenCalledExactlyOnceWith("Lim Aircon"); // not created twice
    expect(actions.saveContractor).toHaveBeenLastCalledWith(expect.objectContaining({ organizationId: "org-new" }));
  });

  it("says when the company cannot be created, and deletes a contractor without a profile with force", async () => {
    actions.createContractorOrganization.mockResolvedValue({ ok: false, code: "CONFLICT", messageKey: "error.duplicateName", fieldErrors: {} });
    actions.deleteContractor.mockResolvedValue(ok);
    render(<ContractorsView live={await liveOf({ contractorId: "org-z" })} />);
    fireEvent.click(button(/^Add contractor/));
    fireEvent.change(dialog().getByLabelText(/^Contractor organization/), { target: { value: "new" } });
    fireEvent.change(dialog().getByLabelText(/^Company name/), { target: { value: "Cool Partners" } });
    fireEvent.change(dialog().getByLabelText(/^Registration number/), { target: { value: "SSM 7" } });
    fireEvent.change(dialog().getByLabelText(/^Service areas/), { target: { value: "KL" } });
    fireEvent.change(dialog().getByLabelText(/^Contact e-mail/), { target: { value: "x@x.example" } });
    fireEvent.click(dialog().getByRole("button", { name: "Add contractor" }));
    await flush();
    expect(actions.saveContractor).not.toHaveBeenCalled();
    expect(screen.queryByRole("dialog")).toBeNull(); // the refusal shows on the page
    fireEvent.click(button("Delete contractor…"));
    fireEvent.change(dialog().getByLabelText(/^Reason/), { target: { value: "Test company" } });
    fireEvent.click(dialog().getByRole("checkbox", { name: /Force delete/ }));
    fireEvent.change(dialog().getByLabelText(/^Type Zeta Cool to confirm/), { target: { value: "Zeta Cool" } });
    fireEvent.click(dialog().getByRole("button", { name: "Delete" }));
    await flush();
    expect(actions.deleteContractor).toHaveBeenCalledWith({ id: "org-z", version: 1 }, null, "Test company", true);
    expect(nav.patch).toHaveBeenCalledWith({ contractorId: null });
  });

  it("closes each dialog with Escape or Cancel, saving nothing", async () => {
    render(<ContractorsView live={await liveOf()} />);
    for (const name of ["Edit", "Suspend offers", "Edit rate card", "Delete rate card…", "Verify uploads (1)"]) {
      fireEvent.click(button(name));
      fireEvent.keyDown(screen.getByRole("dialog"), { key: "Escape" });
      expect(screen.queryByRole("dialog")).toBeNull();
    }
    for (const name of ["Edit", "Suspend offers", "Edit rate card", "Delete rate card…"]) {
      fireEvent.click(button(name));
      fireEvent.click(dialog().getByRole("button", { name: "Cancel" }));
    }
    cleanup();
    render(<ContractorsView live={await liveOf({ contractorId: "org-z" })} />);
    fireEvent.click(button("Delete contractor…"));
    fireEvent.keyDown(screen.getByRole("dialog"), { key: "Escape" });
    fireEvent.click(button("Delete contractor…"));
    fireEvent.click(dialog().getByRole("button", { name: "Cancel" }));
    expect(Object.values(actions).every((f) => f.mock.calls.length === 0)).toBe(true);
  });

  it("asks for the first contractor, says when the URL's one is gone, and opens New job", async () => {
    render(<ContractorsView live={await liveOf({ contractorId: "org-gone" })} />);
    expect(screen.getByText("That contractor no longer exists.")).toBeDefined();
    fireEvent.click(button(/New job/));
    fireEvent.click(dialog().getByRole("button", { name: "Cancel" }));
    cleanup();
    render(<ContractorsView live={await liveOf({ contractorId: "" }, { orgs: [] })} />);
    expect(screen.getByText(/^Add contractor creates the first company/)).toBeDefined();
  });
});
