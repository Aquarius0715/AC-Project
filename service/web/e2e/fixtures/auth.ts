// Signing in through the UI as a person would: the app's sign-in page (the e-mail address) → the identity provider (the
// address prefilled by the login hint) → password → for HQ a one-time code → back in the role home. The credentials are the local demo realm's
// test values, read from its import file (docker/keycloak/realm-ac.json) so they live in one place; E2E_PASSWORD and
// E2E_TOTP_SECRET replace them for another identity provider.
import { readFileSync } from "node:fs";
import path from "node:path";
import { createHmac } from "node:crypto";
import type { Page } from "@playwright/test";
import type { App } from "./apps";
import { must } from "./must";
import { z } from "zod";

const credentialSchema = z.object({ type: z.string(), value: z.string().optional(), secretData: z.string().optional() });
const realmSchema = z.object({ users: z.array(z.object({ username: z.string(), credentials: z.array(credentialSchema).optional() })).optional() });
type Credential = z.infer<typeof credentialSchema>;
const REALM = path.resolve(__dirname, "../../../../docker/keycloak/realm-ac.json");

function credential(user: string, type: string): Credential | undefined {
  const realm = realmSchema.parse(JSON.parse(readFileSync(REALM, "utf8")));
  return realm.users?.find((u) => u.username === user)?.credentials?.find((c) => c.type === type);
}

function passwordOf(user: string): string {
  const value = process.env.E2E_PASSWORD ?? credential(user, "password")?.value;
  if (!value) throw new Error(`No test password for ${user}: set E2E_PASSWORD`);
  return value;
}

function otpSecretOf(user: string): string {
  const raw = credential(user, "otp")?.secretData;
  const value = process.env.E2E_TOTP_SECRET ?? (raw ? z.object({ value: z.string().optional() }).parse(JSON.parse(raw)).value : undefined);
  if (!value) throw new Error(`No one-time-code secret for ${user}: set E2E_TOTP_SECRET`);
  return value;
}

/** RFC 6238 code (HMAC-SHA1, 30 s, 6 digits) over the identity provider's raw secret. */
function totp(secret: string, at = Date.now()): string {
  const counter = Buffer.alloc(8);
  counter.writeBigUInt64BE(BigInt(Math.floor(at / 30_000)));
  const h = createHmac("sha1", Buffer.from(secret, "utf8")).update(counter).digest();
  const o = must(h.at(-1)) & 15;
  return String((h.readUInt32BE(o) & 0x7F_FF_FF_FF) % 1_000_000).padStart(6, "0");
}

/** RFC 6238 code over a base32 key, as the identity provider shows it when a device is set up (spaces ignored). */
export function totpBase32(key: string, at = Date.now()): string {
  const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZ234567";
  let bits = "";
  for (const ch of key.replaceAll(/[\s=]/g, "").toUpperCase()) bits += alphabet.indexOf(ch).toString(2).padStart(5, "0");
  const bytes = Buffer.from((bits.match(/.{8}/g) ?? []).map((b) => Number.parseInt(b, 2)));
  const counter = Buffer.alloc(8);
  counter.writeBigUInt64BE(BigInt(Math.floor(at / 30_000)));
  const h = createHmac("sha1", bytes).update(counter).digest();
  const o = must(h.at(-1)) & 15;
  return String((h.readUInt32BE(o) & 0x7F_FF_FF_FF) % 1_000_000).padStart(6, "0");
}

/** `ok` when the wait ends in time, `timedOut` when it fails (a Playwright wait rejects on its timeout). */
export async function settles<T>(wait: Promise<unknown>, ok: T, timedOut: T): Promise<T> {
  try {
    await wait;
    return ok;
  } catch {
    return timedOut;
  }
}
const home = (app: App) => (u: URL) => u.origin === new URL(app.url).origin && (u.pathname === app.home || u.pathname.startsWith(app.home + "/"));

/** The e-mail address of a demo account: its user name at the realm's domain. */
export const emailOf = (user: string): string => (user.includes("@") ? user : `${user}@ac.local`);

/** The app's own sign-in step (IR646): the e-mail address, then Continue to the identity provider, which shows it already filled in. */
export async function startSignIn(page: Page, email: string): Promise<void> {
  const form = page.locator('form[action="/bff/auth/login"]'); // the same in every display language
  await form.locator('input[name="login_hint"]').fill(email);
  await form.getByRole("button").click();
}

/** The identity provider asks for the password again (prompt=login, after a revocation, IR385): enter it for `user`.
The account may already be shown; the credentials stay in this file's realm reader. */
export async function enterPassword(page: Page, user: string): Promise<void> {
  const name = page.locator("#username");
  await page.locator("#password").waitFor(); // the form is drawn: isVisible does not wait
  if (await name.isVisible()) await name.fill(user);
  await page.locator("#password").fill(passwordOf(user));
  await page.locator("#kc-login").click();
}

export async function signIn(page: Page, app: App, as = app.user): Promise<void> {
  await page.goto(`${app.url}/login`);
  await startSignIn(page, emailOf(as));
  await page.locator("#password").fill(passwordOf(as));
  await page.locator("#kc-login").click();
  if (app.otp) {
    // a code is accepted once per 30 s window: if the last run used this window's code, the identity provider refuses it
    // and the next window's is taken. The app may take a while to finish the sign-in (a container just started), so its
    // page or the refusal is waited for, not a fixed time: 8 s once ran out on a slow first page, and the retry then
    // waited for a code field that never came.
    const refused = page.locator("#input-error-otp, #input-error-otp-code, #input-error-totp").or(page.getByText(/invalid authenticator code/i)).first();
    const atHome = home(app);
    for (let attempt = 0; attempt < 2; attempt++) {
      await page.locator("#otp").fill(totp(otpSecretOf(as)));
      await page.locator("#kc-login").click();
      const outcome = await Promise.race([
        settles(page.waitForURL(atHome, { timeout: 60_000 }), "home", "slow"),
        settles(refused.waitFor({ timeout: 60_000 }), "refused", "slow"),
      ]);
      if (outcome !== "refused") break;
      await page.waitForTimeout(31_000 - (Date.now() % 30_000));
    }
  }
  await page.waitForURL(home(app));
}
