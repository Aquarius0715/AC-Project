"use client";

import { createContext, useContext } from "react";
import { DEFAULT_DISPLAY, translate, type Display, type T } from "@ac/web/lib/i18n";

const DisplayContext = createContext<Display>(DEFAULT_DISPLAY);

/** The display language and time zone of everything under the shell (FR-X01, IR258, IR259); AppShell sets them. */
export function I18nProvider({ display, children }: { display: Display; children: React.ReactNode }) {
  return <DisplayContext.Provider value={display}>{children}</DisplayContext.Provider>;
}

export const useDisplay = () => useContext(DisplayContext);
export const useLocale = () => useContext(DisplayContext).locale;

/** t(text, params): the English text in the current display language. */
export function useT(): T {
  const locale = useLocale();
  return (text, params) => translate(locale, text, params);
}
