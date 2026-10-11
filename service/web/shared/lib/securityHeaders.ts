// Security headers of the web apps (network architecture §6, IR354). The production edge adds them too; every app sends
// them itself, so local, staging and production responses match. The page Content-Security-Policy follows the Next.js
// CSP guide: proxy.ts makes a fresh nonce per page request, and Next.js puts it on its own scripts and styles. React
// style attributes stay allowed (style-src-attr), and forms may post to the app and the identity provider only.
// Pure code, unit-tested.

/** The Content-Security-Policy of one page response. `dev` adds what `next dev` needs (eval, inline styles); `https`
 * upgrades subresource requests once the app is served over HTTPS. */
export function contentSecurityPolicy(nonce: string, o: { dev: boolean; https: boolean; formActions: string[] }): string {
  return [
    "default-src 'self'",
    `script-src 'self' 'nonce-${nonce}' 'strict-dynamic'${o.dev ? " 'unsafe-eval'" : ""}`,
    `style-src 'self' ${o.dev ? "'unsafe-inline'" : `'nonce-${nonce}'`}`,
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
