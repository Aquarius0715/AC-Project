"use client";

import { useState } from "react";
import { Badge, Banner, Btn, Card, DataTable, Field, Input, LinkBtn, Modal, Page, PageHead, Select, Textarea } from "@ac/web/components/ui";
import { useT } from "@ac/web/components/I18n";
import { useAction } from "@ac/web/lib/useAction";
import { inviteError, type ClientUserRow } from "@ac/web/lib/assets";
import { reasonError } from "@ac/web/lib/partnerUsers";
import { inviteUser, removeUser, resendInvite, updateUser } from "../actions";
import { Icon } from "@ac/web/components/Icon";

type Role = "owner" | "member";
type Dialog = { kind: "invite" } | { kind: "edit" | "role" | "disable" | "enable" | "remove"; row: ClientUserRow };

/** Users (FR-C19, IR417) in API mode: the owner manages the customer's users — invites members and owners, edits or
 * resends pending invites, changes roles, disables / enables and removes users, each change with a reason; the own row
 * has no actions and the last active owner cannot be demoted, disabled or removed. Texts in the display language. */
export function UsersView({ live }: { live: { customerId: string; customer: string; rows: ClientUserRow[]; all: { id: string; email: string }[] } | null }) {
  const t = useT();
  const [pending, run] = useAction();
  const [dialog, setDialog] = useState<Dialog | null>(null);
  const [email, setEmail] = useState("");
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
    setEmail("row" in d ? d.row.email : ""); setRole("row" in d ? (d.kind === "role" ? (d.row.role === "owner" ? "member" : "owner") : d.row.role) : "member");
  };
  const close = () => setDialog(null);
  const withEmail = dialog?.kind === "invite" || dialog?.kind === "edit";
  const others = live.all.filter((u) => !(dialog && "row" in dialog && u.id === dialog.row.id));
  const emailErr = withEmail ? inviteError(email, others, t) : undefined;
  const reasonErr = dialog && dialog.kind !== "invite" ? reasonError(reason, t) : undefined;
  const name = (r: ClientUserRow) => r.name ?? r.email;
  const submit = () => {
    setTried(true);
    if (!dialog || emailErr || reasonErr) return;
    if (dialog.kind === "invite") return run(() => inviteUser(live.customerId, email, role), t("Invitation e-mail queued for {email}", { email: email.trim() }), close);
    const r = dialog.row;
    if (dialog.kind === "remove") return run(() => removeUser(r.id, r.version, reason), t("{name} removed", { name: name(r) }), close);
    if (dialog.kind === "edit") return run(() => updateUser(r.id, r.version, live.customerId, email, role, null, reason), t("Invite changed for {email}", { email: email.trim() }), close);
    if (dialog.kind === "role") return run(() => updateUser(r.id, r.version, live.customerId, r.email, role, null, reason), t(role === "owner" ? "{name} is now an owner" : "{name} is now a member", { name: name(r) }), close);
    const status = dialog.kind === "disable" ? "disabled" : "active";
    run(() => updateUser(r.id, r.version, live.customerId, r.email, r.role, status, reason), t(status === "disabled" ? "{name} disabled — they can no longer sign in" : "{name} enabled again", { name: name(r) }), close);
  };
  const title = !dialog ? "" : dialog.kind === "invite" ? (live.customer ? t("Invite a user to {company}", { company: live.customer }) : t("Invite a user"))
    : t({ edit: "Edit invite", role: "Change role of {name}", disable: "Disable {name}", enable: "Enable {name}", remove: "Remove {name}" }[dialog.kind], { name: name(dialog.row) });
  const action = !dialog ? "" : t({ invite: "Send invite", edit: "Save invite", role: "Change role", disable: "Disable", enable: "Enable", remove: "Remove" }[dialog.kind]);
  const roleBadge = (u: ClientUserRow) => <Badge tone={u.role === "owner" ? "primary" : "muted"}>{t(u.role === "owner" ? "Owner" : "Member")}</Badge>;
  const statusBadge = (u: ClientUserRow) => <Badge tone={u.status === "active" ? "ok" : u.status === "invited" ? "warn" : "muted"}>{t(u.status === "active" ? "Active" : u.status === "invited" ? "Invite pending" : "Disabled")}</Badge>;
  return (
    <Page>
      <PageHead title={live.customer ? t("Users of {customer}", { customer: live.customer }) : t("Users")} sub={t("People who can sign in to this customer app. As an owner you invite users, change their role and access, and remove them.")} action={<Btn variant="primary" onClick={() => open({ kind: "invite" })}><Icon name="add" /> {t("Invite user")}</Btn>} />
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
                {invited && <Btn size="sm" variant="ghost" disabled={pending} onClick={() => run(() => resendInvite(u.id), t("Invitation e-mail queued again for {email}", { email: u.email }))}><Icon name="send" />{t("Resend invite")}</Btn>}
                {invited && <Btn size="sm" variant="ghost" onClick={() => open({ kind: "edit", row: u })}><Icon name="edit" />{t("Edit")}</Btn>}
                {!invited && !u.lastOwner && <Btn size="sm" variant="ghost" onClick={() => open({ kind: "role", row: u })}>{t(u.role === "owner" ? "Make member" : "Make owner")}</Btn>}
                {u.status === "active" && !u.lastOwner && <Btn size="sm" variant="ghost" onClick={() => open({ kind: "disable", row: u })}>{t("Disable")}</Btn>}
                {u.status === "disabled" && <Btn size="sm" variant="ghost" onClick={() => open({ kind: "enable", row: u })}>{t("Enable")}</Btn>}
                {!u.lastOwner && <Btn size="sm" variant="ghost" onClick={() => open({ kind: "remove", row: u })}><Icon name="delete" />{t("Remove")}</Btn>}
                {u.lastOwner && <span className="text-xs text-muted-foreground">{t("Last active owner")}</span>}
              </div>
            );
          } },
        ]} />
      </Card>
      <Banner>{t("Your customer always keeps one active owner, and you can’t change your own account. An accepted user keeps their e-mail. For a password reset, contact HQ. Only owners see this page.")}</Banner>
      <p className="text-[11px] text-muted-foreground">{t("Invitation e-mails are sent by the notification service (IR418); in development they arrive in the local Mailpit inbox.")}</p>
      <Modal open={!!dialog} onClose={close} title={title} footer={<><Btn onClick={close}>{t("Cancel")}</Btn><Btn variant={dialog?.kind === "remove" || dialog?.kind === "disable" ? "danger" : "primary"} disabled={pending} onClick={submit}>{action}</Btn></>}>
        {withEmail && <Field label={t("Email")} error={tried ? emailErr : undefined}><Input type="email" value={email} onChange={(e) => setEmail(e.target.value)} placeholder="lim.ka@example.com" /></Field>}
        {(withEmail || dialog?.kind === "role") && (
          <Field label={t("Role")} hint={t("Owners manage users and everything members can do; members use the app.")}>
            <Select aria-label={t("Role")} value={role} onChange={(e) => setRole(e.target.value as Role)}>
              <option value="member">{t("Member")}</option>
              <option value="owner">{t("Owner")}</option>
            </Select>
          </Field>
        )}
        {dialog?.kind === "invite" && <Banner>{t("They get an invitation to sign in to the customer app and see only your properties and units. The invite stays “pending” until their account is opened.")}</Banner>}
        {dialog?.kind === "disable" && <Banner tone="warn">{t("Their access ends now. You can enable them again later.")}</Banner>}
        {dialog?.kind === "remove" && <Banner tone="warn">{dialog.row.status === "invited" ? t("The invite is withdrawn.") : t("Their access ends now and they leave your user list.")}</Banner>}
        {dialog && dialog.kind !== "invite" && <Field label={t("Reason")} error={tried ? reasonErr : undefined}><Textarea value={reason} onChange={(e) => setReason(e.target.value)} placeholder={t("e.g. moved to another team")} /></Field>}
      </Modal>
    </Page>
  );
}
