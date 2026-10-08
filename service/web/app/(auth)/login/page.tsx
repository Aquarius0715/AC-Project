"use client";

import Link from "next/link";
import { useState } from "react";
import { ROLES, ROLE_KEY, Role } from "@/lib/nav";
import { Badge, Btn, Card, cx } from "@/components/ui";
import { useBffSession } from "@/lib/useOp";

export default function Login() {
  const [role, setRole] = useState<Role>("client");
  const cfg = ROLES[role];
  const bff = useBffSession();
  // DATA_SOURCE=api: sign in through the BFF (OIDC with the demo identity); the demo keeps the local role switch
  const href = bff?.dataSource === "api" ? `/bff/auth/login?login_hint=${encodeURIComponent(cfg.loginUser)}&returnTo=${encodeURIComponent(cfg.base)}` : cfg.base;
  return (
    <main className="grid min-h-dvh place-items-center px-4 py-10">
      <div className="w-full max-w-[920px]">
        <div className="mb-6 text-center">
          <span className="mx-auto mb-3 grid h-12 w-12 place-items-center rounded-2xl bg-primary text-2xl text-white">❄</span>
          <h1 className="text-xl font-bold">AC Project — {cfg.sub.replace(" app", "")}</h1>
          <p className="text-[13px] text-muted">Sign in to manage your units and account</p>
        </div>
        <div className="grid-fluid mb-5" style={{ ["--min" as string]: "190px" }}>
          {(Object.keys(ROLES) as Role[]).map((r) => (
            <button key={r} onClick={() => setRole(r)} aria-pressed={role === r} className={cx("rounded-2xl border p-4 text-left transition-colors", role === r ? "border-primary bg-primary-soft/60" : "border-line bg-surface hover:bg-surface2")}>
              <div className="text-[13px] font-bold">{ROLES[r].sub}</div>
              <div className="mt-0.5 text-xs text-muted">{ROLES[r].loginUser}</div>
            </button>
          ))}
        </div>
        <Card>
          <div className="mx-auto flex max-w-md flex-col gap-3">
            <div className="rounded-xl border border-line bg-surface2/60 p-3">
              <div className="font-bold">{cfg.loginUser}</div>
              <div className="text-xs text-muted">{cfg.loginDesc}</div>
            </div>
            <Link onClick={() => { try { localStorage.setItem(ROLE_KEY, role); } catch {} }} href={href} className="inline-flex items-center justify-center rounded-control bg-primary px-4 py-2.5 text-[13px] font-semibold text-white hover:bg-[#004bc4]">Continue as {cfg.loginUser}</Link>
            <Link href="/forgot-password" className="text-center text-xs font-semibold text-primary hover:underline">Forgot password?</Link>
            <p className="text-center text-[11px] text-muted">Looking for a different AC Project service? Pick it above — each service has its own sign-in.</p>
            <div className="text-center"><Badge tone="warn">DEMO</Badge> <span className="text-[11px] text-muted">Demo authentication only — no real accounts, emails, or passwords are used.</span></div>
          </div>
        </Card>
      </div>
    </main>
  );
}
