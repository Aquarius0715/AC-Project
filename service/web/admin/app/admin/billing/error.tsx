"use client"; // error boundaries are Client Components

import { RouteError } from "@ac/web/components/RouteStates";

export default function SegmentError({ error, retry }: { error: Error & { digest?: string }; retry: () => void }) {
  return <RouteError what="Billing" error={error} retry={retry} />;
}
