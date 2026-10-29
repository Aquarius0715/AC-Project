import Link from "next/link";
import { Card } from "@ac/web/components/ui";
import { Icon } from "@ac/web/components/Icon";
import { coreDisplay } from "@ac/web/lib/dal";
import { i18nOf } from "@ac/web/lib/i18n";
// In the signed-in user's display language; English when nobody is signed in (coreDisplay falls back).
export default async function Forbidden() {
  const { t } = i18nOf(await coreDisplay());
  return (
    <div className="mx-auto max-w-lg pt-10">
      <Card title={t("This page isn’t available")} sub={t("The page doesn’t exist, or your account can’t open it. Nothing was navigated automatically.")}>
        <p className="mb-3 text-xs text-muted-foreground">{t("One shared screen for both FORBIDDEN and NOT_FOUND, so it never reveals whether the resource exists (D01/IR57).")}</p>
        <Link href="/login" className="text-xs font-semibold text-primary hover:underline">{t("Go to sign in")} <Icon name="arrowRight" /></Link>
      </Card>
    </div>
  );
}
