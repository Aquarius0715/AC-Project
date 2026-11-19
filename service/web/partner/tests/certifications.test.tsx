// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import type { ActionFailure } from "@ac/web/lib/actionMessage";
import type { ApiTeamMember } from "@ac/web/lib/partnerTeam";
import type { ApiCertificate } from "@ac/web/lib/adminContractors";

// The Certifications tab of Team & capacity in API mode (FR-P09, Figma Contractor 04-6/04-7): the filters through the
// URL, the KPIs, one row per technician and qualification with its actions, the assignment impact, and the renewal,
// new-certificate, training and view dialogs with their checks and refusals. The live data comes from the real loader
// over a mocked DAL, so the tab gets what the page gives it.
const dal = vi.hoisted(() => ({
  coreNow: vi.fn(), coreDisplay: vi.fn(), coreIdentity: vi.fn(), corePrincipal: vi.fn(), coreOp: vi.fn(), coreAll: vi.fn(), coreItems: vi.fn(),
  orWhenRefused: async <T, F>(read: Promise<T>, fallback: F): Promise<T | F> => { try { return await read; } catch { return fallback; } },
}));
vi.mock("server-only", () => ({}));
vi.mock("@ac/web/lib/dal", () => dal);
const actions = vi.hoisted(() => ({ setUnavailability: vi.fn(), clearUnavailability: vi.fn(), submitCertificate: vi.fn(), requestTraining: vi.fn() }));
vi.mock("../app/partner/team/actions", () => actions);
const nav = vi.hoisted(() => vi.fn());
vi.mock("@ac/web/lib/useUrlPatch", () => ({ useUrlPatch: () => nav }));
// the success toasts, and the codes of the refusals once the tab's failure handler has run
const toasts = vi.hoisted((): string[] => []);
const failures = vi.hoisted((): string[] => []);
type Result = { ok: true; value: never } | ({ ok: false } & ActionFailure);
vi.mock("@ac/web/lib/useAction", () => ({
  useAction: () => [false, (fn: () => Promise<Result>, ok: string | ((v: never) => string), after?: (v: never) => void, failed?: (f: ActionFailure) => void) => {
    void fn().then((r) => {
      if (r.ok) { toasts.push(typeof ok === "string" ? ok : ok(r.value)); after?.(r.value); } else { failed?.(r); failures.push(r.code); }
    });
  }] as const,
}));

import { loadTeam } from "../app/partner/team/_lib/load";
import { Certifications } from "../app/partner/team/_components/certifications";

const aiman: ApiTeamMember = {
  id: "m-1", userId: "u-1", displayName: "Aiman", role: "technician", organizationId: "org-a", employment: null, validFrom: "2026-01-01T00:00:00Z", validUntil: null, revokedAt: null,
  qualifications: [{ code: "demo_indoor", validFrom: "2026-01-01T00:00:00Z", validUntil: "2026-12-01T00:00:00Z", revokedAt: null }],
};
const dina: ApiTeamMember = { ...aiman, id: "m-2", userId: "u-2", displayName: "Dina", qualifications: [{ code: "demo_outdoor", validFrom: "2026-01-01T00:00:00Z", validUntil: "2027-06-01T00:00:00Z", revokedAt: null }] }; // an HQ grant without a certificate on file
const badrul: ApiTeamMember = { ...aiman, id: "m-3", userId: "u-3", displayName: "Badrul", validUntil: "2026-10-01T00:00:00Z", qualifications: [] };
const cert = (over: Partial<ApiCertificate>): ApiCertificate => ({
  id: "c-1", version: 4, membershipId: "m-1", organizationId: "org-a", code: "demo_outdoor", name: "Outdoor licence", number: "OUT-1", issuedAt: "2025-12-09T16:00:00Z",
  expiresAt: "2026-12-09T16:00:00Z", fileName: "out.pdf", status: "expiring", renewalOf: null, createdAt: "2025-12-10T00:00:00Z", ...over,
});
const certificates: ApiCertificate[] = [
  cert({}), // Aiman's outdoor licence, expiring in 22 days, with the document
  cert({ id: "c-5", membershipId: "m-1", code: "demo_indoor", name: "", number: "IN-9", status: "pending_verification", createdAt: "2026-11-10T00:00:00Z" }), // the upload HQ has not checked
  cert({ id: "c-2", membershipId: "m-2", code: "demo_electrical", name: "Wiring", number: "EL-2", expiresAt: "2026-11-01T16:00:00Z", fileName: null, status: "expired", trainingRequestedAt: "2026-11-05T02:00:00Z" }),
  cert({ id: "c-4", membershipId: "m-3", code: "demo_indoor", name: "Indoor card", number: "IN-4", expiresAt: "2027-06-01T16:00:00Z", status: "valid" }),
];
const job = (id: string, membershipId: string, unitId: string, day: string) => ({
  id, unitId, status: "assigned", projection: "summary", version: 1, type: "maintenance", technicianMembershipId: membershipId,
  scheduledSlot: { startAt: `${day}T01:00:00Z`, endAt: `${day}T03:00:00Z` },
});
// Aiman's outdoor job comes after the licence ends; Dina's indoor job needs a qualification she does not hold
const jobs: Record<string, unknown[]> = { "m-1": [job("job-2222bbbb", "m-1", "unit-out", "2026-12-15")], "m-2": [job("job-3333cccc", "m-2", "unit-in", "2026-11-20")] };
const units = [{ id: "unit-out", displayName: "Rooftop", serviceScope: ["outdoor"] }, { id: "unit-in", displayName: "Lobby", serviceScope: ["indoor"] }];

interface Setup { members?: ApiTeamMember[]; certs?: ApiCertificate[] }
async function live(sp: Parameters<typeof loadTeam>[0] = {}, { members = [aiman, dina, badrul], certs = certificates }: Setup = {}) {
  const lists = new Map<string, unknown[]>([["members.list", members], ["certificates.list", certs], ["units.list", units]]);
  dal.coreAll.mockImplementation((operation: string) => Promise.resolve(lists.get(operation) ?? []));
  const r = await loadTeam({ date: "2026-11-18", ...sp });
  if (r.notFound) throw new Error("the roster is not found");
  return r;
}
const show = async (sp: Parameters<typeof loadTeam>[0] = {}, setup: Setup = {}) => render(<Certifications live={await live(sp, setup)} />);
const dialog = () => screen.getByRole("dialog");
const rowOf = (technician: string, certificate: string) => {
  const row = screen.getAllByRole("row").find((r) => within(r).queryByText(technician) && within(r).queryByText(certificate));
  if (!row) throw new Error(`no row of ${technician} / ${certificate}`);
  return row;
};
const pdf = (name = "renewal.pdf", size = 2_400_000) => {
  const f = new File(["%PDF-1.7"], name, { type: "application/pdf" });
  Object.defineProperty(f, "size", { value: size });
  return f;
};
const sent = (): Record<string, FormDataEntryValue> => {
  const form: unknown = actions.submitCertificate.mock.calls.at(-1)?.[0];
  if (!(form instanceof FormData)) throw new TypeError("no form was sent");
  return Object.fromEntries(form);
};
const refusal = (code: string, fieldErrors: Record<string, string> = {}): Result => ({ ok: false, messageKey: `error.${code.toLowerCase()}`, code, fieldErrors });
/** The value of the dialog's control with this label. */
const controlValue = (d: HTMLElement, label: RegExp) => {
  const control = within(d).getByLabelText(label);
  if (!(control instanceof HTMLInputElement || control instanceof HTMLSelectElement)) throw new TypeError("not an input or select");
  return control.value;
};
/** The fields of the last certificate form sent, its file left out. */
const sentFields = () => {
  const { file, ...fields } = sent();
  expect(file).toBeInstanceOf(File);
  return fields;
};
const openTraining = async () => {
  await show();
  fireEvent.click(within(rowOf("Aiman", "Outdoor licence")).getByRole("button", { name: "Request training" }));
  return dialog();
};

beforeEach(() => {
  vi.resetAllMocks(); // results too, so no test inherits another's action result
  toasts.length = 0;
  failures.length = 0;
  vi.stubGlobal("requestAnimationFrame", (f: FrameRequestCallback) => { f(0); return 0; }); // focusField runs at once
  dal.coreNow.mockResolvedValue(new Date("2026-11-18T02:00:00Z"));
  dal.coreDisplay.mockResolvedValue({ locale: "en", timeZone: "Asia/Kuala_Lumpur" });
  dal.coreIdentity.mockResolvedValue({ displayName: "Owner", organizationName: "Contractor A" });
  dal.corePrincipal.mockResolvedValue({ organizationId: "org-a" });
  dal.coreItems.mockResolvedValue([]);
  dal.coreOp.mockImplementation((_operation: string, input: { filters: { membershipId: string } }) => Promise.resolve({ items: jobs[input.filters.membershipId] ?? [], total: 1 }));
});
afterEach(() => { cleanup(); vi.unstubAllGlobals(); });

describe("Certifications tab", () => {
  it("groups the technicians by active and inactive and moves every filter through the URL, leaving the defaults out", async () => {
    await show();
    const technician = screen.getByRole("combobox", { name: "Technician" });
    expect(within(technician).getAllByRole("group").map((g) => [g.getAttribute("label"), within(g).getAllByRole("option").map((o) => o.textContent)])).toStrictEqual([["Active", ["Aiman", "Dina"]], ["Inactive", ["Badrul"]]]);
    fireEvent.change(technician, { target: { value: "m-2" } });
    expect(nav).toHaveBeenLastCalledWith({ membershipId: "m-2" });
    fireEvent.change(technician, { target: { value: "" } });
    expect(nav).toHaveBeenLastCalledWith({ membershipId: null });
    const status = screen.getByRole("combobox", { name: "Status" });
    fireEvent.change(status, { target: { value: "expired" } });
    expect(nav).toHaveBeenLastCalledWith({ status: "expired" });
    fireEvent.change(status, { target: { value: "" } });
    expect(nav).toHaveBeenLastCalledWith({ status: null });
    const window = screen.getByRole("combobox", { name: "Expiring within" });
    expect(window).toHaveProperty("value", "60");
    fireEvent.change(window, { target: { value: "30" } });
    expect(nav).toHaveBeenLastCalledWith({ expiringWithinDays: "30" });
    fireEvent.change(window, { target: { value: "60" } });
    expect(nav).toHaveBeenLastCalledWith({ expiringWithinDays: null }); // 60 days is the default
  });

  it("shows the KPIs, a row per technician and qualification with its actions, and the first job a lapse blocks", async () => {
    await show();
    const kpi = (label: string) => within(screen.getByRole("region", { name: label })).getAllByText(/.+/).map((x) => x.textContent);
    expect(kpi("Valid")).toStrictEqual(["Valid", "4", "in date"]);
    expect(kpi("Expiring ≤ 60 days")).toStrictEqual(["Expiring ≤ 60 days", "2", "Aiman · 13 d"]);
    expect(kpi("Expired")).toStrictEqual(["Expired", "1", "Dina"]);
    expect(kpi("Pending HQ verification")).toStrictEqual(["Pending HQ verification", "1", "uploads you sent"]);
    // Aiman's licence: renew (the primary action while expiring), train or view; it blocks the December job
    const licence = rowOf("Aiman", "Outdoor licence");
    expect(within(licence).getByText("Expiring · 22 d")).toBeTruthy();
    expect(within(licence).getByText("job-2222 (15 Dec)")).toBeTruthy();
    expect(within(licence).getAllByRole("button").map((b) => b.textContent)).toStrictEqual(["Upload renewal", "Request training", "View"]);
    expect(within(licence).getByRole("button", { name: "Upload renewal" }).className).toContain("bg-primary");
    // the HQ grant whose upload waits: no action, the pending note
    const grant = rowOf("Aiman", "granted by HQ · the upload waits for HQ");
    expect(within(grant).queryAllByRole("button")).toHaveLength(0);
    expect(within(grant).getByText("Renewal pending HQ verification")).toBeTruthy();
    expect(within(grant).getByText("—")).toBeTruthy(); // no job needs indoor work of Aiman
    // Dina's expired wiring certificate: renewal, not the primary action; the training already asked for
    const wiring = rowOf("Dina", "Wiring");
    expect(within(wiring).getAllByRole("button").map((b) => b.textContent)).toStrictEqual(["Upload renewal", "View"]);
    expect(within(wiring).getByRole("button", { name: "Upload renewal" }).className).not.toContain("bg-primary");
    expect(within(wiring).getByText("Training requested 5 Nov 2026")).toBeTruthy();
    expect(within(rowOf("Dina", "not held")).getByRole("button", { name: "Add certificate" })).toBeTruthy();
    expect(within(rowOf("Dina", "granted by HQ · no certificate on file")).getByRole("button", { name: "Upload certificate" })).toBeTruthy();
    // an inactive technician's certificate can only be viewed
    expect(within(rowOf("Badrul", "Indoor card")).getAllByRole("button").map((b) => b.textContent)).toStrictEqual(["View"]);
    expect(screen.getByText(/Dina does not hold .*\. job-3333 \(Lobby, 20 Nov\) needs it/)).toBeTruthy();
    expect(screen.getByRole("link", { name: "Open assignment" }).getAttribute("href")).toBe("/partner/schedule?jobId=job-3333cccc");
  });

  it("says when no row matches or no job is blocked, and offers no new certificate when nobody is active", async () => {
    await show({}, { members: [badrul], certs: [] });
    expect(screen.getByText("No certificates match these filters.")).toBeTruthy();
    expect(screen.getByText("No booked job depends on an expiring, expired or missing certificate.")).toBeTruthy();
    expect(screen.getByRole("button", { name: "Add certificate" })).toHaveProperty("disabled", true);
    expect(within(screen.getByRole("combobox", { name: "Technician" })).getAllByRole("group").map((g) => g.getAttribute("label"))).toStrictEqual(["Inactive"]);
    expect(screen.getByRole("region", { name: "Expired" }).textContent).toContain("—");
  });

  it("shows a certificate with its document link, and one without a document", async () => {
    await show();
    fireEvent.click(within(rowOf("Aiman", "Outdoor licence")).getByRole("button", { name: "View" }));
    expect(within(dialog()).getByText("OUT-1")).toBeTruthy();
    expect(within(dialog()).getByRole("link", { name: "Open the document" }).getAttribute("href")).toBe("/partner/certificates/c-1/document");
    fireEvent.click(within(dialog()).getAllByRole("button", { name: "Close" }).at(-1) ?? document.body);
    expect(screen.queryByRole("dialog")).toBeNull();
    fireEvent.click(within(rowOf("Dina", "Wiring")).getByRole("button", { name: "View" }));
    expect(within(dialog()).getByText("No document was uploaded with this certificate.")).toBeTruthy();
    expect(within(dialog()).queryByRole("link")).toBeNull();
  });
});

describe("Certificate dialog", () => {
  it("renews a certificate: checks the days and the file, focuses the first field to fix, and sends the renewal", async () => {
    await show();
    fireEvent.click(within(rowOf("Aiman", "Outdoor licence")).getByRole("button", { name: "Upload renewal" }));
    const d = dialog();
    expect(within(d).getByText("Upload renewal — Outdoor licence")).toBeTruthy();
    expect(within(d).getByText("OUT-1 · expires 10 Dec 2026")).toBeTruthy();
    expect(within(d).queryByRole("combobox")).toBeNull(); // the technician and the qualification are the row's
    fireEvent.click(within(d).getByRole("button", { name: "Submit for verification" }));
    expect(within(d).getByText("Choose the issue date")).toBeTruthy();
    expect(within(d).getByText("Choose the expiry date")).toBeTruthy();
    expect(within(d).getByText("Attach the certificate file")).toBeTruthy();
    expect(within(d).getByText("1–64 characters")).toBeTruthy();
    expect(document.activeElement).toBe(within(d).getByLabelText(/^Issued on/));
    fireEvent.change(within(d).getByLabelText(/^Issued on/), { target: { value: "2026-11-10" } });
    fireEvent.change(within(d).getByLabelText(/^Expires on/), { target: { value: "2026-11-09" } });
    expect(within(d).getByLabelText(/^Expires on/).getAttribute("min")).toBe("2026-11-10");
    expect(within(d).getByText("The expiry date must be after the issue date")).toBeTruthy();
    fireEvent.change(within(d).getByLabelText(/^Expires on/), { target: { value: "2027-11-09" } });
    fireEvent.change(within(d).getByLabelText(/^Certificate number/), { target: { value: "OUT-2" } });
    fireEvent.click(within(d).getByRole("button", { name: "Submit for verification" }));
    expect(document.activeElement).toBe(within(d).getByLabelText(/Choose a file/));
    expect(actions.submitCertificate).not.toHaveBeenCalled();
    fireEvent.change(within(d).getByLabelText(/Choose a file/), { target: { files: [pdf()] } });
    expect(within(d).getByText("renewal.pdf · 2.4 MB")).toBeTruthy();
    expect(within(d).getByText("PDF / JPG / PNG, up to 10 MB · Replace")).toBeTruthy();
    fireEvent.change(within(d).getByLabelText(/renewal\.pdf/), { target: { files: [] } }); // the picker closed without a choice
    expect(within(d).getByText("PDF / JPG / PNG, up to 10 MB")).toBeTruthy();
    fireEvent.change(within(d).getByLabelText(/^Choose a file/), { target: { files: [pdf()] } });
    actions.submitCertificate.mockResolvedValue({ ok: true, value: { id: "c-9" } });
    fireEvent.click(within(d).getByRole("button", { name: "Submit for verification" }));
    await waitFor(() => { expect(screen.queryByRole("dialog")).toBeNull(); });
    expect(sentFields()).toStrictEqual({ membershipId: "m-1", code: "demo_outdoor", name: "Outdoor licence", number: "OUT-2", issued: "2026-11-10", expires: "2027-11-09", renewalOf: "c-1" });
    expect(toasts).toStrictEqual(["Submitted for HQ verification — it counts for eligibility once HQ approves it."]);
  });

  it("adds a new certificate: the first active technician and qualification by default, the name follows the qualification, no renewal sent", async () => {
    await show();
    fireEvent.click(screen.getAllByRole("button", { name: "Add certificate" })[0] ?? document.body);
    const d = dialog();
    expect(within(d).getByRole("heading", { name: "Add certificate" })).toBeTruthy();
    const technician = within(d).getByLabelText(/^Technician/);
    expect(within(technician).getAllByRole("option").map((o) => o.textContent)).toStrictEqual(["Aiman", "Dina"]); // active only
    expect(technician).toHaveProperty("value", "m-1");
    const name = within(d).getByLabelText(/^Certificate name/);
    expect(name).toHaveProperty("value", "Indoor unit work");
    fireEvent.change(within(d).getByLabelText(/^Qualification/), { target: { value: "other" } });
    expect(name).toHaveProperty("value", "Other");
    fireEvent.change(technician, { target: { value: "m-2" } });
    fireEvent.change(name, { target: { value: "  " } });
    fireEvent.change(within(d).getByLabelText(/^Issued on/), { target: { value: "2026-11-10" } });
    fireEvent.change(within(d).getByLabelText(/^Expires on/), { target: { value: "2027-11-09" } });
    fireEvent.change(within(d).getByLabelText(/^Certificate number/), { target: { value: "X-1" } });
    fireEvent.change(within(d).getByLabelText(/Choose a file/), { target: { files: [new File(["x"], "notes.txt", { type: "text/plain" })] } });
    fireEvent.click(within(d).getByRole("button", { name: "Submit for verification" }));
    expect(within(d).getByText("1–120 characters")).toBeTruthy();
    expect(within(d).getByText("Use a PDF, JPG or PNG file")).toBeTruthy();
    expect(document.activeElement).toBe(name);
    fireEvent.change(name, { target: { value: "Gas handling" } });
    fireEvent.change(within(d).getByLabelText(/notes\.txt/), { target: { files: [pdf("gas.pdf", 412_000)] } });
    expect(within(d).getByText("gas.pdf · 412 KB")).toBeTruthy();
    actions.submitCertificate.mockResolvedValue({ ok: true, value: { id: "c-9" } });
    fireEvent.click(within(d).getByRole("button", { name: "Submit for verification" }));
    await waitFor(() => { expect(screen.queryByRole("dialog")).toBeNull(); });
    expect(sentFields()).toStrictEqual({ membershipId: "m-2", code: "other", name: "Gas handling", number: "X-1", issued: "2026-11-10", expires: "2027-11-09" });
  });

  it("starts from a missing qualification's row, shows the fields the Core API refused and the reason, and clears them on a change", async () => {
    await show();
    fireEvent.click(within(rowOf("Dina", "not held")).getByRole("button", { name: "Add certificate" }));
    const d = dialog();
    expect(controlValue(d, /^Technician/)).toBe("m-2");
    expect(controlValue(d, /^Qualification/)).toBe("demo_indoor");
    fireEvent.change(within(d).getByLabelText(/^Issued on/), { target: { value: "2026-11-10" } });
    fireEvent.change(within(d).getByLabelText(/^Expires on/), { target: { value: "2027-11-09" } });
    fireEvent.change(within(d).getByLabelText(/^Certificate number/), { target: { value: "IN-7" } });
    fireEvent.change(within(d).getByLabelText(/Choose a file/), { target: { files: [pdf()] } });
    actions.submitCertificate.mockResolvedValue(refusal("VALIDATION", { number: "error.length", file: "error.invalidFile" }));
    fireEvent.click(within(d).getByRole("button", { name: "Submit for verification" }));
    expect(await within(d).findByText("Too long")).toBeTruthy();
    expect(within(d).getByText("Use a PDF, JPG or PNG file of at most 10 MB")).toBeTruthy();
    fireEvent.change(within(d).getByLabelText(/^Certificate number/), { target: { value: "IN-8" } });
    expect(within(d).queryByText("Too long")).toBeNull();
    actions.submitCertificate.mockResolvedValue(refusal("NOT_FOUND"));
    fireEvent.click(within(d).getByRole("button", { name: "Submit for verification" }));
    expect(await within(d).findByText("That technician or certificate is no longer in your company — reload the page.")).toBeTruthy();
    expect(failures).toStrictEqual(["VALIDATION", "NOT_FOUND"]);
    fireEvent.click(within(d).getByRole("button", { name: "Cancel" }));
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(toasts).toStrictEqual([]);
  });
});

describe("Training dialog", () => {
  it("needs a note of 1–1000 characters after trimming, and sends the note to the certificate's version", async () => {
    const d = await openTraining();
    expect(within(d).getByText("Request training — Outdoor licence")).toBeTruthy();
    expect(within(d).getByText("Outdoor licence · Expiring · 22 d")).toBeTruthy();
    fireEvent.click(within(d).getByRole("button", { name: "Request training" }));
    expect(within(d).getByText("A note is required (1–1000 characters)")).toBeTruthy();
    const note = within(d).getByRole("textbox");
    fireEvent.change(note, { target: { value: "x".repeat(1001) } });
    expect(within(d).getByText("A note is required (1–1000 characters)")).toBeTruthy(); // too long counts as wrong too
    fireEvent.click(within(d).getByRole("button", { name: "Request training" }));
    expect(actions.requestTraining).not.toHaveBeenCalled();
    fireEvent.change(note, { target: { value: "x".repeat(1000) } });
    expect(within(d).queryByText("A note is required (1–1000 characters)")).toBeNull();
    fireEvent.change(note, { target: { value: "  Refresher in December  " } });
    expect(within(d).queryByText("A note is required (1–1000 characters)")).toBeNull();
    actions.requestTraining.mockResolvedValue({ ok: true, value: null });
    fireEvent.click(within(d).getByRole("button", { name: "Request training" }));
    await waitFor(() => { expect(screen.queryByRole("dialog")).toBeNull(); });
    expect(actions.requestTraining).toHaveBeenCalledExactlyOnceWith("c-1", 4, "  Refresher in December  "); // the action trims it
    expect(toasts).toStrictEqual(["Training requested."]);
  });

  it("says why a request was refused: the changed certificate, the refused note, nothing beyond the toast for an outage; an edit clears them", async () => {
    const d = await openTraining();
    const note = within(d).getByRole("textbox");
    const changed = "The certificate changed in the meantime — the page shows the latest state.";
    const submit = () => { fireEvent.click(within(d).getByRole("button", { name: "Request training" })); };
    fireEvent.change(note, { target: { value: "Refresher" } });
    actions.requestTraining.mockResolvedValue(refusal("CONFLICT"));
    submit();
    expect(await within(d).findByText(changed)).toBeTruthy();
    fireEvent.change(note, { target: { value: "Refresher course" } });
    expect(within(d).queryByText(changed)).toBeNull();
    // the refused note under its field, the earlier reason gone
    submit();
    expect(await within(d).findByText(changed)).toBeTruthy();
    actions.requestTraining.mockResolvedValue(refusal("VALIDATION", { note: "error.length" }));
    submit();
    expect(await within(d).findByText("Too long")).toBeTruthy();
    expect(within(d).queryByText(changed)).toBeNull();
    fireEvent.change(note, { target: { value: "Refresher" } });
    expect(within(d).queryByText("Too long")).toBeNull();
    // a VALIDATION naming no field asks to check the input
    actions.requestTraining.mockResolvedValue(refusal("VALIDATION"));
    submit();
    expect(await within(d).findByText("Check the input.")).toBeTruthy();
    // an outage is the toast's to say (useAction): the dialog keeps no earlier message and does not blame the input
    actions.requestTraining.mockResolvedValue(refusal("UNAVAILABLE"));
    submit();
    await waitFor(() => { expect(within(d).queryByText("Check the input.")).toBeNull(); });
    expect(failures).toStrictEqual(["CONFLICT", "CONFLICT", "VALIDATION", "VALIDATION", "UNAVAILABLE"]);
    expect(within(d).queryByText(changed)).toBeNull();
    expect(within(d).queryByText("Too long")).toBeNull();
    fireEvent.click(within(d).getByRole("button", { name: "Cancel" }));
    expect(screen.queryByRole("dialog")).toBeNull();
  });
});
