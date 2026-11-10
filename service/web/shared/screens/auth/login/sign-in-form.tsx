"use client";

import { Btn, Field, Hidden, Input } from "@ac/web/components/ui";
import { Icon } from "@ac/web/components/Icon";

/** The e-mail step of the sign-in (IR646): a plain GET form to the BFF, which starts the OIDC authorization-code flow with
PKCE and passes the address on as the identity provider's login hint. The password, the one-time code and a reset are
entered on the identity provider's own pages, never here, so the form works without JavaScript too. */
export function SignInForm({ returnTo, locale, labels }: Readonly<{ returnTo: string; locale: string; labels: { email: string; hint: string; submit: string } }>) {
  return (
    <form action="/bff/auth/login" method="get" className="mt-5 flex flex-col gap-3 text-left">
      <Hidden name="returnTo" value={returnTo} />
      <Hidden name="ui_locales" value={locale} />
      <Field label={labels.email} hint={labels.hint}>
        <Input type="email" name="login_hint" required autoComplete="username" autoCapitalize="none" spellCheck={false} maxLength={320} inputMode="email" placeholder="name@example.com" />
      </Field>
      <Btn type="submit" variant="primary" className="w-full">{labels.submit} <Icon name="arrowRight" /></Btn>
    </form>
  );
}
