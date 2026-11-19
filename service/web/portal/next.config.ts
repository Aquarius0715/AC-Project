import path from "node:path";
import type { NextConfig } from "next";

// Portal (portal): the page that lists every AC Project service address (IR791, service/web/portal). It has no sign-in and
// no data of its own. Shared code comes from the @ac/web workspace package, as in the four apps.
const nextConfig: NextConfig = {
  output: "standalone", // self-contained server bundle for the Docker image (container design §3)
  poweredByHeader: false,
  transpilePackages: ["@ac/web"],
  outputFileTracingRoot: path.join(__dirname, ".."),
  turbopack: { root: path.join(__dirname, "..") },
  headers: () => Promise.resolve([{ source: "/:path*", headers: [
      { key: "X-Content-Type-Options", value: "nosniff" },
      { key: "Referrer-Policy", value: "strict-origin-when-cross-origin" },
      { key: "X-Frame-Options", value: "DENY" },
      { key: "Permissions-Policy", value: "camera=(), microphone=(), geolocation=(), payment=(), usb=(), display-capture=()" },
    ] }]),
};

export default nextConfig;
