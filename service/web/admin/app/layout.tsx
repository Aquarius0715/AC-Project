import type { Metadata, Viewport } from "next";
import "@ac/web/globals.css";

export const metadata: Metadata = { title: "AC Project — Admin", description: "AC Project Admin app" };
export const viewport: Viewport = { width: "device-width", initialScale: 1 };

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="en">
      <body>{children}</body>
    </html>
  );
}
