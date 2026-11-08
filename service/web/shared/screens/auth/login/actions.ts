"use server";

// Server Action of the sign-in page (IR646): the language chosen before there is a session. Server Functions are public
// endpoints, so the value is checked against the display languages and nothing else is read or written. The page
// renders again with the cookie, in the chosen language; the BFF passes it on to the identity provider as ui_locales.
import { cookies } from "next/headers";
import { isLocale } from "@ac/web/lib/i18n";
import { SIGN_IN_LOCALE_COOKIE } from "@ac/web/lib/login";

const ONE_YEAR = 60 * 60 * 24 * 365;

export async function chooseSignInLanguage(form: FormData): Promise<void> {
  const locale = form.get("locale");
  if (!isLocale(locale)) return;
  const jar = await cookies();
  jar.set(SIGN_IN_LOCALE_COOKIE, locale, { path: "/", sameSite: "lax", httpOnly: true, secure: (process.env.APP_URL ?? "").startsWith("https"), maxAge: ONE_YEAR });
}
