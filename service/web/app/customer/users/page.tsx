"use client";

import { useState } from "react";
import { Badge, Banner, Btn, Card, DataTable, Field, Input, LinkBtn, Modal, Page, PageHead, useToast } from "@/components/ui";
import { CURRENT_CLIENT, ClientUser, clientUserActions, emailError, useClientUsers } from "@/lib/clientUsers";

/** FR-C19 / IR114 — the customer owner lists users, invites members and resends invites (Figma Client 11a–11c). */
export default function CustomerUsers() {
  const toast = useToast();
  const users = useClientUsers();
  const [open, setOpen] = useState(false);
  const [email, setEmail] = useState("");
  const [tried, setTried] = useState(false);
  const err = tried ? emailError(email, users) : undefined;

  if (CURRENT_CLIENT.role !== "owner") {
    return (
      <Page>
        <Card title="Page unavailable">
          <p className="text-[13px] text-muted">This page doesn’t exist or you don’t have access to it.</p>
          <div className="mt-3"><LinkBtn href="/customer" variant="primary">Go to your home (or sign in)</LinkBtn></div>
        </Card>
      </Page>
    );
  }

  const close = () => { setOpen(false); setTried(false); setEmail(""); };
  const send = () => {
    setTried(true);
    const e = clientUserActions.invite(email, "member", CURRENT_CLIENT.name);
    if (e) return;
    toast(`Invitation preview created for ${email.trim()} — nothing is sent in the demo`);
    close();
  };
  const resend = (u: ClientUser) => {
    const e = clientUserActions.resend(u.id);
    toast(e ?? `Invitation preview re-created for ${u.email}`, e ? "warn" : undefined);
  };

  return (
    <Page>
      <PageHead title="Users of customer-a" sub="People who can sign in to this customer app. As the owner you can invite members." action={<Btn variant="primary" onClick={() => setOpen(true)}>+ Invite member</Btn>} />
      <Card pad={false}>
        <DataTable rows={users} rowKey={(u) => u.id} cols={[
          { key: "u", label: "User", render: (u) => <div><b>{u.name ? `${u.name}${u.id === CURRENT_CLIENT.userId ? " (you)" : ""}` : u.email}</b><div className="text-xs text-muted">{u.name ? u.email : `invited ${u.invitedAt ?? ""} by ${u.invitedBy ?? "—"}`}</div></div> },
          { key: "r", label: "Role", render: (u) => <Badge tone={u.role === "owner" ? "primary" : "muted"}>{u.role === "owner" ? "Owner" : "Member"}</Badge> },
          { key: "s", label: "Status", render: (u) => <Badge tone={u.status === "active" ? "ok" : u.status === "invited" ? "warn" : "muted"}>{u.status === "active" ? "Active" : u.status === "invited" ? "Invite pending" : "Disabled"}</Badge> },
          { key: "l", label: "Last sign-in", render: (u) => u.lastSignIn ?? "—", hideBelow: "sm" },
          { key: "a", label: "Actions", render: (u) => (u.status === "invited" ? <Btn size="sm" variant="ghost" onClick={() => resend(u)}>↻ Resend invite</Btn> : <span className="text-muted">—</span>) },
        ]} />
      </Card>
      <Banner>To change a role, disable sign-in, reset a password or remove someone, contact HQ — they manage it in Customers &amp; units › Users. Only owners see this page.</Banner>
      <p className="text-[11px] text-muted">Invites are previews only in the demo — no e-mail is sent (FR-C19, IR114).</p>

      <Modal open={open} onClose={close} title="Invite a member to customer-a" footer={<><Btn onClick={close}>Cancel</Btn><Btn variant="primary" onClick={send}>Send invite</Btn></>}>
        <Field label="Email" error={err}><Input type="email" value={email} onChange={(e) => setEmail(e.target.value)} placeholder="lim.ka@example.com" /></Field>
        <Field label="Role" hint="Owners can invite members only. Ask HQ to make someone an owner."><div className="rounded-control border border-line bg-surface2 px-3 py-2 text-[13px] font-semibold">🔒 Member</div></Field>
        <Banner>They get an invitation to sign in to the customer app and see only customer-a’s properties and units. Demo: an invitation preview is shown — nothing is sent. The invite stays “pending” until they sign in.</Banner>
      </Modal>
    </Page>
  );
}
