import { connection } from "next/server";
import { appLinks } from "@ac/web/lib/apps";
import { LoginScreen } from "@ac/web/screens/auth/login/page";

export default async function LoginPage() {
  await connection(); // the other apps' URLs are runtime settings
  return <LoginScreen role="contractor" others={appLinks()} />;
}
