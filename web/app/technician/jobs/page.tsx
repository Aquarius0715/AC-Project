"use client";

import { Card, Page } from "@/components/ui";
import { tjobs } from "@/lib/tech";
import { JobList } from "@/components/TechJobList";

export default function AllJobs() {
  return <Page><Card title="All assigned jobs" sub="Sidebar › Assigned jobs · only jobs assigned to you"><JobList jobs={tjobs} /></Card></Page>;
}
