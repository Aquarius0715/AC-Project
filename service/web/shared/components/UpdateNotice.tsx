"use client";

import { useEffect, useState } from "react";
import { z } from "zod";
import { APP_VERSION } from "@ac/web/lib/appVersion";
import { Banner, Btn } from "./ui";
import { useT } from "./I18n";

const served = z.object({ version: z.string() });

/** How often an open page asks which build the server runs (IR856). */
export const VERSION_POLL_MS = 60_000;

/** The build the server answers with, or null when it cannot be read (offline, a deployment in progress). */
export async function servedVersion(): Promise<string | null> {
  try {
    const res = await fetch("/bff/version", { cache: "no-store" });
    if (!res.ok) return null;
    const body = served.safeParse(await res.json());
    return body.success ? body.data.version : null;
  } catch {
    return null;
  }
}

/** Whether the app was deployed since this page loaded: the server runs another build than the page's. It asks every VERSION_POLL_MS while the page is in view and when the page comes back into view, and stops once it knows. */
function useNewBuild(): boolean {
  const [isNew, setIsNew] = useState(false);
  useEffect(() => {
    let done = false;
    const check = () => {
      if (done || document.visibilityState !== "visible") return;
      void servedVersion().then((version) => {
        if (done || version === null || version === APP_VERSION) return;
        done = true;
        setIsNew(true);
      });
    };
    const timer = setInterval(check, VERSION_POLL_MS);
    document.addEventListener("visibilitychange", check);
    addEventListener("focus", check);
    return () => {
      done = true;
      clearInterval(timer);
      document.removeEventListener("visibilitychange", check);
      removeEventListener("focus", check);
    };
  }, []);
  return isNew;
}

/** The offer to reload after a deployment (IR856): the page keeps working until the person chooses to reload, so a form being filled in is not lost. */
export function UpdateNotice() {
  const t = useT();
  const isNew = useNewBuild();
  if (!isNew) return null;
  return (
    <div className="px-4 pt-4 sm:px-6 lg:px-8">
      <Banner icon="refresh" action={<Btn size="sm" variant="primary" onClick={() => { location.reload(); }}>{t("Reload page")}</Btn>}>
        {t("A new version of the app is available. Reload the page to use it; save what you are entering first.")}
      </Banner>
    </div>
  );
}
