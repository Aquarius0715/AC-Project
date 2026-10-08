// Streaming fallback while the Server Component reads the inbox (Next.js loading.js convention).
import { Card, Page } from "@/components/ui";

export default function Loading() {
  return (
    <Page className="max-w-3xl">
      <Card title="Loading notifications…" />
    </Page>
  );
}
