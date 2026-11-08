import Link from "next/link";
import { Banner, Btn } from "@ac/web/components/ui";
import { Icon } from "@ac/web/components/Icon";
import { roleConfig, type Role } from "@ac/web/lib/nav";
import { demoHref, loginError, otherServices, safeReturnTo } from "@ac/web/lib/login";
import { coreDisplay } from "@ac/web/lib/dal";
import { LANGUAGE_NAMES, LOCALES, translator } from "@ac/web/lib/i18n";
import { entryOf, lookup } from "@ac/web/lib/lookup";
import { chooseSignInLanguage } from "./actions";
import { ContinueLink } from "./continue-link";
import { SignInForm } from "./sign-in-form";

export type AppLinks = Partial<Record<Role, string>>;
type Params = Record<string, string | string[] | undefined>;
const one = (v: string | string[] | undefined) => (Array.isArray(v) ? v[0] : v);

const NO_LINKS: AppLinks = {};
const NO_PARAMS: Params = {};

/** Sign-in of one role's app (FR-X01, DDC-07, SCR-X-login, IR251, IR646, Figma Login frames), rendered on the server: the
person gives an e-mail address and goes on to the identity provider's own pages (password, one-time code, reset) in the
language chosen here; the kept returnTo decides where they land, and a sign-in that came back says why. `others` are the
base URLs of the other apps (plain links: separate apps). */
export async function LoginScreen({ appRole: role, others = NO_LINKS, params = NO_PARAMS, api }: Readonly<{ appRole: Role; others?: AppLinks; params?: Params; api: boolean }>) {
  const config = roleConfig(role);
  const { locale } = await coreDisplay();
  const t = translator(locale);
  const returnTo = safeReturnTo(role, one(params.returnTo));
  const error = loginError(one(params.error));
  const names = otherServices(role);
  return (
    <main className="relative grid min-h-dvh place-items-center px-4 py-16">
      <form action={chooseSignInLanguage} className="absolute right-4 top-4 flex items-center gap-1 rounded-control border border-line bg-surface p-1 text-xs" aria-label={t("Language")}>
        <Icon name="language" />
        {LOCALES.map((id) => (
          <Btn key={id} type="submit" name="locale" value={id} lang={id} size="sm" variant={id === locale ? "primary" : "ghost"} aria-pressed={id === locale}>{entryOf(LANGUAGE_NAMES, id)}</Btn>
        ))}
      </form>
      <div className="w-full max-w-[380px] rounded-2xl border border-line bg-surface p-6 text-center shadow-sm">
        <span aria-hidden className="mx-auto mb-3 grid h-10 w-10 place-items-center rounded-xl bg-primary text-xl text-white"><Icon name="cooling" /></span>
        <p className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">AC Project · {t(config.loginTitle)}</p>
        <h1 className="mt-1 text-lg font-bold">{t("Sign in")}</h1>
        <p className="mt-1 text-[13px] text-muted-foreground">{t(config.loginSub)}</p>
        {error && <div className="mt-4 text-left"><Banner tone="crit">{t(error)}</Banner></div>}
        {api ? (
          <SignInForm returnTo={returnTo} locale={locale} labels={{ email: t("Email address"), hint: t("You enter your password on the next page, where you can also reset it."), submit: t("Continue") }} />
        ) : (
          <>
            <p className="mt-4 text-left text-xs text-muted-foreground">{t("This preview has no sign-in service. Continue opens the app.")}</p>
            <ContinueLink appRole={role} href={demoHref(role)}>{t("Continue")} <Icon name="arrowRight" /></ContinueLink>
          </>
        )}
        {!api && <Link href="/forgot-password" className="mt-4 inline-block text-xs font-semibold text-primary hover:underline">{t("Forgot password?")}</Link>}
        <nav aria-label={t("Other AC Project services")} className="mt-5 border-t border-line pt-4 text-[11px] text-muted-foreground">
          <p>{t("Other AC Project services")}</p>
          <ul className="mt-1 flex flex-wrap justify-center gap-x-3 gap-y-1">
            {names.map((r) => {
              const url = lookup(others, r);
              const name = t(roleConfig(r).loginTitle);
              return <li key={r}>{url ? <a className="font-semibold text-primary hover:underline" href={`${url}/login`}>{name}</a> : name}</li>;
            })}
          </ul>
        </nav>
      </div>
    </main>
  );
}
