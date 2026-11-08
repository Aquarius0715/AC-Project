import { redirect } from "next/navigation";
import { connection } from "next/server";
import { apiMode } from "@ac/web/lib/dal";
import Forgot from "./page";

/** /forgot-password: with an identity service (API mode) the password is reset on its own pages, which the sign-in leads to (IR646), so this address goes to the sign-in; the browser demo keeps the preview. */
export default async function ForgotPasswordEntry() {
  await connection(); // the data source is a runtime setting
  if (apiMode()) redirect("/login");
  return <Forgot />;
}
