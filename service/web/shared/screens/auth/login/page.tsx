import { redirect } from "next/navigation";
import { Banner, LinkButton } from "@ac/web/components/ui";
import { Icon } from "@ac/web/components/Icon";
import { roleConfig, type Role } from "@ac/web/lib/nav";
import { demoHref, loginError, safeReturnTo } from "@ac/web/lib/login";
import { coreDisplay } from "@ac/web/lib/dal";
import { translator } from "@ac/web/lib/i18n";
import { ContinueLink } from "./continue-link";

type Params = Record<string, string | string[] | undefined>;
const one = (v: string | string[] | undefined) => (Array.isArray(v) ? v[0] : v);

const NO_PARAMS: Params = {};

/** Sign-in of one role's app (FR-X01, DDC-07, SCR-X-login, IR251, IR790, Figma Login frames), rendered on the server. With
the identity service there is one sign-in screen, the identity provider's own (e-mail and password on one page, its
language choice, one-time code, reset): a plain visit goes straight on to it through the BFF, which keeps returnTo. Only a
sign-in that came back (?error=) stops here, saying why, with a button to start again, so there is no redirect loop. The
browser demo has no identity service, so Continue opens the app. */
export async function LoginScreen({ appRole: role, params = NO_PARAMS, api }: Readonly<{ appRole: Role; params?: Params; api: boolean }>) {
  const returnTo = safeReturnTo(role, one(params.returnTo));
  const error = loginError(one(params.error));
  const start = `/bff/auth/login?returnTo=${encodeURIComponent(returnTo)}`;
  if (api && !error) redirect(start);
  const config = roleConfig(role);
  const { locale } = await coreDisplay();
  const t = translator(locale);
  return (
    <main className="relative grid min-h-dvh place-items-center px-4 py-16">
      <div className="w-full max-w-[380px] rounded-2xl border border-line bg-surface p-6 text-center shadow-sm">
        <span aria-hidden className="mx-auto mb-3 grid h-10 w-10 place-items-center rounded-xl bg-primary text-xl text-white"><Icon name="cooling" /></span>
        <p className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">AC Project · {t(config.loginTitle)}</p>
        <h1 className="mt-1 text-lg font-bold">{t("Sign in")}</h1>
        <p className="mt-1 text-[13px] text-muted-foreground">{t(config.loginSub)}</p>
        {error && <div className="mt-4 text-left"><Banner tone="crit">{t(error)}</Banner></div>}
        {api ? (
          <LinkButton href={start} variant="primary" className="mt-5 w-full">{t("Sign in")} <Icon name="arrowRight" /></LinkButton>
        ) : (
          <>
            <p className="mt-4 text-left text-xs text-muted-foreground">{t("This preview has no sign-in service. Continue opens the app.")}</p>
            <ContinueLink appRole={role} href={demoHref(role)}>{t("Continue")} <Icon name="arrowRight" /></ContinueLink>
          </>
        )}
      </div>
    </main>
  );
}
