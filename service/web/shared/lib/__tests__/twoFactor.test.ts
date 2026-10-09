import { describe, expect, it } from "vitest";
import jsQR from "jsqr";
import { codeComplete, codeDigits, groupKey, otpauthUri, qrPath, type QrPath } from "@ac/web/lib/twoFactor";

/** Paints the path's rectangles at `scale` pixels per module, with `pad` more white modules around, as RGBA. */
function raster(qr: QrPath, scale = 4, pad = 2) {
  const n = (qr.size + 2 * pad) * scale;
  const px = new Uint8ClampedArray(n * n * 4).fill(255);
  for (const m of qr.path.matchAll(/M(\d+) (\d+)h(\d+)v1h-\d+z/g)) {
    const [x, y, w] = [Number(m[1]), Number(m[2]), Number(m[3])];
    for (let yy = (y + pad) * scale; yy < (y + pad + 1) * scale; yy++)
      for (let xx = (x + pad) * scale; xx < (x + pad + w) * scale; xx++) px.fill(0, (yy * n + xx) * 4, (yy * n + xx) * 4 + 3);
  }
  return { px, n };
}

describe("Two-step verification setup (FR-X08, IR247)", () => {
  it("builds the authenticator key URI and groups the key", () => {
    expect(otpauthUri("6wug qkfw hdvs xim5", "customer-a")).toBe("otpauth://totp/AC%20Project:customer-a?secret=6WUGQKFWHDVSXIM5&issuer=AC%20Project&algorithm=SHA1&digits=6&period=30");
    expect(otpauthUri("JBSWY3DPEHPK3PXP", "Demo User:1")).toMatch(/^otpauth:\/\/totp\/AC%20Project:Demo%20User%3A1\?secret=JBSWY3DPEHPK3PXP&/);
    expect([groupKey("JBSWY3DPEHPK3PXP"), groupKey("ABCDEF"), groupKey("")]).toEqual(["JBSW Y3DP EHPK 3PXP", "ABCD EF", ""]);
  });

  it("draws a QR code that decodes back to the URI", () => {
    const uri = otpauthUri("6WUGQKFWHDVSXIM5", "customer-a");
    const qr = qrPath(uri);
    expect((qr.size - 4 - 17) % 4).toBe(0); // 4 × version + 17 modules plus the 2-module border on each side
    const { px, n } = raster(qr);
    expect(jsQR(px, n, n)?.data).toBe(uri);
  });

  it("keeps the code to six digits", () => {
    expect([codeDigits("12a3 4-56789"), codeDigits("")]).toEqual(["123456", ""]);
    expect([codeComplete("123456"), codeComplete("12345"), codeComplete("12345a")]).toEqual([true, false, false]);
  });
});
