// @vitest-environment jsdom
import { beforeEach, describe, expect, it } from "vitest";
import { partnerUserDemo } from "@ac/web/lib/partnerUsersDemo";

// The Phase 1A company users of contractor-a (FR-P11, IR416), with the API's rules. Seed: contractor-a (the demo
// session, active contractor), tech-external-a (active technician) and new.tech@example.com (a technician added, not signed in yet).
beforeEach(() => {
  sessionStorage.clear();
  partnerUserDemo.reset();
});

describe("the company users of the demo", () => {
  it("adds a user once per e-mail (any case) and sets only a technician's access end (IR461)", () => {
    expect(partnerUserDemo.invite(" lim@example.com ", "technician", "2026-09-15T00:00:00Z", "Lim", "2026-12-31T15:59:59Z")).toBeUndefined();
    expect(partnerUserDemo.invite("LIM@example.com", "contractor", "2026-09-15T00:00:00Z", "Lim")).toBe("errors.email_taken");
    expect(partnerUserDemo.setAccess("pu-tech-external-a", "2027-03-31T15:59:59Z")).toBeUndefined();
    expect(partnerUserDemo.setAccess("pu-contractor-a", "2027-03-31T15:59:59Z")).toBe("error.notFound"); // not a technician
  });

  it("never changes the own account, keeps an active contractor, and disables / enables / removes the others", () => {
    expect(partnerUserDemo.setStatus("pu-contractor-a", "disabled")).toBe("errors.own_account");
    expect(partnerUserDemo.remove("pu-contractor-a")).toBe("errors.own_account");
    expect(partnerUserDemo.setStatus("pu-tech-external-a", "disabled")).toBeUndefined();
    expect(partnerUserDemo.setStatus("pu-tech-external-a", "active")).toBeUndefined();
    expect(partnerUserDemo.remove("pu-invite")).toBeUndefined();
    expect(partnerUserDemo.remove("pu-invite")).toBe("error.notFound");
  });
});
