// The frame of the shared screens (notifications, preferences, demo): the signed-in role's in API mode with the user,
// organization and badges from the Core API (IR241); the demo detects the last used role.
import { AppShell } from "@ac/web/components/AppShell";
import { apiMode, getSession } from "@ac/web/lib/dal";
import { loadShell } from "@ac/web/lib/shell";

export default async function L({ children }: { children: React.ReactNode }) {
  const s = apiMode() ? await getSession() : null;
  return <AppShell role={s?.role} live={s ? await loadShell(s.role) : undefined}>{children}</AppShell>;
}
