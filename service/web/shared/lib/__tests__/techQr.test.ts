import { describe, expect, it } from "vitest";
import { qrMatch, qrRefusal, scanCode } from "@ac/web/lib/techQr";

const NOW = Date.parse("2026-09-21T02:04:00Z"); // Mon 10:04 KL
const unit = { id: "u1", displayName: "Bedroom AC", location: { pathLabels: ["Home A", "1F", "Bedroom"] }, capabilities: { manufacturer: "DemoAir", model: "SPL-200V" } };

describe("technician QR scan", () => {
  it("describes the matched unit and the user's job on it", () => {
    const today = qrMatch({ unitId: "u1", jobId: "job-c-aaaa" }, unit, { id: "job-c-aaaa", type: "reactive", status: "in_progress", scheduledSlot: { startAt: "2026-09-21T02:00:00Z", endAt: "2026-09-21T04:00:00Z" }, alertIds: ["a1"] }, NOW);
    expect(today).toEqual({
      title: "Bedroom AC", sub: "Home A › 1F › Bedroom · DemoAir SPL-200V",
      job: { label: "Your job today: job-c-aa", text: "10:00–12:00 · Repair · in progress · 1 linked alert", tone: "ok" },
      unitHref: "/technician/units/u1?jobId=job-c-aaaa", jobHref: "/technician/jobs/job-c-aaaa",
    });
    const later = qrMatch({ unitId: "u1", jobId: "job-d" }, unit, { id: "job-d", type: "periodic", status: "assigned", scheduledSlot: { startAt: "2026-09-23T02:00:00Z", endAt: "2026-09-23T04:00:00Z" } }, NOW);
    expect(later.job).toEqual({ label: "Your next job: job-d", text: "09-23 10:00–12:00 · Periodic inspection · assigned", tone: "primary" });
    expect(qrMatch({ unitId: "u1", jobId: null }, unit, null, NOW)).toMatchObject({ job: null, unitHref: "/technician/units/u1", jobHref: null });
    expect(qrMatch({ unitId: "u1", jobId: null }, null, null, NOW).title).toBe("Assigned unit");
    expect(qrMatch({ unitId: "u1", jobId: "job-d" }, null, null, NOW, { name: "Meeting room AC", locked: true })).toMatchObject({ title: "Meeting room AC", sub: "Unit details open when your work window starts (IR94)." });
  });

  it("names the refusals and reads the label", () => {
    expect(qrRefusal({ code: "NOT_FOUND", messageKey: "error.notFound" }).title).toBe("Page unavailable");
    expect(qrRefusal({ code: "VALIDATION", messageKey: "error.validation" }).title).toBe("Not a label");
    expect(qrRefusal({ code: "UNAVAILABLE", messageKey: "error.unavailable" }).text).toBe("UNAVAILABLE — try again.");
    expect(scanCode("  ac-unit:u1 ")).toBe("ac-unit:u1");
  });
});
