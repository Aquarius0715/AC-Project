"use client";

import { useId, useRef, useState } from "react";
import { Badge, Banner, Btn, Card, DataTable, Field, Input, LinkButton, Modal, Page, PageHead, Select, Textarea } from "@ac/web/components/ui";
import { useT } from "@ac/web/components/I18n";
import { useAction } from "@ac/web/lib/useAction";
import { fieldErrorTexts, type ActionFailure } from "@ac/web/lib/actionMessage";
import { focusField } from "@ac/web/lib/focusField";
import { inviteError, type ClientUserRow } from "@ac/web/lib/assets";
import { nameError, reasonError } from "@ac/web/lib/partnerUsers";
import { inviteUser, removeUser, resendInvite, updateUser } from "../actions";
import { Icon } from "@ac/web/components/Icon";

type Dialog = { kind: "invite" } | { kind: "role" | "disable" | "enable" | "remove"; row: ClientUserRow };
/** The dialog's fields, top to bottom, and the Core API fields that land on them. */
type FieldKey = "name" | "email" | "reason";
const FIELDS: FieldKey[] = ["name", "email", "reason"];
const API_FIELDS: Record<string, FieldKey> = { displayName: "name", email: "email", reason: "reason" };

/** Users (FR-C19, IR417, IR461) in API mode: the owner manages the customer's users — adds members and owners (the
account exists at once and the sign-in service e-mails the set-up link), resends that e-mail until they sign in,
changes roles, disables / enables and removes users, each change after the adding with a reason; the own row has no
actions and the last active owner cannot be demoted, disabled or removed. Texts in the display language. */
export function UsersView({ live }: Readonly<{ live: { customerId: string; customer: string; rows: ClientUserRow[]; all: { id: string; email: string }[] } | null }>) {
  type Role = "owner" | "member";
  const t = useT();
  const [pending, run] = useAction();
  const [dialog, setDialog] = useState<Dialog | null>(null);
  const [email, setEmail] = useState("");
  const [userName, setUserName] = useState("");
  const [role, setRole] = useState<Role>("member");
  const [reason, setReason] = useState("");
  const [tried, setTried] = useState(false);
  const [server, setServer] = useState<Partial<Record<FieldKey, string>>>({});
  const ids: Record<FieldKey, string> = { name: useId(), email: useId(), reason: useId() };
  const gone = useRef(false); // the removed row took the dialog's trigger with it: focus goes to the list
  if (!live) {
    return (
      <Page>
        <Card title={t("Page unavailable")}>
          <p className="text-[13px] text-muted-foreground">{t("This page doesn’t exist or you don’t have access to it.")}</p>
          <div className="mt-3"><LinkButton href="/customer" variant="primary">{t("Go to your home (or sign in)")}</LinkButton></div>
        </Card>
      </Page>
    );
  }
  const open = (d: Dialog) => {
    setDialog(d); setTried(false); setReason(""); setServer({}); gone.current = false;
    setEmail(""); setUserName(""); setRole("row" in d ? (d.kind === "role" ? (d.row.role === "owner" ? "member" : "owner") : d.row.role) : "member");
  };
  const close = () => { setDialog(null); };
  const isAdding = dialog?.kind === "invite";
  const emailError = isAdding ? inviteError(email, live.all, t) : undefined;
  const nameProblem = isAdding ? nameError(userName, t) : undefined;
  const reasonProblem = dialog && !isAdding ? reasonError(reason, t) : undefined;
  const name = (r: ClientUserRow) => r.name ?? r.email;
  const edit = (k: FieldKey) => { setServer((s) => (s[k] ? { ...s, [k]: undefined } : s)); };
  const submit = () => {
    setTried(true);
    if (!dialog) return;
    // a refused submit takes focus to the first field to fix, top to bottom (IR586, IR593)
    const local: Partial<Record<FieldKey, string>> = { name: nameProblem, email: emailError, reason: reasonProblem };
    const bad = FIELDS.find((k) => local[k]);
    if (bad) { focusField(ids[bad]); return; }
    // what the Core API refuses on a field (an address someone else still holds, IR462) shows under it as well
    const refused = (f: ActionFailure) => {
      const fields = fieldErrorTexts(f, API_FIELDS, t);
      setServer(fields);
      const first = FIELDS.find((k) => fields[k]);
      if (first) focusField(ids[first]);
    };
    if (dialog.kind === "invite") { run(() => inviteUser(live.customerId, email, role, userName), t("{name} added — a set-up e-mail is on its way to {email}", { name: userName.trim(), email: email.trim() }), close, refused); return; }
    const r = dialog.row;
    if (dialog.kind === "remove") { run(() => removeUser(r.id, r.version, reason), t("{name} removed", { name: name(r) }), () => { gone.current = true; close(); }, refused); return; }
    if (dialog.kind === "role") { run(() => updateUser(r.id, r.version, live.customerId, r.email, role, null, reason), t(role === "owner" ? "{name} is now an owner" : "{name} is now a member", { name: name(r) }), close, refused); return; }
    const status = dialog.kind === "disable" ? "disabled" : "active";
    run(() => updateUser(r.id, r.version, live.customerId, r.email, r.role, status, reason), t(status === "disabled" ? "{name} disabled — they can no longer sign in" : "{name} enabled again", { name: name(r) }), close, refused);
  };
  const title = dialog ? dialog.kind === "invite" ? (live.customer ? t("Add a user to {company}", { company: live.customer }) : t("Add a user"))
    : t({ role: "Change role of {name}", disable: "Disable {name}", enable: "Enable {name}", remove: "Remove {name}" }[dialog.kind], { name: name(dialog.row) }) : "";
  const action = dialog ? t({ invite: "Add user", role: "Change role", disable: "Disable", enable: "Enable", remove: "Remove" }[dialog.kind]) : "";
  const roleBadge = (u: ClientUserRow) => <Badge tone={u.role === "owner" ? "primary" : "muted"}>{t(u.role === "owner" ? "Owner" : "Member")}</Badge>;
  const statusBadge = (u: ClientUserRow) => u.revoked ? <Badge tone="crit">{t("Access ended by HQ")}</Badge> : <Badge tone={u.status === "active" ? "ok" : (u.status === "invited" ? "warn" : "muted")}>{t(u.status === "active" ? "Active" : (u.status === "invited" ? "Not signed in yet" : "Disabled"))}</Badge>;
  return (
    <Page>
      <PageHead title={live.customer ? t("Users of {customer}", { customer: live.customer }) : t("Users")} sub={t("People who can sign in to this customer app. As an owner you add users, change their role and access, and remove them.")} action={<Btn variant="primary" onClick={() => { open({ kind: "invite" }); }}><Icon name="add" /> {t("Add user")}</Btn>} />
      <Card pad={false}>
        <div id="users-list" tabIndex={-1} role="region" aria-label={t("Users")} className="rounded-xl outline-none focus-visible:ring-2 focus-visible:ring-ring/50">
        <DataTable rows={live.rows} rowKey={(u) => u.id} cols={[
          // on a phone the role and status sit under the name, so the actions keep their room
          { key: "u", label: t("User"), render: (u) => <div><b>{u.name ?? u.email}{u.you ? ` ${t("(you)")}` : ""}</b><div className="text-xs text-muted-foreground">{u.sub}</div><div className="mt-1 flex flex-wrap gap-1 sm:hidden">{roleBadge(u)}{statusBadge(u)}</div></div> },
          { key: "r", label: t("Role"), render: roleBadge, hideBelow: "sm" },
          { key: "s", label: t("Status"), render: statusBadge, hideBelow: "sm" },
          { key: "l", label: t("Last sign-in"), render: (u) => u.lastSignIn, hideBelow: "sm" },
          { key: "a", label: t("Actions"), render: (u) => {
            if (u.you) return <span className="text-xs text-muted-foreground">{t("Your account")}</span>;
            if (u.revoked) return ( // HQ ended the access (IR508): only removing the user is left here
              <div className="flex flex-wrap items-center gap-1">
                {!u.lastOwner && <Btn size="sm" variant="ghost" onClick={() => { open({ kind: "remove", row: u }); }}><Icon name="delete" />{t("Remove")}</Btn>}
                <span className="text-xs text-muted-foreground">{t("Only HQ can give the access back")}</span>
              </div>
            );
            const isInvited = u.status === "invited";
            return (
              <div className="flex flex-wrap gap-1">
                {isInvited && <Btn size="sm" variant="ghost" disabled={pending} onClick={() => { run(() => resendInvite(u.id), t("Set-up e-mail sent again to {email}", { email: u.email })); }}><Icon name="send" />{t("Resend set-up e-mail")}</Btn>}
                {!u.lastOwner && <Btn size="sm" variant="ghost" onClick={() => { open({ kind: "role", row: u }); }}>{t(u.role === "owner" ? "Make member" : "Make owner")}</Btn>}
                {/* Disable and Enable share one place: the button that opened the dialog stays, with the focus, once the user has changed */}
                {((u.status === "active" && !u.lastOwner) || u.status === "disabled") && <Btn size="sm" variant="ghost" onClick={() => { open({ kind: u.status === "active" ? "disable" : "enable", row: u }); }}>{t(u.status === "active" ? "Disable" : "Enable")}</Btn>}
                {!u.lastOwner && <Btn size="sm" variant="ghost" onClick={() => { open({ kind: "remove", row: u }); }}><Icon name="delete" />{t("Remove")}</Btn>}
                {u.lastOwner && <span className="text-xs text-muted-foreground">{t("Last active owner")}</span>}
              </div>
            );
          } },
        ]} />
        </div>
      </Card>
      <Banner>{t("Your customer always keeps one active owner, and you can’t change your own account. A user keeps their e-mail — to change it, remove them and add them again. For a password reset, contact HQ. Only owners see this page.")}</Banner>
      <p className="text-[11px] text-muted-foreground">{t("The set-up e-mail comes from the sign-in service; in development it arrives in the local Mailpit inbox.")}</p>
      <Modal open={!!dialog} onClose={close} title={title} finalFocus={() => (gone.current ? document.querySelector("#users-list") : true)} footer={<><Btn onClick={close}>{t("Cancel")}</Btn><Btn variant={dialog?.kind === "remove" || dialog?.kind === "disable" ? "danger" : "primary"} disabled={pending} onClick={submit}>{action}</Btn></>}>
        {isAdding && <Field label={t("Name")} error={(tried ? nameProblem : undefined) ?? server.name}><Input id={ids.name} value={userName} maxLength={120} onChange={(e) => { setUserName(e.target.value); edit("name"); }} placeholder="Lim Ka Wai" /></Field>}
        {isAdding && <Field label={t("Email")} error={(tried ? emailError : undefined) ?? server.email}><Input id={ids.email} type="email" value={email} onChange={(e) => { setEmail(e.target.value); edit("email"); }} placeholder="lim.ka@example.com" /></Field>}
        {(isAdding || dialog?.kind === "role") && (
          <Field label={t("Role")} hint={t("Owners manage users and everything members can do; members use the app.")}>
            <Select aria-label={t("Role")} value={role} onChange={(e) => { setRole(e.target.value as Role); }}>
              <option value="member">{t("Member")}</option>
              <option value="owner">{t("Owner")}</option>
            </Select>
          </Field>
        )}
        {isAdding && <Banner>{t("Their account is created now and they get an e-mail to set their password. They see only your properties and units; until they sign in they are listed as “Not signed in yet”.")}</Banner>}
        {dialog?.kind === "disable" && <Banner tone="warn">{t("Their access ends now. You can enable them again later.")}</Banner>}
        {dialog?.kind === "remove" && <Banner tone="warn">{t(dialog.row.status === "invited" ? "Their account is removed before they signed in." : "Their access ends now and they leave your user list.")}</Banner>}
        {dialog && dialog.kind !== "invite" && <Field label={t("Reason")} error={(tried ? reasonProblem : undefined) ?? server.reason}><Textarea id={ids.reason} value={reason} onChange={(e) => { setReason(e.target.value); edit("reason"); }} placeholder={t("e.g. moved to another team")} /></Field>}
      </Modal>
    </Page>
  );
}
