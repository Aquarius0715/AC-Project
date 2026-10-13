"use client";

import { useState } from "react";
import { Badge, Banner, Btn, Card, DataTable, EmptyState, Field, Input, LinkBtn, Modal, Page, PageHead, Select, Textarea } from "@ac/web/components/ui";
import { useT } from "@ac/web/components/I18n";
import { Icon } from "@ac/web/components/Icon";
import { useAction } from "@ac/web/lib/useAction";
import type { ActionFailure } from "@ac/web/lib/actionMessage";
import { PARTNER_ROLES, PARTNER_STATUSES, partnerInviteError, reasonError, roleLabel, statusLabel, type PartnerRole, type PartnerStatus, type PartnerUserRow } from "@ac/web/lib/partnerUsers";

type Result = { ok: true; value: null } | ({ ok: false } & ActionFailure);
export type UsersOps = {
  invite: (email: string, role: PartnerRole) => Promise<Result>;
  edit: (r: PartnerUserRow, email: string, role: PartnerRole, reason: string) => Promise<Result>;
  setStatus: (r: PartnerUserRow, status: "active" | "disabled", reason: string) => Promise<Result>;
  remove: (r: PartnerUserRow, reason: string) => Promise<Result>;
  resend: (r: PartnerUserRow) => Promise<Result>;
};
export type UsersData = { company: string; status: PartnerStatus | null; rows: PartnerUserRow[]; all: { id: string; email: string }[] };

type Dialog =
  | { kind: "invite" }
  | { kind: "edit"; row: PartnerUserRow }
  | { kind: "disable" | "enable" | "remove"; row: PartnerUserRow };

/** Users (FR-P11): the contractor's company accounts and invites, with the actions each row allows; one dialog at a
 * time (invite, edit invite, disable, enable, remove), each change after the invite with a reason. */
export function UsersView({ data, ops, onStatus, sends = false }: { data: UsersData | null; ops: UsersOps; onStatus: (s: PartnerStatus | null) => void; sends?: boolean }) {
  const t = useT();
  const [pending, run] = useAction();
  const [dialog, setDialog] = useState<Dialog | null>(null);
  const [email, setEmail] = useState("");
  const [role, setRole] = useState<PartnerRole>("technician");
  const [reason, setReason] = useState("");
  const [tried, setTried] = useState(false);
  if (!data) {
    return (
      <Page>
        <Card title={t("Page unavailable")}>
          <p className="text-[13px] text-muted-foreground">{t("This page doesn’t exist or you don’t have access to it.")}</p>
          <div className="mt-3"><LinkBtn href="/partner" variant="primary">{t("Go to your home (or sign in)")}</LinkBtn></div>
        </Card>
      </Page>
    );
  }
  const open = (d: Dialog) => {
    setDialog(d); setTried(false); setReason("");
    setEmail(d.kind === "edit" ? d.row.email : ""); setRole(d.kind === "edit" ? d.row.role : "technician");
  };
  const close = () => setDialog(null);
  const self = dialog?.kind === "edit" ? dialog.row.id : undefined;
  const emailErr = dialog?.kind === "invite" || dialog?.kind === "edit" ? partnerInviteError(email, data.all, t, self) : undefined;
  const reasonErr = dialog && dialog.kind !== "invite" ? reasonError(reason, t) : undefined;
  const submit = () => {
    setTried(true);
    if (!dialog || emailErr || reasonErr) return;
    const who = dialog.kind === "invite" ? email.trim() : dialog.row.name ?? dialog.row.email;
    if (dialog.kind === "invite") run(() => ops.invite(email, role), sends ? t("Invitation e-mail queued for {email}", { email: who }) : t("Invitation preview created for {email} — nothing is sent in the demo", { email: who }), close);
    // a corrected address gets the invitation (IR426)
    else if (dialog.kind === "edit") run(() => ops.edit(dialog.row, email, role, reason), t(sends && email.trim().toLowerCase() !== dialog.row.email.toLowerCase() ? "Invite changed — invitation e-mail queued for {email}" : "Invite changed for {email}", { email: email.trim() }), close);
    else if (dialog.kind === "remove") run(() => ops.remove(dialog.row, reason), t("{name} removed", { name: who }), close);
    else run(() => ops.setStatus(dialog.row, dialog.kind === "disable" ? "disabled" : "active", reason), t(dialog.kind === "disable" ? "{name} disabled — they can no longer sign in" : "{name} enabled again", { name: who }), close);
  };
  const title = !dialog ? "" : {
    invite: t("Invite a user to {company}", { company: data.company || t("your company") }),
    edit: t("Edit invite"),
    disable: t("Disable {name}", { name: "row" in dialog ? dialog.row.name ?? dialog.row.email : "" }),
    enable: t("Enable {name}", { name: "row" in dialog ? dialog.row.name ?? dialog.row.email : "" }),
    remove: t("Remove {name}", { name: "row" in dialog ? dialog.row.name ?? dialog.row.email : "" }),
  }[dialog.kind];
  const action = !dialog ? "" : { invite: t("Send invite"), edit: t("Save invite"), disable: t("Disable"), enable: t("Enable"), remove: t("Remove") }[dialog.kind];
  const roleBadge = (u: PartnerUserRow) => <Badge tone={u.role === "contractor" ? "primary" : "muted"}>{t(roleLabel[u.role])}</Badge>;
  const statusBadge = (u: PartnerUserRow) => <Badge tone={u.ended ? "muted" : u.status === "active" ? "ok" : u.status === "invited" ? "warn" : "muted"}>{u.ended ? t("Access ended") : t(statusLabel[u.status])}</Badge>;
  return (
    <Page>
      <PageHead title={data.company ? t("Users of {company}", { company: data.company }) : t("Users")} sub={t("Contractors and technicians of your company who can sign in. You can invite, disable and remove them.")}
        action={<Btn variant="primary" onClick={() => open({ kind: "invite" })}><Icon name="add" /> {t("Invite user")}</Btn>} />
      <div className="flex flex-wrap items-center gap-2">
        <span className="text-[13px] font-semibold">{t("Status")}</span>
        <Select aria-label={t("Status")} className="w-48" value={data.status ?? ""} onChange={(e) => onStatus((e.target.value || null) as PartnerStatus | null)}>
          <option value="">{t("All")}</option>
          {PARTNER_STATUSES.map((s) => <option key={s} value={s}>{t(statusLabel[s])}</option>)}
        </Select>
      </div>
      <Card pad={false}>
        {data.rows.length === 0 ? <EmptyState title={t("No users")}>{t("No account of your company has this status.")}</EmptyState> : (
          <DataTable rows={data.rows} rowKey={(u) => u.id} cols={[
            // on a phone the role and status sit under the name, so the actions keep their room
            { key: "u", label: t("User"), render: (u) => <div><b>{u.name ?? u.email}{u.you ? ` ${t("(you)")}` : ""}</b><div className="text-xs text-muted-foreground">{u.sub}</div><div className="mt-1 flex flex-wrap gap-1 sm:hidden">{roleBadge(u)}{statusBadge(u)}</div></div> },
            { key: "r", label: t("Role"), render: roleBadge, hideBelow: "sm" },
            { key: "s", label: t("Status"), render: statusBadge, hideBelow: "sm" },
            { key: "x", label: t("Access until"), render: (u) => u.access, hideBelow: "md" },
            { key: "l", label: t("Last sign-in"), render: (u) => u.lastSignIn, hideBelow: "sm" },
            { key: "a", label: t("Actions"), render: (u) => {
              const a = u.actions;
              if (!a.resend && !a.edit && !a.disable && !a.enable && !a.remove) return <span className="text-xs text-muted-foreground">{u.you ? t("Your account") : u.lastContractor ? t("Last active contractor") : "—"}</span>;
              return (
                <div className="flex flex-wrap gap-1">
                  {a.resend && <Btn size="sm" variant="ghost" disabled={pending} onClick={() => run(() => ops.resend(u), sends ? t("Invitation e-mail queued again for {email}", { email: u.email }) : t("Invitation preview re-created for {email} — nothing is sent in the demo", { email: u.email }))}><Icon name="send" />{t("Resend invite")}</Btn>}
                  {a.edit && <Btn size="sm" variant="ghost" onClick={() => open({ kind: "edit", row: u })}><Icon name="edit" />{t("Edit")}</Btn>}
                  {a.disable && <Btn size="sm" variant="ghost" onClick={() => open({ kind: "disable", row: u })}>{t("Disable")}</Btn>}
                  {a.enable && <Btn size="sm" variant="ghost" onClick={() => open({ kind: "enable", row: u })}>{t("Enable")}</Btn>}
                  {a.remove && <Btn size="sm" variant="ghost" onClick={() => open({ kind: "remove", row: u })}><Icon name="delete" />{t("Remove")}</Btn>}
                </div>
              );
            } },
          ]} />
        )}
      </Card>
      <Banner>{t("HQ sets permissions, qualifications and access periods in Access & roles. An accepted account keeps its e-mail and role — to change them, remove it and invite again. You can’t change your own account, and your company always keeps one active contractor.")}</Banner>
      <p className="text-[11px] text-muted-foreground">{sends ? t("Invitation e-mails are sent by the notification service (IR418); in development they arrive in the local Mailpit inbox.") : t("Invites are previews only in the demo — no e-mail is sent (FR-P11, IR416).")}</p>
      <Modal open={!!dialog} onClose={close} title={title} footer={<><Btn onClick={close}>{t("Cancel")}</Btn><Btn variant={dialog?.kind === "remove" || dialog?.kind === "disable" ? "danger" : "primary"} disabled={pending} onClick={submit}>{action}</Btn></>}>
        {(dialog?.kind === "invite" || dialog?.kind === "edit") && <>
          <Field label={t("Email")} error={tried ? emailErr : undefined}><Input type="email" value={email} onChange={(e) => setEmail(e.target.value)} placeholder="lim.tech@example.com" /></Field>
          <Field label={t("Role")} hint={t("Contractors manage jobs, the team and users; technicians do the work on site.")}>
            <Select aria-label={t("Role")} value={role} onChange={(e) => setRole(e.target.value as PartnerRole)}>{PARTNER_ROLES.map((r) => <option key={r} value={r}>{t(roleLabel[r])}</option>)}</Select>
          </Field>
        </>}
        {dialog?.kind === "invite" && <Banner>{t("They get an invitation to sign in. HQ then opens their account with its permissions and access period; until then the invite stays “pending”. Demo: an invitation preview is shown — nothing is sent.")}</Banner>}
        {dialog?.kind === "disable" && <Banner tone="warn">{t("Their access ends now. Enabling them again restores their access period, if it has not ended.")}</Banner>}
        {dialog?.kind === "remove" && <Banner tone="warn">{dialog.row.status === "invited" ? t("The invite is withdrawn.") : t("Their access ends now and they leave your company's user list.")}</Banner>}
        {dialog && dialog.kind !== "invite" && <Field label={t("Reason")} error={tried ? reasonErr : undefined}><Textarea value={reason} onChange={(e) => setReason(e.target.value)} placeholder={t("e.g. on leave until October")} /></Field>}
      </Modal>
    </Page>
  );
}
