import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  // Self-contained server bundle for the Docker image (docs/02-design/container-design.md §3).
  output: "standalone",
};

export default nextConfig;
