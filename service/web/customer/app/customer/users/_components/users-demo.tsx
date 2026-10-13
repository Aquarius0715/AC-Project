"use client";

import { useState } from "react";
import { Badge, Banner, Btn, Card, DataTable, Field, Input, LinkBtn, Modal, Page, PageHead, Select, useToast } from "@ac/web/components/ui";
import { CURRENT_CLIENT, ClientUser, clientUserActions, emailError, useClientUsers } from "@ac/web/lib/clientUsers";
import { Icon } from "@ac/web/components/Icon";

/** FR-C19 / IR417 — the customer owner lists users, invites members and owners, resends invites, changes roles,
 * disables / enables and removes users (Figma Client 11a–11e); the demo applies changes at once. */
export function UsersDemo() {
  const toast = useToast();
  const users = useClientUsers();
  const [open, setOpen] = useState(false);
  const [email, setEmail] = useState("");
  const [tried, setTried] = useState(false);
  const [role, setRole] = useState<"member" | "owner">("member");
  const err = tried ? emailError(email, users) : undefined;

  if (CURRENT_CLIENT.role !== "owner") {
    return (
      <Page>
        <Card title="Page unavailable">
          <p className="text-[13px] text-muted-foreground">This page doesn’t exist or you don’t have access to it.</p>
          <div className="mt-3"><LinkBtn href="/customer" variant="primary">Go to your home (or sign in)</LinkBtn></div>
        </Card>
      </Page>
    );
  }

  const close = () => { setOpen(false); setTried(false); setEmail(""); setRole("member"); };
  const send = () => {
    setTried(true);
    const e = clientUserActions.invite(email, role, CURRENT_CLIENT.name);
    if (e) return;
    toast(`Invitation preview created for ${email.trim()} — nothing is sent in the demo`);
    close();
  };
  const done = (refused: string | undefined, ok: string) => toast(refused ?? ok, refused ? "warn" : undefined);
  const resend = (u: ClientUser) => {
    const e = clientUserActions.resend(u.id);
    toast(e ?? `Invitation preview re-created for ${u.email}`, e ? "warn" : undefined);
  };

  return (
    <Page>
      <PageHead title="Users of customer-a" sub="People who can sign in to this customer app. As an owner you invite users, change their role and access, and remove them." action={<Btn variant="primary" onClick={() => setOpen(true)}><Icon name="add" /> Invite user</Btn>} />
      <Card pad={false}>
        <DataTable rows={users} rowKey={(u) => u.id} cols={[
          { key: "u", label: "User", render: (u) => <div><b>{u.name ? `${u.name}${u.id === CURRENT_CLIENT.userId ? " (you)" : ""}` : u.email}</b><div className="text-xs text-muted-foreground">{u.name ? u.email : `invited ${u.invitedAt ?? ""} by ${u.invitedBy ?? "—"}`}</div></div> },
          { key: "r", label: "Role", render: (u) => <Badge tone={u.role === "owner" ? "primary" : "muted"}>{u.role === "owner" ? "Owner" : "Member"}</Badge> },
          { key: "s", label: "Status", render: (u) => <Badge tone={u.status === "active" ? "ok" : u.status === "invited" ? "warn" : "muted"}>{u.status === "active" ? "Active" : u.status === "invited" ? "Invite pending" : "Disabled"}</Badge> },
          { key: "l", label: "Last sign-in", render: (u) => u.lastSignIn ?? "—", hideBelow: "sm" },
          { key: "a", label: "Actions", render: (u) => (u.id === CURRENT_CLIENT.userId ? <span className="text-xs text-muted-foreground">Your account</span> : (
            <div className="flex flex-wrap gap-1">
              {u.status === "invited" && <Btn size="sm" variant="ghost" onClick={() => resend(u)}><Icon name="send" />Resend invite</Btn>}
              {u.status !== "invited" && <Btn size="sm" variant="ghost" onClick={() => done(clientUserActions.setRole(u.id, u.role === "owner" ? "member" : "owner"), `${u.name ?? u.email} is now ${u.role === "owner" ? "a member" : "an owner"}`)}>{u.role === "owner" ? "Make member" : "Make owner"}</Btn>}
              {u.status !== "invited" && <Btn size="sm" variant="ghost" onClick={() => done(clientUserActions.setStatus(u.id, u.status === "active" ? "disabled" : "active"), `${u.name ?? u.email} ${u.status === "active" ? "disabled" : "enabled again"}`)}>{u.status === "active" ? "Disable" : "Enable"}</Btn>}
              <Btn size="sm" variant="ghost" onClick={() => done(clientUserActions.remove(u.id), `${u.name ?? u.email} removed`)}><Icon name="delete" />Remove</Btn>
            </div>
          )) },
        ]} />
      </Card>
      <Banner>Your customer always keeps one active owner, and you can’t change your own account. For a password reset, contact HQ. Only owners see this page.</Banner>
      <p className="text-[11px] text-muted-foreground">Invites are previews only in the demo — no e-mail is sent (FR-C19, IR417).</p>

      <Modal open={open} onClose={close} title="Invite a user to customer-a" footer={<><Btn onClick={close}>Cancel</Btn><Btn variant="primary" onClick={send}>Send invite</Btn></>}>
        <Field label="Email" error={err}><Input type="email" value={email} onChange={(e) => setEmail(e.target.value)} placeholder="lim.ka@example.com" /></Field>
        <Field label="Role" hint="Owners manage users and everything members can do; members use the app."><Select aria-label="Role" value={role} onChange={(e) => setRole(e.target.value as "member" | "owner")}><option value="member">Member</option><option value="owner">Owner</option></Select></Field>
        <Banner>They get an invitation to sign in to the customer app and see only customer-a’s properties and units. Demo: an invitation preview is shown — nothing is sent. The invite stays “pending” until they sign in.</Banner>
      </Modal>
    </Page>
  );
}
