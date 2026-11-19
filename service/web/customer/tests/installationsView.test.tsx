// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import type { ActionFailure } from "@ac/web/lib/actionMessage";
import type { ApiPropertyRow, ApiSpaceRow } from "@ac/web/lib/assets";
import type { ApiCapability } from "@ac/web/lib/devices";
import type { InstallationRow } from "@ac/web/lib/installationRequests";

// Installation requests (FR-C20, DEC-107): the rows, cancelling a submitted request, and the new-request dialog with its
// local checks, the Core API's field refusals and the input it sends
type Created = { ok: true; value: { id: string } } | ({ ok: false } & ActionFailure);
type Cancelled = { ok: true; value: null } | ({ ok: false } & ActionFailure);
const actions = vi.hoisted(() => ({
  createInstallationRequest: vi.fn<(input: unknown) => Promise<Created>>(),
  cancelInstallationRequest: vi.fn<(id: string, version: number) => Promise<Cancelled>>(),
}));
vi.mock("../app/customer/installations/actions", () => actions);
// the dialog's preferred-times input comes from the maintenance view, whose Server Actions are server-only
vi.mock("../app/customer/maintenance/actions", () => ({}));
const runs = vi.hoisted((): { toasts: string[] } => ({ toasts: [] }));
vi.mock("@ac/web/lib/useAction", () => ({
  useAction: () => [false, <T,>(fn: () => Promise<{ ok: true; value: T } | ({ ok: false } & ActionFailure)>, ok: string | ((v: T) => string), after?: (v: T) => void, failed?: (f: ActionFailure) => void) => {
    void fn().then((r) => {
      if (r.ok) { runs.toasts.push(typeof ok === "string" ? ok : ok(r.value)); after?.(r.value); } else failed?.(r);
    });
  }] as const,
}));

import { InstallationsView } from "../app/customer/installations/_components/installations-view";
import { InstallationsDemo } from "../app/customer/installations/_components/installations-demo";

const property = (id: string, name: string): ApiPropertyRow => ({
  id, name, version: 1, customerOrgId: "org-a", kind: "home", address: null, accessInstructions: null, location: null, geofenceRadiusMeters: null, archived: false, updatedAt: "2026-09-01T00:00:00Z",
});
const space = (id: string, propertyId: string, name: string): ApiSpaceRow => ({ id, propertyId, name, version: 1, parentSpaceId: null, kind: "room", archived: false });
const model: ApiCapability = {
  id: "cap-1", version: 1, updatedAt: "2026-09-01T00:00:00Z", manufacturer: "Daikin", model: "FTKF", control: true, modeControl: true, fanControl: true,
  temperature: null, modes: [], fanLevels: [], ventilation: false, ventilationLevels: [],
};
const row = (id: string, over: Partial<InstallationRow> = {}): InstallationRow => ({
  id, version: 2, number: id.slice(0, 8), place: "Home", quantity: 2, model: "Not decided", withIot: false, status: "submitted", statusLabel: "Submitted", tone: "warn",
  firstTime: "Sep 20, 10:00", note: null, declineReason: null, jobIds: [], customerOrgId: "org-a", propertyId: "p-1", modelId: null, slots: [], contactWindow: null, ...over,
});
const live = (rows: InstallationRow[] = []) => ({
  rows, now: "2026-09-15T00:00:00Z", models: [model],
  properties: [property("p-1", "Home"), property("p-2", "Office")], spaces: [space("s-1", "p-1", "Bedroom"), space("s-2", "p-2", "Lobby")],
});
const dialog = () => within(screen.getByRole("dialog"));
const send = () => { fireEvent.click(dialog().getByRole("button", { name: "Send request" })); };
const field = (name: string | RegExp) => dialog().getByLabelText(name);

beforeEach(() => {
  runs.toasts = [];
  actions.createInstallationRequest.mockReset().mockResolvedValue({ ok: true, value: { id: "abcdef12-3456" } });
  actions.cancelInstallationRequest.mockReset().mockResolvedValue({ ok: true, value: null });
});
afterEach(() => { cleanup(); });

describe("InstallationsView", () => {
  it("shows Page unavailable to a member (no data)", () => {
    render(<InstallationsView live={null} />);
    expect(screen.getByText("Page unavailable")).toBeDefined();
    expect(screen.queryByRole("button", { name: /New request/ })).toBeNull();
  });

  it("lists the requests: the IoT choice, HQ's reason, the job links and Cancel only while submitted", () => {
    render(<InstallationsView live={live([
      row("req-sub-1", { withIot: true }),
      row("req-dec-1", { status: "declined", statusLabel: "Declined", tone: "crit", declineReason: "No power line" }),
      row("req-app-1", { status: "approved", statusLabel: "Approved", tone: "ok", jobIds: ["job-1234567890", "job-abcdefghij"] }),
    ])} />);
    const rows = screen.getAllByRole("row").slice(1);
    expect(rows).toHaveLength(3);
    const [sub, dec, app] = rows.map((r) => within(r));
    expect(sub?.getByText("with an IoT device")).toBeDefined();
    expect(sub?.getByRole("button", { name: "Cancel request" })).toBeDefined();
    expect(dec?.getByText("HQ: No power line")).toBeDefined();
    expect(dec?.getByText("without an IoT device")).toBeDefined();
    expect(dec?.queryByRole("button", { name: "Cancel request" })).toBeNull();
    expect(dec?.getByText("—")).toBeDefined();
    expect(app?.getByRole("link", { name: "job-1234" }).getAttribute("href")).toBe("/customer/maintenance?jobId=job-1234567890");
    expect(app?.getByRole("link", { name: "job-abcd" })).toBeDefined();
    expect(app?.queryByRole("button", { name: "Cancel request" })).toBeNull();
  });

  it("cancels a submitted request with its id and version, and Keep it closes without a call", async () => {
    render(<InstallationsView live={live([row("req-sub-1")])} />);
    fireEvent.click(screen.getByRole("button", { name: "Cancel request" }));
    expect(screen.getByRole("dialog").textContent).toContain("Cancel request req-sub-");
    fireEvent.click(dialog().getByRole("button", { name: "Keep it" }));
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(actions.cancelInstallationRequest).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button", { name: "Cancel request" }));
    fireEvent.click(dialog().getByRole("button", { name: "Cancel request" }));
    await waitFor(() => { expect(screen.queryByRole("dialog")).toBeNull(); });
    expect(actions.cancelInstallationRequest).toHaveBeenCalledWith("req-sub-1", 2);
    expect(runs.toasts).toStrictEqual(["Request req-sub- cancelled"]);
  });

  it("sends the defaults with the chosen space, model and IoT device, and names the new request", async () => {
    render(<InstallationsView live={live()} />);
    fireEvent.click(screen.getByRole("button", { name: /New request/ }));
    const spaces = field("Space (optional)");
    expect([...spaces.querySelectorAll("option")].map((o) => o.textContent)).toStrictEqual(["Any space of the property", "Bedroom"]);
    fireEvent.change(spaces, { target: { value: "s-1" } });
    fireEvent.change(field("Model"), { target: { value: "cap-1" } });
    fireEvent.click(dialog().getByRole("checkbox"));
    fireEvent.change(field("Number of units (1–10)"), { target: { value: "3" } });
    fireEvent.change(field(/^Note \(optional\)/), { target: { value: " two rooms " } });
    expect(dialog().getByText("9 / 2000")).toBeDefined();
    send();
    await waitFor(() => { expect(screen.queryByRole("dialog")).toBeNull(); });
    expect(actions.createInstallationRequest).toHaveBeenCalledOnce();
    expect(actions.createInstallationRequest.mock.calls[0]?.[0]).toMatchObject({ propertyId: "p-1", spaceId: "s-1", modelId: "cap-1", withIot: true, quantity: 3, note: " two rooms ", contactWindow: "" });
    expect(runs.toasts).toStrictEqual(["Request abcdef12 sent — HQ will decide and offer a visit time."]);
  });

  it("a new property lists its own spaces and drops the chosen space", () => {
    render(<InstallationsView live={live()} />);
    fireEvent.click(screen.getByRole("button", { name: /New request/ }));
    fireEvent.change(field("Space (optional)"), { target: { value: "s-1" } });
    fireEvent.change(field("Property"), { target: { value: "p-2" } });
    const spaces = field("Space (optional)");
    expect(spaces instanceof HTMLSelectElement && spaces.value).toBe("");
    expect([...spaces.querySelectorAll("option")].map((o) => o.textContent)).toStrictEqual(["Any space of the property", "Lobby"]);
  });

  it("refuses a bad quantity and a long note before sending, focusing the first field to fix", async () => {
    render(<InstallationsView live={live()} />);
    fireEvent.click(screen.getByRole("button", { name: /New request/ }));
    const quantity = field("Number of units (1–10)");
    fireEvent.change(quantity, { target: { value: "11" } });
    fireEvent.change(field(/^Note \(optional\)/), { target: { value: "x".repeat(2001) } });
    expect(dialog().queryByText("A whole number of 1–10")).toBeNull(); // no error before the first try
    send();
    expect(dialog().getByText("A whole number of 1–10")).toBeDefined();
    expect(dialog().getByText("Up to 2000 characters")).toBeDefined();
    await waitFor(() => { expect(document.activeElement).toBe(quantity); });
    expect(actions.createInstallationRequest).not.toHaveBeenCalled();
  });

  it("refuses a contact window with a phone number and a time on today, focusing the times first", async () => {
    render(<InstallationsView live={live()} />);
    fireEvent.click(screen.getByRole("button", { name: /New request/ }));
    const contact = field(/^Contact window \(optional\)/);
    fireEvent.change(contact, { target: { value: "call 0123456789" } });
    const firstDate = dialog().getByLabelText("Date 1");
    fireEvent.change(firstDate, { target: { value: "2026-09-15" } });
    send();
    expect(dialog().getByText("No e-mail addresses or phone numbers — use HH:mm for times (example: Weekdays 09:00-18:00).")).toBeDefined();
    expect(dialog().getByText("Times start on a later day than today (Kuala Lumpur).")).toBeDefined();
    await waitFor(() => { expect(document.activeElement).toBe(firstDate); });
    expect(actions.createInstallationRequest).not.toHaveBeenCalled();
  });

  it("asks for a property when the customer has none", () => {
    render(<InstallationsView live={{ ...live(), properties: [], spaces: [] }} />);
    fireEvent.click(screen.getByRole("button", { name: /New request/ }));
    send();
    expect(dialog().getByText("Choose a property")).toBeDefined();
    expect(actions.createInstallationRequest).not.toHaveBeenCalled();
  });

  it("shows a Core API refusal under its field until that field is edited", async () => {
    actions.createInstallationRequest.mockResolvedValue({ ok: false, code: "VALIDATION", messageKey: "error.validation", fieldErrors: { quantity: "out_of_range" } });
    render(<InstallationsView live={live()} />);
    fireEvent.click(screen.getByRole("button", { name: /New request/ }));
    send();
    const quantity = field("Number of units (1–10)");
    await waitFor(() => { expect(document.activeElement).toBe(quantity); });
    expect(screen.getByRole("dialog").textContent).toContain("out of range");
    fireEvent.change(quantity, { target: { value: "2" } });
    expect(screen.getByRole("dialog").textContent).not.toContain("out of range");
  });

  it("a refused space shows under Property and clears once another space is chosen", async () => {
    actions.createInstallationRequest.mockResolvedValue({ ok: false, code: "VALIDATION", messageKey: "error.validation", fieldErrors: { spaceId: "not_found" } });
    render(<InstallationsView live={live()} />);
    fireEvent.click(screen.getByRole("button", { name: /New request/ }));
    fireEvent.change(field("Space (optional)"), { target: { value: "s-1" } });
    send();
    await waitFor(() => { expect(screen.getByRole("dialog").textContent).toContain("not found"); });
    fireEvent.change(field("Space (optional)"), { target: { value: "" } });
    expect(screen.getByRole("dialog").textContent).not.toContain("not found");
  });

  it("a refusal of a field with no error text of its own moves no focus, and Cancel closes", async () => {
    actions.createInstallationRequest.mockResolvedValue({ ok: false, code: "VALIDATION", messageKey: "error.validation", fieldErrors: { modelId: "not_found" } });
    render(<InstallationsView live={live()} />);
    fireEvent.click(screen.getByRole("button", { name: /New request/ }));
    await new Promise(requestAnimationFrame); // the dialog's own first focus
    const modelBox = field("Model");
    modelBox.focus();
    send();
    await waitFor(() => { expect(actions.createInstallationRequest).toHaveBeenCalledOnce(); });
    await new Promise(requestAnimationFrame);
    expect(document.activeElement).toBe(modelBox);
    expect(screen.getByRole("dialog").textContent).not.toContain("not found");
    fireEvent.click(dialog().getByRole("button", { name: "Cancel" }));
    expect(screen.queryByRole("dialog")).toBeNull();
  });
});

describe("InstallationsDemo", () => {
  it("says the demo keeps no requests", () => {
    render(<InstallationsDemo />);
    expect(screen.getByText("Not in the demo")).toBeDefined();
  });
});
