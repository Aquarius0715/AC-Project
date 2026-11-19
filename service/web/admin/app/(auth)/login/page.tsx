import { connection } from "next/server";
import { apiMode } from "@ac/web/lib/dal";
import { LoginScreen } from "@ac/web/screens/auth/login/page";

export default async function LoginPage({ searchParams }: Readonly<{ searchParams: Promise<Record<string, string | string[] | undefined>> }>) {
  await connection(); // the data source is a runtime setting
  return <LoginScreen appRole="admin" params={await searchParams} api={apiMode()} />;
}
