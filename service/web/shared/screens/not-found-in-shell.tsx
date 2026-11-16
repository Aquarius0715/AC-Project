import { coreDisplay } from "@ac/web/lib/dal";
import { i18nOf } from "@ac/web/lib/i18n";
import { UnavailableLink } from "@ac/web/screens/unavailable-link";
// The same notice as the root not-found, but inside the signed-in app's layout: the sidebar stays, so the next screen is
// one tap away instead of only “Go to Overview” (UI/UX loop R54). An unknown address under the app's area reaches it
// through that area's [...missing] route.
export default async function NotFoundInShell() {
  const { t } = i18nOf(await coreDisplay());
  return (
    <div className="grid place-items-center px-4 py-16">
      <div className="w-full max-w-lg rounded-2xl border border-line bg-surface p-6">
        <h1 className="text-lg font-bold">{t("This page isn’t available")}</h1>
        <p className="mt-1 text-[13px] text-muted-foreground">{t("The page doesn’t exist, or your account can’t open it. Nothing was navigated automatically.")}</p>
        <UnavailableLink t={t} className="mt-4" />
      </div>
    </div>
  );
}
