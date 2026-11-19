import { afterEach, describe, expect, it, vi } from "vitest";
import { relayApiPage } from "@/lib/relay";

vi.mock("server-only", () => ({}));
afterEach(() => { vi.unstubAllGlobals(); });

const gateway = (res: Response) => {
  const f = vi.fn<(url: string) => Promise<Response>>(() => Promise.resolve(res));
  vi.stubGlobal("fetch", f);
  return f;
};

// IR791: the portal relays three fixed paths of the gateway, nothing else
describe("API documentation relay", () => {
  it("fetches the fixed path from the gateway and keeps the page's content type and policy", async () => {
    const f = gateway(new Response("<html></html>", { headers: { "content-type": "text/html", "content-security-policy": "default-src 'none'", "set-cookie": "a=b" } }));
    const res = await relayApiPage("/docs", { CORE_API_URL: "http://localhost:8080/" });
    expect(f.mock.calls[0]?.[0]).toBe("http://localhost:8080/docs");
    expect([res.status, res.headers.get("content-type"), res.headers.get("content-security-policy"), res.headers.get("set-cookie")]).toStrictEqual([200, "text/html", "default-src 'none'", null]);
    expect(await res.text()).toBe("<html></html>");
  });

  it("answers 502 when the gateway refuses or is unreachable, and 404 when there is no gateway", async () => {
    gateway(new Response("no", { status: 500 }));
    const swagger = await relayApiPage("/swagger.json", { CORE_API_URL: "http://localhost:8080" });
    expect(swagger.status).toBe(502);
    vi.stubGlobal("fetch", vi.fn(() => Promise.reject(new Error("down"))));
    const down = await relayApiPage("/docs", { CORE_API_URL: "http://localhost:8080" });
    expect(down.status).toBe(502);
    const none = await relayApiPage("/docs", {});
    expect(none.status).toBe(404);
  });
});
