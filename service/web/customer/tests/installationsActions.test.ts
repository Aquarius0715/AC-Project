import { beforeEach, describe, expect, it, vi } from "vitest";
import { isValidElement, type ComponentProps } from "react";

// The Server Actions of the customer Installations page (FR-C20, DEC-107): a new request leaves out an empty space,
// model, contact window and note, trims the texts, and a cancel goes at the row version; both re-read the page whatever
// happens. The page renders the demo outside API mode, Page unavailable for a member, and otherwise the rows with the
// properties and spaces that are not archived; its route states name the screen.
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
  return { coreOp: vi.fn(), apiMode: vi.fn(), coreAll: vi.fn(), corePrincipal: vi.fn(), coreDisplay: vi.fn(), CoreError };
});
const cache = vi.hoisted(() => ({ refresh: vi.fn() }));
vi.mock("server-only", () => ({}));
vi.mock("next/cache", () => cache);
vi.mock("next/server", () => ({ connection: () => Promise.resolve() }));
vi.mock("@ac/web/lib/dal", () => dal);
vi.mock("../app/customer/installations/_components/installations-demo", () => ({ InstallationsDemo: () => null }));
vi.mock("../app/customer/installations/_components/installations-view", () => ({ InstallationsView: () => null }));

import { cancelInstallationRequest, createInstallationRequest, type NewRequest } from "../app/customer/installations/actions";
import CustomerInstallationsPage from "../app/customer/installations/page";
import { InstallationsDemo } from "../app/customer/installations/_components/installations-demo";
import { InstallationsView } from "../app/customer/installations/_components/installations-view";
import InstallationsLoading from "../app/customer/installations/loading";
import InstallationsErrorState from "../app/customer/installations/error";
import { RouteError, RouteLoading } from "@ac/web/components/RouteStates";

const SLOTS = [{ startAt: "2026-09-20T02:00:00Z", endAt: "2026-09-20T04:00:00Z" }];
const input = (over: Partial<NewRequest> = {}): NewRequest => ({
  propertyId: "p-1", spaceId: "", quantity: 2, modelId: "", withIot: false, slots: SLOTS, contactWindow: "  ", note: "", ...over,
});

beforeEach(() => {
  vi.resetAllMocks();
  dal.coreOp.mockResolvedValue({ id: "req-1", version: 1 });
});

describe("installation request actions", () => {
  it("a new request leaves out what is empty, keeps the rest trimmed, returns the id and re-reads", async () => {
    await expect(createInstallationRequest(input())).resolves.toStrictEqual({ ok: true, value: { id: "req-1" } });
    expect(dal.coreOp.mock.lastCall).toStrictEqual(["installationRequests.create", { propertyId: "p-1", quantity: 2, withIot: false, preferredSlots: SLOTS }, { write: true }]);
    await createInstallationRequest(input({ spaceId: "s-1", modelId: "cap-1", withIot: true, contactWindow: " Weekdays 09:00-18:00 ", note: " two rooms " }));
    expect(dal.coreOp.mock.lastCall).toStrictEqual(["installationRequests.create", {
      propertyId: "p-1", quantity: 2, withIot: true, preferredSlots: SLOTS, spaceId: "s-1", modelId: "cap-1", contactWindow: "Weekdays 09:00-18:00", note: "two rooms",
    }, { write: true }]);
    expect(cache.refresh).toHaveBeenCalledTimes(2);
  });

  it("a cancel goes at the row version, and a refusal comes back with its code and fields after a re-read", async () => {
    await expect(cancelInstallationRequest("req-1", 4)).resolves.toStrictEqual({ ok: true, value: null });
    expect(dal.coreOp.mock.lastCall).toStrictEqual(["installationRequests.cancel", { id: "req-1" }, { write: true, expectedVersion: 4 }]);
    dal.coreOp.mockRejectedValueOnce(new dal.CoreError({ code: "CONFLICT", messageKey: "error.versionConflict", fieldErrors: {}, retryAfterSeconds: null }));
    await expect(cancelInstallationRequest("req-1", 3)).resolves.toMatchObject({ ok: false, code: "CONFLICT", messageKey: "error.versionConflict" });
    dal.coreOp.mockRejectedValueOnce(new dal.CoreError({ code: "VALIDATION", messageKey: "error.validation", fieldErrors: { quantity: "out_of_range" }, retryAfterSeconds: null }));
    await expect(createInstallationRequest(input({ quantity: 11 }))).resolves.toMatchObject({ ok: false, code: "VALIDATION", fieldErrors: { quantity: "out_of_range" } });
    expect(cache.refresh).toHaveBeenCalledTimes(3);
  });
});

const viewProps = (element: unknown) => {
  if (!isValidElement<ComponentProps<typeof InstallationsView>>(element) || element.type !== InstallationsView) throw new TypeError("not InstallationsView");
  return element.props.live;
};

describe("installations page", () => {
  it("renders the demo outside API mode without reading the Core API", async () => {
    dal.apiMode.mockReturnValue(false);
    const page = await CustomerInstallationsPage();
    expect(isValidElement(page) && page.type).toBe(InstallationsDemo);
    expect(dal.corePrincipal).not.toHaveBeenCalled();
  });

  it("shows Page unavailable to a member without reading the requests", async () => {
    dal.apiMode.mockReturnValue(true);
    dal.corePrincipal.mockResolvedValue({ clientRole: "member" });
    expect(viewProps(await CustomerInstallationsPage())).toBeNull();
    expect(dal.coreAll).not.toHaveBeenCalled();
  });

  it("gives an owner the rows with names, and only the properties and spaces that are not archived", async () => {
    dal.apiMode.mockReturnValue(true);
    dal.corePrincipal.mockResolvedValue({ clientRole: "owner" });
    dal.coreDisplay.mockResolvedValue({ locale: "en", timeZone: "Asia/Kuala_Lumpur" });
    const properties = [{ id: "p-1", name: "Home", archived: false }, { id: "p-old", name: "Old", archived: true }];
    const spaces = [{ id: "s-1", propertyId: "p-1", parentSpaceId: null, name: "Bedroom", archived: false }, { id: "s-old", propertyId: "p-1", parentSpaceId: null, name: "Gone", archived: true }];
    const models = [{ id: "cap-1", manufacturer: "Daikin", model: "FTKF" }];
    const request = {
      id: "req-12345678", version: 2, customerOrgId: "org-a", propertyId: "p-1", spaceId: "s-1", quantity: 2, modelId: "cap-1", withIot: true, status: "submitted",
      preferredSlots: SLOTS, contactWindow: null, note: null, declineReason: null, jobIds: [],
    };
    const lists = new Map<string, unknown[]>([["installationRequests.list", [request]], ["properties.list", properties], ["spaces.list", spaces], ["capabilities.list", models]]);
    dal.coreAll.mockImplementation((op: string) => Promise.resolve(lists.get(op)));
    const live = viewProps(await CustomerInstallationsPage());
    expect(live?.properties.map((p) => p.id)).toStrictEqual(["p-1"]);
    expect(live?.spaces.map((s) => s.id)).toStrictEqual(["s-1"]);
    expect(live?.models).toBe(models);
    expect(live?.rows).toMatchObject([{ id: "req-12345678", number: "req-1234", place: "Home › Bedroom", model: "Daikin FTKF", statusLabel: "Submitted" }]);
    expect(Number.isNaN(Date.parse(live?.now ?? ""))).toBe(false);
  });

  it("names the screen in its loading and error states", () => {
    const error = new Error("boom");
    const retry = vi.fn();
    const loading = InstallationsLoading();
    expect(loading.type).toBe(RouteLoading);
    expect(loading.props).toStrictEqual({ what: "installation requests" });
    const failed = InstallationsErrorState({ error, retry });
    expect(failed.type).toBe(RouteError);
    expect(failed.props).toStrictEqual({ what: "Installations", error, retry });
  });
});
