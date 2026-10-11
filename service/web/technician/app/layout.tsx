import type { Metadata, Viewport } from "next";
import { connection } from "next/server";
import "@ac/web/globals.css";

export const metadata: Metadata = { title: "AC Project — Technician", description: "AC Project Technician app" };
export const viewport: Viewport = { width: "device-width", initialScale: 1 };

export default async function RootLayout({ children }: { children: React.ReactNode }) {
  await connection(); // every page renders per request, so it carries that request's CSP nonce (Next.js CSP guide, IR354)
  return (
    <html lang="en">
      <body>{children}</body>
    </html>
  );
}
