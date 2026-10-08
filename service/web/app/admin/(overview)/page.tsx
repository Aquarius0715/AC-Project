// /admin dashboard (FR-A01): in API mode a Server Component reads admin.summary for today in Asia/Kuala_Lumpur,
// ending at the Core API business clock, through the DAL. The Phase 1A demo keeps the fixture KPIs.
import { connection } from "next/server";
import { apiMode, coreNow, coreOp } from "@/lib/dal";
import { kpisFrom, todayRange, type AdminSummary } from "@/lib/adminSummary";
import { AdminOverviewView } from "./_components/overview-view";

export default async function AdminOverviewPage() {
  await connection();
  if (!apiMode()) return <AdminOverviewView />;
  const summary = await coreOp<AdminSummary>("admin.summary", todayRange(await coreNow()));
  return <AdminOverviewView kpis={kpisFrom(summary)} />;
}
