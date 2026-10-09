// Signing in through the UI as a person would: the app's sign-in page → the identity provider (account prefilled by the
// login hint) → password → for HQ a one-time code → back in the role home. The credentials are the local demo realm's
// test values, read from its import file (docker/keycloak/realm-ac.json) so they live in one place; E2E_PASSWORD and
// E2E_TOTP_SECRET replace them for another identity provider.
import { readFileSync } from "node:fs";
import path from "node:path";
import { createHmac } from "node:crypto";
import type { Page } from "@playwright/test";
import type { App } from "./apps";

type Credential = { type: string; value?: string; secretData?: string };
const REALM = path.resolve(__dirname, "../../../../docker/keycloak/realm-ac.json");

function credential(user: string, type: string): Credential | undefined {
  const realm = JSON.parse(readFileSync(REALM, "utf8")) as { users?: { username: string; credentials?: Credential[] }[] };
  return realm.users?.find((u) => u.username === user)?.credentials?.find((c) => c.type === type);
}

function passwordOf(user: string): string {
  const value = process.env.E2E_PASSWORD ?? credential(user, "password")?.value;
  if (!value) throw new Error(`No test password for ${user}: set E2E_PASSWORD`);
  return value;
}

function otpSecretOf(user: string): string {
  const raw = credential(user, "otp")?.secretData;
  const value = process.env.E2E_TOTP_SECRET ?? (raw ? (JSON.parse(raw) as { value?: string }).value : undefined);
  if (!value) throw new Error(`No one-time-code secret for ${user}: set E2E_TOTP_SECRET`);
  return value;
}

/** RFC 6238 code (HMAC-SHA1, 30 s, 6 digits) over the identity provider's raw secret. */
export function totp(secret: string, at = Date.now()): string {
  const counter = Buffer.alloc(8);
  counter.writeBigUInt64BE(BigInt(Math.floor(at / 30_000)));
  const h = createHmac("sha1", Buffer.from(secret, "utf8")).update(counter).digest();
  const o = h[h.length - 1] & 15;
  return String((h.readUInt32BE(o) & 0x7fffffff) % 1_000_000).padStart(6, "0");
}

const home = (app: App) => (u: URL) => u.origin === new URL(app.url).origin && (u.pathname === app.home || u.pathname.startsWith(app.home + "/"));

export async function signIn(page: Page, app: App): Promise<void> {
  await page.goto(`${app.url}/login`);
  await page.getByRole("link", { name: `Continue as ${app.user} →` }).click();
  await page.locator("#password").fill(passwordOf(app.user));
  await page.locator("#kc-login").click();
  if (app.otp) {
    // a code is accepted once per 30 s window: if the last run used this window's code, the next window's is taken
    for (let attempt = 0; attempt < 2; attempt++) {
      await page.locator("#otp").fill(totp(otpSecretOf(app.user)));
      await page.locator("#kc-login").click();
      if (await page.waitForURL(home(app), { timeout: 8_000 }).then(() => true, () => false)) return;
      await page.waitForTimeout(31_000 - (Date.now() % 30_000));
    }
  }
  await page.waitForURL(home(app));
}
