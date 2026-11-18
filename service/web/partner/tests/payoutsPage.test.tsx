import { beforeEach, describe, expect, it, vi } from "vitest";
import { isValidElement } from "react";

// the payouts page (FR-P10): the fixture demo outside API mode; in API mode the loader gets the URL keys, a repeated
// or empty key read as absent
const dal = vi.hoisted(() => ({ apiMode: vi.fn() }));
const load = vi.hoisted(() => ({ loadPayouts: vi.fn() }));
vi.mock("next/server", () => ({ connection: () => Promise.resolve() }));
vi.mock("@ac/web/lib/dal", () => dal);
vi.mock("../app/partner/payouts/_lib/load", () => load);
vi.mock("../app/partner/payouts/_components/payouts-demo", () => ({ PayoutsDemo: () => null }));
vi.mock("../app/partner/payouts/_components/payouts-view", () => ({ PayoutsView: () => null }));

import PartnerPayoutsPage from "../app/partner/payouts/page";
import { PayoutsDemo } from "../app/partner/payouts/_components/payouts-demo";
import { PayoutsView } from "../app/partner/payouts/_components/payouts-view";
import Loading from "../app/partner/payouts/loading";
import ErrorState from "../app/partner/payouts/error";
import { RouteError, RouteLoading } from "@ac/web/components/RouteStates";

type Query = Record<string, string | string[] | undefined>;
const page = (query: Query) => PartnerPayoutsPage({ params: Promise.resolve({}), searchParams: Promise.resolve(query) });

beforeEach(() => {
  vi.clearAllMocks();
  load.loadPayouts.mockResolvedValue({ marker: "live" });
});

describe("Partner payouts page", () => {
  it("renders the demo outside API mode without reading", async () => {
    dal.apiMode.mockReturnValue(false);
    const element = await page({ statementId: "stmt-1" });
    expect(isValidElement(element) && element.type).toBe(PayoutsDemo);
    expect(load.loadPayouts).not.toHaveBeenCalled();
  });

  it("passes single URL keys to the loader, drops empty and repeated ones, and renders the view with the result", async () => {
    dal.apiMode.mockReturnValue(true);
    const element = await page({ statementId: "stmt-1", period: ["2026-10", "2026-09"], status: "" });
    expect(load.loadPayouts).toHaveBeenCalledOnce();
    // JSON drops the keys read as absent; a repeated or empty value passed on would show here
    expect(JSON.stringify(load.loadPayouts.mock.calls[0]?.[0])).toBe(`{"statementId":"stmt-1"}`);
    expect(isValidElement(element) && element.type).toBe(PayoutsView);
    expect(isValidElement(element) && element.props).toStrictEqual({ live: { marker: "live" } });
  });
});

describe("Partner payouts route states", () => {
  it("names the payouts in the loading and error states and hands the retry on", () => {
    const loading = Loading();
    expect(loading.type).toBe(RouteLoading);
    expect(loading.props).toStrictEqual({ what: "payouts" });
    const error = new Error("boom");
    const retry = vi.fn();
    const failed = ErrorState({ error, retry });
    expect(failed.type).toBe(RouteError);
    expect(failed.props).toStrictEqual({ what: "Payouts", error, retry });
  });
});
