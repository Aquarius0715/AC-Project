"use client";

/**
 * Phase 1A demo store of the contractor's company users (FR-P11, IR416): the same PartnerUser rows and rules as
 * partnerUsers.* (own account untouched, the last active contractor kept, an accepted account's e-mail and role fixed).
 * State lives in the tab (lib/demoStore: a reload or /demo reset restores the seed, FR-X05); nothing is sent.
 */
import { useSyncExternalStore } from "react";
import { readDemo, writeDemo } from "@ac/web/lib/demoStore";
import type { ApiPartnerUser, PartnerRole } from "@ac/web/lib/partnerUsers";

const KEY = "ac-partner-users-v1";
const ORG = "org-contractor-a";
/** The demo session signs in as contractor-a. */
export const DEMO_SELF = "contractor-a";
const at = "2026-09-01T00:00:00Z";
export const PARTNER_USERS_SEED: ApiPartnerUser[] = [
  { id: "pu-contractor-a", version: 1, organizationId: ORG, membershipId: "contractor-a", email: "contractor-a@ac.local", displayName: "contractor-a", partnerRole: "contractor", status: "active", lastSignInAt: "2026-09-14T23:55:00Z", accessUntil: "2026-10-01T00:00:00Z", invitedAt: at, invitedByMembershipId: "contractor-a" },
  { id: "pu-tech-external-a", version: 1, organizationId: ORG, membershipId: "tech-external-a", email: "tech-external-a@ac.local", displayName: "tech-external-a", partnerRole: "technician", status: "active", lastSignInAt: "2026-09-14T22:10:00Z", accessUntil: "2026-10-01T00:00:00Z", invitedAt: at, invitedByMembershipId: "contractor-a" },
  // added with an account (IR461), not signed in yet
  { id: "pu-invite", version: 1, organizationId: ORG, membershipId: "m-pu-invite", email: "new.tech@example.com", displayName: "New Tech", partnerRole: "technician", status: "invited", lastSignInAt: null, accessUntil: "2026-12-31T15:59:59Z", invitedAt: "2026-09-12T02:00:00Z", invitedByMembershipId: "contractor-a" },
];

let state: ApiPartnerUser[] | null = null;
const listeners = new Set<() => void>();
const load = () => (state ??= readDemo<ApiPartnerUser[]>(KEY) ?? PARTNER_USERS_SEED);
const save = (next: ApiPartnerUser[]) => {
  state = next;
  writeDemo(KEY, next);
  listeners.forEach((l) => l());
};
const subscribe = (l: () => void) => { listeners.add(l); return () => listeners.delete(l); };
export const usePartnerUsersDemo = () => useSyncExternalStore(subscribe, load, () => PARTNER_USERS_SEED);

const lastContractor = (us: ApiPartnerUser[], u: ApiPartnerUser) =>
  u.partnerRole === "contractor" && u.status === "active" && !us.some((x) => x.id !== u.id && x.partnerRole === "contractor" && x.status === "active");
/** A refusal as the API words it (messageKey), or undefined when done. */
const guard = (us: ApiPartnerUser[], u: ApiPartnerUser | undefined) =>
  !u ? "error.notFound" : u.membershipId === DEMO_SELF ? "errors.own_account" : lastContractor(us, u) ? "errors.last_contractor" : undefined;

export const partnerUserDemo = {
  reset() { save(PARTNER_USERS_SEED); },
  /** Adds a user: the account exists at once (IR461) and is listed as not signed in yet. */
  invite(email: string, role: PartnerRole, now: string, name = "", accessUntil: string | null = null) {
    const us = load();
    if (us.some((u) => u.email.toLowerCase() === email.trim().toLowerCase())) return "errors.email_taken";
    const id = `pu-${Date.now()}`;
    save([...us, { id, version: 1, organizationId: ORG, membershipId: `m-${id}`, email: email.trim(), displayName: name.trim() || null, partnerRole: role, status: "invited", lastSignInAt: null,
      accessUntil: role === "technician" ? accessUntil : null, invitedAt: now, invitedByMembershipId: DEMO_SELF }]);
    return undefined;
  },
  /** A technician's access end (IR461). */
  setAccess(id: string, accessUntil: string) {
    const us = load();
    const u = us.find((x) => x.id === id);
    if (!u || u.partnerRole !== "technician") return "error.notFound";
    if (u.membershipId === DEMO_SELF) return "errors.own_account";
    save(us.map((x) => (x.id === id ? { ...x, accessUntil, version: x.version + 1 } : x)));
    return undefined;
  },
  setStatus(id: string, status: "active" | "disabled") {
    const us = load();
    const u = us.find((x) => x.id === id);
    const no = status === "disabled" ? guard(us, u) : !u ? "error.notFound" : u.membershipId === DEMO_SELF ? "errors.own_account" : undefined;
    if (no) return no;
    save(us.map((x) => (x.id === id ? { ...x, status, version: x.version + 1 } : x)));
    return undefined;
  },
  remove(id: string) {
    const us = load();
    const no = guard(us, us.find((x) => x.id === id));
    if (no) return no;
    save(us.filter((x) => x.id !== id));
    return undefined;
  },
};
