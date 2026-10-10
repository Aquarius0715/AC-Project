// @vitest-environment jsdom
import { beforeEach, describe, expect, it } from "vitest";
import { CLIENT_USERS_SEED, clientUserActions, emailError } from "@ac/web/lib/clientUsers";

// The Phase 1A client users (FR-A17, FR-C19, IR114): the owner and HQ share the rows of a tab. Seed: Tan Wei (active
// owner), Mei Tan (active member) and guest@example.com (invited member).
const owner = "cu-tan-wei", member = "cu-mei-tan", guest = "cu-guest";

beforeEach(() => {
  sessionStorage.clear();
  clientUserActions.reset();
});

describe("the client users of the demo", () => {
  it("checks an invitation's e-mail: present, well formed and not already a user (any case)", () => {
    expect([emailError(" ", CLIENT_USERS_SEED), emailError("tan.wei@", CLIENT_USERS_SEED), emailError(" MEI.TAN@example.com ", CLIENT_USERS_SEED), emailError("new@example.com", CLIENT_USERS_SEED)])
      .toEqual(["Enter an e-mail address", "Enter a valid e-mail address", "This person is already a user of customer-a (Mei Tan · Member). E-mails are matched without case.", undefined]);
  });

  it("invites a user who stays invited until they sign in, and re-sends only to the invited", () => {
    expect(clientUserActions.invite(" new@example.com ", "member", "customer-a")).toBeUndefined();
    expect(clientUserActions.invite("NEW@example.com", "member", "customer-a")).toMatch(/already a user/);
    expect([clientUserActions.resend(guest), clientUserActions.resend(member)]).toEqual([undefined, "Only invited users can be re-invited"]);
  });

  it("keeps one active owner: the last one cannot be demoted, disabled or removed", () => {
    expect(clientUserActions.setRole(owner, "member")).toBe("CONFLICT — the last active owner cannot be demoted");
    expect(clientUserActions.setStatus(owner, "disabled")).toBe("CONFLICT — the last active owner cannot be disabled");
    expect(clientUserActions.remove(owner)).toBe("CONFLICT — the last active owner cannot be removed");
    // with a second active owner each of them may go
    expect(clientUserActions.setRole(member, "owner")).toBeUndefined();
    expect(clientUserActions.setRole(owner, "member")).toBeUndefined();
    expect(clientUserActions.setStatus(owner, "disabled")).toBeUndefined();
    expect(clientUserActions.remove(guest)).toBeUndefined();
    expect(clientUserActions.remove(member)).toBe("CONFLICT — the last active owner cannot be removed");
  });
});
