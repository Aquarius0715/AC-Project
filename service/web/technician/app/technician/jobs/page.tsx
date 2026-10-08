import { redirect } from "next/navigation";
// Sidebar › Assigned jobs is /technician?tab=all (SCR-T01, Figma Technician 01-2).
export default function AllJobs() { redirect("/technician?tab=all"); }
