// The portal's copy of the Core API's documentation pages (IR791). The gateway is not published to the network in the shared
// test environment (IR373), so these three fixed paths are fetched from it here: /docs, /docs/init.js and /swagger.json.
// Nothing else is relayed, and the path is never taken from the request.
import "server-only";

export type ApiPagePath = "/docs" | "/docs/init.js" | "/swagger.json";

/** The headers of the gateway's answer that the page depends on. */
const KEPT = ["content-type", "content-security-policy", "x-content-type-options", "referrer-policy"];

export async function relayApiPage(path: ApiPagePath, env: Readonly<Record<string, string | undefined>> = process.env): Promise<Response> {
  const base = env.CORE_API_URL;
  if (!base) return new Response("Not found", { status: 404 });
  try {
    const upstream = await fetch(`${base.replace(/\/$/, "")}${path}`, { cache: "no-store", signal: AbortSignal.timeout(5000) });
    if (!upstream.ok) return new Response("The API documentation is not available", { status: 502 });
    const headers = new Headers();
    for (const name of KEPT) {
      const value = upstream.headers.get(name);
      if (value) headers.set(name, value);
    }
    return new Response(await upstream.arrayBuffer(), { status: 200, headers });
  } catch {
    return new Response("The API documentation is not available", { status: 502 });
  }
}
