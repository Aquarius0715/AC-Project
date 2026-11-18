// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import type { ActionFailure } from "@ac/web/lib/actionMessage";
import type { ApiTeamMember, ApiUnavailability } from "@ac/web/lib/partnerTeam";
import type { ApiCapacity } from "@ac/web/lib/partnerOverview";

// Team & capacity in API mode (FR-P06, IR612): the roster of another company, an empty team, the URL filters, the
// technician rows and the week, the expired members the list hides, adding unavailable days (the conflicts, the
// all-technicians choice, the form's checks and the refusals) and removing them from a chosen day on. The live data
// comes from the real loader over a mocked DAL, so the view gets what the page gives it.
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
// the success toasts, and the codes of the refusals once the view's failure handler has run
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
import { TeamView } from "../app/partner/team/_components/team-view";

const aiman: ApiTeamMember = {
  id: "m-1", userId: "u-1", displayName: "Aiman", role: "technician", organizationId: "org-a", employment: null, validFrom: "2026-01-01T00:00:00Z", validUntil: null, revokedAt: null,
  qualifications: [{ code: "demo_indoor", validFrom: "2026-01-01T00:00:00Z", validUntil: "2026-12-01T00:00:00Z", revokedAt: null }],
};
const badrul: ApiTeamMember = { ...aiman, id: "m-2", userId: "u-2", displayName: "Badrul", validUntil: "2026-10-01T00:00:00Z", qualifications: [] };
const chen: ApiTeamMember = { ...badrul, id: "m-3", userId: "u-3", displayName: "Chen" };
const slot = { startAt: "2026-11-18T01:00:00Z", endAt: "2026-11-18T05:00:00Z" }; // 09:00–13:00 in Kuala Lumpur, today
const job = { id: "job-1111aaaa", status: "assigned", scheduledSlot: slot, projection: "summary", version: 1, unitId: "unit-1", type: "maintenance", technicianMembershipId: "m-1" };
const leave: ApiUnavailability = { id: "un-1", version: 3, membershipId: "m-1", from: "2026-11-25", to: "2026-11-27", type: "annual_leave", note: "Trip" };
/** Aiman's capacity: 8 h on weekdays, nothing set at the weekend, today's 09:00–13:00 assigned. */
const capacity = (date: string): ApiCapacity[] => {
  const weekend = ["2026-11-21", "2026-11-22"].includes(date);
  const today = date === "2026-11-18";
  return [{
    date, membershipId: "m-1", availableSlots: weekend ? [] : [{ startAt: `${date}T01:00:00Z`, endAt: `${date}T09:00:00Z` }],
    assignedSlots: today ? [slot] : [], availableMinutes: weekend ? null : 480, assignedMinutes: today ? 240 : 0, unavailability: null,
  }];
};

interface Setup { members?: ApiTeamMember[]; unavailable?: ApiUnavailability[]; timeZone?: string }
async function live(sp: Parameters<typeof loadTeam>[0] = {}, { members = [aiman, badrul], unavailable = [leave], timeZone = "Asia/Kuala_Lumpur" }: Setup = {}) {
  dal.coreDisplay.mockResolvedValue({ locale: "en", timeZone });
  const lists = new Map<string, unknown[]>([["members.list", members], ["members.unavailability", unavailable]]);
  dal.coreAll.mockImplementation((operation: string) => Promise.resolve(lists.get(operation) ?? []));
  return loadTeam({ date: "2026-11-18", ...sp });
}
const show = async (sp: Parameters<typeof loadTeam>[0] = {}, setup: Setup = {}) => render(<TeamView live={await live(sp, setup)} />);
const dialog = () => screen.getByRole("dialog");
const technicianRow = (name: string) => {
  const row = screen.getAllByRole("listitem").find((li) => within(li).queryByText(name, { selector: "b" }));
  if (!row) throw new Error(`no row of ${name}`);
  return row;
};

beforeEach(() => {
  vi.resetAllMocks(); // results too, so no test inherits another's action result
  toasts.length = 0;
  failures.length = 0;
  vi.stubGlobal("ResizeObserver", class { observe = vi.fn(); unobserve = vi.fn(); disconnect = vi.fn(); }); // jsdom has none; the week's scroll box uses it
  history.replaceState(null, "", "/partner/team");
  dal.coreNow.mockResolvedValue(new Date("2026-11-18T02:00:00Z"));
  dal.coreIdentity.mockResolvedValue({ displayName: "Owner", organizationName: "Contractor A" });
  dal.corePrincipal.mockResolvedValue({ organizationId: "org-a" });
  dal.coreItems.mockImplementation((_operation: string, input: { date: string }) => Promise.resolve(capacity(input.date)));
  dal.coreOp.mockResolvedValue({ items: [job], total: 1 });
});
afterEach(() => { cleanup(); vi.unstubAllGlobals(); });

describe("Team view", () => {
  it("says another company's roster is not found, with the way back to the own team", async () => {
    await show({ orgId: "org-b" });
    expect(screen.getByText("Roster not found")).toBeTruthy();
    expect(screen.getByText(/You can only see Contractor A’s own technicians/)).toBeTruthy();
    expect(screen.getByRole("link", { name: "Back to my team" }).getAttribute("href")).toBe("/partner/team");
    expect(screen.getAllByRole("tab").map((x) => x.textContent)).toStrictEqual(["Members", "Certifications"]);
  });

  it("shows one empty state when the company has no technicians, on both tabs", async () => {
    await show({}, { members: [] });
    expect(screen.getByText("No technicians")).toBeTruthy();
    fireEvent.click(screen.getByRole("tab", { name: "Certifications" }));
    expect(screen.getByText("No technicians")).toBeTruthy();
    expect(screen.queryByText(/Certificates —/)).toBeNull();
  });

  it("moves the date, qualification and Active only through the URL, leaving the defaults out", async () => {
    const view = await show({ date: "2026-11-20" });
    const date = screen.getByLabelText("Date");
    fireEvent.change(date, { target: { value: "2026-11-21" } });
    expect(nav).toHaveBeenLastCalledWith({ date: "2026-11-21" });
    fireEvent.change(date, { target: { value: "2026-11-18" } });
    expect(nav).toHaveBeenLastCalledWith({ date: null }); // today is the default
    nav.mockClear();
    fireEvent.change(date, { target: { value: "" } });
    expect(nav).not.toHaveBeenCalled(); // a cleared date box keeps the day
    fireEvent.change(screen.getByRole("combobox", { name: "Qualification" }), { target: { value: "demo_outdoor" } });
    expect(nav).toHaveBeenLastCalledWith({ qualification: "demo_outdoor" });
    fireEvent.change(screen.getByRole("combobox", { name: "Qualification" }), { target: { value: "" } });
    expect(nav).toHaveBeenLastCalledWith({ qualification: null });
    const active = screen.getByRole("button", { name: "Active only" });
    expect(active.getAttribute("aria-pressed")).toBe("true");
    fireEvent.click(active);
    expect(nav).toHaveBeenLastCalledWith({ activeOnly: "false" });
    view.rerender(<TeamView live={await live({ activeOnly: "false" })} />);
    const include = screen.getByRole("button", { name: "Include expired" });
    expect(include.getAttribute("aria-pressed")).toBe("false");
    fireEvent.click(include);
    expect(nav).toHaveBeenLastCalledWith({ activeOnly: null });
  });

  it("lists the current technician with the week, the chosen day and the utilization, and the hidden expired ones", async () => {
    await show({}, { members: [aiman, badrul, chen] });
    expect(screen.getByText("Technicians — Contractor A · 1 active")).toBeTruthy();
    const row = technicianRow("Aiman");
    expect(within(row).getByText("09:00–13:00 assigned (job-1111)")).toBeTruthy();
    expect(within(row).getByText("13:00–17:00 free")).toBeTruthy();
    expect(within(row).getByText("50%")).toBeTruthy();
    expect(within(row).getByText("4 h / 40 h")).toBeTruthy();
    expect(screen.queryByText("Badrul", { selector: "b" })).toBeNull();
    expect(screen.getByText("2 expired members hidden (Active only).")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Include expired" }));
    expect(nav).toHaveBeenLastCalledWith({ activeOnly: "false" });
    // the week: Aiman only, today's column marked, the stats and the weekend note; Kuala Lumpur needs no zone note
    const table = screen.getByRole("table");
    expect(within(table).getAllByRole("row")).toHaveLength(2);
    expect(within(table).getAllByRole("columnheader").filter((h) => h.className.includes("text-primary")).map((h) => h.textContent)).toStrictEqual(["Wed 18"]);
    expect(screen.getByText("Free hours Wed")).toBeTruthy();
    expect(screen.queryByText(/Kuala Lumpur time/)).toBeNull();
  });

  it("says one hidden member in the singular, and lists the expired ones by name when they are included", async () => {
    const view = await show();
    expect(screen.getByText("1 expired member hidden (Active only).")).toBeTruthy();
    view.rerender(<TeamView live={await live({ activeOnly: "false" }, { members: [aiman, badrul, chen] })} />);
    expect(screen.queryByText(/expired members? hidden/)).toBeNull();
    expect(screen.getByText(/Expired memberships \(Badrul, Chen\) are listed/)).toBeTruthy();
    const row = technicianRow("Badrul");
    expect(within(row).getByText("Expired — cannot be assigned").className).toContain("text-crit");
    expect(within(row).queryByRole("progressbar")).toBeNull();
  });

  it("offers the renewal of an expiring qualification on the Certifications tab, and says the Kuala Lumpur time elsewhere", async () => {
    await show({}, { timeZone: "Asia/Tokyo" });
    expect(screen.getByText(/Days and hours are in Kuala Lumpur time/)).toBeTruthy();
    expect(screen.getByText("Expiring · 13 d")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Upload renewal" }));
    expect(screen.getByText("Certificates — Contractor A")).toBeTruthy();
    expect(new URLSearchParams(location.search).get("tab")).toBe("certifications");
  });

  it("asks to clear the qualification filter when nobody holds it, without the week and the grants", async () => {
    await show({ qualification: "demo_outdoor" });
    expect(screen.getByText("No Contractor A technicians hold this qualification")).toBeTruthy();
    expect(screen.queryByRole("table")).toBeNull();
    expect(screen.queryByText("Expiring · 13 d")).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Clear filter" }));
    expect(nav).toHaveBeenLastCalledWith({ qualification: null });
  });
});

/** Opens the add dialog over the given team. */
const openAdd = async (setup: Setup = {}) => {
  await show({}, setup);
  fireEvent.click(screen.getByRole("button", { name: "Unavailable days" }));
  return dialog();
};
/** Opens the remove dialog of Aiman's first row. */
const openRemove = async (rows: ApiUnavailability[] = [leave]) => {
  await show({}, { unavailable: rows });
  fireEvent.click(screen.getByRole("button", { name: /^Remove unavailable days of Aiman/ }));
  return dialog();
};

describe("Add unavailable days", () => {
  it("is off without a current technician, and says there are no upcoming days", async () => {
    await show({}, { members: [badrul], unavailable: [] });
    expect(screen.getByRole("button", { name: "Unavailable days" })).toHaveProperty("disabled", true);
    expect(screen.getByText("No upcoming unavailable days. Add them with Unavailable days above.")).toBeTruthy();
  });

  it("lists the confirmed assignment the days overlap, names the technician for the whole team, and links to it", async () => {
    const d = await openAdd();
    expect(within(d).getByText(/Conflicts with 1 confirmed assignment: job-1111 · .*\. Saving keeps the assignment/)).toBeTruthy();
    expect(within(d).getByRole("link", { name: "Open schedule" }).getAttribute("href")).toBe("/partner/schedule?jobId=job-1111aaaa");
    fireEvent.change(within(d).getByLabelText("Technician"), { target: { value: "" } });
    expect(within(d).getByText(/Conflicts with 1 confirmed assignment: job-1111 · Aiman · /)).toBeTruthy();
    fireEvent.change(within(d).getByLabelText("From"), { target: { value: "2026-11-19" } });
    fireEvent.change(within(d).getByLabelText(/^To/), { target: { value: "2026-11-19" } });
    expect(within(d).queryByText(/Conflicts with/)).toBeNull();
    expect(within(d).getByRole("link", { name: "Open schedule" }).getAttribute("href")).toBe("/partner/schedule");
  });

  it("counts two overlapping assignments in the plural, and says the days are Kuala Lumpur days in another zone", async () => {
    dal.coreOp.mockResolvedValue({ items: [job, { ...job, id: "job-2222bbbb", scheduledSlot: { startAt: "2026-11-18T06:00:00Z", endAt: "2026-11-18T08:00:00Z" } }], total: 2 });
    const d = await openAdd({ timeZone: "Asia/Tokyo" });
    expect(within(d).getByText(/^Conflicts with 2 confirmed assignments: job-1111 · .*; job-2222 · .*\. Saving keeps the assignments/)).toBeTruthy();
    expect(within(d).getByText(/The days are Kuala Lumpur days\./)).toBeTruthy();
  });

  it("checks the days before saving, then sends the technician or null, the type and the trimmed note", async () => {
    actions.setUnavailability.mockResolvedValue({ ok: true, value: { conflicts: 0 } });
    const d = await openAdd();
    fireEvent.change(within(d).getByLabelText(/^To/), { target: { value: "2026-11-17" } });
    fireEvent.click(within(d).getByRole("button", { name: "Save" }));
    expect(within(d).getByText("The last day must be on or after the first day")).toBeTruthy();
    expect(actions.setUnavailability).not.toHaveBeenCalled();
    fireEvent.change(within(d).getByLabelText(/^To/), { target: { value: "2026-11-19" } });
    fireEvent.change(within(d).getByLabelText("Type"), { target: { value: "sick" } });
    fireEvent.change(within(d).getByLabelText(/^Note \(optional\)/), { target: { value: "  Flu  " } });
    fireEvent.click(within(d).getByRole("button", { name: "Save" }));
    await waitFor(() => { expect(screen.queryByRole("dialog")).toBeNull(); });
    expect(actions.setUnavailability).toHaveBeenCalledWith({ membershipId: "m-1", from: "2026-11-18", to: "2026-11-19", type: "sick", note: "Flu" });
    expect(toasts).toStrictEqual(["Unavailable days saved — the available hours of those days are 0."]);
    // the whole team, no note: null and no note at all; the toast counts the overlapping assignments
    for (const [conflicts, text] of [[1, "1 confirmed assignment overlaps them"], [3, "3 confirmed assignments overlap them"]] as const) {
      actions.setUnavailability.mockResolvedValue({ ok: true, value: { conflicts } });
      fireEvent.click(screen.getByRole("button", { name: "Unavailable days" }));
      fireEvent.change(within(dialog()).getByLabelText("Technician"), { target: { value: "" } });
      fireEvent.click(within(dialog()).getByRole("button", { name: "Save" }));
      await waitFor(() => { expect(screen.queryByRole("dialog")).toBeNull(); });
      expect(actions.setUnavailability).toHaveBeenLastCalledWith({ membershipId: null, from: "2026-11-18", to: "2026-11-18", type: "annual_leave" }); // and no note
      expect(toasts.at(-1)).toContain(text);
    }
  });

  it("shows the fields the Core API refused, clears them on an edit, and says a technician who left", async () => {
    actions.setUnavailability.mockResolvedValue({ ok: false, code: "VALIDATION", fieldErrors: { note: "error.length" } });
    const d = await openAdd();
    fireEvent.click(within(d).getByRole("button", { name: "Save" }));
    expect(await within(d).findByText("At most 500 characters")).toBeTruthy();
    fireEvent.change(within(d).getByLabelText(/^Note \(optional\)/), { target: { value: "x" } });
    expect(within(d).queryByText("At most 500 characters")).toBeNull();
    actions.setUnavailability.mockResolvedValue({ ok: false, code: "NOT_FOUND", fieldErrors: {} });
    fireEvent.click(within(d).getByRole("button", { name: "Save" }));
    expect(await within(d).findByText("That technician is no longer in your company — reload the page.")).toBeTruthy();
    fireEvent.click(within(d).getByRole("button", { name: "Cancel" }));
    expect(screen.queryByRole("dialog")).toBeNull();
  });
});

describe("Remove unavailable days", () => {
  it("lists the row and removes all of it from its first day", async () => {
    actions.clearUnavailability.mockResolvedValue({ ok: true, value: { kept: false } });
    const d = await openRemove();
    expect(screen.getByText("Annual leave · Trip")).toBeTruthy();
    expect(within(d).getByText("All of these days are removed.")).toBeTruthy();
    fireEvent.click(within(d).getByRole("button", { name: "Remove" }));
    await waitFor(() => { expect(actions.clearUnavailability).toHaveBeenCalledWith("un-1", 3, "2026-11-25"); });
    expect(toasts).toStrictEqual(["Unavailable days removed — Aiman can take visits on those days again."]);
  });

  it("keeps the days before a later day, and refuses a day outside the row before sending", async () => {
    actions.clearUnavailability.mockResolvedValue({ ok: true, value: { kept: true } });
    const d = await openRemove();
    const from = within(d).getByLabelText("Remove from");
    fireEvent.change(from, { target: { value: "2026-11-28" } });
    fireEvent.click(within(d).getByRole("button", { name: "Remove" }));
    expect(within(d).getByText("Choose a day from Wed, 25 Nov to Fri, 27 Nov")).toBeTruthy();
    expect(actions.clearUnavailability).not.toHaveBeenCalled();
    fireEvent.change(from, { target: { value: "2026-11-26" } });
    expect(within(d).queryByText(/Choose a day/)).toBeNull();
    expect(within(d).getByText("Still unavailable: Wed, 25 Nov; the days from Thu, 26 Nov on are removed.")).toBeTruthy();
    fireEvent.click(within(d).getByRole("button", { name: "Remove" }));
    await waitFor(() => { expect(toasts).toStrictEqual(["Unavailable days shortened — Aiman can take visits again from Thu, 26 Nov."]); });
    expect(actions.clearUnavailability).toHaveBeenCalledWith("un-1", 3, "2026-11-26");
  });

  it("starts a row that has begun at today, and keeps the dialog open only on a VALIDATION refusal", async () => {
    actions.clearUnavailability.mockResolvedValue({ ok: false, code: "VALIDATION", fieldErrors: {} });
    const d = await openRemove([{ ...leave, from: "2026-11-16", to: "2026-11-20", note: null }]);
    expect(screen.getByText("Annual leave")).toBeTruthy();
    expect(within(d).getByLabelText("Remove from")).toHaveProperty("value", "2026-11-18");
    expect(within(d).getByText("Still unavailable: Mon 16 – Tue 17 Nov; the days from Wed, 18 Nov on are removed. Days before today stay recorded.")).toBeTruthy();
    fireEvent.click(within(d).getByRole("button", { name: "Remove" }));
    await waitFor(() => { expect(failures).toStrictEqual(["VALIDATION"]); }); // the view's failure handler has run
    expect(screen.getByRole("dialog")).toBeTruthy();
    actions.clearUnavailability.mockResolvedValue({ ok: false, code: "CONFLICT", fieldErrors: {} });
    fireEvent.click(within(d).getByRole("button", { name: "Remove" }));
    await waitFor(() => { expect(screen.queryByRole("dialog")).toBeNull(); });
    expect(failures).toStrictEqual(["VALIDATION", "CONFLICT"]);
  });
});
