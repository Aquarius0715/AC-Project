import { describe, expect, it } from "vitest";
import { uploadType } from "@ac/web/lib/files";

const bytes = (...b: (number | string)[]) => Uint8Array.from(b.flatMap((x) => (typeof x === "string" ? [...x].map((c) => c.charCodeAt(0)) : [x])));

describe("the type of an upload is the type of its bytes (IR308)", () => {
  it("names PNG, JPEG and PDF by their leading bytes, whatever the file name says", () => {
    expect([
      uploadType(bytes(0x89, "PNG\r\n", 0x1a, "\n", "rest"), "image/jpeg"),
      uploadType(bytes(0xff, 0xd8, 0xff, 0xe0, "rest"), "image/png"), // a JPEG photo renamed to .png
      uploadType(bytes("%PDF-1.7\n"), "application/octet-stream"),
    ]).toEqual(["image/png", "image/jpeg", "application/pdf"]);
  });

  it("keeps the browser's type for anything else, which the Core API then refuses", () => {
    expect([
      uploadType(bytes("<svg onload=alert(1)>"), "image/png"),
      uploadType(bytes(0x89, "PNG"), "image/png"), // only part of the PNG signature
      uploadType(bytes("GIF89a"), "image/gif"),
      uploadType(new Uint8Array(), "image/heic"),
    ]).toEqual(["image/png", "image/png", "image/gif", "image/heic"]);
  });
});
