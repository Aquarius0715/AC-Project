"use client";

import Link from "next/link";
import { ROLE_KEY, type Role } from "@ac/web/lib/nav";

const cls = "mt-4 flex w-full items-center justify-center rounded-control bg-primary px-4 py-2.5 text-[13px] font-semibold text-white hover:bg-[#004bc4]";

/** Continue on the sign-in page. The BFF sign-in (API mode) is a full navigation to the identity provider; the browser
 * demo opens the role home and remembers the chosen role for the shared screens' frame. */
export function ContinueLink({ role, href, children }: { role: Role; href: string; children: React.ReactNode }) {
  if (href.startsWith("/bff/")) return <a href={href} className={cls}>{children}</a>;
  return <Link onClick={() => { try { localStorage.setItem(ROLE_KEY, role); } catch {} }} href={href} className={cls}>{children}</Link>;
}
