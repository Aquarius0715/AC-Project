// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, fireEvent, render, screen, within } from "@testing-library/react";

// The app frame (FR-X01, IR241, IR258): the demo sidebar of each role with its owner-only and contractor-only links,
// the active item, the demo's own unread badge and sign-out link; in API mode the Core API's badges (none for 0),
// the locked items, the organization and user (or the role's defaults), the sign-out form; the demo page's header;
// and the mobile menu that closes on navigation.
const nav = vi.hoisted(() => ({ pathname: "/customer" }));
vi.mock("next/navigation", () => ({ usePathname: () => nav.pathname, useSearchParams: () => new URLSearchParams(), useRouter: () => ({ push: vi.fn(), replace: vi.fn(), refresh: vi.fn() }) }));

const { AppShell } = await import("@ac/web/components/AppShell");

const display = { locale: "en" as const, timeZone: "Asia/Kuala_Lumpur" };
const settle = () => act(async () => { await Promise.resolve(); });
const sidebar = () => within(screen.getAllByRole("complementary")[0] ?? document.body);

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("the demo sidebar", () => {
  it("offers the demo owner Installations and Users, marks the active item, and signs out by link", async () => {
    nav.pathname = "/customer/units/u-1";
    render(<AppShell appRole="client"><p>page</p></AppShell>);
    await settle();
    const side = sidebar();
    expect(["Installations", "Users"].map((name) => side.getByRole("link", { name }).getAttribute("href"))).toStrictEqual(["/customer/installations", "/customer/users"]);
    expect(side.getByRole("link", { name: "Units & locations" }).getAttribute("aria-current")).toBe("page"); // a unit page sits under Units & locations
    expect(side.getByRole("link", { name: /Sign out/ }).getAttribute("href")).toBe("/login");
    expect(screen.getByRole("heading", { level: 1 }).textContent).toBe("Units & locations");
  });

  it("offers the contractor its Users page, and names a page outside the menu AC Project", async () => {
    nav.pathname = "/partner/elsewhere";
    render(<AppShell appRole="contractor"><p>page</p></AppShell>);
    await settle();
    expect(sidebar().getByRole("link", { name: "Users" }).getAttribute("href")).toBe("/partner/users");
    expect(screen.getByRole("heading", { level: 1 }).textContent).toBe("AC Project");
  });

  it("marks the demo page in its header", async () => {
    nav.pathname = "/demo";
    render(<AppShell appRole="admin"><p>page</p></AppShell>);
    await settle();
    const heading = screen.getByRole("heading", { level: 1 });
    expect(heading.textContent).toBe("Demo controlsAlways labelled demo"); // its title and the always-on demo badge
    expect(heading.className).toMatch(/text-warn/);
  });
});

describe("the sidebar in API mode", () => {
  it("shows the Core API's counts (none for 0), the locked items, the organization, the user and the sign-out form", async () => {
    vi.stubGlobal("fetch", vi.fn(() => Promise.reject(new Error("offline"))));
    nav.pathname = "/admin";
    const { container } = render(<AppShell appRole="admin" live={{ display, user: "Ops One", organization: "hq tenant", badges: { "/admin/alerts": 3, "/admin/jobs": 0, "/notifications": 2 }, locked: ["/admin/audit"] }}><p>page</p></AppShell>);
    await settle();
    const side = sidebar();
    expect(side.getByRole("link", { name: /^Alert policies/ }).textContent).toBe("Alert policies3");
    expect(side.getByRole("link", { name: /^Maintenance jobs/ }).textContent).toBe("Maintenance jobs");
    expect(side.getByRole("link", { name: /^Notifications/ }).textContent).toBe("Notifications2");
    expect(side.getByRole("link", { name: /^Audit/ }).textContent).toBe("Audit · Your role can’t open this");
    expect(side.getByText("HQ TENANT")).toBeDefined();
    expect(screen.getAllByText("Admin — Ops One").length).toBeGreaterThan(0);
    expect(container.querySelector('form[action="/bff/auth/logout"]')).not.toBeNull();
  });

  it("falls back to the role's scope and says signed in without a name", async () => {
    vi.stubGlobal("fetch", vi.fn(() => Promise.reject(new Error("offline"))));
    nav.pathname = "/partner";
    render(<AppShell appRole="contractor" live={{ display, user: "", organization: "", badges: {} }}><p>page</p></AppShell>);
    await settle();
    expect(sidebar().getByText("CONTRACTOR-A")).toBeDefined();
    expect(screen.getAllByText("Contractor — signed in").length).toBeGreaterThan(0);
  });
});

describe("the mobile menu", () => {
  it("opens from the header and closes when the route changes", async () => {
    nav.pathname = "/technician";
    const { rerender } = render(<AppShell appRole="technician"><p>page</p></AppShell>);
    await settle();
    fireEvent.click(screen.getByRole("button", { name: "Open menu" }));
    expect(screen.getByRole("dialog", { name: "Menu" })).toBeDefined();
    nav.pathname = "/technician/devices";
    rerender(<AppShell appRole="technician"><p>page</p></AppShell>);
    await settle();
    expect(screen.queryByRole("dialog", { name: "Menu" })).toBeNull();
  });
});
