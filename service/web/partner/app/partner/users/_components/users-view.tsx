"use client";

import { useState } from "react";
import { Badge, Banner, Btn, Card, DataTable, EmptyState, Field, Input, LinkBtn, Modal, Page, PageHead, Select, Textarea } from "@ac/web/components/ui";
import { useT } from "@ac/web/components/I18n";
import { Icon } from "@ac/web/components/Icon";
import { useAction } from "@ac/web/lib/useAction";
import type { ActionFailure } from "@ac/web/lib/actionMessage";
import { ACCESS_MAX_DAYS, accessEnd, accessError, addDays, nameError, PARTNER_ROLES, PARTNER_STATUSES, partnerInviteError, reasonError, roleLabel, statusLabel, type PartnerRole, type PartnerStatus, type PartnerUserRow } from "@ac/web/lib/partnerUsers";

type Result = { ok: true; value: null } | ({ ok: false } & ActionFailure);
export type UsersOps = {
  invite: (email: string, role: PartnerRole, name: string, accessUntil: string | null) => Promise<Result>;
  access: (r: PartnerUserRow, accessUntil: string, reason: string) => Promise<Result>;
  setStatus: (r: PartnerUserRow, status: "active" | "disabled", reason: string) => Promise<Result>;
  remove: (r: PartnerUserRow, reason: string) => Promise<Result>;
  resend: (r: PartnerUserRow) => Promise<Result>;
};
/** `today` is the business day in Kuala Lumpur (YYYY-MM-DD): a technician's access end runs from it to a year ahead. */
export type UsersData = { company: string; status: PartnerStatus | null; rows: PartnerUserRow[]; all: { id: string; email: string }[]; today: string };

type Dialog =
  | { kind: "invite" }
  | { kind: "access" | "disable" | "enable" | "remove"; row: PartnerUserRow };

/** Users (FR-P11, IR461): the contractor's company accounts, with the actions each row allows; one dialog at a time
 * (add, access end, disable, enable, remove), each change after the adding with a reason. */
export function UsersView({ data, ops, onStatus, sends = false }: { data: UsersData | null; ops: UsersOps; onStatus: (s: PartnerStatus | null) => void; sends?: boolean }) {
  const t = useT();
  const [pending, run] = useAction();
  const [dialog, setDialog] = useState<Dialog | null>(null);
  const [email, setEmail] = useState("");
  const [name, setName] = useState("");
  const [until, setUntil] = useState("");
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
  // a technician's access ends at most a year ahead (IR461); three months is offered first
  const open = (d: Dialog) => {
    setDialog(d); setTried(false); setReason(""); setEmail(""); setName(""); setRole("technician");
    setUntil(addDays(data.today, 90));
  };
  const close = () => setDialog(null);
  const adding = dialog?.kind === "invite";
  const emailErr = adding ? partnerInviteError(email, data.all, t) : undefined;
  const nameErr = adding ? nameError(name, t) : undefined;
  const untilErr = (adding && role === "technician") || dialog?.kind === "access" ? accessError(until, data.today, t) : undefined;
  const reasonErr = dialog && !adding ? reasonError(reason, t) : undefined;
  const submit = () => {
    setTried(true);
    if (!dialog || emailErr || nameErr || untilErr || reasonErr) return;
    const who = dialog.kind === "invite" ? name.trim() || email.trim() : dialog.row.name ?? dialog.row.email;
    if (dialog.kind === "invite") run(() => ops.invite(email, role, name, role === "technician" ? accessEnd(until) : null), sends ? t("{name} added — a set-up e-mail is on its way to {email}", { name: who, email: email.trim() }) : t("{name} added — nothing is sent in the demo", { name: who }), close);
    else if (dialog.kind === "access") run(() => ops.access(dialog.row, accessEnd(until), reason), t("Access of {name} now ends {date}", { name: who, date: until }), close);
    else if (dialog.kind === "remove") run(() => ops.remove(dialog.row, reason), t("{name} removed", { name: who }), close);
    else run(() => ops.setStatus(dialog.row, dialog.kind === "disable" ? "disabled" : "active", reason), t(dialog.kind === "disable" ? "{name} disabled — they can no longer sign in" : "{name} enabled again", { name: who }), close);
  };
  const title = !dialog ? "" : {
    invite: t("Add a user to {company}", { company: data.company || t("your company") }),
    access: t("Access of {name}", { name: "row" in dialog ? dialog.row.name ?? dialog.row.email : "" }),
    disable: t("Disable {name}", { name: "row" in dialog ? dialog.row.name ?? dialog.row.email : "" }),
    enable: t("Enable {name}", { name: "row" in dialog ? dialog.row.name ?? dialog.row.email : "" }),
    remove: t("Remove {name}", { name: "row" in dialog ? dialog.row.name ?? dialog.row.email : "" }),
  }[dialog.kind];
  const action = !dialog ? "" : { invite: t("Add user"), access: t("Save access"), disable: t("Disable"), enable: t("Enable"), remove: t("Remove") }[dialog.kind];
  const roleBadge = (u: PartnerUserRow) => <Badge tone={u.role === "contractor" ? "primary" : "muted"}>{t(roleLabel[u.role])}</Badge>;
  const statusBadge = (u: PartnerUserRow) => <Badge tone={u.ended ? "muted" : u.status === "active" ? "ok" : u.status === "invited" ? "warn" : "muted"}>{u.ended ? t("Access ended") : t(statusLabel[u.status])}</Badge>;
  return (
    <Page>
      <PageHead title={data.company ? t("Users of {company}", { company: data.company }) : t("Users")} sub={t("Contractors and technicians of your company who can sign in. You can add, disable and remove them.")}
        action={<Btn variant="primary" onClick={() => open({ kind: "invite" })}><Icon name="add" /> {t("Add user")}</Btn>} />
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
              // why Disable or Remove is missing, next to what is still offered (a technician's access end, IR461)
              const hint = u.you ? t("Your account") : u.lastContractor ? t("Last active contractor") : u.booked ? t("Has an assigned job — reassign it first") : null;
              if (!a.resend && !a.access && !a.disable && !a.enable && !a.remove) return <span className="text-xs text-muted-foreground">{hint ?? "—"}</span>;
              return (
                <div className="flex flex-wrap items-center gap-1">
                  {a.resend && <Btn size="sm" variant="ghost" disabled={pending} onClick={() => run(() => ops.resend(u), sends ? t("Set-up e-mail sent again to {email}", { email: u.email }) : t("Set-up e-mail re-sent to {email} — nothing is sent in the demo", { email: u.email }))}><Icon name="send" />{t("Resend set-up e-mail")}</Btn>}
                  {a.access && <Btn size="sm" variant="ghost" onClick={() => { open({ kind: "access", row: u }); if (u.accessDate) setUntil(u.accessDate); }}><Icon name="edit" />{t("Access")}</Btn>}
                  {a.disable && <Btn size="sm" variant="ghost" onClick={() => open({ kind: "disable", row: u })}>{t("Disable")}</Btn>}
                  {a.enable && <Btn size="sm" variant="ghost" onClick={() => open({ kind: "enable", row: u })}>{t("Enable")}</Btn>}
                  {a.remove && <Btn size="sm" variant="ghost" onClick={() => open({ kind: "remove", row: u })}><Icon name="delete" />{t("Remove")}</Btn>}
                  {hint && <span className="text-xs text-muted-foreground">{hint}</span>}
                </div>
              );
            } },
          ]} />
        )}
      </Card>
      <Banner>{t("A technician you add can be assigned once HQ has verified their certificates. An account keeps its e-mail and role — to change them, remove it and add it again. You can’t change your own account, and your company always keeps one active contractor.")}</Banner>
      <p className="text-[11px] text-muted-foreground">{sends ? t("The set-up e-mail comes from the sign-in service; in development it arrives in the local Mailpit inbox.") : t("Nothing is sent in the demo (FR-P11, IR461).")}</p>
      <Modal open={!!dialog} onClose={close} title={title} footer={<><Btn onClick={close}>{t("Cancel")}</Btn><Btn variant={dialog?.kind === "remove" || dialog?.kind === "disable" ? "danger" : "primary"} disabled={pending} onClick={submit}>{action}</Btn></>}>
        {adding && <>
          <Field label={t("Name")} error={tried ? nameErr : undefined}><Input value={name} maxLength={120} onChange={(e) => setName(e.target.value)} placeholder="Lim Wei Ming" /></Field>
          <Field label={t("Email")} error={tried ? emailErr : undefined}><Input type="email" value={email} onChange={(e) => setEmail(e.target.value)} placeholder="lim.tech@example.com" /></Field>
          <Field label={t("Role")} hint={t("Contractors manage jobs, the team and users; technicians do the work on site.")}>
            <Select aria-label={t("Role")} value={role} onChange={(e) => setRole(e.target.value as PartnerRole)}>{PARTNER_ROLES.map((r) => <option key={r} value={r}>{t(roleLabel[r])}</option>)}</Select>
          </Field>
        </>}
        {((adding && role === "technician") || dialog?.kind === "access") &&
          <Field label={t("Access until")} hint={t("The last day they can sign in, at most a year ahead (Kuala Lumpur time).")} error={tried ? untilErr : undefined}>
            <Input type="date" value={until} min={data.today} max={addDays(data.today, ACCESS_MAX_DAYS)} onChange={(e) => setUntil(e.target.value)} />
          </Field>}
        {adding && <Banner>{t("Their account is created now and they get an e-mail to set their password; until they sign in they are listed as “Not signed in yet”.")}</Banner>}
        {dialog?.kind === "disable" && <Banner tone="warn">{t("Their access ends now. Enabling them again restores their access period, if it has not ended.")}</Banner>}
        {dialog?.kind === "remove" && <Banner tone="warn">{dialog.row.status === "invited" ? t("Their account is removed before they signed in.") : t("Their access ends now and they leave your company's user list.")}</Banner>}
        {dialog && dialog.kind !== "invite" && <Field label={t("Reason")} error={tried ? reasonErr : undefined}><Textarea value={reason} onChange={(e) => setReason(e.target.value)} placeholder={t("e.g. on leave until October")} /></Field>}
      </Modal>
    </Page>
  );
}
