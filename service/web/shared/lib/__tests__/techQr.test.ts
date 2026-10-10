import { describe, expect, it } from "vitest";
import { qrMatch, qrRefusal, scanCode } from "@ac/web/lib/techQr";
import { i18nOf } from "@ac/web/lib/i18n";

const MS = i18nOf({ locale: "ms", timeZone: "Asia/Tokyo" });

const NOW = Date.parse("2026-09-21T02:04:00Z"); // Mon 10:04 KL
const unit = { id: "u1", displayName: "Bedroom AC", location: { pathLabels: ["Home A", "1F", "Bedroom"] }, capabilities: { manufacturer: "DemoAir", model: "SPL-200V" } };

describe("technician QR scan", () => {
  it("describes the matched unit and the user's job on it", () => {
    const today = qrMatch({ unitId: "u1", jobId: "job-c-aaaa" }, unit, { projection: "detail", id: "job-c-aaaa", type: "reactive", status: "in_progress", scheduledSlot: { startAt: "2026-09-21T02:00:00Z", endAt: "2026-09-21T04:00:00Z" }, alertIds: ["a1"] }, NOW);
    expect(today).toEqual({
      title: "Bedroom AC", sub: "Home A › 1F › Bedroom · DemoAir SPL-200V",
      job: { label: "Your job today: job-c-aa", text: "21 Sept, 10:00 am – 12:00 pm MYT · Repair · in progress · 1 linked alert", tone: "ok" },
      unitHref: "/technician/units/u1?jobId=job-c-aaaa", jobHref: "/technician/jobs/job-c-aaaa",
    });
    const later = qrMatch({ unitId: "u1", jobId: "job-d" }, unit, { projection: "detail", id: "job-d", type: "periodic", status: "assigned", scheduledSlot: { startAt: "2026-09-23T02:00:00Z", endAt: "2026-09-23T04:00:00Z" }, alertIds: [] }, NOW);
    expect(later.job).toEqual({ label: "Your next job: job-d", text: "23 Sept, 10:00 am – 12:00 pm MYT · Periodic inspection · assigned", tone: "primary" });
    // a job whose window ended reads as its snapshot (IR124): the card names the job only
    expect(qrMatch({ unitId: "u1", jobId: "job-d" }, unit, { projection: "history" }, NOW).job).toEqual({ label: "Your next job: job-d", text: "", tone: "primary" });
    expect(qrMatch({ unitId: "u1", jobId: null }, unit, null, NOW)).toMatchObject({ job: null, unitHref: "/technician/units/u1", jobHref: null });
    expect(qrMatch({ unitId: "u1", jobId: null }, null, null, NOW).title).toBe("Assigned unit");
    expect(qrMatch({ unitId: "u1", jobId: "job-d" }, null, null, NOW, { name: "Meeting room AC", locked: true })).toMatchObject({ title: "Meeting room AC", sub: "Unit details open when your work window starts (IR94)." });
  });

  it("names the refusals and reads the label", () => {
    expect(qrRefusal({ code: "NOT_FOUND", messageKey: "error.notFound" })).toMatchObject({ title: "Page unavailable", absent: true });
    expect(qrRefusal({ code: "VALIDATION", messageKey: "error.validation" }).title).toBe("Not a label");
    expect(qrRefusal({ code: "UNAVAILABLE", messageKey: "error.unavailable" }).text).toBe("UNAVAILABLE — try again.");
    expect(scanCode("  ac-unit:u1 ")).toBe("ac-unit:u1");
  });
});

describe("the QR scan in Malay with the display time zone (IR288)", () => {
  it("words the match and the refusals", () => {
    const m = qrMatch({ unitId: "u1", jobId: "job-c-aaaa" }, unit, { projection: "detail", id: "job-c-aaaa", type: "reactive", status: "in_progress", scheduledSlot: { startAt: "2026-09-21T02:00:00Z", endAt: "2026-09-21T04:00:00Z" }, alertIds: ["a1", "a2"] }, NOW, {}, MS);
    expect(m.job).toEqual({ label: "Kerja anda hari ini: job-c-aa", text: "21 Sep, 11:00 PG – 1:00 PTG GMT+9 · Pembaikan · sedang berjalan · 2 amaran berpaut", tone: "ok" });
    expect(qrMatch({ unitId: "u1", jobId: null }, null, null, NOW, { locked: true }, MS)).toMatchObject({ title: "Unit yang ditugaskan", sub: "Butiran unit dibuka apabila tetingkap kerja anda bermula (IR94)." });
    expect(qrRefusal({ code: "NOT_FOUND", messageKey: "error.notFound" }, MS.t).title).toBe("Halaman tidak tersedia");
    expect(qrRefusal({ code: "TIMEOUT", messageKey: "error.timeout" }, MS.t)).toMatchObject({ title: "Imbasan gagal", text: "TIMEOUT — cuba lagi.", absent: false });
  });
});
