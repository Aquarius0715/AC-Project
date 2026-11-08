// The addresses a browser reaches the apps and the identity provider by (IR651). A deployment has one canonical address
// (APP_URL, OIDC_ISSUER), and PUBLIC_HOSTS lists every host name or IP the same servers answer on (for example the lab
// network's address and the Tailnet's). A request that came by a listed host gets that host in the sign-in redirects,
// the callback's address and the identity provider's address; any other Host header, which anyone can send, gets the
// canonical one, so no link, redirect or Content-Security-Policy ever carries a host the deployment did not name.
// Unset, every address is the canonical one: nothing differs from before.
import { z } from "zod";

const hostList = z.string().transform((s) => s.split(",").map((h) => h.trim().toLowerCase()).filter(Boolean));

/** The host names and IPs in PUBLIC_HOSTS (lower case, no ports), or none. */
export function publicHosts(env: Readonly<Record<string, string | undefined>> = process.env): string[] {
  return hostList.parse(env.PUBLIC_HOSTS ?? "");
}

/** The host name of a Host header: no port, lower case; null when it is not a plain host name or IP (IPv6 is not served). */
export function hostnameOf(header: string | null | undefined): string | null {
  const match = /^([a-z0-9]([a-z0-9.-]*[a-z0-9])?)(:\d{1,5})?$/i.exec(header?.trim() ?? "");
  return match?.[1]?.toLowerCase() ?? null;
}

/** The headers that tell Keycloak, on a call by its internal address, which public address a token came from: its issuer
(`iss`), when that is a listed host. Keycloak checks that a refresh token's issuer is the realm's address *as the request
names it*, and a sign-in on a listed address carries that address; so a refresh or a logout of such a sign-in names the same
one. Nothing for a token of the canonical or an unlisted address, or when no host is listed. The token is the server's
own, read from the stored session, never from the request. */
export function issuerHeaders(token: string, env: Readonly<Record<string, string | undefined>> = process.env): Record<string, string> {
  if (publicHosts(env).length === 0) return {};
  try {
    const payload: unknown = JSON.parse(Buffer.from(token.split(".", 2)[1] ?? "", "base64url").toString("utf8"));
    const iss = z.object({ iss: z.string() }).parse(payload).iss;
    const url = new URL(iss);
    return publicHosts(env).includes(url.hostname.toLowerCase()) ? { "X-Forwarded-Host": url.host, "X-Forwarded-Proto": url.protocol.replace(/:$/, "") } : {};
  } catch {
    return {};
  }
}

/** `base` (an absolute URL) with its host name replaced by `host`, keeping its scheme, port and path. */
export function withHost(base: string, host: string): string {
  const url = new URL(base);
  url.hostname = host;
  return url.href.replace(/\/$/, "");
}

/** The address of `base` for a request that came by `hostHeader`: the listed host it named, else `base` itself. */
export function addressFor(base: string, hostHeader: string | null | undefined, env: Readonly<Record<string, string | undefined>> = process.env): string {
  const host = hostnameOf(hostHeader);
  const listed = host !== null && new URL(base).hostname.toLowerCase() !== host && publicHosts(env).includes(host);
  return listed ? withHost(base, host) : base.replace(/\/$/, "");
}
