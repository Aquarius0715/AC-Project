// Security headers of the web apps (network architecture §6, IR354). The production edge adds them too; every app sends
// them itself, so local, staging and production responses match. The page Content-Security-Policy follows the Next.js
// CSP guide: proxy.ts makes a fresh nonce per page request, and Next.js puts it on its own scripts and styles. React
// style attributes stay allowed (style-src-attr), and forms may post to the app and the identity provider only.
// Pure code, unit-tested. A <style> a library adds at run time without the nonce is allowed by its hash only
// (styleHashes.gen.ts: Sonner's stylesheet, DEC-75).
import { STYLE_HASHES } from "./styleHashes.gen";

/** The Content-Security-Policy of one page response. `dev` adds what `next dev` needs (eval, inline styles); `https`
 * upgrades subresource requests once the app is served over HTTPS. */
export function contentSecurityPolicy(nonce: string, o: { dev: boolean; https: boolean; formActions: string[] }): string {
  return [
    "default-src 'self'",
    `script-src 'self' 'nonce-${nonce}' 'strict-dynamic'${o.dev ? " 'unsafe-eval'" : ""}`,
    `style-src 'self' ${o.dev ? "'unsafe-inline'" : [`'nonce-${nonce}'`, ...STYLE_HASHES].join(" ")}`,
    "style-src-attr 'unsafe-inline'",
    "img-src 'self' blob: data:",
    "font-src 'self'",
    "connect-src 'self'",
    "object-src 'none'",
    "base-uri 'self'",
    ["form-action 'self'", ...o.formActions].join(" "),
    "frame-ancestors 'none'",
    ...(o.https ? ["upgrade-insecure-requests"] : []),
  ].join("; ");
}

/** A fresh nonce: a random UUID in base64, as in the Next.js guide. */
export const newNonce = (): string => Buffer.from(crypto.randomUUID()).toString("base64");

/** The origin of an absolute URL, or null. */
export function originOf(url: string): string | null {
  try {
    return new URL(url).origin;
  } catch {
    return null;
  }
}

/** Strict-Transport-Security for a request that reached the app over HTTPS (directly or through the edge). */
export const HSTS = "max-age=63072000; includeSubDomains";

/** Whether a browser request comes from this app's own pages (CSRF protection of the BFF's POST routes, backend
 * architecture §6, IR358), as Next.js checks Server Actions: a `Sec-Fetch-Site` other than same-origin, or an `Origin`
 * whose host is not the request's host (`X-Forwarded-Host` behind the edge), is refused. A request with neither header
 * does not come from a browser page, so it carries no victim's cookie and passes. */
export function sameOriginRequest(h: Headers): boolean {
  const site = h.get("sec-fetch-site");
  if (site && site !== "same-origin" && site !== "none") return false;
  const origin = h.get("origin");
  if (!origin) return true;
  const host = h.get("x-forwarded-host") ?? h.get("host");
  try {
    return new URL(origin).host === host;
  } catch {
    return false;
  }
}
