import path from "node:path";
import type { NextConfig } from "next";

// Technician web app (technician): one Next.js app per entry point (IR178, service/web/technician). Shared code comes from the @ac/web workspace
// package (Next.js docs: transpilePackages for monorepo packages); tracing starts at the workspace root so the
// standalone server includes it.
const nextConfig: NextConfig = {
  env: { AC_APP_ROLE: "technician" }, // this app's role (session cookie name, sign-in checks)
  output: "standalone", // self-contained server bundle for the Docker image (container design §3)
  transpilePackages: ["@ac/web"],
  outputFileTracingRoot: path.join(__dirname, ".."),
  turbopack: { root: path.join(__dirname, "..") },
  // report photos and the sign-off reach the Server Actions as files (JPEG/PNG up to 5 MiB, DD-T09 / DD-T15)
  experimental: { serverActions: { bodySizeLimit: "12mb" } },
};

export default nextConfig;
