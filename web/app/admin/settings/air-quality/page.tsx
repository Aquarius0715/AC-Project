import { redirect } from "next/navigation";
// Air-quality policies were merged into alert policies (product decision 2026-09-30).
export default function AirQualityPolicies() { redirect("/admin/alerts?tab=policies"); }
