import { coreDisplay } from "@ac/web/lib/dal";
import { i18nOf } from "@ac/web/lib/i18n";
import { UnavailableLink } from "@ac/web/screens/unavailable-link";
// In the signed-in user's display language; English when nobody is signed in (coreDisplay falls back).
export default async function NotFound() {
  const { t } = i18nOf(await coreDisplay());
  return (
    <main className="grid min-h-dvh place-items-center px-4">
      <div className="w-full max-w-lg rounded-2xl border border-line bg-surface p-6">
        <h1 className="text-lg font-bold">{t("This page isn’t available")}</h1>
        <p className="mt-1 text-[13px] text-muted-foreground">{t("The page doesn’t exist, or your account can’t open it. Nothing was navigated automatically.")}</p>
        <UnavailableLink t={t} className="mt-4" />
      </div>
    </main>
  );
}
