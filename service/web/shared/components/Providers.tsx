"use client";

import { useEffect } from "react";
import { CSPProvider } from "@base-ui/react/csp-provider";

/** What the shadcn/ui components need around every page (DEC-75): the request's CSP nonce, which the Base UI parts put
 * on the <style> tags they add (the CSP refuses a <style> without it). */
export function UIProviders({ nonce, children }: { nonce?: string; children: React.ReactNode }) {
  // <html data-hydrated="1"> once React runs the page: the E2E suite waits for it before it types or clicks, since an
  // event before hydration is lost (the flaky specs of 67b7072 and 1b4cc87)
  useEffect(() => {
    document.documentElement.dataset.hydrated = "1";
  }, []);
  return <CSPProvider nonce={nonce}>{children}</CSPProvider>;
}
