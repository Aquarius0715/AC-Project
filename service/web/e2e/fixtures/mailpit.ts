// The local Mailpit inbox (DEC-78): the stack's e-mails, the sign-in service's account set-up e-mails among them
// (IR461). Specs that read it skip on a stack without it.
import { expect } from "@playwright/test";

export const MAILPIT = "http://localhost:8025/api/v1";

/** Whether the inbox answers. */
export const mailpitUp = async (): Promise<boolean> => !!(await fetch(`${MAILPIT}/messages?limit=1`).catch(() => null))?.ok;

/** The set-up link of the newest e-mail to `email`, waiting up to 30 s for it. */
export async function setupLink(email: string): Promise<string> {
  const search = `${MAILPIT}/search?query=${encodeURIComponent(`to:"${email}"`)}`;
  let id = "";
  await expect.poll(async () => {
    id = ((await (await fetch(search)).json()) as { messages: { ID: string }[] }).messages[0]?.ID ?? "";
    return id;
  }, { timeout: 30_000 }).not.toBe("");
  const text = ((await (await fetch(`${MAILPIT}/message/${id}`)).json()) as { Text: string }).Text;
  const link = text.match(/https?:\/\/\S+\/login-actions\/action-token\?\S+/)?.[0];
  if (!link) throw new Error("the set-up e-mail has no link");
  return link;
}
