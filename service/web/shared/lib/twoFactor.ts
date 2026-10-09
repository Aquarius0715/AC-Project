// Two-step verification helpers (FR-X08, IR144, IR247; Figma Client 10g): the authenticator setup QR built from the
// setup key in the key URI format of authenticator apps, the key in groups of four, and the 6-digit code as typed.
import { encode } from "uqr";

export const ISSUER = "AC Project";

/** otpauth://totp/AC%20Project:<account>?secret=<key>&issuer=AC%20Project&algorithm=SHA1&digits=6&period=30 */
export function otpauthUri(setupKey: string, account: string): string {
  const label = encodeURIComponent(`${ISSUER}:${account}`).replace("%3A", ":");
  return `otpauth://totp/${label}?secret=${setupKey.replace(/\s/g, "").toUpperCase()}&issuer=${encodeURIComponent(ISSUER)}&algorithm=SHA1&digits=6&period=30`;
}

/** "JBSW Y3DP EHPK 3PXP": the key as people type it into an authenticator app. */
export const groupKey = (key: string) => key.replace(/\s/g, "").match(/.{1,4}/g)?.join(" ") ?? "";

export type QrPath = { size: number; path: string };
/** The QR code of `text` (error correction M, a 2-module border; the white frame around it adds the rest of the quiet
 * zone) as one SVG path in module units: one rectangle per run of dark modules in a row. */
export function qrPath(text: string): QrPath {
  const { size, data } = encode(text, { ecc: "M", border: 2 });
  let path = "";
  data.forEach((row, y) => {
    for (let x = 0; x < size; x++) {
      if (!row[x]) continue;
      let w = 1;
      while (x + w < size && row[x + w]) w++;
      path += `M${x} ${y}h${w}v1h-${w}z`;
      x += w - 1;
    }
  });
  return { size, path };
}

/** The code as typed: digits only, at most six. */
export const codeDigits = (v: string) => v.replace(/\D/g, "").slice(0, 6);
export const codeComplete = (v: string) => /^\d{6}$/.test(v);
