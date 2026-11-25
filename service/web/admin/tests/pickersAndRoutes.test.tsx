import { beforeEach, describe, expect, it, vi } from "vitest";
import { isValidElement, type ReactNode } from "react";

// The unit and device pickers of HQ (FR-A02, FR-A04): one page of the search, each option with the customer, place
// and aircon model (a unit) or the IoT model, node ID and unit (a device), archived units left out, a device already on
// a unit marked when only free ones may be chosen. Also the small entries of the app: the certificate document route,
// the root redirect, the not-found catch-all, the admin frame and the sign-in page.
const dal = vi.hoisted(() => ({
  apiMode: vi.fn(() => true),
  coreOp: vi.fn<(op: string, input?: unknown) => Promise<unknown>>(),
  coreAll: vi.fn<(op: string, input?: unknown) => Promise<unknown>>(),
}));
const next = vi.hoisted(() => ({ redirect: vi.fn((to: string) => { throw new Error(`NEXT_REDIRECT ${to}`); }), notFound: vi.fn(() => { throw new Error("NEXT_NOT_FOUND"); }) }));
const documents = vi.hoisted(() => ({ documentResponse: vi.fn(() => Promise.resolve(new Response("pdf"))) }));
const shell = vi.hoisted(() => ({ loadShell: vi.fn(() => Promise.resolve({ user: "HQ Ops" })) }));
vi.mock("server-only", () => ({}));
vi.mock("next/server", () => ({ connection: () => Promise.resolve() }));
vi.mock("next/navigation", () => next);
vi.mock("@ac/web/lib/dal", () => dal);
vi.mock("@ac/web/lib/documentResponse", () => documents);
vi.mock("@ac/web/lib/shell", () => shell);
vi.mock("@ac/web/components/AppShell", () => ({ AppShell: () => null }));
vi.mock("@ac/web/screens/auth/login/page", () => ({ LoginScreen: () => null }));

import { deviceOptions, unitOptions } from "../app/admin/_lib/pickers";

const unit = (id: string, over: Record<string, unknown> = {}) => ({ id, version: 2, displayName: `Unit ${id}`, customerOrgId: "org-a", propertyId: "p-1", spaceId: "s-1", modelId: "m-1", archived: false, ...over });

beforeEach(() => {
  dal.coreOp.mockReset(); dal.coreAll.mockReset();
  dal.coreAll.mockImplementation((op) => {
    const lists = new Map<string, unknown[]>([
      ["customers.list", [{ id: "cust-a", name: "Alpha", organizationId: "org-a" }]], ["capabilities.list", [{ id: "m-1", manufacturer: "AC-Co", model: "Split 1" }]],
      ["properties.list", [{ id: "p-1", name: "Tower" }]], ["spaces.list", [{ id: "s-1", name: "Lobby" }]], ["iotModels.list", [{ id: "i-1", manufacturer: "IoT-Co", model: "Node" }]],
    ]);
    return Promise.resolve(lists.get(op) ?? []);
  });
});

describe("the unit picker", () => {
  it("searches one page, leaves archived units out, and writes the customer, place and model beside each name", async () => {
    dal.coreOp.mockResolvedValue({ items: [unit("u-1"), unit("u-2", { spaceId: null, modelId: "m-gone", customerOrgId: "org-x" }), unit("u-old", { archived: true })] });
    expect(await unitOptions(" lobby ", true)).toStrictEqual([
      { id: "u-1", version: 2, unitId: "u-1", blocked: null, label: "Unit u-1", detail: "Alpha · Tower › Lobby · AC-Co Split 1" },
      { id: "u-2", version: 2, unitId: "u-2", blocked: null, label: "Unit u-2", detail: "Tower" }, // no customer, room or model known
    ]);
    expect(dal.coreOp).toHaveBeenCalledWith("units.list", { limit: 20, sort: { field: "id", direction: "asc" }, filters: { search: "lobby", withoutDevice: true } });
    expect(dal.coreAll).toHaveBeenCalledWith("properties.list", { filters: { customerId: "org-a" } });
    await unitOptions("  ", false);
    expect(dal.coreOp).toHaveBeenLastCalledWith("units.list", expect.objectContaining({ filters: {} }));
  });
});

describe("the device picker", () => {
  it("writes the IoT model, node ID and unit beside each serial, and marks one on a unit when a free one is asked for", async () => {
    dal.coreOp.mockImplementation((op) => Promise.resolve(op === "devices.list"
      ? { items: [
        { id: "d-1", version: 3, serial: "AC-1", unitId: "u-1", iotModelId: "i-1", mqttNode: "node_1" }, { id: "d-2", version: 1, serial: "AC-2", unitId: null, iotModelId: null, mqttNode: null },
        { id: "d-3", version: 1, serial: "AC-3", unitId: "u-unlisted", iotModelId: "i-gone", mqttNode: null },
      ] }
      : { items: [unit("u-1")] }));
    expect(await deviceOptions(" AC ", null, true)).toStrictEqual([
      { id: "d-1", version: 3, unitId: "u-1", label: "AC-1", detail: "IoT-Co Node · node_1 · Unit u-1", blocked: "Unit u-1" },
      { id: "d-2", version: 1, unitId: null, label: "AC-2", detail: "", blocked: null },
      { id: "d-3", version: 1, unitId: "u-unlisted", label: "AC-3", detail: "u-unlisted", blocked: "u-unlisted" },
    ]);
    expect(dal.coreOp).toHaveBeenCalledWith("devices.list", { limit: 20, sort: { field: "serial", direction: "asc" }, filters: { search: "AC" } });
    expect(dal.coreOp).toHaveBeenCalledWith("units.list", { limit: 100, filters: { unitIds: ["u-1", "u-unlisted"] } });
    const bound = await deviceOptions("", false, false);
    expect(bound.every((o) => o.blocked === null)).toBe(true); // any device may be chosen
    expect(dal.coreOp).toHaveBeenCalledWith("devices.list", expect.objectContaining({ filters: { bound: false } }));
  });

  it("reads no units when no device is on one", async () => {
    dal.coreOp.mockResolvedValue({ items: [{ id: "d-2", version: 1, serial: "AC-2", unitId: null, iotModelId: null, mqttNode: null }] });
    await deviceOptions("", null, false);
    expect(dal.coreOp).toHaveBeenCalledOnce();
  });
});

describe("the small entries of the app", () => {
  it("answers a certificate document through the shared document response", async () => {
    const { GET } = await import("../app/admin/certificates/[id]/document/route");
    const res = await GET(new Request("https://hq.example/admin/certificates/cert-1/document"), { params: Promise.resolve({ id: "cert-1" }) });
    expect(await res.text()).toBe("pdf");
    expect(documents.documentResponse).toHaveBeenCalledWith("cert-1");
  });

  it("sends the root to /admin and an unknown address to not-found", async () => {
    const { default: Home } = await import("../app/page");
    expect(() => { Home(); }).toThrow("NEXT_REDIRECT /admin");
    const { default: Missing } = await import("../app/admin/[...missing]/page");
    expect(() => { Missing(); }).toThrow("NEXT_NOT_FOUND");
  });

  it("frames the admin app with the shell's live data, and shows the sign-in with the URL's parameters", async () => {
    const { default: Layout } = await import("../app/admin/layout");
    const frame: ReactNode = await Layout({ children: "page" });
    expect(isValidElement<{ appRole: string; live: unknown }>(frame) && [frame.props.appRole, frame.props.live]).toStrictEqual(["admin", { user: "HQ Ops" }]);
    expect(shell.loadShell).toHaveBeenCalledWith("admin");
    const { default: LoginPage } = await import("../app/(auth)/login/page");
    const login: ReactNode = await LoginPage({ searchParams: Promise.resolve({ next: "/admin/jobs" }) });
    expect(isValidElement<{ appRole: string; params: unknown; api: boolean }>(login) && [login.props.appRole, login.props.params, login.props.api]).toStrictEqual(["admin", { next: "/admin/jobs" }, true]);
  });
});
