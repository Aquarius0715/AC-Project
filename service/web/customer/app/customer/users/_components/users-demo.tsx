"use client";

import { useState } from "react";
import { Badge, Banner, Btn, Card, DataTable, Field, Input, LinkBtn, Modal, Page, PageHead, Select, useToast } from "@ac/web/components/ui";
import { CURRENT_CLIENT, ClientUser, clientUserActions, emailError, useClientUsers } from "@ac/web/lib/clientUsers";
import { Icon } from "@ac/web/components/Icon";

/** FR-C19 / IR417 / IR461 — the customer owner lists users, adds members and owners (the account exists at once), resends
 * the set-up e-mail, changes roles, disables / enables and removes users (Figma Client 11a–11e); the demo applies
 * changes at once. */
export function UsersDemo() {
  const toast = useToast();
  const users = useClientUsers();
  const [open, setOpen] = useState(false);
  const [email, setEmail] = useState("");
  const [name, setName] = useState("");
  const [tried, setTried] = useState(false);
  const [role, setRole] = useState<"member" | "owner">("member");
  const err = tried ? emailError(email, users) : undefined;
  const nameErr = tried && ([...name.trim()].length < 1 || [...name.trim()].length > 120) ? "A name of 1–120 characters" : undefined;

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

  const close = () => { setOpen(false); setTried(false); setEmail(""); setName(""); setRole("member"); };
  const send = () => {
    setTried(true);
    const n = [...name.trim()].length;
    if (n < 1 || n > 120) return;
    const e = clientUserActions.invite(email, role, CURRENT_CLIENT.name, name);
    if (e) return;
    toast(`${name.trim()} added — nothing is sent in the demo`);
    close();
  };
  const done = (refused: string | undefined, ok: string) => toast(refused ?? ok, refused ? "warn" : undefined);
  const resend = (u: ClientUser) => {
    const e = clientUserActions.resend(u.id);
    toast(e ?? `Set-up e-mail re-sent to ${u.email} — nothing is sent in the demo`, e ? "warn" : undefined);
  };

  return (
    <Page>
      <PageHead title="Users of customer-a" sub="People who can sign in to this customer app. As an owner you add users, change their role and access, and remove them." action={<Btn variant="primary" onClick={() => setOpen(true)}><Icon name="add" /> Add user</Btn>} />
      <Card pad={false}>
        <DataTable rows={users} rowKey={(u) => u.id} cols={[
          { key: "u", label: "User", render: (u) => <div><b>{u.name ? `${u.name}${u.id === CURRENT_CLIENT.userId ? " (you)" : ""}` : u.email}</b><div className="text-xs text-muted-foreground">{u.name ? u.email : `invited ${u.invitedAt ?? ""} by ${u.invitedBy ?? "—"}`}</div></div> },
          { key: "r", label: "Role", render: (u) => <Badge tone={u.role === "owner" ? "primary" : "muted"}>{u.role === "owner" ? "Owner" : "Member"}</Badge> },
          { key: "s", label: "Status", render: (u) => <Badge tone={u.status === "active" ? "ok" : u.status === "invited" ? "warn" : "muted"}>{u.status === "active" ? "Active" : u.status === "invited" ? "Not signed in yet" : "Disabled"}</Badge> },
          { key: "l", label: "Last sign-in", render: (u) => u.lastSignIn ?? "—", hideBelow: "sm" },
          { key: "a", label: "Actions", render: (u) => (u.id === CURRENT_CLIENT.userId ? <span className="text-xs text-muted-foreground">Your account</span> : (
            <div className="flex flex-wrap gap-1">
              {u.status === "invited" && <Btn size="sm" variant="ghost" onClick={() => resend(u)}><Icon name="send" />Resend set-up e-mail</Btn>}
              {<Btn size="sm" variant="ghost" onClick={() => done(clientUserActions.setRole(u.id, u.role === "owner" ? "member" : "owner"), `${u.name ?? u.email} is now ${u.role === "owner" ? "a member" : "an owner"}`)}>{u.role === "owner" ? "Make member" : "Make owner"}</Btn>}
              {u.status !== "invited" && <Btn size="sm" variant="ghost" onClick={() => done(clientUserActions.setStatus(u.id, u.status === "active" ? "disabled" : "active"), `${u.name ?? u.email} ${u.status === "active" ? "disabled" : "enabled again"}`)}>{u.status === "active" ? "Disable" : "Enable"}</Btn>}
              <Btn size="sm" variant="ghost" onClick={() => done(clientUserActions.remove(u.id), `${u.name ?? u.email} removed`)}><Icon name="delete" />Remove</Btn>
            </div>
          )) },
        ]} />
      </Card>
      <Banner>Your customer always keeps one active owner, and you can’t change your own account. For a password reset, contact HQ. Only owners see this page.</Banner>
      <p className="text-[11px] text-muted-foreground">Nothing is sent in the demo (FR-C19, IR461).</p>

      <Modal open={open} onClose={close} title="Add a user to customer-a" footer={<><Btn onClick={close}>Cancel</Btn><Btn variant="primary" onClick={send}>Add user</Btn></>}>
        <Field label="Name" error={nameErr}><Input value={name} maxLength={120} onChange={(e) => setName(e.target.value)} placeholder="Lim Ka Wai" /></Field>
        <Field label="Email" error={err}><Input type="email" value={email} onChange={(e) => setEmail(e.target.value)} placeholder="lim.ka@example.com" /></Field>
        <Field label="Role" hint="Owners manage users and everything members can do; members use the app."><Select aria-label="Role" value={role} onChange={(e) => setRole(e.target.value as "member" | "owner")}><option value="member">Member</option><option value="owner">Owner</option></Select></Field>
        <Banner>Their account is created now and they get an e-mail to set their password. They see only customer-a’s properties and units; until they sign in they are listed as “Not signed in yet”. Demo: nothing is sent.</Banner>
      </Modal>
    </Page>
  );
}
