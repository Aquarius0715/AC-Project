// Sign-in of a virtual user: the password grant of the load client (HQ: of the HQ app's client, whose tokens alone carry
// HQ's memberships), then the refresh grant before the 5-minute access token ends. The token of a user lives in the
// virtual user that holds it (each has its own JavaScript runtime), so a VU signs in once and refreshes about every 4
// minutes, as a browser session does.
import http from "k6/http";
import { env } from "./env.ts";
import { record } from "./metrics.ts";
import type { User } from "./manifest.ts";

interface Token { accessToken: string; refreshToken: string; expiresAtMs: number }

const REFRESH_BEFORE_MS = 30_000;
let held: Token | null = null;

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function tokenOf(body: unknown): Token | null {
  const { access_token: access, refresh_token: refresh, expires_in: expires } = isRecord(body) ? body : {};
  return typeof access === "string" && typeof refresh === "string" && typeof expires === "number"
    ? { accessToken: access, refreshToken: refresh, expiresAtMs: Date.now() + expires * 1000 }
    : null;
}

function request(form: Record<string, string>, name: string): Token | null {
  const res = http.post(env.tokenUrl, form, { tags: { name, kind: "auth" }, responseType: "text" });
  const token = res.status === 200 ? tokenOf(JSON.parse(res.body)) : null;
  record("auth", name, res.status, res.timings.duration, token !== null);
  return token;
}

/** A valid access token of the user; null when the identity provider refuses (counted as an error). */
export function accessTokenOf(user: User): string | null {
  if (held && held.expiresAtMs - Date.now() > REFRESH_BEFORE_MS) return held.accessToken;
  const hq = user.group === "hq";
  const client = hq
    ? { client_id: env.adminClientId, client_secret: env.adminClientSecret }
    : { client_id: env.clientId, client_secret: env.clientSecret };
  const refreshed = held ? request({ grant_type: "refresh_token", refresh_token: held.refreshToken, ...client }, "token refresh") : null;
  held = refreshed ?? request({ grant_type: "password", username: user.username, password: user.password, ...client }, "token password");
  return held?.accessToken ?? null;
}
