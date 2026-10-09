import { connection } from "next/server";
import { appLinks } from "@ac/web/lib/apps";
import { apiMode } from "@ac/web/lib/dal";
import { LoginScreen } from "@ac/web/screens/auth/login/page";

export default async function LoginPage({ searchParams }: { searchParams: Promise<Record<string, string | string[] | undefined>> }) {
  await connection(); // the other apps' URLs and the data source are runtime settings
  return <LoginScreen role="contractor" others={appLinks()} params={await searchParams} api={apiMode()} />;
}
