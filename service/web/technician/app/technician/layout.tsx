// The technician app frame: in API mode the signed-in user, the organization and the sidebar badges from the Core API (IR241).
import { AppShell } from "@ac/web/components/AppShell";
import { loadShell } from "@ac/web/lib/shell";

export default async function L({ children }: { children: React.ReactNode }) {
  return <AppShell role="technician" live={await loadShell("technician")}>{children}</AppShell>;
}
