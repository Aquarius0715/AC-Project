const CUSTOMER_A = "customer-a";
const CUSTOMER_B = "customer-b";
const TECH_A = "tech-external-a";
export interface PJob { id: string; unit: string; cust: string; line: string; tech: string | null; techSub: string; slot: string; deadline: string; status: "Offered" | "Active" | "Review" | "Completed" | "Overdue"; label: string; tone: "primary" | "warn" | "ok" | "crit" | "unknown" }
export const pjobs: PJob[] = [
  { id: "job-p09", unit: "Lobby AC", cust: CUSTOMER_B, line: "Offered by HQ · expires 2026-09-15 01:00", tech: null, techSub: "not assignable before acceptance", slot: "Answer by 09-15 01:00", deadline: "15 h left", status: "Offered", label: "Offered", tone: "primary" },
  { id: "job-contractor-a", unit: "Bedroom AC", cust: CUSTOMER_A, line: "Assigned tech-external-a · 2026-09-21 10:00–12:00", tech: TECH_A, techSub: "Refrigerant handling", slot: "09-21 10:00–12:00", deadline: "Delegation to 09-22 00:00", status: "Active", label: "Assigned", tone: "warn" },
  { id: "job-p05", unit: "Server room AC", cust: CUSTOMER_B, line: "Submitted by tech-external-a · awaiting review", tech: TECH_A, techSub: "Author of report v1", slot: "Submitted 09-20 16:40", deadline: "Review within 2 days", status: "Review", label: "In review", tone: "primary" },
  { id: "job-p02", unit: "Rooftop unit", cust: CUSTOMER_B, line: "Accepted 09-14 10:02 · Reactive maintenance · refrigerant cert required", tech: null, techSub: "2 qualified free", slot: "Delegation ends 09-22 00:00", deadline: "Assign before 09-21", status: "Active", label: "Unassigned", tone: "warn" },
  { id: "job-p12", unit: "Lobby AC", cust: CUSTOMER_B, line: "Accepted 09-19 09:15 · Preventive maintenance · general", tech: null, techSub: "1 qualified free", slot: "Delegation ends 09-24 18:00", deadline: "Visit 09-23 14:00–16:00 (fixed)", status: "Active", label: "Unassigned", tone: "warn" },
  { id: "job-p07", unit: "Server room AC", cust: CUSTOMER_A, line: "Work window ended 09-20 17:00 · report not submitted", tech: TECH_A, techSub: "General maintenance", slot: "Ended 09-20 17:00", deadline: "Extend or reassign", status: "Overdue", label: "Overdue", tone: "crit" },
  { id: "job-p03", unit: "Office AC", cust: CUSTOMER_A, line: "Quality review accepted · report v2 visible to customer", tech: TECH_A, techSub: "Report author", slot: "Completed 09-21 17:20", deadline: "Rework 1× (09-21 14:05)", status: "Completed", label: "Completed", tone: "ok" },
  { id: "job-p04", unit: "Meeting room AC", cust: CUSTOMER_A, line: "Quality review accepted · preventive maintenance", tech: "tech-external-a2", techSub: "Report author", slot: "Completed 09-19 15:10", deadline: "No rework", status: "Completed", label: "Completed", tone: "ok" },
];
