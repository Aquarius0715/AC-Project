"use client";

import { useUrlPatch } from "@ac/web/lib/useUrlPatch";
import { editPartnerInvite, invitePartnerUser, removePartnerUser, resendPartnerInvite, setPartnerUserStatus } from "../actions";
import { UsersView, type UsersData, type UsersOps } from "./users-view";

const ops: UsersOps = {
  invite: invitePartnerUser,
  edit: (r, email, role, reason) => editPartnerInvite(r.id, r.version, email, role, reason),
  setStatus: (r, status, reason) => setPartnerUserStatus(r.id, r.version, r.email, r.role, status, reason),
  remove: (r, reason) => removePartnerUser(r.id, r.version, reason),
  resend: (r) => resendPartnerInvite(r.id),
};

/** Users (FR-P11) in API mode: the Server Actions, and the status filter in the URL (`?status=`). */
export function UsersLive({ live }: { live: UsersData | null }) {
  const patch = useUrlPatch();
  return <UsersView data={live} ops={ops} onStatus={(s) => patch({ status: s })} />;
}
