// Uploads (IR308): the Core API stores and serves a file only as what its bytes are — the PNG signature, the JPEG
// start-of-image marker or the PDF header. The browser's File.type follows the file name, so the Server Actions send
// the type of the bytes instead; a file that is none of these keeps the browser's type and is refused by the API.

const signatures: [string, number[]][] = [
  ["image/png", [0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A]],
  ["image/jpeg", [0xFF, 0xD8, 0xFF]],
  ["application/pdf", [0x25, 0x50, 0x44, 0x46, 0x2D]], // "%PDF-"
];

/** The type of an upload from its leading bytes, or the browser's type when the bytes are no known type. */
export function uploadType(bytes: Uint8Array, browserType: string): string {
  return signatures.find(([, sig]) => sig.every((b, i) => bytes[i] === b))?.[0] ?? browserType;
}

// File names (IR368). The Core API refuses a name with control characters, path separators or bidirectional
// formatting characters. The browser's File.name can hold them, so the Server Actions clean it first; the name is only a
// label, and the bytes and their type are what count.
const UNSAFE_NAME = /[\u0000-\u001F\u007F-\u009F\u061C\u200E\u200F\u202A-\u202E\u2066-\u2069]/g;

/** An upload's name as the API accepts it: unsafe characters dropped, "/" and "\" made "_", at most 200 characters
(the extension kept), "upload" when nothing is left. */
export function uploadName(name: string): string {
  const clean = name.replaceAll(UNSAFE_NAME, "").replaceAll(/[/\\]/g, "_").trim();
  const chars = Array.from(clean);
  if (chars.length === 0) return "upload";
  if (chars.length <= 200) return clean;
  const dot = clean.lastIndexOf(".");
  const suffix = dot > 0 && clean.length - dot <= 10 ? clean.slice(dot) : "";
  return Array.from(clean.slice(0, clean.length - suffix.length)).slice(0, 200 - Array.from(suffix).length).join("").trimEnd() + suffix;
}

// the HTTP status of a file the Core API refused: forbidden, not found, or any other failure as a bad gateway
const REFUSAL_STATUS = new Map([["FORBIDDEN", 403], ["NOT_FOUND", 404]]);
export const refusalStatus = (code: string): number => REFUSAL_STATUS.get(code) ?? 502;

/** A Content-Disposition value for a stored file's name (RFC 6266): a plain-ASCII fallback in filename and the full
name in filename* (RFC 5987, UTF-8). Header values carry no characters above U+00FF, so a name such as "写真.jpg"
written into filename as it is makes the response throw (IR368). */
export function contentDisposition(name: string, disposition: "inline" | "attachment" = "inline"): string {
  const clean = name.replaceAll(UNSAFE_NAME, "").trim() || "file";
  const ascii = clean.replaceAll(/[^\x20-\x7E]/g, "_").replaceAll(/["\\]/g, "");
  const encoded = encodeURIComponent(clean).replaceAll(/['()*]/g, (c) => `%${c.charCodeAt(0).toString(16).toUpperCase()}`);
  return `${disposition}; filename="${ascii}"; filename*=UTF-8''${encoded}`;
}
