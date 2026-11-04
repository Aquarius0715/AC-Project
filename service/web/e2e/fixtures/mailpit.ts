// The local Mailpit inbox (DEC-78): the stack's e-mails, the sign-in service's account set-up e-mails among them
// (IR461). Specs that read it skip on a stack without it.
import { expect } from "@playwright/test";

export const MAILPIT = "http://localhost:8025/api/v1";

/** Whether the inbox answers. */
export const mailpitUp = async (): Promise<boolean> => answers(`${MAILPIT}/messages?limit=1`);

/** Whether `url` answers with a success. */
export async function answers(url: string): Promise<boolean> {
  const res = await fetch(url).catch(() => null);
  return res?.ok ?? false;
}

/** How many e-mails a Mailpit search URL finds. */
export async function messageCount(search: string): Promise<number> {
  const res = await fetch(search);
  const body = (await res.json()) as { messages_count: number };
  return body.messages_count;
}

/** The set-up link of the newest e-mail to `email`, waiting up to 30 s for it. */
export async function setupLink(email: string): Promise<string> {
  const search = `${MAILPIT}/search?query=${encodeURIComponent(`to:"${email}"`)}`;
  let id = "";
  await expect.poll(async () => {
    const res = await fetch(search);
    const body = (await res.json()) as { messages: { ID: string }[] };
    id = body.messages[0]?.ID ?? "";
    return id;
  }, { timeout: 30_000 }).not.toBe("");
  const res = await fetch(`${MAILPIT}/message/${id}`);
  const { Text: text } = (await res.json()) as { Text: string };
  const link = (/https?:\/\/\S+\/login-actions\/action-token\?\S+/.exec(text))?.[0];
  if (!link) throw new Error("the set-up e-mail has no link");
  return link;
}
