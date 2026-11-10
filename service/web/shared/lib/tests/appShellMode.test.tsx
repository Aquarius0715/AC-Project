// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";

const nav = vi.hoisted(() => ({ pathname: "/technician" }));
vi.mock("next/navigation", () => ({ usePathname: () => nav.pathname, useSearchParams: () => new URLSearchParams(), useRouter: () => ({ push: vi.fn(), replace: vi.fn(), refresh: vi.fn() }) }));

const { AppShell } = await import("@ac/web/components/AppShell");

afterEach(cleanup);

// API mode is the layout's `live` (IR241). The BFF session is read in the browser and falls back to the mock session
// when it cannot be read; until then, and after such a failure, the shell is still in API mode (IR716).
const live = { user: "Tech One", organization: "Cool Partner", badges: {}, display: { locale: "en" as const, timeZone: "Asia/Kuala_Lumpur" } };

describe("the app shell in API mode while the BFF session is not read", () => {
  it("opens the real QR scan, never the demo one with its fixture match", async () => {
    vi.stubGlobal("fetch", vi.fn(() => Promise.reject(new Error("offline"))));
    nav.pathname = "/technician";
    render(<AppShell appRole="technician" live={live}><p>page</p></AppShell>);
    await act(async () => { await Promise.resolve(); });
    fireEvent.click(screen.getByRole("button", { name: "Scan QR" }));
    expect(screen.queryByText("[ camera preview ]")).toBeNull();
    expect(screen.getByRole("textbox", { name: "Label code, device serial or unit ID" })).toBeTruthy();
    vi.unstubAllGlobals();
  });

  it("does not offer the owners' Users page to a client whose role is not known", async () => {
    vi.stubGlobal("fetch", vi.fn(() => Promise.reject(new Error("offline"))));
    nav.pathname = "/customer";
    render(<AppShell appRole="client" live={live}><p>page</p></AppShell>);
    await act(async () => { await Promise.resolve(); });
    expect(screen.queryAllByRole("link", { name: "Users" })).toHaveLength(0);
    vi.unstubAllGlobals();
  });
});
