"use client";
const cls = "mt-5 flex w-full items-center justify-center gap-1.5 rounded-control bg-primary px-4 py-2.5 text-[13px] font-semibold text-white hover:bg-[#004bc4]";

import Link from "next/link";
import { ROLE_KEY, type Role } from "@ac/web/lib/nav";

/** Continue of the browser demo build, which has no identity service: it opens the role home and remembers the chosen role for the shared screens' frame (API mode signs in with SignInForm). */
export function ContinueLink({ appRole: role, href, children }: Readonly<{ appRole: Role; href: string; children: React.ReactNode }>) {
  return <Link onClick={() => { try { localStorage.setItem(ROLE_KEY, role); } catch { /* storage is blocked: the role is not remembered */ } }} href={href} className={cls}>{children}</Link>;
}
