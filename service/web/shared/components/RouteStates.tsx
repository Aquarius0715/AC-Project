"use client";

// Shared UI for the loading.tsx / error.tsx file conventions of each route segment (Next.js loading.js / error.js), in
// the display language (IR269). `what` is the English name of what loads (“air quality”, “Air quality”), a
// dictionary key like the other texts; the i18n key check collects it.
import { useEffect } from "react";
import { Btn, Card, Page } from "@ac/web/components/ui";
import { useT } from "@ac/web/components/I18n";
import { z } from "@ac/web/lib/zod";
import { signInAgainHref } from "@ac/web/lib/login";
import { orNull } from "@ac/web/lib/promises";

export function RouteLoading({ what }: Readonly<{ what: string }>) {
  const t = useT();
  return (
    <Page>
      <Card title={t("Loading {what}…", { what: t(what) })} />
    </Page>
  );
}

const sessionAnswer = z.object({ dataSource: z.string().optional(), signedIn: z.boolean().optional() });
async function readSession(): Promise<unknown> {
  const res = await fetch("/bff/session", { cache: "no-store" });
  return res.json();
}

export function RouteError({ what, error, retry }: Readonly<{ what: string; error: Error & { digest?: string }; retry: () => void }>) {
  const t = useT();
  // a page that failed because the session was refused (IR385): the DAL dropped it, so sign in again and come back
  useEffect(() => {
    void (async () => {
      // no session answer: the error page stays as it is
      const answer = sessionAnswer.safeParse(await orNull(readSession()));
      if (answer.success && answer.data.dataSource === "api" && !answer.data.signedIn) location.assign(signInAgainHref("expired", location.pathname + location.search));
    })();
  }, []);
  return (
    <Page className="max-w-3xl">
      <Card title={t("{what} could not be loaded", { what: t(what) })} sub={error.digest ? t("Reference {id}", { id: error.digest }) : t("Please try again.")}>
        <Btn variant="primary" onClick={() => { retry(); }}>{t("Try again")}</Btn>
      </Card>
    </Page>
  );
}
