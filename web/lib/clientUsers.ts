"use client";

/**
 * Shared client-user mock store (FR-A17 / FR-C19 / IR114). The customer owner (Client app,
 * /customer/users) and HQ (Admin › Customers & units › Users) read and write the same rows.
 * Demo only: invitations and resends are previews; state lives in localStorage and is reset from /demo.
 */
import { useSyncExternalStore } from "react";

export type ClientRole = "owner" | "member";
export type ClientUser = { id: string; name: string | null; email: string; role: ClientRole; status: "invited" | "active" | "disabled"; lastSignIn: string | null; invitedBy?: string; invitedAt?: string };

const KEY = "ac-client-users-v1";
export const CLIENT_USERS_SEED: ClientUser[] = [
  { id: "cu-tan-wei", name: "Tan Wei", email: "tan.wei@example.com", role: "owner", status: "active", lastSignIn: "2026-09-30 08:12" },
  { id: "cu-mei-tan", name: "Mei Tan", email: "mei.tan@example.com", role: "member", status: "active", lastSignIn: "2026-09-27 21:40" },
  { id: "cu-guest", name: null, email: "guest@example.com", role: "member", status: "invited", lastSignIn: null, invitedBy: "hq-operator", invitedAt: "2026-09-29" },
];
/** Demo session customer-a signs in as Tan Wei (fixture clientRole = owner). */
export const CURRENT_CLIENT = { userId: "cu-tan-wei", name: "Tan Wei", role: "owner" as ClientRole };

let state: ClientUser[] | null = null;
const listeners = new Set<() => void>();
const load = () => {
  if (state) return state;
  state = CLIENT_USERS_SEED;
  try { const s = localStorage.getItem(KEY); if (s) state = JSON.parse(s); } catch {}
  return state!;
};
const save = (next: ClientUser[]) => {
  state = next;
  try { localStorage.setItem(KEY, JSON.stringify(next)); } catch {}
  listeners.forEach((l) => l());
};
const subscribe = (l: () => void) => { listeners.add(l); return () => listeners.delete(l); };
export const useClientUsers = () => useSyncExternalStore(subscribe, load, () => CLIENT_USERS_SEED);

export const emailError = (email: string, users: ClientUser[]) => {
  const e = email.trim();
  if (!e) return "Enter an e-mail address";
  if (!/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(e)) return "Enter a valid e-mail address";
  const dup = users.find((u) => u.email.toLowerCase() === e.toLowerCase());
  if (dup) return `This person is already a user of customer-a (${dup.name ?? dup.email} · ${dup.role === "owner" ? "Owner" : "Member"}). E-mails are matched without case.`;
  return undefined;
};

export const clientUserActions = {
  reset() { save(CLIENT_USERS_SEED); },
  /** clientUsers.save without id. Owners may only invite members (IR114); HQ may invite owners too. */
  invite(email: string, role: ClientRole, by: string) {
    const users = load();
    const err = emailError(email, users);
    if (err) return err;
    save([...users, { id: `cu-${Date.now()}`, name: null, email: email.trim(), role, status: "invited", lastSignIn: null, invitedBy: by, invitedAt: "2026-10-06" }]);
    return undefined;
  },
  /** clientUsers.resendInvite — invited users only, preview only. */
  resend(id: string) { return load().find((u) => u.id === id)?.status === "invited" ? undefined : "Only invited users can be re-invited"; },
  /** HQ only (DD-A17): the last active owner cannot be demoted, disabled or removed. */
  setRole(id: string, role: ClientRole) {
    const users = load();
    if (role === "member" && users.filter((u) => u.role === "owner" && u.status === "active" && u.id !== id).length === 0) return "CONFLICT — the last active owner cannot be demoted";
    save(users.map((u) => (u.id === id ? { ...u, role } : u)));
    return undefined;
  },
  setStatus(id: string, status: "active" | "disabled") {
    const users = load();
    const u = users.find((x) => x.id === id);
    if (status === "disabled" && u?.role === "owner" && users.filter((x) => x.role === "owner" && x.status === "active" && x.id !== id).length === 0) return "CONFLICT — the last active owner cannot be disabled";
    save(users.map((x) => (x.id === id ? { ...x, status } : x)));
    return undefined;
  },
  remove(id: string) {
    const users = load();
    const u = users.find((x) => x.id === id);
    if (u?.role === "owner" && users.filter((x) => x.role === "owner" && x.status === "active" && x.id !== id).length === 0) return "CONFLICT — the last active owner cannot be removed";
    save(users.filter((x) => x.id !== id));
    return undefined;
  },
};
