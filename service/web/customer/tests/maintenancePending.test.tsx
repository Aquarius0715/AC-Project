// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import type { MaintenanceLive } from "../app/customer/maintenance/_lib/load";

// While the other tab is on its way (the router's transition is pending), My requests and Filter care each show a
// loading card instead of the tab the server sent last. Its own file: React's useTransition is held pending here.
vi.mock("react", async (importOriginal) => ({ ...(await importOriginal<Record<string, unknown>>()), useTransition: () => [true, (fn: () => void) => { fn(); }] }));
vi.mock("next/navigation", () => ({ useRouter: () => ({ push: vi.fn(), replace: vi.fn() }), useSearchParams: () => new URLSearchParams() }));
vi.mock("../app/customer/maintenance/actions", () => ({}));

import { MaintenanceView } from "../app/customer/maintenance/_components/maintenance-view";

const settings = { id: "fs", version: 0, customerId: "c", thresholdHours: null, fallbackDays: 90, recipients: "owners" as const, channels: ["inApp" as const] };
const base = { now: "2026-09-15T01:00:00.000Z", owner: true, units: [], display: { locale: "en" as const, timeZone: "Asia/Kuala_Lumpur" }, requestUnitId: null, rateBanner: null, detail: null };

afterEach(() => { cleanup(); });

describe("MaintenanceView while a tab is on its way", () => {
  it("shows a loading card for the requests and for filter care", () => {
    const requests: MaintenanceLive = { ...base, page: "requests", rows: [], filters: null };
    const { unmount } = render(<MaintenanceView live={requests} />);
    expect(screen.getByText("Loading your requests…")).toBeDefined();
    unmount();
    const filters: MaintenanceLive = { ...base, page: "filters", rows: [], filters: { settings, lines: [], reminders: [], form: { useDefault: true, hours: "250", days: "90", recipients: "owners", email: false } } };
    render(<MaintenanceView live={filters} />);
    expect(screen.getByText("Loading filter care…")).toBeDefined();
  });
});
