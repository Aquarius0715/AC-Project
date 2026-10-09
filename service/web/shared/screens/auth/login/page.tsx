import Link from "next/link";
import { Badge, Banner } from "@ac/web/components/ui";
import { ROLES, type Role } from "@ac/web/lib/nav";
import { continueHref, loginError, otherServices, safeReturnTo } from "@ac/web/lib/login";
import { ContinueLink } from "./continue-link";

export type AppLinks = Partial<Record<Role, string>>;
type Params = Record<string, string | string[] | undefined>;
const one = (v: string | string[] | undefined) => (Array.isArray(v) ? v[0] : v);

/** Sign-in of one role's app (FR-X01, DDC-07, SCR-X-login, Figma Login frames), rendered on the server (IR251): the data
 * source and the kept returnTo decide where Continue goes, and a sign-in that came back says why. `others` are the
 * base URLs of the other apps (plain links: separate apps). */
export function LoginScreen({ role, others = {}, params = {}, api }: { role: Role; others?: AppLinks; params?: Params; api: boolean }) {
  const cfg = ROLES[role];
  const href = continueHref(api, role, safeReturnTo(role, one(params.returnTo)));
  const error = loginError(one(params.error));
  const names = otherServices(role);
  return (
    <main className="relative grid min-h-dvh place-items-center px-4 py-10">
      <span className="absolute right-4 top-4 inline-flex items-center gap-1.5 rounded-control border border-line bg-surface px-3 py-1.5 text-xs font-semibold" aria-label="Language: English, the only language in this build">🌐 English</span>
      <div className="w-full max-w-[350px] rounded-2xl border border-line bg-surface p-6 text-center shadow-sm">
        <span aria-hidden className="mx-auto mb-3 grid h-10 w-10 place-items-center rounded-xl bg-primary text-xl text-white">❄</span>
        <h1 className="text-lg font-bold">AC Project — {cfg.loginTitle}</h1>
        <p className="mt-1 text-[13px] text-muted">{cfg.loginSub}</p>
        <div className="mt-2"><Badge className="uppercase tracking-wide">Not real authentication</Badge></div>
        {error && <div className="mt-4 text-left"><Banner tone="crit">{error}</Banner></div>}
        <div className="mt-4 flex items-center gap-3 rounded-xl bg-primary-soft/70 p-3 text-left">
          <span aria-hidden className="grid h-8 w-8 shrink-0 place-items-center rounded-lg bg-primary text-sm text-white">☻</span>
          <span className="min-w-0"><span className="block text-[13px] font-bold">{cfg.loginUser}</span><span className="block text-[11px] text-muted">{cfg.loginDesc}</span></span>
        </div>
        <ContinueLink role={role} href={href}>Continue as {cfg.loginUser} →</ContinueLink>
        <p className="mt-4 text-[11px] text-muted">Looking for a different AC Project service?{" "}
          {names.map((r, i) => (
            <span key={r}>{i > 0 && (i === names.length - 1 ? ", and " : ", ")}{others[r] ? <a className="font-semibold text-primary hover:underline" href={`${others[r]}/login`}>{ROLES[r].loginTitle}</a> : ROLES[r].loginTitle}</span>
          ))}{" "}each have their own sign-in.
        </p>
        <Link href="/forgot-password" className="mt-3 inline-block text-xs font-semibold text-primary hover:underline">Forgot password?</Link>
        <p className="mt-3 text-[11px] text-muted">Demo authentication only — no real accounts, emails, or passwords are used.</p>
      </div>
    </main>
  );
}
