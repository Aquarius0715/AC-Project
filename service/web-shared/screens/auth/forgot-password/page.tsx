"use client";

import Link from "next/link";
import { useState } from "react";
import { Btn, Card, Field, Input, Banner } from "@ac/web/components/ui";

export default function Forgot() {
  const [email, setEmail] = useState("");
  const [sent, setSent] = useState(false);
  const [err, setErr] = useState<string>();
  const submit = (e: React.FormEvent) => {
    e.preventDefault();
    if (!/^\S+@\S+\.\S+$/.test(email)) return setErr("Enter an email like name@example.com");
    setErr(undefined);
    setSent(true);
  };
  return (
    <main className="grid min-h-dvh place-items-center px-4 py-10">
      <div className="w-full max-w-md">
        <Card title="Reset your password" sub="Enter your email — we'll send a reset link if an account exists">
          <form onSubmit={submit} className="flex flex-col gap-3">
            <Field label="Email" error={err}><Input type="text" value={email} onChange={(e) => setEmail(e.target.value)} placeholder="name@example.com" /></Field>
            <Btn variant="primary" type="submit">Send reset preview</Btn>
            {sent && <Banner tone="ok">If an account exists for that address, a reset link preview has been sent.<div className="text-xs text-muted">deliveryState: preview · no real email sent · same message shown whether or not the address is registered</div></Banner>}
            <Link href="/login" className="text-xs font-semibold text-primary hover:underline">← Back to sign in</Link>
            <p className="text-[11px] text-muted">Demo authentication only — email format is checked but no message is actually delivered.</p>
          </form>
        </Card>
      </div>
    </main>
  );
}
