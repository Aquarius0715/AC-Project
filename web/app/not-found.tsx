import Link from "next/link";
export default function NotFound() {
  return (
    <main className="grid min-h-dvh place-items-center px-4">
      <div className="w-full max-w-lg rounded-2xl border border-line bg-surface p-6">
        <h1 className="text-lg font-bold">This page isn’t available</h1>
        <p className="mt-1 text-[13px] text-muted">The page doesn’t exist, or your account can’t open it. Nothing was navigated automatically.</p>
        <Link href="/login" className="mt-4 inline-block text-xs font-semibold text-primary hover:underline">Go to sign in →</Link>
      </div>
    </main>
  );
}
