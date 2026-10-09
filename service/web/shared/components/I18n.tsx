"use client";

import { createContext, useContext } from "react";
import { translate, type Locale, type T } from "@ac/web/lib/i18n";

const LocaleContext = createContext<Locale>("en");

/** The display language of everything under the shell (FR-X01, IR258); AppShell sets it. */
export function I18nProvider({ locale, children }: { locale: Locale; children: React.ReactNode }) {
  return <LocaleContext.Provider value={locale}>{children}</LocaleContext.Provider>;
}

export const useLocale = () => useContext(LocaleContext);

/** t(text, params): the English text in the current display language. */
export function useT(): T {
  const locale = useLocale();
  return (text, params) => translate(locale, text, params);
}
