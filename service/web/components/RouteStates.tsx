"use client";

// Shared UI for the loading.tsx / error.tsx file conventions of each route segment (Next.js loading.js / error.js).
import { Btn, Card, Page } from "@/components/ui";

export function RouteLoading({ what }: { what: string }) {
  return (
    <Page>
      <Card title={`Loading ${what}…`} />
    </Page>
  );
}

export function RouteError({ what, error, retry }: { what: string; error: Error & { digest?: string }; retry: () => void }) {
  return (
    <Page className="max-w-3xl">
      <Card title={`${what} could not be loaded`} sub={error.digest ? `Reference ${error.digest}` : "Please try again."}>
        <Btn variant="primary" onClick={() => retry()}>Try again</Btn>
      </Card>
    </Page>
  );
}
