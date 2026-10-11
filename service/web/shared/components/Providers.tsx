"use client";

import { CSPProvider } from "@base-ui/react/csp-provider";

/** What the shadcn/ui components need around every page (DEC-75): the request's CSP nonce, which the Base UI parts put
 * on the <style> tags they add (the CSP refuses a <style> without it). */
export function UIProviders({ nonce, children }: { nonce?: string; children: React.ReactNode }) {
  return <CSPProvider nonce={nonce}>{children}</CSPProvider>;
}
