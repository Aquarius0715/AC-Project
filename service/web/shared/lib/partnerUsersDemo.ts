"use client";
const CONTRACTOR = "contractor";
const TECHNICIAN = "technician";
const KEY = "ac-partner-users-v1";
const ORG = "org-contractor-a";
const NOT_FOUND = "error.notFound";
const OWN_ACCOUNT = "errors.own_account";
const at = "2026-09-01T00:00:00Z";

/**
Phase 1A demo store of the contractor's company users (FR-P11, IR416): the same PartnerUser rows and rules as
partnerUsers.* (own account untouched, the last active contractor kept, an accepted account's e-mail and role fixed).
State lives in the tab (lib/demoStore: a reload or /demo reset restores the seed, FR-X05); nothing is sent.
*/
import { useSyncExternalStore } from "react";
import { z } from "zod";
import { readDemoAs, writeDemo } from "@ac/web/lib/demoStore";
import type { ApiPartnerUser, PartnerRole } from "@ac/web/lib/partnerUsers";

/** The demo session signs in as contractor-a. */
export const DEMO_SELF = "contractor-a";
export const PARTNER_USERS_SEED: ApiPartnerUser[] = [
  { id: "pu-contractor-a", version: 1, organizationId: ORG, membershipId: DEMO_SELF, email: "contractor-a@ac.local", displayName: DEMO_SELF, partnerRole: CONTRACTOR, status: "active", lastSignInAt: "2026-09-14T23:55:00Z", accessUntil: "2026-10-01T00:00:00Z", invitedAt: at, invitedByMembershipId: DEMO_SELF },
  { id: "pu-tech-external-a", version: 1, organizationId: ORG, membershipId: "tech-external-a", email: "tech-external-a@ac.local", displayName: "tech-external-a", partnerRole: TECHNICIAN, status: "active", lastSignInAt: "2026-09-14T22:10:00Z", accessUntil: "2026-10-01T00:00:00Z", invitedAt: at, invitedByMembershipId: DEMO_SELF },
  // added with an account (IR461), not signed in yet
  { id: "pu-invite", version: 1, organizationId: ORG, membershipId: "m-pu-invite", email: "new.tech@example.com", displayName: "New Tech", partnerRole: TECHNICIAN, status: "invited", lastSignInAt: null, accessUntil: "2026-12-31T15:59:59Z", invitedAt: "2026-09-12T02:00:00Z", invitedByMembershipId: DEMO_SELF },
];

const partnerUser = z.object({
  id: z.string(), version: z.number(), organizationId: z.string(), membershipId: z.string().nullable(), email: z.string(), displayName: z.string().nullable(),
  partnerRole: z.enum([CONTRACTOR, TECHNICIAN]), status: z.enum(["invited", "active", "disabled"]), lastSignInAt: z.string().nullable(),
  accessUntil: z.string().nullable(), invitedAt: z.string(), invitedByMembershipId: z.string(), booked: z.boolean().optional(), revokedAt: z.string().nullable().optional(),
}) satisfies z.ZodType<ApiPartnerUser>;

let state: ApiPartnerUser[] | null = null;
const listeners = new Set<() => void>();
const load = () => (state ??= readDemoAs(KEY, z.array(partnerUser)) ?? PARTNER_USERS_SEED);
const save = (next: ApiPartnerUser[]) => {
  state = next;
  writeDemo(KEY, next);
  listeners.forEach((l) => { l(); });
};
const subscribe = (l: () => void) => { listeners.add(l); return () => listeners.delete(l); };
export const usePartnerUsersDemo = () => useSyncExternalStore(subscribe, load, () => PARTNER_USERS_SEED);

const lastContractor = (us: ApiPartnerUser[], u: ApiPartnerUser) =>
  u.partnerRole === CONTRACTOR && u.status === "active" && us.every((x) => !(x.id !== u.id && x.partnerRole === CONTRACTOR && x.status === "active"));
/** A refusal as the API words it (messageKey), or undefined when done. */
const guard = (us: ApiPartnerUser[], u: ApiPartnerUser | undefined) => {
  if (!u) return NOT_FOUND;
  if (u.membershipId === DEMO_SELF) return OWN_ACCOUNT;
  return lastContractor(us, u) ? "errors.last_contractor" : undefined;
};
/** The refusal of a change to a user's own account, or of one that does not exist. */
const selfGuard = (u: ApiPartnerUser | undefined) => {
  if (!u) return NOT_FOUND;
  return u.membershipId === DEMO_SELF ? OWN_ACCOUNT : undefined;
};

export const partnerUserDemo = {
  reset() { save(PARTNER_USERS_SEED); },
  /** Adds a user: the account exists at once (IR461) and is listed as not signed in yet. */
  invite(email: string, role: PartnerRole, now: string, name = "", accessUntil: string | null = null) {
    const us = load();
    if (us.some((u) => u.email.toLowerCase() === email.trim().toLowerCase())) return "errors.email_taken";
    const id = `pu-${String(Date.now())}`;
    save([...us, { id, version: 1, organizationId: ORG, membershipId: `m-${id}`, email: email.trim(), displayName: name.trim() || null, partnerRole: role, status: "invited", lastSignInAt: null,
      accessUntil: role === TECHNICIAN ? accessUntil : null, invitedAt: now, invitedByMembershipId: DEMO_SELF }]);
    return null;
  },
  /** A technician's access end (IR461). */
  setAccess(id: string, accessUntil: string) {
    const us = load();
    const u = us.find((x) => x.id === id);
    if (u?.partnerRole !== TECHNICIAN) return NOT_FOUND;
    if (u.membershipId === DEMO_SELF) return OWN_ACCOUNT;
    save(us.map((x) => (x.id === id ? { ...x, accessUntil, version: x.version + 1 } : x)));
    return null;
  },
  setStatus(id: string, status: "active" | "disabled") {
    const us = load();
    const u = us.find((x) => x.id === id);
    const no = status === "disabled" ? guard(us, u) : selfGuard(u);
    if (no) return no;
    save(us.map((x) => (x.id === id ? { ...x, status, version: x.version + 1 } : x)));
    return null;
  },
  remove(id: string) {
    const us = load();
    const no = guard(us, us.find((x) => x.id === id));
    if (no) return no;
    save(us.filter((x) => x.id !== id));
    return null;
  },
};
