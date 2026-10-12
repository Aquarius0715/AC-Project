import path from "node:path";
import type { NextConfig } from "next";

// Contractor web app (partner): one Next.js app per entry point (IR178, service/web/partner). Shared code comes from the @ac/web workspace
// package (Next.js docs: transpilePackages for monorepo packages); tracing starts at the workspace root so the
// standalone server includes it.
const nextConfig: NextConfig = {
  env: { AC_APP_ROLE: "contractor" }, // this app's role (session cookie name, sign-in checks)
  output: "standalone", // self-contained server bundle for the Docker image (container design §3)
  poweredByHeader: false, // no X-Powered-By: Next.js on responses (it names the framework to attackers)
  transpilePackages: ["@ac/web"],
  outputFileTracingRoot: path.join(__dirname, ".."),
  turbopack: { root: path.join(__dirname, "..") },
  // the security headers of network architecture §6 on every response (IR354); the page CSP comes from proxy.ts
  async headers() {
    return [{ source: "/:path*", headers: [
      { key: "X-Content-Type-Options", value: "nosniff" },
      { key: "Referrer-Policy", value: "strict-origin-when-cross-origin" },
      { key: "X-Frame-Options", value: "DENY" },
      // the apps use none of these browser features, so a page cannot ask for them even if a script were injected
      // (IR383); the simulated QR camera needs no camera. A real scanner would allow camera=(self) on its app only
      { key: "Permissions-Policy", value: "camera=(), microphone=(), geolocation=(), payment=(), usb=(), display-capture=()" },
    ] }];
  },
  experimental: { serverActions: { bodySizeLimit: "11mb" } }, // a certificate file of up to 10 MB (DD-P09, IR277)
};

export default nextConfig;
