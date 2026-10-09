import { describe, expect, it } from "vitest";
import { jobRow, pages, slotText, sortOf, sortSpec, tabFilters, tabOf } from "@ac/web/lib/partnerJobs";
import type { ApiPartnerJob } from "@ac/web/lib/partnerOverview";

const NOW = Date.parse("2026-09-21T01:30:00Z"); // Mon 09:30 in Kuala Lumpur
const slot = (s: string, e: string) => ({ startAt: s, endAt: e });
const units = new Map([["u-1", "Bedroom AC"]]);
const names = new Map([["m-a", "tech-external-a"]]);
const summary = (status: string, over: Record<string, unknown> = {}): ApiPartnerJob => ({ projection: "summary", id: "11111111-aaaa", version: 3, unitId: "u-1", type: "periodic", status, dueAt: "2026-09-25T10:00:00Z", requestedSlot: slot("2026-09-21T02:00:00Z", "2026-09-21T04:00:00Z"), scheduledSlot: null, assignmentId: null, displayStatus: status, technicianMembershipId: null, ...over } as ApiPartnerJob);

describe("partner job list", () => {
  it("maps tabs, sorts and pages", () => {
    expect(tabOf("review")).toBe("review");
    expect(tabOf("nope")).toBe("all");
    expect(tabFilters("all", "F", "T")).toEqual({ from: "F", to: "T" });
    expect(tabFilters("active", "F", "T")).toEqual({ from: "F", to: "T", statuses: ["accepted", "assigned", "in_progress", "on_hold", "rework_requested"] });
    expect(sortOf("dueAt:desc")).toBe("dueAt:desc");
    expect(sortOf("x")).toBe("status:asc");
    expect(sortSpec("severity:desc")).toEqual({ field: "severity", direction: "desc" });
    expect(pages(0, 1)).toEqual({ page: 1, of: 1 });
    expect(pages(26, 2)).toEqual({ page: 2, of: 2 });
    expect(slotText(slot("2026-09-21T02:00:00Z", "2026-09-21T04:00:00Z"))).toBe("09-21 10:00–12:00");
    expect(slotText(slot("2026-09-21T02:00:00Z", "2026-09-22T04:00:00Z"))).toBe("09-21 10:00 → 09-22 12:00");
  });

  it("shows an offer with its answer deadline and an accepted offer before the window", () => {
    const offered: ApiPartnerJob = { projection: "offer", status: "offered", jobId: "22222222-bbbb", jobVersion: 2, offerId: "o", type: "reactive", siteAddress: "2 Demo Avenue", requestedSlot: slot("2026-09-22T01:00:00Z", "2026-09-22T03:00:00Z"), dueAt: "2026-09-24T10:00:00Z", offerExpiresAt: "2026-09-21T16:30:00Z", visitSlot: slot("2026-09-22T01:00:00Z", "2026-09-22T03:00:00Z") };
    expect(jobRow(offered, NOW, units, names)).toEqual({
      id: "22222222-bbbb", title: "22222222 · 2 Demo Avenue", origin: null, href: "/partner/jobs/22222222-bbbb",
      line: "Offered by HQ · visit 09-22 09:00–11:00 (fixed)", tech: { name: "—", sub: "not assignable before acceptance" },
      slot: { main: "Answer by 09-22 00:30", sub: "15 h left", tone: undefined }, badge: { text: "Offered", tone: "primary" },
    });
    const proposed = jobRow({ ...offered, partnerSlotProposal: { status: "sent_to_client" } } as ApiPartnerJob, NOW, units, names);
    expect(proposed.badge).toEqual({ text: "Time proposed", tone: "warn" });
    const accepted = jobRow({ ...offered, status: "accepted" }, NOW, units, names);
    expect(accepted).toMatchObject({ href: "/partner/schedule?jobId=22222222-bbbb", tech: { name: "Unassigned", tone: "warn" }, badge: { text: "Accepted" } });
  });

  it("follows a delegated job from unassigned to completed", () => {
    expect(jobRow(summary("accepted"), NOW, units, names)).toMatchObject({ title: "11111111 · Bedroom AC", href: "/partner/schedule?jobId=11111111-aaaa", tech: { name: "Unassigned" }, slot: { main: "Due 09-25 18:00" }, badge: { text: "Accepted" } });
    const assigned = { assignmentId: "as", technicianMembershipId: "m-a", assignmentAcknowledgement: "pending", scheduledSlot: slot("2026-09-21T02:00:00Z", "2026-09-21T04:00:00Z") };
    expect(jobRow(summary("assigned", assigned), NOW, units, names)).toMatchObject({ line: "Assigned tech-external-a · 09-21 10:00–12:00", tech: { name: "tech-external-a", sub: "awaiting the technician’s acceptance" }, slot: { main: "09-21 10:00–12:00", sub: "Due 09-25 18:00" }, badge: { text: "Assigned", tone: "primary" } });
    expect(jobRow(summary("assigned", { ...assigned, assignmentAcknowledgement: "cant_make" }), NOW, units, names).tech).toEqual({ name: "tech-external-a", sub: "can’t make it ⚠ — reassign", tone: "warn" });
    expect(jobRow(summary("in_progress", { ...assigned, scheduledSlot: slot("2026-09-20T01:00:00Z", "2026-09-20T09:00:00Z") }), NOW, units, names)).toMatchObject({ line: "Work window ended 09-20 17:00 · report not submitted", slot: { main: "Ended 09-20 17:00", tone: "crit" }, badge: { text: "Overdue", tone: "crit" } });
    expect(jobRow(summary("in_progress", assigned), NOW, units, names)).toMatchObject({ line: "In progress · tech-external-a on site", badge: { text: "In progress" } });
    expect(jobRow(summary("submitted", assigned), NOW, units, names)).toMatchObject({ href: "/partner/jobs/11111111-aaaa/review", line: "Submitted by tech-external-a · awaiting your review", badge: { text: "Submitted" } });
    expect(jobRow(summary("rework_requested", assigned), NOW, units, names).badge).toEqual({ text: "Rework", tone: "warn" });
    expect(jobRow(summary("on_hold", assigned), NOW, units, names).badge).toEqual({ text: "On hold", tone: "warn" });
    expect(jobRow(summary("completed", assigned), NOW, units, names)).toMatchObject({ badge: { text: "Completed", tone: "ok" }, tech: { name: "tech-external-a", sub: "report author" } });
    expect(jobRow(summary("cancelled"), NOW, units, names).badge).toEqual({ text: "Cancelled", tone: "muted" });
  });

  it("keeps only the history after the delegation", () => {
    expect(jobRow({ projection: "history", jobId: "33333333-cccc", type: "reactive", status: "completed", completedAt: "2026-09-10T06:00:00Z" }, NOW, units, names)).toEqual({
      id: "33333333-cccc", title: "33333333 · Repair", origin: null, href: "/partner/history?jobId=33333333-cccc", line: "Completed · delegation ended, history only",
      tech: { name: "—", sub: "history only" }, slot: { main: "Completed 09-10 14:00", sub: "access closed" }, badge: { text: "Completed", tone: "ok" },
    });
  });
});
