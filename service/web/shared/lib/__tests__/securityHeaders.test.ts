// @vitest-environment node
import { afterEach, describe, expect, it, vi } from "vitest";
import { NextRequest } from "next/server";
import { contentSecurityPolicy, HSTS, newNonce, originOf } from "@ac/web/lib/securityHeaders";
import { authProxy } from "@ac/web/proxy";

// the proxy reads the session module, which is server code
vi.mock("server-only", () => ({}));
vi.mock("next/headers", () => ({ cookies: async () => ({ get: vi.fn(), set: vi.fn(), delete: vi.fn() }) }));

afterEach(() => vi.unstubAllEnvs());

const directives = (csp: string) => Object.fromEntries(csp.split("; ").map((d) => [d.split(" ")[0], d.split(" ").slice(1).join(" ")]));

// IR354: the security headers of network architecture §6, the page CSP as in the Next.js CSP guide
describe("the page Content-Security-Policy", () => {
  it("allows the app's own scripts and styles with the request's nonce only", () => {
    const d = directives(contentSecurityPolicy("abc=", { dev: false, https: false, formActions: ["http://localhost:8081"] }));
    expect(d).toEqual({
      "default-src": "'self'", "script-src": "'self' 'nonce-abc=' 'strict-dynamic'", "style-src": "'self' 'nonce-abc='", "style-src-attr": "'unsafe-inline'",
      "img-src": "'self' blob: data:", "font-src": "'self'", "connect-src": "'self'", "object-src": "'none'", "base-uri": "'self'",
      "form-action": "'self' http://localhost:8081", "frame-ancestors": "'none'",
    });
  });
  it("adds what next dev needs, and upgrades requests over HTTPS", () => {
    const dev = directives(contentSecurityPolicy("n", { dev: true, https: false, formActions: [] }));
    expect([dev["script-src"], dev["style-src"], dev["form-action"]]).toEqual(["'self' 'nonce-n' 'strict-dynamic' 'unsafe-eval'", "'self' 'unsafe-inline'", "'self'"]);
    expect(contentSecurityPolicy("n", { dev: false, https: true, formActions: [] }).endsWith("; upgrade-insecure-requests")).toBe(true);
  });
  it("makes a fresh nonce each time and reads origins", () => {
    const [a, b] = [newNonce(), newNonce()];
    expect(a).not.toBe(b);
    expect(a).toMatch(/^[A-Za-z0-9+/]+=*$/);
    expect([originOf("https://id.example.com/realms/ac"), originOf("not a url")]).toEqual(["https://id.example.com", null]);
  });
});

describe("the proxy's security headers", () => {
  const cspOf = (h: Headers) => h.get("content-security-policy") ?? "";
  it("sends the CSP on every page, with the nonce also on the request Next.js renders", async () => {
    vi.stubEnv("DATA_SOURCE", "mock");
    vi.stubEnv("OIDC_ISSUER", "https://id.example.com/realms/ac");
    const res = await authProxy(new NextRequest("http://localhost:3000/customer"), "client");
    const csp = cspOf(res.headers);
    const nonce = csp.match(/'nonce-([^']+)'/)?.[1];
    expect(nonce).toBeTruthy();
    expect(directives(csp)["form-action"]).toBe("'self' https://id.example.com");
    // NextResponse.next({ request: { headers } }) forwards the request headers as x-middleware-request-*
    expect(res.headers.get("x-middleware-request-x-nonce")).toBe(nonce);
    expect(res.headers.get("x-middleware-request-content-security-policy")).toBe(csp);
    expect(res.headers.get("strict-transport-security")).toBeNull(); // plain HTTP
    const again = await authProxy(new NextRequest("http://localhost:3000/customer"), "client");
    expect(cspOf(again.headers)).not.toBe(csp); // a fresh nonce per request
  });
  it("adds HSTS behind an HTTPS edge, in API mode too, and redirects a signed-out user", async () => {
    vi.stubEnv("DATA_SOURCE", "api");
    const req = new NextRequest("http://app.internal:3000/settings/preferences", { headers: { "x-forwarded-proto": "https" } });
    const res = await authProxy(req, "client");
    expect(res.status).toBe(307); // no session: to sign-in
    expect(res.headers.get("location")).toContain("/login?returnTo=%2Fsettings%2Fpreferences");
    const open = await authProxy(new NextRequest("http://app.internal:3000/login", { headers: { "x-forwarded-proto": "https" } }), "client");
    expect(open.headers.get("strict-transport-security")).toBe(HSTS);
    expect(cspOf(open.headers)).toContain("upgrade-insecure-requests");
  });
});
