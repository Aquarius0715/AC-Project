"use client";

import Link from "next/link";
import { ROLES, ROLE_KEY, Role } from "@ac/web/lib/nav";
import { Badge, Card } from "@ac/web/components/ui";
import { useBffSession } from "@ac/web/lib/useOp";
import { useSearchParam } from "@ac/web/lib/urlState";

export type AppLinks = Partial<Record<Role, string>>;

/** Sign-in of one role's app. `others` are the base URLs of the other apps (plain links: separate apps). */
export function LoginScreen({ role, others = {} }: { role: Role; others?: AppLinks }) {
  const cfg = ROLES[role];
  const bff = useBffSession();
  // DATA_SOURCE=api: sign in through the BFF (OIDC with the demo identity); the demo keeps the local role switch
  // a returnTo set by proxy.ts is kept when it belongs to the chosen role's area (the BFF validates it again)
  const asked = useSearchParam("returnTo");
  const returnTo = asked && (asked === cfg.base || asked.startsWith(cfg.base + "/")) ? asked : cfg.base;
  const href = bff?.dataSource === "api" ? `/bff/auth/login?login_hint=${encodeURIComponent(cfg.loginUser)}&returnTo=${encodeURIComponent(returnTo)}` : cfg.base;
  return (
    <main className="grid min-h-dvh place-items-center px-4 py-10">
      <div className="w-full max-w-[920px]">
        <div className="mb-6 text-center">
          <span className="mx-auto mb-3 grid h-12 w-12 place-items-center rounded-2xl bg-primary text-2xl text-white">❄</span>
          <h1 className="text-xl font-bold">AC Project — {cfg.sub.replace(" app", "")}</h1>
          <p className="text-[13px] text-muted">Sign in to manage your units and account</p>
        </div>
        <Card>
          <div className="mx-auto flex max-w-md flex-col gap-3">
            <div className="rounded-xl border border-line bg-surface2/60 p-3">
              <div className="font-bold">{cfg.loginUser}</div>
              <div className="text-xs text-muted">{cfg.loginDesc}</div>
            </div>
            <Link onClick={() => { try { localStorage.setItem(ROLE_KEY, role); } catch {} }} href={href} className="inline-flex items-center justify-center rounded-control bg-primary px-4 py-2.5 text-[13px] font-semibold text-white hover:bg-[#004bc4]">Continue as {cfg.loginUser}</Link>
            <Link href="/forgot-password" className="text-center text-xs font-semibold text-primary hover:underline">Forgot password?</Link>
            <p className="text-center text-[11px] text-muted">Looking for a different AC Project service? Each service has its own sign-in:{" "}
              {(Object.keys(ROLES) as Role[]).filter((r) => r !== role && others[r]).map((r, i) => <span key={r}>{i > 0 && " · "}<a className="font-semibold text-primary hover:underline" href={`${others[r]}/login`}>{ROLES[r].sub}</a></span>)}
            </p>
            <div className="text-center"><Badge tone="warn">DEMO</Badge> <span className="text-[11px] text-muted">Demo authentication only — no real accounts, emails, or passwords are used.</span></div>
          </div>
        </Card>
      </div>
    </main>
  );
}
