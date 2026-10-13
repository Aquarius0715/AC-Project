// @vitest-environment jsdom
import { beforeEach, describe, expect, it } from "vitest";
import { partnerUserDemo } from "@ac/web/lib/partnerUsersDemo";

// The Phase 1A company users of contractor-a (FR-P11, IR416), with the API's rules. Seed: contractor-a (the demo
// session, active contractor), tech-external-a (active technician) and new.tech@example.com (invited technician).
beforeEach(() => {
  sessionStorage.clear();
  partnerUserDemo.reset();
});

describe("the company users of the demo", () => {
  it("invites once per e-mail (any case) and edits only a pending invite", () => {
    expect(partnerUserDemo.invite(" lim@example.com ", "technician", "2026-09-15T00:00:00Z")).toBeUndefined();
    expect(partnerUserDemo.invite("LIM@example.com", "contractor", "2026-09-15T00:00:00Z")).toBe("errors.email_taken");
    expect(partnerUserDemo.editInvite("pu-invite", "tech-external-a@AC.local", "technician")).toBe("errors.email_taken");
    expect(partnerUserDemo.editInvite("pu-invite", "second@example.com", "contractor")).toBeUndefined();
    expect(partnerUserDemo.editInvite("pu-tech-external-a", "x@example.com", "contractor")).toBe("error.notFound"); // accepted: fixed
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
