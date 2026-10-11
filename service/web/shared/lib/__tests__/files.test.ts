import { describe, expect, it } from "vitest";
import { contentDisposition, uploadName, uploadType } from "@ac/web/lib/files";

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

describe("file names (IR368)", () => {
  it("cleans a browser file name to what the API accepts", () => {
    expect([uploadName("photo.jpg"), uploadName("写真 1.jpg"), uploadName("  padded.png  "), uploadName("a\r\nb.png"), uploadName("photo\u202egpj.exe"),
      uploadName("dir/sub\\x.png"), uploadName("\u0000\u200e"), uploadName("")]).toEqual(["photo.jpg", "写真 1.jpg", "padded.png", "ab.png", "photogpj.exe", "dir_sub_x.png", "upload", "upload"]);
    const long = uploadName("x".repeat(300) + ".jpeg");
    expect([Array.from(long).length, long.endsWith(".jpeg")]).toEqual([200, true]);
    expect(Array.from(uploadName("写".repeat(250))).length).toBe(200);
  });

  it("names a stored file in a header any name fits: an ASCII fallback and the UTF-8 name", () => {
    const value = contentDisposition("写真 (1).jpg");
    expect(value).toBe(`inline; filename="__ (1).jpg"; filename*=UTF-8''%E5%86%99%E7%9C%9F%20%281%29.jpg`);
    // a header value takes no character above U+00FF: the name itself would make the response throw
    expect(() => new Headers({ "Content-Disposition": `inline; filename="写真.jpg"` })).toThrow();
    expect(new Headers({ "Content-Disposition": value }).get("Content-Disposition")).toBe(value);
    expect(contentDisposition('a"b\\c\r\nSet-Cookie: x.png', "attachment")).toBe(`attachment; filename="abcSet-Cookie: x.png"; filename*=UTF-8''a%22b%5CcSet-Cookie%3A%20x.png`);
    expect(contentDisposition("\u202e")).toBe(`inline; filename="file"; filename*=UTF-8''file`);
  });
});
