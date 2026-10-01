export const tjobs = [
  { id: "job-contractor-a", unit: "Bedroom AC", cust: "customer-a", type: "Reactive", win: "10:00–12:00", note: "critical alert linked", status: "In progress", progress: "Indoor 7/8 done · window ends in 15 min", today: true, tone: "warn" as const },
  { id: "job-t07", unit: "Bedroom AC", cust: "customer-a", type: "Preventive", win: "14:00–16:00", note: "window starts 14:00 today", status: "Assigned", progress: "opens at 14:00 (read-only until then)", today: true, tone: "primary" as const },
  { id: "job-p05", unit: "Server room AC", cust: "customer-b", type: "Reactive", win: "09-20", note: "returned for rework", status: "Returned", progress: "“Photo evidence missing” — resume as v2", today: false, tone: "crit" as const },
  { id: "job-p09", unit: "Lobby AC", cust: "customer-b", type: "Periodic", win: "09-19", note: "v1 submitted", status: "Submitted", progress: "awaiting quality review · read-only", today: false, tone: "ok" as const },
];
