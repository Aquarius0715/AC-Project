"use client";

// Shared UI for the loading.tsx / error.tsx file conventions of each route segment (Next.js loading.js / error.js), in
// the display language (IR269). `what` is the English name of what loads (“air quality”, “Air quality”), a
// dictionary key like the other texts; the i18n key check collects it.
import { useEffect } from "react";
import { Btn, Card, Page } from "@ac/web/components/ui";
import { Skeleton } from "@ac/web/components/ui/skeleton";
import { useT } from "@ac/web/components/I18n";
import { z } from "@ac/web/lib/zod";
import { signInAgainHref } from "@ac/web/lib/login";
import { orNull } from "@ac/web/lib/promises";

// The shape of the page that is loading, so the layout does not jump when the data arrives: a list (the default),
// the overview's KPI tiles and panels (dashboard), or a detail page's facts and sections (detail).
export type LoadingVariant = "list" | "dashboard" | "detail";

export function RouteLoading({ what, variant = "list" }: Readonly<{ what: string; variant?: LoadingVariant }>) {
  const t = useT();
  return (
    // the status region is announced once; the skeleton blocks themselves are hidden from assistive tech
    <Page>
      <div role="status" aria-busy="true" className="flex flex-col gap-4">
        <span className="sr-only">{t("Loading {what}…", { what: t(what) })}</span>
        <div aria-hidden="true" className="flex flex-col gap-4">
          <SkeletonHead />
          {variant === "dashboard" && <DashboardSkeleton />}
          {variant === "detail" && <DetailSkeleton />}
          {variant === "list" && <ListSkeleton />}
        </div>
      </div>
    </Page>
  );
}

function SkeletonHead() {
  return (
    <div className="flex flex-wrap items-end justify-between gap-3">
      <div className="flex flex-col gap-2">
        <Skeleton className="h-6 w-48" />
        <Skeleton className="h-3 w-72 max-w-full" />
      </div>
      <Skeleton className="h-8 w-28" />
    </div>
  );
}

function ListSkeleton() {
  return (
    <Card>
      <div className="flex flex-col gap-3">
        <Skeleton className="h-8 w-full sm:w-64" />
        <div className="flex flex-col gap-2">
          {Array.from({ length: 6 }, (_, i) => (
            <Skeleton key={i} className="h-10 w-full" />
          ))}
        </div>
      </div>
    </Card>
  );
}

function DashboardSkeleton() {
  return (
    <>
      <div className="grid grid-cols-2 gap-3 lg:grid-cols-4">
        {Array.from({ length: 4 }, (_, i) => (
          <Card key={i}>
            <div className="flex flex-col gap-2">
              <Skeleton className="h-3 w-20" />
              <Skeleton className="h-7 w-24" />
            </div>
          </Card>
        ))}
      </div>
      <div className="grid gap-3 md:grid-cols-2">
        {Array.from({ length: 2 }, (_, i) => (
          <Card key={i}>
            <Skeleton className="h-48 w-full" />
          </Card>
        ))}
      </div>
    </>
  );
}

function DetailSkeleton() {
  return (
    <div className="grid gap-3 md:grid-cols-2">
      <Card>
        <div className="flex flex-col gap-3">
          {Array.from({ length: 4 }, (_, i) => (
            <div key={i} className="flex flex-col gap-1.5">
              <Skeleton className="h-3 w-24" />
              <Skeleton className="h-5 w-40 max-w-full" />
            </div>
          ))}
        </div>
      </Card>
      <Card>
        <div className="flex flex-col gap-2">
          {Array.from({ length: 3 }, (_, i) => (
            <Skeleton key={i} className="h-12 w-full" />
          ))}
        </div>
      </Card>
    </div>
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
