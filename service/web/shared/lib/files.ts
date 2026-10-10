// Uploads (IR308): the Core API stores and serves a file only as what its bytes are — the PNG signature, the JPEG
// start-of-image marker or the PDF header. The browser's File.type follows the file name, so the Server Actions send
// the type of the bytes instead; a file that is none of these keeps the browser's type and is refused by the API.

const signatures: [string, number[]][] = [
  ["image/png", [0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a]],
  ["image/jpeg", [0xff, 0xd8, 0xff]],
  ["application/pdf", [0x25, 0x50, 0x44, 0x46, 0x2d]], // "%PDF-"
];

/** The type of an upload from its leading bytes, or the browser's type when the bytes are no known type. */
export function uploadType(bytes: Uint8Array, browserType: string): string {
  return signatures.find(([, sig]) => sig.every((b, i) => bytes[i] === b))?.[0] ?? browserType;
}
