"use client";

import { useState } from "react";
import { useT } from "@ac/web/components/I18n";
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
  invite: (email, role) => done(partnerUserDemo.invite(email, role, new Date().toISOString())),
  edit: (r, email, role) => done(partnerUserDemo.editInvite(r.id, email, role)),
  setStatus: (r, status) => done(partnerUserDemo.setStatus(r.id, status)),
  remove: (r) => done(partnerUserDemo.remove(r.id)),
  resend: (r) => done(r.status === "invited" ? undefined : "errors.invite_not_pending"),
};

/** Phase 1A demo of Users (FR-P11): contractor-a's accounts in the tab's store, the same rules as the API. */
export function UsersDemo() {
  const t = useT();
  const users = usePartnerUsersDemo();
  const [status, setStatus] = useState<PartnerStatus | null>(null);
  const rows = partnerUserRows(users, DEMO_SELF, undefined).filter((r) => !status || r.status === status);
  return <UsersView data={{ company: "contractor-a", status, rows, all: users.map((u) => ({ id: u.id, email: u.email })) }} ops={ops} onStatus={setStatus} />;
}
