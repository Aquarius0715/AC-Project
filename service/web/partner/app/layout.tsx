import type { Metadata, Viewport } from "next";
import { headers } from "next/headers";
import { connection } from "next/server";
import { UIProviders } from "@ac/web/components/Providers";
import { coreDisplay } from "@ac/web/lib/dal";
import { documentTitle } from "@ac/web/lib/nav";
import "@ac/web/globals.css";

/** The document title names the page in the user's display language (WCAG 2.4.2, IR490); the proxy passes the path. */
export async function generateMetadata(): Promise<Metadata> {
  const [{ locale }, h] = await Promise.all([coreDisplay(), headers()]);
  return { title: documentTitle("contractor", h.get("x-pathname") ?? "", locale), description: "AC Project Contractor app" };
}
export const viewport: Viewport = { width: "device-width", initialScale: 1 };

export default async function RootLayout({ children }: { children: React.ReactNode }) {
  await connection(); // every page renders per request, so it carries that request's CSP nonce (Next.js CSP guide, IR354)
  const { locale } = await coreDisplay(); // the page speaks the user's display language (WCAG 3.1.1, IR489); English before sign-in
  return (
    <html lang={locale}>
      {/* the shadcn/ui components' Base UI parts put this request's nonce on the <style> tags they add (DEC-75) */}
      <body><UIProviders nonce={(await headers()).get("x-nonce") ?? undefined}>{children}</UIProviders></body>
    </html>
  );
}
