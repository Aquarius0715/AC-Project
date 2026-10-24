"use client";

import { useState } from "react";
import { partnerUserRows, type PartnerStatus } from "@ac/web/lib/partnerUsers";
import { DEMO_SELF, partnerUserDemo, usePartnerUsersDemo } from "@ac/web/lib/partnerUsersDemo";
import { UsersView, type UsersOps } from "./users-view";

/** A demo refusal as the API would answer it. */
const done = (refused: string | undefined): Promise<Awaited<ReturnType<UsersOps["invite"]>>> => {
  if (!refused) return Promise.resolve({ ok: true, value: null });
  const fieldErrors: Record<string, string> = refused === "errors.email_taken" ? { email: refused } : {};
  return Promise.resolve({ ok: false, code: refused === "error.notFound" ? "NOT_FOUND" : refused === "errors.email_taken" ? "VALIDATION" : "CONFLICT", messageKey: refused, fieldErrors });
};

const ops: UsersOps = {
  invite: (email, role, name, until) => done(partnerUserDemo.invite(email, role, new Date().toISOString(), name, until)),
  access: (r, until) => done(partnerUserDemo.setAccess(r.id, until)),
  setStatus: (r, status) => done(partnerUserDemo.setStatus(r.id, status)),
  remove: (r) => done(partnerUserDemo.remove(r.id)),
  resend: (r) => done(r.status === "invited" ? undefined : "errors.invite_not_pending"),
};

/** Phase 1A demo of Users (FR-P11): contractor-a's accounts in the tab's store, the same rules as the API. */
export function UsersDemo() {
  const users = usePartnerUsersDemo();
  const [status, setStatus] = useState<PartnerStatus | null>(null);
  const rows = partnerUserRows(users, DEMO_SELF, undefined, new Date("2026-09-15T00:00:00Z")).filter((r) => !status || r.status === status);
  return <UsersView data={{ company: "contractor-a", status, rows, all: users.map((u) => ({ id: u.id, email: u.email })), today: "2026-09-15" }} ops={ops} onStatus={setStatus} />;
}
