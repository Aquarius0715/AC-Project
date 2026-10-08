"use client"; // error boundaries are Client Components

import { RouteError } from "@/components/RouteStates";

export default function SegmentError({ error, retry }: { error: Error & { digest?: string }; retry: () => void }) {
  return <RouteError what="Overview" error={error} retry={retry} />;
}
