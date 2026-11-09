// One call of the Core API through the gateway, as the web apps' BFF makes it (shared/lib/dal.ts): the bearer token, the
// tenant and membership headers, and for a write an idempotency key. The answer must be a 2xx with the {data} envelope;
// anything else is counted as an error, whatever its speed.
import http from "k6/http";
import { randomBytes } from "k6/crypto";
import { env } from "./env.ts";
import { accessTokenOf } from "./auth.ts";
import { record, type Kind } from "./metrics.ts";
import type { User } from "./manifest.ts";

export interface Call {
  name: string;
  method: "GET" | "POST" | "PUT";
  path: string;
  body?: unknown;
}

/** The answer: `data` is the envelope's data, null when the call failed or the body had none. */
export interface Answer { ok: boolean; status: number; data: unknown }

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

/** A random UUID (version 4) for an Idempotency-Key. */
function uuid(): string {
  const hex = Array.from(new Uint8Array(randomBytes(16)), (b) => b.toString(16).padStart(2, "0")).join("");
  const variant = ((Number.parseInt(hex.slice(16, 18), 16) & 0x3F) | 0x80).toString(16);
  return `${hex.slice(0, 8)}-${hex.slice(8, 12)}-4${hex.slice(13, 16)}-${variant}${hex.slice(18, 20)}-${hex.slice(20, 32)}`;
}

function dataOf(text: string): unknown {
  try {
    const body: unknown = JSON.parse(text);
    return isRecord(body) ? (body.data ?? null) : null;
  } catch {
    return null;
  }
}

/** How many failed calls a virtual user writes to the log: enough to see what fails, not a flood. */
const MAX_LOGGED = 5;
let failures = 0;

/** Makes the call as the user and records it; `data` is the envelope's data, or undefined when there is none. */
export function call(user: User, c: Call): Answer {
  const kind: Kind = c.method === "GET" ? "read" : "write";
  const access = accessTokenOf(user);
  if (access === null) return { data: null, ok: false, status: 0 };
  const headers: Record<string, string> = {
    Authorization: `Bearer ${access}`,
    "X-Tenant-Id": user.tenantId,
    "X-Membership-Id": user.membershipId,
  };
  if (c.body !== undefined) headers["Content-Type"] = "application/json";
  if (kind === "write") headers["Idempotency-Key"] = uuid();
  const res = http.request(c.method, `${env.gatewayUrl}${c.path}`, c.body === undefined ? null : JSON.stringify(c.body), {
    headers,
    tags: { name: c.name, kind },
    responseType: "text",
  });
  const data = res.status >= 200 && res.status < 300 ? dataOf(res.body) : null;
  const ok = data !== null;
  record(kind, c.name, res.status, res.timings.duration, ok);
  if (!ok && failures < MAX_LOGGED) {
    failures += 1;
    console.error(`failed as ${user.group}: ${c.method} ${c.path} -> ${String(res.status)} ${res.body.slice(0, 200)}`);
  }
  return { data, ok, status: res.status };
}
