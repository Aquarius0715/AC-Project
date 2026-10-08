"use client"; // Error boundaries must be Client Components (Next.js error.js convention)

import { Btn, Card, Page } from "@/components/ui";

export default function NotificationsError({ error, retry }: { error: Error & { digest?: string }; retry: () => void }) {
  return (
    <Page className="max-w-3xl">
      <Card title="Notifications could not be loaded" sub={error.digest ? `Reference ${error.digest}` : "Please try again."}>
        <Btn variant="primary" onClick={() => retry()}>Try again</Btn>
      </Card>
    </Page>
  );
}
