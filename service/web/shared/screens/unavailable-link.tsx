import { Icon } from "@ac/web/components/Icon";
import { LinkButton } from "@ac/web/components/ui";
import { apiMode, getSession } from "@ac/web/lib/dal";
import { roleHome } from "@ac/web/lib/session";
import type { T } from "@ac/web/lib/i18n";

/** The one way on from Page unavailable (IR57), as its primary button: a signed-in user back to their Overview, anyone
else to sign in (IR571). */
export async function UnavailableLink({ t, className }: Readonly<{ t: T; className?: string }>) {
  const s = apiMode() ? await getSession() : null;
  return <LinkButton href={s ? roleHome[s.role] : "/login"} variant="primary" className={className}>{t(s ? "Go to Overview" : "Go to sign in")} <Icon name="arrowRight" /></LinkButton>;
}
