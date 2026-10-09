"use client";

import Link from "next/link";
import { usePathname, useSearchParams } from "next/navigation";
import { Suspense, useEffect, useState } from "react";
import { ROLES, ROLE_KEY, Role, roleFromPath, type ShellLive } from "@ac/web/lib/nav";
import { Badge, Banner, Btn, Modal, SummaryList, ToastProvider, cx } from "./ui";
import { useJobStore } from "@ac/web/lib/jobs";
import { AssistantPanel } from "./Assistant";
import { QrScan } from "./QrScan";
import { CURRENT_CLIENT } from "@ac/web/lib/clientUsers";
import { setStoredValue, useStoredValue } from "@ac/web/lib/urlState";
import { isLocale, LOCALE_KEY, translate } from "@ac/web/lib/i18n";
import { I18nProvider } from "./I18n";
import { useBffSession } from "@ac/web/lib/useOp";

export function useStoredRole(): Role {
  const pathname = usePathname();
  const fromPath = roleFromPath(pathname);
  const saved = useStoredValue(ROLE_KEY) as Role | null; // last role, external store (no effect)
  useEffect(() => {
    if (fromPath) setStoredValue(ROLE_KEY, fromPath); // sync the external system only
  }, [fromPath]);
  return fromPath ?? (saved && ROLES[saved] ? saved : "client");
}

const signOut = "flex w-full items-center gap-2.5 rounded-control px-3 py-2 text-[13px] font-semibold text-crit hover:bg-crit-soft/50";
const SHARED = [
  { href: "/notifications", label: "Notifications", icon: "🔔" },
  { href: "/settings/preferences", label: "Preferences", icon: "✲" },
];

function NavLink({ href, label, icon, badge, active }: { href: string; label: string; icon: string; badge?: string; active: boolean }) {
  return (
    <Link href={href} aria-current={active ? "page" : undefined} className={cx("flex items-center gap-2.5 rounded-control px-3 py-2 text-[13px] font-semibold", active ? "bg-primary-soft text-primary" : "text-ink hover:bg-surface2")}>
      <span aria-hidden className="w-4 text-center text-muted">{icon}</span>
      <span className="min-w-0 flex-1 leading-tight">{label}</span>
      {badge && <span className="grid h-[18px] min-w-[18px] place-items-center rounded-full bg-crit px-1 text-[10px] font-bold text-white">{badge}</span>}
    </Link>
  );
}

type NavItem = { href: string; label: string; icon: string; badge?: string; match?: string[] };
/** The role's sidebar items with the active one: the longest matching path, where an item whose href carries a query
 * (Sidebar › Assigned jobs = /technician?tab=all) wins on its path only when the query matches too. */
function RoleNav({ items, base, pathname, search }: { items: NavItem[]; base: string; pathname: string; search: string }) {
  const query = new URLSearchParams(search);
  const score = (href: string, match?: string[]) => {
    const hits = [href, ...(match ?? [])].filter((h) => {
      const [path, q] = h.split("?");
      if (q) return pathname === path && [...new URLSearchParams(q)].every(([k, v]) => query.get(k) === v);
      return path === base ? pathname === path : pathname === path || pathname.startsWith(path + "/");
    });
    return hits.length ? Math.max(...hits.map((h) => h.length)) : 0;
  };
  const best = Math.max(0, ...items.map((i) => score(i.href, i.match)));
  const activeHref = best ? items.find((i) => score(i.href, i.match) === best)?.href : undefined;
  return <nav className="flex flex-col gap-0.5">{items.map((i) => <NavLink key={i.href} {...i} active={i.href === activeHref} />)}</nav>;
}
function RoleNavWithQuery(props: { items: NavItem[]; base: string; pathname: string }) {
  return <RoleNav {...props} search={useSearchParams().toString()} />; // the search params need a Suspense boundary (static routes)
}

/** The app frame. In API mode the layout passes `live` (IR241): the organization as the scope label, the signed-in user
 * in the chip, the sidebar badges counted by the Core API (no badge for 0) and the user's display language (IR258);
 * the demo keeps the fixed ones and the language chosen in this browser. */
export function AppShell({ role: forced, live, children }: { role?: Role; live?: ShellLive; children: React.ReactNode }) {
  const detected = useStoredRole();
  const role = forced ?? detected;
  const cfg = ROLES[role];
  const stored = useStoredValue(LOCALE_KEY);
  const locale = live ? live.locale : isLocale(stored) ? stored : "en";
  const t = (text: string) => translate(locale, text);
  const scope = live ? (live.organization || cfg.scope).toUpperCase() : cfg.scope;
  const [roleWord, persona] = cfg.chip.split(" — ");
  const chip = `${t(roleWord)} — ${live ? live.user || t("signed in") : persona}`;
  const count = (href: string) => (live?.badges[href] ? String(live.badges[href]) : undefined);
  const pathname = usePathname();
  const session = useBffSession();
  // Users is owner-only (FR-C19): the session's client role in API mode, the demo persona otherwise
  const owner = session?.dataSource === "api" ? session.clientRole === "owner" : CURRENT_CLIENT.role === "owner";
  const [open, setOpen] = useState(false);
  // close the mobile menu when the route changes: adjust state during render (React: "You might not need an effect")
  const [menuPath, setMenuPath] = useState(pathname);
  if (menuPath !== pathname) {
    setMenuPath(pathname);
    setOpen(false);
  }
  const [assistant, setAssistant] = useState(false);
  const [qr, setQr] = useState(false);
  const unread = useJobStore().notes.filter((n) => n.role === role && !n.read).length;

  const items = (live ? cfg.nav.map((i) => ({ ...i, badge: count(i.href) })) : cfg.nav).map((i) => ({ ...i, label: t(i.label) }));
  // longest-prefix match so /customer/units/x highlights "Units & locations"
  const score = (href: string, match?: string[]) => {
    const hits = [href, ...(match ?? [])].filter((h) => (h === cfg.base ? pathname === h : pathname === h || pathname.startsWith(h + "/")));
    return hits.length ? Math.max(...hits.map((h) => h.length)) : 0;
  };
  const best = Math.max(0, ...items.map((i) => score(i.href, i.match)));
  const activeHref = best ? items.find((i) => score(i.href, i.match) === best)?.href : undefined;
  const sharedActive = SHARED.find((s) => pathname === s.href);
  const title = sharedActive ? t(sharedActive.label) : pathname === "/customer/users" ? t("Users") : pathname === "/demo" ? t("Demo controls") : items.find((i) => i.href === activeHref)?.label ?? "AC Project";
  const demo = pathname === "/demo";


  return (
    <I18nProvider locale={locale}>
    <ToastProvider>
      <div className="min-h-dvh lg:grid lg:grid-cols-[240px_minmax(0,1fr)]">
        {open && <div className="fixed inset-0 z-30 bg-ink/40 lg:hidden" onClick={() => setOpen(false)} />}
        <aside className={cx("fixed inset-y-0 left-0 z-40 flex w-[260px] flex-col gap-1 overflow-y-auto border-r border-line bg-surface p-4 transition-transform lg:sticky lg:top-0 lg:h-dvh lg:w-auto lg:translate-x-0", open ? "translate-x-0" : "-translate-x-full")}>
          <Link href={cfg.base} className="mb-3 flex items-center gap-2.5">
            <span className="grid h-9 w-9 place-items-center rounded-xl bg-primary text-lg text-white">❄</span>
            <span className="leading-tight"><span className="block text-[15px] font-bold">{cfg.app}</span><span className="block text-[11px] text-muted">{t(cfg.sub)}</span></span>
          </Link>
          <div className="px-3 pb-1 text-[10px] font-bold tracking-wider text-muted">{scope}</div>
          <Suspense fallback={<RoleNav items={items} base={cfg.base} pathname={pathname} search="" />}><RoleNavWithQuery items={items} base={cfg.base} pathname={pathname} /></Suspense>
          <div className="my-2 border-t border-line" />
          <nav className="flex flex-col gap-0.5">
            {role === "client" && owner && <NavLink href="/customer/users" label={t("Users")} icon="☺" active={pathname === "/customer/users"} />}
            {SHARED.map((s) => <NavLink key={s.href} {...s} label={t(s.label)} active={pathname === s.href} badge={s.href !== "/notifications" ? undefined : live ? count(s.href) : String(3 + unread)} />)}
            <Link href="/demo" className={cx("flex items-center gap-2.5 rounded-control px-3 py-2 text-[13px] font-semibold text-warn hover:bg-warn-soft/50", pathname === "/demo" && "bg-warn-soft/60")}>
              <span aria-hidden className="w-4 text-center">✦</span>
              <span className="flex-1">{t("Demo controls")}</span>
              <Badge tone="warn">DEMO</Badge>
            </Link>
          </nav>
          <div className="mt-auto pt-4">
            {live ? (
              // API mode (FR-X01, IR250): sign-out ends the BFF session; the full navigation also clears the screen and its cache
              <form action="/bff/auth/logout" method="post"><button type="submit" className={signOut}><span aria-hidden className="w-4 text-center">↦</span>{t("Sign out")}</button></form>
            ) : <Link href="/login" className={signOut}><span aria-hidden className="w-4 text-center">↦</span>{t("Sign out")}</Link>}
          </div>
        </aside>

        <div className="flex min-w-0 flex-col">
          <header className={cx("sticky top-0 z-20 flex h-16 items-center justify-between gap-3 border-b px-4 backdrop-blur sm:px-6 lg:px-8", demo ? "border-[#fdba74] bg-warn-soft" : "border-line bg-surface/95")}>
            <div className="flex min-w-0 items-center gap-3">
              <button aria-label={t("Open menu")} onClick={() => setOpen(true)} className="grid h-9 w-9 place-items-center rounded-control border border-line lg:hidden">☰</button>
              {/* Demo controls are always labelled demo (FR-X05, Figma 10e) */}
              <h1 className={cx("flex min-w-0 items-center gap-2 text-[15px] font-bold", demo && "text-warn")}>{demo && <span aria-hidden>✦</span>}<span className="truncate">{title}</span>{demo && <Badge tone="warn" className="uppercase tracking-wide max-sm:hidden">{t("Always labelled demo")}</Badge>}</h1>
            </div>
            <div className="flex shrink-0 items-center gap-2">
              {role === "technician" && <button onClick={() => setQr(true)} className="inline-flex items-center gap-1.5 rounded-control border border-line px-3 py-1.5 text-xs font-bold hover:bg-surface2">▣ <span className="max-sm:hidden">{t("Scan QR")}</span></button>}
              {role === "client" && (
                <button onClick={() => setAssistant(true)} className="inline-flex items-center gap-1.5 rounded-full border border-primary bg-primary-soft/50 px-3 py-1.5 text-xs font-bold text-primary hover:bg-primary-soft">🎙 <span className="max-sm:hidden">{t("Assistant")}</span></button>
              )}
              <span className="inline-flex items-center gap-1.5 rounded-control bg-surface2 px-3 py-1.5 text-xs font-semibold">☻ <span className="max-sm:hidden">{chip}</span><span className="sm:hidden">{chip.split(" — ")[0]}</span></span>
            </div>
          </header>
          <main className="min-w-0 flex-1 px-4 py-5 sm:px-6 lg:px-8">{children}</main>
        </div>
      </div>
      {role === "technician" && session?.dataSource === "api" && <QrScan open={qr} onClose={() => setQr(false)} />}
      <Modal open={qr && session?.dataSource !== "api"} onClose={() => setQr(false)} title="Scan unit QR" footer={<><Btn onClick={() => setQr(false)}>Close</Btn><Link href="/technician/jobs/job-contractor-a" onClick={() => setQr(false)} className="inline-flex items-center rounded-control border border-primary bg-primary px-3.5 py-2 text-[13px] font-semibold text-white">Open job →</Link></>}>
        <div className="grid h-40 place-items-center rounded-xl bg-ink/90 text-xs text-white">[ camera preview ]</div>
        <SummaryList items={[["Result", <Badge key="m" tone="ok">✓ Matched</Badge>], ["Label", "AC-QR-online-rto · scanned 10:04"], ["Unit", "Bedroom AC · unit-online-rto · customer-a · Home A › 1F › Bedroom"], ["Your job today", "job-contractor-a"]]} />
        <Banner>Units outside your assignments show “Not in your assignments” (NOT_FOUND).</Banner>
      </Modal>
      {role === "client" && <AssistantPanel open={assistant} onClose={() => setAssistant(false)} context={title} />}
    </ToastProvider>
    </I18nProvider>
  );
}
