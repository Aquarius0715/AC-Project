// The frame of the shared screens (notifications, preferences, demo): the signed-in role's in API mode with the user,
// organization and badges from the Core API (IR241); the demo detects the last used role. It renders per request
// (IR248): the build has neither a session nor the Core API, so a prerendered frame (/demo is otherwise static) would be
// the demo one, and the client router reused that cached segment after a Server Action's refresh().
import { connection } from "next/server";
import { AppShell } from "@ac/web/components/AppShell";
import { apiMode, getSession } from "@ac/web/lib/dal";
import { APP_ROLE } from "@ac/web/lib/session";
import { loadShell } from "@ac/web/lib/shell";

export default async function L({ children }: { children: React.ReactNode }) {
  await connection();
  const s = apiMode() ? await getSession() : null;
  return <AppShell role={s?.role ?? (apiMode() ? APP_ROLE : undefined)} live={s ? await loadShell(s.role) : undefined}>{children}</AppShell>;
}
