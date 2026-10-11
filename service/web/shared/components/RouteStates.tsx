"use client";

// Shared UI for the loading.tsx / error.tsx file conventions of each route segment (Next.js loading.js / error.js), in
// the display language (IR269). `what` is the English name of what loads (“air quality”, “Air quality”), a
// dictionary key like the other texts; the i18n key check collects it.
import { useEffect } from "react";
import { Btn, Card, Page } from "@ac/web/components/ui";
import { useT } from "@ac/web/components/I18n";
import { signInAgainHref } from "@ac/web/lib/login";

export function RouteLoading({ what }: { what: string }) {
  const t = useT();
  return (
    <Page>
      <Card title={t("Loading {what}…", { what: t(what) })} />
    </Page>
  );
}

export function RouteError({ what, error, retry }: { what: string; error: Error & { digest?: string }; retry: () => void }) {
  const t = useT();
  // a page that failed because the session was refused (IR385): the DAL dropped it, so sign in again and come back
  useEffect(() => {
    fetch("/bff/session", { cache: "no-store" }).then((r) => r.json()).then((s: { dataSource?: string; signedIn?: boolean }) => {
      if (s.dataSource === "api" && !s.signedIn) window.location.assign(signInAgainHref("expired", window.location.pathname + window.location.search));
    }).catch(() => {});
  }, []);
  return (
    <Page className="max-w-3xl">
      <Card title={t("{what} could not be loaded", { what: t(what) })} sub={error.digest ? t("Reference {id}", { id: error.digest }) : t("Please try again.")}>
        <Btn variant="primary" onClick={() => retry()}>{t("Try again")}</Btn>
      </Card>
    </Page>
  );
}
