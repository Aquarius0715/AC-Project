import type { Metadata, Viewport } from "next";
import "@ac/web/globals.css";

export const metadata: Metadata = { title: "AC Project services", description: "Every AC Project service address in one place" };
export const viewport: Viewport = { width: "device-width", initialScale: 1 };

export default function RootLayout({ children }: Readonly<{ children: React.ReactNode }>) {
  return (
    <html lang="en">
      <body>{children}</body>
    </html>
  );
}
