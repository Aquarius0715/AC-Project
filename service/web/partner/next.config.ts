import path from "node:path";
import type { NextConfig } from "next";

// Contractor web app (partner): one Next.js app per entry point (IR178, service/web/partner). Shared code comes from the @ac/web workspace
// package (Next.js docs: transpilePackages for monorepo packages); tracing starts at the workspace root so the
// standalone server includes it.
const nextConfig: NextConfig = {
  env: { AC_APP_ROLE: "contractor" }, // this app's role (session cookie name, sign-in checks)
  output: "standalone", // self-contained server bundle for the Docker image (container design §3)
  transpilePackages: ["@ac/web"],
  outputFileTracingRoot: path.join(__dirname, ".."),
  turbopack: { root: path.join(__dirname, "..") },
  experimental: { serverActions: { bodySizeLimit: "11mb" } }, // a certificate file of up to 10 MB (DD-P09, IR277)
};

export default nextConfig;
