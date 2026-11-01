import { Card } from "@ac/web/components/ui";
import { coreDisplay } from "@ac/web/lib/dal";
import { i18nOf } from "@ac/web/lib/i18n";
import { UnavailableLink } from "@ac/web/screens/unavailable-link";
// In the signed-in user's display language; English when nobody is signed in (coreDisplay falls back).
export default async function Forbidden() {
  const { t } = i18nOf(await coreDisplay());
  return (
    <div className="mx-auto max-w-lg pt-10">
      <Card title={t("This page isn’t available")} sub={t("The page doesn’t exist, or your account can’t open it. Nothing was navigated automatically.")}>
        <UnavailableLink t={t} />
      </Card>
    </div>
  );
}
