"use client";

import { useState } from "react";
import { Badge, Banner, Btn, Card, DataTable, Field, Input, LinkBtn, Modal, Page, PageHead, Select, Textarea } from "@ac/web/components/ui";
import { useT } from "@ac/web/components/I18n";
import { useAction } from "@ac/web/lib/useAction";
import { inviteError, type ClientUserRow } from "@ac/web/lib/assets";
import { nameError, reasonError } from "@ac/web/lib/partnerUsers";
import { inviteUser, removeUser, resendInvite, updateUser } from "../actions";
import { Icon } from "@ac/web/components/Icon";

type Role = "owner" | "member";
type Dialog = { kind: "invite" } | { kind: "role" | "disable" | "enable" | "remove"; row: ClientUserRow };

/** Users (FR-C19, IR417, IR461) in API mode: the owner manages the customer's users — adds members and owners (the
 * account exists at once and the sign-in service e-mails the set-up link), resends that e-mail until they sign in,
 * changes roles, disables / enables and removes users, each change after the adding with a reason; the own row has no
 * actions and the last active owner cannot be demoted, disabled or removed. Texts in the display language. */
export function UsersView({ live }: { live: { customerId: string; customer: string; rows: ClientUserRow[]; all: { id: string; email: string }[] } | null }) {
  const t = useT();
  const [pending, run] = useAction();
  const [dialog, setDialog] = useState<Dialog | null>(null);
  const [email, setEmail] = useState("");
  const [userName, setUserName] = useState("");
  const [role, setRole] = useState<Role>("member");
  const [reason, setReason] = useState("");
  const [tried, setTried] = useState(false);
  if (!live) {
    return (
      <Page>
        <Card title={t("Page unavailable")}>
          <p className="text-[13px] text-muted-foreground">{t("This page doesn’t exist or you don’t have access to it.")}</p>
          <div className="mt-3"><LinkBtn href="/customer" variant="primary">{t("Go to your home (or sign in)")}</LinkBtn></div>
        </Card>
      </Page>
    );
  }
  const open = (d: Dialog) => {
    setDialog(d); setTried(false); setReason("");
    setEmail(""); setUserName(""); setRole("row" in d ? (d.kind === "role" ? (d.row.role === "owner" ? "member" : "owner") : d.row.role) : "member");
  };
  const close = () => setDialog(null);
  const adding = dialog?.kind === "invite";
  const emailErr = adding ? inviteError(email, live.all, t) : undefined;
  const nameErr = adding ? nameError(userName, t) : undefined;
  const reasonErr = dialog && !adding ? reasonError(reason, t) : undefined;
  const name = (r: ClientUserRow) => r.name ?? r.email;
  const submit = () => {
    setTried(true);
    if (!dialog || emailErr || nameErr || reasonErr) return;
    if (dialog.kind === "invite") return run(() => inviteUser(live.customerId, email, role, userName), t("{name} added — a set-up e-mail is on its way to {email}", { name: userName.trim(), email: email.trim() }), close);
    const r = dialog.row;
    if (dialog.kind === "remove") return run(() => removeUser(r.id, r.version, reason), t("{name} removed", { name: name(r) }), close);
    if (dialog.kind === "role") return run(() => updateUser(r.id, r.version, live.customerId, r.email, role, null, reason), t(role === "owner" ? "{name} is now an owner" : "{name} is now a member", { name: name(r) }), close);
    const status = dialog.kind === "disable" ? "disabled" : "active";
    run(() => updateUser(r.id, r.version, live.customerId, r.email, r.role, status, reason), t(status === "disabled" ? "{name} disabled — they can no longer sign in" : "{name} enabled again", { name: name(r) }), close);
  };
  const title = !dialog ? "" : dialog.kind === "invite" ? (live.customer ? t("Add a user to {company}", { company: live.customer }) : t("Add a user"))
    : t({ role: "Change role of {name}", disable: "Disable {name}", enable: "Enable {name}", remove: "Remove {name}" }[dialog.kind], { name: name(dialog.row) });
  const action = !dialog ? "" : t({ invite: "Add user", role: "Change role", disable: "Disable", enable: "Enable", remove: "Remove" }[dialog.kind]);
  const roleBadge = (u: ClientUserRow) => <Badge tone={u.role === "owner" ? "primary" : "muted"}>{t(u.role === "owner" ? "Owner" : "Member")}</Badge>;
  const statusBadge = (u: ClientUserRow) => <Badge tone={u.status === "active" ? "ok" : u.status === "invited" ? "warn" : "muted"}>{t(u.status === "active" ? "Active" : u.status === "invited" ? "Not signed in yet" : "Disabled")}</Badge>;
  return (
    <Page>
      <PageHead title={live.customer ? t("Users of {customer}", { customer: live.customer }) : t("Users")} sub={t("People who can sign in to this customer app. As an owner you add users, change their role and access, and remove them.")} action={<Btn variant="primary" onClick={() => open({ kind: "invite" })}><Icon name="add" /> {t("Add user")}</Btn>} />
      <Card pad={false}>
        <DataTable rows={live.rows} rowKey={(u) => u.id} cols={[
          // on a phone the role and status sit under the name, so the actions keep their room
          { key: "u", label: t("User"), render: (u) => <div><b>{u.name ?? u.email}{u.you ? ` ${t("(you)")}` : ""}</b><div className="text-xs text-muted-foreground">{u.sub}</div><div className="mt-1 flex flex-wrap gap-1 sm:hidden">{roleBadge(u)}{statusBadge(u)}</div></div> },
          { key: "r", label: t("Role"), render: roleBadge, hideBelow: "sm" },
          { key: "s", label: t("Status"), render: statusBadge, hideBelow: "sm" },
          { key: "l", label: t("Last sign-in"), render: (u) => u.lastSignIn, hideBelow: "sm" },
          { key: "a", label: t("Actions"), render: (u) => {
            if (u.you) return <span className="text-xs text-muted-foreground">{t("Your account")}</span>;
            const invited = u.status === "invited";
            return (
              <div className="flex flex-wrap gap-1">
                {invited && <Btn size="sm" variant="ghost" disabled={pending} onClick={() => run(() => resendInvite(u.id), t("Set-up e-mail sent again to {email}", { email: u.email }))}><Icon name="send" />{t("Resend set-up e-mail")}</Btn>}
                {!u.lastOwner && <Btn size="sm" variant="ghost" onClick={() => open({ kind: "role", row: u })}>{t(u.role === "owner" ? "Make member" : "Make owner")}</Btn>}
                {u.status === "active" && !u.lastOwner && <Btn size="sm" variant="ghost" onClick={() => open({ kind: "disable", row: u })}>{t("Disable")}</Btn>}
                {u.status === "disabled" && <Btn size="sm" variant="ghost" onClick={() => open({ kind: "enable", row: u })}>{t("Enable")}</Btn>}
                {!u.lastOwner && <Btn size="sm" variant="ghost" onClick={() => open({ kind: "remove", row: u })}><Icon name="delete" />{t("Remove")}</Btn>}
                {u.lastOwner && <span className="text-xs text-muted-foreground">{t("Last active owner")}</span>}
              </div>
            );
          } },
        ]} />
      </Card>
      <Banner>{t("Your customer always keeps one active owner, and you can’t change your own account. A user keeps their e-mail — to change it, remove them and add them again. For a password reset, contact HQ. Only owners see this page.")}</Banner>
      <p className="text-[11px] text-muted-foreground">{t("The set-up e-mail comes from the sign-in service; in development it arrives in the local Mailpit inbox.")}</p>
      <Modal open={!!dialog} onClose={close} title={title} footer={<><Btn onClick={close}>{t("Cancel")}</Btn><Btn variant={dialog?.kind === "remove" || dialog?.kind === "disable" ? "danger" : "primary"} disabled={pending} onClick={submit}>{action}</Btn></>}>
        {adding && <Field label={t("Name")} error={tried ? nameErr : undefined}><Input value={userName} maxLength={120} onChange={(e) => setUserName(e.target.value)} placeholder="Lim Ka Wai" /></Field>}
        {adding && <Field label={t("Email")} error={tried ? emailErr : undefined}><Input type="email" value={email} onChange={(e) => setEmail(e.target.value)} placeholder="lim.ka@example.com" /></Field>}
        {(adding || dialog?.kind === "role") && (
          <Field label={t("Role")} hint={t("Owners manage users and everything members can do; members use the app.")}>
            <Select aria-label={t("Role")} value={role} onChange={(e) => setRole(e.target.value as Role)}>
              <option value="member">{t("Member")}</option>
              <option value="owner">{t("Owner")}</option>
            </Select>
          </Field>
        )}
        {adding && <Banner>{t("Their account is created now and they get an e-mail to set their password. They see only your properties and units; until they sign in they are listed as “Not signed in yet”.")}</Banner>}
        {dialog?.kind === "disable" && <Banner tone="warn">{t("Their access ends now. You can enable them again later.")}</Banner>}
        {dialog?.kind === "remove" && <Banner tone="warn">{dialog.row.status === "invited" ? t("Their account is removed before they signed in.") : t("Their access ends now and they leave your user list.")}</Banner>}
        {dialog && dialog.kind !== "invite" && <Field label={t("Reason")} error={tried ? reasonErr : undefined}><Textarea value={reason} onChange={(e) => setReason(e.target.value)} placeholder={t("e.g. moved to another team")} /></Field>}
      </Modal>
    </Page>
  );
}
