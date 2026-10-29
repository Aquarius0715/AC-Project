// The page's name in the shell header and the document title (WCAG 2.4.2, IR490): the menu item with the longest matching
// path, the shared pages, Users and Demo controls, in the display language.
import { describe, expect, it } from "vitest";
import { documentTitle, pageLabel, ROLES, SHARED_PAGES } from "@ac/web/lib/nav";
import { ms } from "@ac/web/lib/i18n-ms";

describe("page names and document titles (IR490)", () => {
  it("names a page by its menu item, the longest matching path winning", () => {
    expect([
      pageLabel("client", "/customer"), pageLabel("client", "/customer/units/u1"), pageLabel("client", "/customer/payments/i1"),
      pageLabel("admin", "/admin/billing/contracts"), pageLabel("admin", "/admin/billing"), pageLabel("technician", "/technician"),
    ]).toEqual(["Overview", "Units & locations", "Contracts & payments", "Contracts", "Billing", "Overview"]);
    // the pages a job leads to belong to the jobs item: the partner's unit view, the technician's job and unit pages
    expect([pageLabel("contractor", "/partner/units/u1"), pageLabel("technician", "/technician/jobs/j1"), pageLabel("technician", "/technician/units/u1/control")])
      .toEqual(["Jobs", "Assigned jobs", "Assigned jobs"]);
  });

  it("names the shared pages, Users and Demo controls, and nothing off the menu", () => {
    expect([pageLabel("client", "/notifications"), pageLabel("admin", "/settings/preferences"), pageLabel("client", "/customer/users"), pageLabel("contractor", "/partner/users"), pageLabel("technician", "/demo"), pageLabel("client", "/login")])
      .toEqual(["Notifications", "Preferences", "Users", "Users", "Demo controls", null]);
  });

  it("titles the document in the display language", () => {
    expect([documentTitle("client", "/customer/alerts", "en"), documentTitle("client", "/customer/alerts", "ms"), documentTitle("client", "/login", "en")])
      .toEqual(["Alerts — AC Project", "Amaran — AC Project", "AC Project — Customer app"]);
  });

  it("has a Malay name for every menu item and shared page", () => {
    const labels = [...Object.values(ROLES).flatMap((r) => [...r.nav.map((i) => i.label), r.sub]), ...SHARED_PAGES.map((s) => s.label), "Users", "Demo controls"];
    expect(labels.filter((l) => !(l in ms))).toEqual([]);
  });
});
