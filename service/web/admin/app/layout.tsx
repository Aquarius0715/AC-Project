import type { Metadata, Viewport } from "next";
import { headers } from "next/headers";
import { connection } from "next/server";
import { UIProviders } from "@ac/web/components/Providers";
import "@ac/web/globals.css";

export const metadata: Metadata = { title: "AC Project — Admin", description: "AC Project Admin app" };
export const viewport: Viewport = { width: "device-width", initialScale: 1 };

export default async function RootLayout({ children }: { children: React.ReactNode }) {
  await connection(); // every page renders per request, so it carries that request's CSP nonce (Next.js CSP guide, IR354)
  return (
    <html lang="en">
      {/* the shadcn/ui components' Base UI parts put this request's nonce on the <style> tags they add (DEC-75) */}
      <body><UIProviders nonce={(await headers()).get("x-nonce") ?? undefined}>{children}</UIProviders></body>
    </html>
  );
}
