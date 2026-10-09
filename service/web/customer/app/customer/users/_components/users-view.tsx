"use client";

import { useState } from "react";
import { Badge, Banner, Btn, Card, DataTable, Field, Input, LinkBtn, Modal, Page, PageHead } from "@ac/web/components/ui";
import { useAction } from "@ac/web/lib/useAction";
import { inviteError, type ClientUserRow } from "@ac/web/lib/assets";
import { inviteMember, resendInvite } from "../actions";

/** Users (FR-C19) in API mode: the owner lists the customer's users, invites members and resends pending invites. */
export function UsersView({ live }: { live: { customerId: string; rows: ClientUserRow[]; emails: string[] } | null }) {
  const [pending, run] = useAction();
  const [open, setOpen] = useState(false);
  const [email, setEmail] = useState("");
  const [tried, setTried] = useState(false);
  if (!live) {
    return (
      <Page>
        <Card title="Page unavailable">
          <p className="text-[13px] text-muted">This page doesn’t exist or you don’t have access to it.</p>
          <div className="mt-3"><LinkBtn href="/customer" variant="primary">Go to your home (or sign in)</LinkBtn></div>
        </Card>
      </Page>
    );
  }
  const err = inviteError(email, live.emails.map((e) => ({ email: e })));
  const close = () => { setOpen(false); setTried(false); setEmail(""); };
  return (
    <Page>
      <PageHead title="Users" sub="People who can sign in to this customer app. As the owner you can invite members." action={<Btn variant="primary" onClick={() => setOpen(true)}>+ Invite member</Btn>} />
      <Card pad={false}>
        <DataTable rows={live.rows} rowKey={(u) => u.id} cols={[
          { key: "u", label: "User", render: (u) => <div><b>{u.name ?? u.email}{u.you ? " (you)" : ""}</b><div className="text-xs text-muted">{u.sub}</div></div> },
          { key: "r", label: "Role", render: (u) => <Badge tone={u.role === "owner" ? "primary" : "muted"}>{u.role === "owner" ? "Owner" : "Member"}</Badge> },
          { key: "s", label: "Status", render: (u) => <Badge tone={u.status === "active" ? "ok" : u.status === "invited" ? "warn" : "muted"}>{u.status === "active" ? "Active" : u.status === "invited" ? "Invite pending" : "Disabled"}</Badge> },
          { key: "l", label: "Last sign-in", render: (u) => u.lastSignIn, hideBelow: "sm" },
          { key: "a", label: "Actions", render: (u) => (u.status === "invited"
            ? <Btn size="sm" variant="ghost" disabled={pending} onClick={() => run(() => resendInvite(u.id), `Invitation preview re-created for ${u.email} — nothing is sent in the demo`)}>↻ Resend invite</Btn>
            : <span className="text-muted">—</span>) },
        ]} />
      </Card>
      <Banner>To change a role, disable sign-in, reset a password or remove someone, contact HQ — they manage it in Customers &amp; units › Users. Only owners see this page.</Banner>
      <p className="text-[11px] text-muted">Invites are previews only in the demo — no e-mail is sent (FR-C19, IR114).</p>
      <Modal open={open} onClose={close} title="Invite a member" footer={<><Btn onClick={close}>Cancel</Btn><Btn variant="primary" disabled={pending} onClick={() => { setTried(true); if (!err) run(() => inviteMember(live.customerId, email), `Invitation preview created for ${email.trim()} — nothing is sent in the demo`, close); }}>Send invite</Btn></>}>
        <Field label="Email" error={tried ? err : undefined}><Input type="email" value={email} onChange={(e) => setEmail(e.target.value)} placeholder="lim.ka@example.com" /></Field>
        <Field label="Role" hint="Owners can invite members only. Ask HQ to make someone an owner."><div className="rounded-control border border-line bg-surface2 px-3 py-2 text-[13px] font-semibold">🔒 Member</div></Field>
        <Banner>They get an invitation to sign in to the customer app and see only your properties and units. Demo: an invitation preview is shown — nothing is sent. The invite stays “pending” until they sign in.</Banner>
      </Modal>
    </Page>
  );
}
