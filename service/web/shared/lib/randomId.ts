// A random UUID (version 4) in every page. crypto.randomUUID exists only in a secure context (HTTPS or localhost), and
// the shared test environment serves the apps over plain HTTP on the lab network (DEC-93); getRandomValues works in
// every context.

/** A random UUID v4: crypto.randomUUID where the page is a secure context, else one made from getRandomValues. */
export function randomId(): string {
  if (globalThis.isSecureContext) return crypto.randomUUID();
  const hex = Array.from(crypto.getRandomValues(new Uint8Array(16)), (b) => b.toString(16).padStart(2, "0")).join("");
  const variant = "89ab".charAt(Number.parseInt(hex.charAt(16), 16) % 4); // the RFC 9562 variant: 10xx
  return `${hex.slice(0, 8)}-${hex.slice(8, 12)}-4${hex.slice(13, 16)}-${variant}${hex.slice(17, 20)}-${hex.slice(20)}`;
}
