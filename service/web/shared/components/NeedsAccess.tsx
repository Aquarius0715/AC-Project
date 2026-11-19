"use client";

import { EmptyState, Page } from "@ac/web/components/ui";
import { useT } from "@ac/web/components/I18n";
import { permissionLabel } from "@ac/web/lib/permissionLabels";

/** A page the signed-in role may not read: which permission it needs and whom to ask, as Customers & units says it —
not "could not be loaded" with Try again, which reads as a failure (IR581). `what` is the sidebar's name. */
export function NeedsAccess({ what, permission }: Readonly<{ what: string; permission: string }>) {
  const t = useT();
  return (
    <Page>
      <EmptyState title={t("To open {what}, your role needs the “{permission}” permission.", { what: t(what), permission: permissionLabel(permission, t) })}>{t("Ask an identity administrator for {what} access (Access & roles).", { what: t(what) })}</EmptyState>
    </Page>
  );
}
