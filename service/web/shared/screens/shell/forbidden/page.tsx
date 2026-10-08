import Link from "next/link";
import { Card } from "@ac/web/components/ui";
export default function Forbidden() {
  return (
    <div className="mx-auto max-w-lg pt-10">
      <Card title="This page isn’t available" sub="The page doesn’t exist, or your account can’t open it. Nothing was navigated automatically.">
        <p className="mb-3 text-xs text-muted">One shared screen for both FORBIDDEN and NOT_FOUND, so it never reveals whether the resource exists (D01/IR57).</p>
        <Link href="/login" className="text-xs font-semibold text-primary hover:underline">Go to sign in →</Link>
      </Card>
    </div>
  );
}
