// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import type { ActionFailure } from "@ac/web/lib/actionMessage";
import type { ApiAlert } from "@ac/web/lib/alerts";
import type { ApiDetail, ApiOffer, FitRow } from "@ac/web/lib/partnerJobDetail";
import { unitDetail } from "@ac/web/lib/tests/unitFixture";

// The contractor's job detail in API mode (FR-P02, FR-P08): the history snapshot after the delegation period; the offer
// with its decline, accept, propose-another-time and withdraw decisions, the proposal banners and the refusals; the
// detail inside the period with the unit, the linked alerts, the status steps and who can take it
const actions = vi.hoisted(() => ({ acceptOffer: vi.fn(), declineOffer: vi.fn(), proposeSlot: vi.fn(), withdrawProposal: vi.fn() }));
vi.mock("../app/partner/jobs/[id]/(detail)/actions", () => actions);
const runs = vi.hoisted((): { toasts: string[]; options: ({ reload?: boolean } | undefined)[] } => ({ toasts: [], options: [] }));
type Result = { ok: true; value: null } | ({ ok: false } & ActionFailure);
vi.mock("@ac/web/lib/useAction", () => ({
  useAction: () => [false, (fn: () => Promise<Result>, ok: string, after?: () => void, failed?: (f: ActionFailure) => void, options?: { reload?: boolean }) => {
    runs.options.push(options);
    void fn().then((r) => {
      if (r.ok) { runs.toasts.push(ok); after?.(); } else failed?.(r);
    });
  }] as const,
}));
// the time picker as one text box: the view only passes its value on
vi.mock(import("@ac/web/components/TimeSelect"), async (importOriginal) => ({
  ...(await importOriginal()),
  TimeSelect: ({ value, onChange }: Readonly<{ value: string; onChange: (v: string) => void }>) => <input aria-label="Time" value={value} onChange={(e) => { onChange(e.target.value); }} />,
}));

import { JobDetailView } from "../app/partner/jobs/[id]/(detail)/_components/job-view";
import type { JobDetailLive } from "../app/partner/jobs/[id]/(detail)/_lib/load";

type OfferLive = Extract<JobDetailLive, { kind: "offer" }>;
type DetailLive = Extract<JobDetailLive, { kind: "detail" }>;
type Proposal = NonNullable<ApiOffer["partnerSlotProposal"]>;

const ok: Result = { ok: true, value: null };
const refusal = (messageKey: string): Result => ({ messageKey, ok: false, code: "CONFLICT", fieldErrors: {} });
const NOW = "2026-09-14T01:35:00Z";
const slot = { startAt: "2026-09-16T02:00:00Z", endAt: "2026-09-16T04:00:00Z" };
const fit = (over: Partial<FitRow> = {}): FitRow => ({ id: "m-1", name: "Aina", sub: "Indoor unit work", days: [], badge: { text: "Fits", tone: "ok" }, ...over });

const offerJob = (over: Partial<ApiOffer> = {}): ApiOffer => ({
  projection: "offer", status: "offered", jobId: "job-p09-aaaa-bbbb", jobVersion: 4, offerId: "off-1234-5678", type: "reactive", siteAddress: "12 Jalan Ampang",
  requiredQualifications: ["demo_indoor", "demo_electrical"], requestedSlot: slot, dueAt: "2026-09-20T00:00:00Z", offerExpiresAt: "2026-09-14T06:00:00Z", termsVersion: "t3",
  origin: "hq_request", candidateSlots: [slot], offeredAt: "2026-09-14T00:00:00Z", accessValidFrom: "2026-09-15T16:00:00Z", accessValidUntil: "2026-09-17T16:00:00Z",
  partnerSlotProposal: null, ...over,
});
const proposal = (status: Proposal["status"], technicianMembershipId = "m-1"): Proposal => ({
  id: "prop-1", offerId: "off-1234-5678", reason: "course", sentAt: "2026-09-14T01:00:00Z", slot, technicianMembershipId, status,
});
const offerText = (over: Partial<OfferLive["text"]> = {}): OfferLive["text"] => ({
  expires: "14 Sep 2:00 pm", visit: "16 Sep 10:00 am–12:00 pm", first: "16 Sep 10:00 am–12:00 pm", count: 1, access: "16 Sep – 18 Sep", accessFrom: "16 Sep 12:00 am",
  offered: "14 Sep 8:00 am", left: "4 h 25 m", proposal: "17 Sep 10:00 am–12:00 pm", nth: "2nd", propose: { zone: "Asia/Kuala_Lumpur", date: "2026-09-17", from: "10:00", to: "12:00" },
  offers: 2, ...over,
});
const offer = (job: Partial<ApiOffer> = {}, text: Partial<OfferLive["text"]> = {}, over: Partial<Omit<OfferLive, "kind" | "job" | "text">> = {}): OfferLive => ({
  kind: "offer", events: [], fits: [fit()], text: offerText(text), now: NOW, job: offerJob(job), names: { "m-1": "Aina" }, technicians: [{ id: "m-1", name: "Aina" }, { id: "m-2", name: "Badrul" }],
  ...over,
});

const assignment = (over: Partial<NonNullable<ApiDetail["assignment"]>> = {}): NonNullable<ApiDetail["assignment"]> => ({
  technicianMembershipId: "m-1", scheduledStart: "2026-09-16T02:00:00Z", scheduledEnd: "2026-09-16T04:00:00Z", status: "assigned", acknowledgement: "accepted", cantMakeReason: null, ...over,
});
const detailJob = (over: Partial<ApiDetail> = {}): ApiDetail => ({
  projection: "detail", id: "job-p02-aaaa-bbbb", version: 3, unitId: "unit-p-01", type: "reactive", status: "assigned", symptom: "", origin: "periodic_plan", alertIds: [],
  requestedSlot: slot, scheduledSlot: null, dueAt: "2026-09-20T00:00:00Z", startedAt: null, completedAt: null, assignmentId: "as-1", partnerSlotProposal: null, reportRefs: [],
  assignment: assignment(),
  offer: { id: "off-1", termsVersion: "t1", candidateSlots: [slot], offeredAt: "2026-09-14T00:00:00Z", accessValidFrom: "2026-09-13T16:00:00Z", accessValidUntil: "2026-09-20T16:00:00Z", decision: "accept", decidedAt: "2026-09-14T01:00:00Z" },
  ...over,
});
const detailText = (over: Partial<DetailLive["text"]> = {}): DetailLive["text"] => ({
  tech: "Aina", banner: null, steps: [], visit: "16 Sep 10:00 am–12:00 pm", access: "14 Sep – 21 Sep", scheduled: "16 Sep 10:00 am–12:00 pm", seen: null, raised: {},
  delegation: { left: "6 d 14 h of delegation left", toSchedule: "Schedule within 6 d 14 h" }, ...over,
});
const alert = (id: string, severity: ApiAlert["severity"], causeCode: string): ApiAlert => ({
  id, severity, causeCode, unitId: "unit-p-01", type: "sensor", status: "open", evidenceKind: "inferred", evidenceText: "", detectedAt: "2026-09-14T00:30:00Z",
});
const detail = (job: Partial<ApiDetail> = {}, text: Partial<DetailLive["text"]> = {}, over: Partial<Omit<DetailLive, "kind" | "job" | "text">> = {}): DetailLive => ({
  kind: "detail", events: [], alerts: [], fits: [fit()], text: detailText(text), now: NOW, job: detailJob(job), names: { "m-1": "Aina" }, unit: null, required: ["demo_indoor"], ...over,
});

beforeEach(() => {
  for (const fn of Object.values(actions)) fn.mockReset();
  runs.toasts = [];
  runs.options = [];
});
afterEach(cleanup);

const button = (name: string) => screen.getByRole("button", { name });
const isDisabled = (name: string) => button(name).hasAttribute("disabled");
const click = (name: string) => { fireEvent.click(button(name)); };
const href = (name: string) => screen.getAllByRole("link", { name }).map((l) => l.getAttribute("href"));
const text = () => document.body.textContent;

describe("job detail after the delegation period", () => {
  type HistoryLive = Extract<JobDetailLive, { kind: "history" }>;
  const history = (status: string, events: HistoryLive["text"]["events"]): HistoryLive => ({
    kind: "history", now: NOW, text: { events, completed: "21 Sep 4:00 pm", snapshot: "22 Sep 12:00 am" },
    job: { status, projection: "history", jobId: "job-p02-aaaa-bbbb", type: "periodic", asOf: "2026-09-21T16:00:00Z", completedAt: "2026-09-21T08:00:00Z", ownDecisionEvents: [] },
  });

  it("shows the company's decisions of a completed job", () => {
    render(<JobDetailView live={history("completed", [{ id: "e1", at: "14 Sep 9:00 am", text: "Offer accepted" }])} />);
    expect(screen.getByText("job-p02- · history")).toBeTruthy();
    expect(screen.getByText(/Delegation ended/)).toBeTruthy();
    expect(screen.getByText("job-p02- · Periodic inspection")).toBeTruthy();
    expect(screen.getAllByText("Completed")).toHaveLength(3); // the badge, the status and the completed row's label
    expect(screen.getByRole("listitem").textContent).toBe("14 Sep 9:00 am · Offer accepted");
    expect(text()).toContain("22 Sep 12:00 am");
  });

  it("words an unknown status from its code and says when no decision was recorded", () => {
    render(<JobDetailView live={history("put_aside", [])} />);
    expect(screen.getAllByText("put aside")).toHaveLength(2);
    expect(screen.getByText("No decision recorded.")).toBeTruthy();
    expect(screen.queryByRole("listitem")).toBeNull();
  });
});

describe("offer before acceptance", () => {
  it("shows what an offer may show and the team's fit", () => {
    const days = [{ label: "Wed", text: "4 h", free: true }, { label: "Thu", text: "0 h", free: false }];
    render(<JobDetailView live={offer({}, {}, { fits: [fit({ days }), fit({ id: "m-2", name: "Badrul", days: [{ label: "Wed", text: "4 h", free: true }] })] })} />);
    expect(screen.getByText("job-p09- · 12 Jalan Ampang · offer")).toBeTruthy();
    expect(screen.getByText("Expires 14 Sep 2:00 pm")).toBeTruthy();
    expect(text()).toContain("12 Jalan Ampang (registered)");
    expect(text()).toContain("OriginHQ request"); // an HQ installation offer is not a client's request (IR859)
    expect(screen.queryByText("Client request")).toBeNull();
    expect(text()).toContain("Indoor unit work · Electrical work");
    expect(text()).toContain("Visit time (fixed)");
    expect(text()).toContain("16 Sep 10:00 am–12:00 pm · requested by HQ");
    expect(text()).toContain("off-1234 · terms t3");
    expect(text()).toContain("Can your team take it? — visit 16 Sep 10:00 am–12:00 pm");
    expect(text()).toContain("holding Indoor unit work, Electrical work");
    expect(text()).toContain("off-1234 (2nd offer)");
    expect(text()).toContain("1 of 2 · answer within 4 h 25 m");
    // a row with more than one day shows each day; a single day shows only the badge
    const day = (label: string) => screen.getAllByText((_, el) => el?.textContent === label);
    expect(day("Thu0 h").map((el) => el.className.includes("bg-surface2"))).toStrictEqual([true]);
    expect(day("Wed4 h").map((el) => el.className.includes("bg-ok/15"))).toStrictEqual([true]);
    expect(href("Team & capacity")).toStrictEqual(["/partner/team"]);
    expect(screen.queryByRole("link", { name: /Assign technician/ })).toBeNull();
  });

  it("shows an accepted offer without an address, qualifications or decisions, with several times and the assign link", () => {
    render(<JobDetailView live={offer({ status: "accepted", siteAddress: null, requiredQualifications: [], origin: "periodic_plan" }, { count: 2, visit: "16 Sep / 17 Sep" }, { fits: [] })} />);
    expect(screen.getByText("job-p09- · Repair · accepted")).toBeTruthy();
    expect(text()).toContain("— (registered)");
    expect(text()).toContain("Required qualificationnone");
    expect(text()).toContain("Times offered · you pick one when you assign a technician");
    expect(text()).toContain("16 Sep / 17 Sep · from the periodic plan");
    expect(text()).toContain("Can your team take it? — earliest time 16 Sep 10:00 am–12:00 pm");
    expect(text()).toContain("holding the required qualifications");
    expect(text()).toContain("the delegation period starts 16 Sep 12:00 am");
    expect(text()).toContain("No technicians in your company yet.");
    expect(href("Assign technician")).toStrictEqual(["/partner/schedule?jobId=job-p09-aaaa-bbbb"]);
    expect(screen.queryByRole("button", { name: "Accept job" })).toBeNull();
    expect(screen.queryByText(/Expires/)).toBeNull();
  });

  it("needs a reason to decline, then declines with the trimmed reason without a reload", async () => {
    actions.declineOffer.mockResolvedValue(ok);
    render(<JobDetailView live={offer()} />);
    click("Decline");
    expect(screen.getByText("A reason is required to decline (1–1000 characters)")).toBeTruthy();
    expect(screen.queryByRole("dialog")).toBeNull();
    fireEvent.change(screen.getByPlaceholderText("No qualified technician available in this window."), { target: { value: "  no one free  " } });
    expect(screen.queryByText("A reason is required to decline (1–1000 characters)")).toBeNull();
    click("Decline");
    expect(screen.getByText("The job returns to HQ with your reason: “no one free”. A new offer would get a new offer ID.")).toBeTruthy();
    click("Cancel");
    expect(screen.queryByRole("dialog")).toBeNull();
    click("Decline");
    fireEvent.keyDown(button("Confirm decline"), { key: "Escape" });
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(actions.declineOffer).not.toHaveBeenCalled();
    click("Decline");
    click("Confirm decline");
    await waitFor(() => { expect(screen.getByText("job-p09- · declined")).toBeTruthy(); });
    expect(actions.declineOffer).toHaveBeenCalledWith("job-p09-aaaa-bbbb", "off-1234-5678", 4, "no one free");
    expect(runs.options).toStrictEqual([{ reload: false }]);
    expect(runs.toasts).toStrictEqual(["Offer declined"]);
    expect(href("Back to jobs")).toStrictEqual(["/partner/jobs"]);
  });

  it("closes the decline dialog on a refusal and says why, then accepting clears it", async () => {
    actions.declineOffer.mockResolvedValue(refusal("errors.offer_expired"));
    actions.acceptOffer.mockResolvedValue(ok);
    render(<JobDetailView live={offer()} />);
    fireEvent.change(screen.getByPlaceholderText("No qualified technician available in this window."), { target: { value: "busy" } });
    click("Decline");
    click("Confirm decline");
    await waitFor(() => { expect(screen.getByText(/This offer expired or was cancelled by HQ/)).toBeTruthy(); });
    expect(screen.queryByRole("dialog")).toBeNull();
    click("Accept job");
    await waitFor(() => { expect(screen.queryByText(/This offer expired or was cancelled by HQ/)).toBeNull(); });
    expect(actions.acceptOffer).toHaveBeenCalledWith("job-p09-aaaa-bbbb", "off-1234-5678", "t3", 4);
    expect(runs.toasts).toStrictEqual(["Offer accepted — assign a technician next"]);
  });

  it("does not accept an offer at or after its expiry", () => {
    render(<JobDetailView live={offer({ offerExpiresAt: NOW })} />);
    expect(isDisabled("Accept job")).toBe(true);
    expect(isDisabled("Decline")).toBe(false);
  });
});

describe("offer with a proposed time", () => {
  it("keeps Accept off while the proposal is with HQ, and withdraws it", async () => {
    actions.withdrawProposal.mockResolvedValueOnce(refusal("errors.terms_changed")).mockResolvedValueOnce(ok);
    render(<JobDetailView live={offer({ partnerSlotProposal: proposal("pending") })} />);
    expect(text()).toContain("Your proposed time is with HQ (17 Sep 10:00 am–12:00 pm · Aina)");
    expect(isDisabled("Accept job")).toBe(true);
    expect(screen.queryByRole("button", { name: "Propose another time…" })).toBeNull();
    click("Withdraw my proposal");
    await waitFor(() => { expect(screen.getByText(/The delegation terms changed/)).toBeTruthy(); });
    click("Withdraw my proposal");
    await waitFor(() => { expect(screen.queryByText(/The delegation terms changed/)).toBeNull(); });
    expect(actions.withdrawProposal).toHaveBeenLastCalledWith("job-p09-aaaa-bbbb", "prop-1", 4);
    expect(runs.toasts).toStrictEqual(["Proposal withdrawn"]);
  });

  it("says the client is asked, naming a technician who is no longer listed generically", () => {
    render(<JobDetailView live={offer({ partnerSlotProposal: proposal("sent_to_client", "m-gone") }, { proposal: null })} />);
    expect(text()).toContain("HQ asked the client about your proposed time ( · technician)");
    expect(isDisabled("Accept job")).toBe(true);
  });

  it("tells an approved and a declined or kept proposal apart, with Accept on", () => {
    render(<JobDetailView live={offer({ partnerSlotProposal: proposal("approved") })} />);
    expect(text()).toContain("The client approved your proposed time — the visit is now 16 Sep 10:00 am–12:00 pm.");
    expect(isDisabled("Accept job")).toBe(false);
    expect(isDisabled("Propose another time…")).toBe(false);
    for (const status of ["declined", "kept"] as const) {
      cleanup();
      render(<JobDetailView live={offer({ partnerSlotProposal: proposal(status) })} />);
      expect(text(), status).toContain("Your time change was not taken — the agreed time stays 16 Sep 10:00 am–12:00 pm.");
      expect(text(), status).not.toContain("approved your proposed time");
      expect(isDisabled("Accept job"), status).toBe(false);
    }
    cleanup();
    render(<JobDetailView live={offer({ partnerSlotProposal: proposal("withdrawn") })} />);
    expect(text()).not.toContain("Your time change was not taken");
    expect(text()).not.toContain("Your proposed time is with HQ");
  });
});

const box = (value: string) => screen.getByDisplayValue(value);

describe("propose another time", () => {
  it("cannot open without a qualified technician", () => {
    render(<JobDetailView live={offer({}, {}, { technicians: [] })} />);
    expect(isDisabled("Propose another time…")).toBe(true);
  });

  it("checks the reason and the order of the times, then sends the instants, technician and trimmed reason", async () => {
    actions.proposeSlot.mockResolvedValue(ok);
    render(<JobDetailView live={offer()} />);
    click("Propose another time…");
    expect(text()).toContain("job-p09- · agreed with the client: 16 Sep 10:00 am–12:00 pm · times in Asia/Kuala_Lumpur");
    fireEvent.change(box("12:00"), { target: { value: "09:00" } });
    click("Send to HQ");
    expect(screen.getByText("A reason is required")).toBeTruthy();
    expect(screen.getByText("The end must be after the start")).toBeTruthy();
    fireEvent.change(box("09:00"), { target: { value: "11:30" } });
    fireEvent.change(box("10:00"), { target: { value: "09:30" } });
    fireEvent.change(box("2026-09-17"), { target: { value: "2026-09-18" } });
    fireEvent.change(screen.getByRole("combobox"), { target: { value: "m-2" } });
    fireEvent.change(screen.getByPlaceholderText("Our qualified technician is on a certificate course that morning."), { target: { value: "  course  " } });
    click("Send to HQ");
    await waitFor(() => { expect(screen.queryByRole("dialog")).toBeNull(); });
    // 09:30–11:30 on 18 Sept in Kuala Lumpur (UTC+8)
    expect(actions.proposeSlot).toHaveBeenCalledWith("job-p09-aaaa-bbbb", "off-1234-5678", 4, { startAt: "2026-09-18T01:30:00.000Z", endAt: "2026-09-18T03:30:00.000Z" }, "m-2", "course");
    expect(runs.toasts).toStrictEqual(["Sent to HQ — waiting for the client"]);
  });

  it("closes on a refusal and shows it on the page; Cancel sends nothing", async () => {
    actions.proposeSlot.mockResolvedValue(refusal("errors.qualification_missing"));
    render(<JobDetailView live={offer()} />);
    click("Propose another time…");
    click("Cancel");
    expect(screen.queryByRole("dialog")).toBeNull();
    click("Propose another time…");
    fireEvent.change(screen.getByPlaceholderText("Our qualified technician is on a certificate course that morning."), { target: { value: "course" } });
    click("Send to HQ");
    await waitFor(() => { expect(screen.getByText(/That technician lacks a required qualification/)).toBeTruthy(); });
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(actions.proposeSlot).toHaveBeenCalledOnce();
    expect(actions.proposeSlot.mock.calls[0]?.[4]).toBe("m-1"); // the first technician by default
  });
});

describe("job detail inside the delegation period", () => {
  it("shows the unit, the client's ready photos, the linked alerts, the steps and an assigned technician", () => {
    const unit = unitDetail({
      id: "unit-p-01", displayName: "Rooftop unit", connection: "online",
      location: { pathLabels: ["Tower A", "Roof"], address: "12 Jalan Ampang", accessInstructions: "Key at the guard house" },
      latestMeasurements: [
        { id: "r0", unitId: "unit-p-01", sensorId: "s", metric: "temperature", value: null, unit: "°C", observedAt: NOW, origin: "measured", quality: "missing", qualityReason: null },
        { id: "r1", unitId: "unit-p-01", sensorId: "s", metric: "refrigerant_pressure", value: 820, unit: "kPa", observedAt: NOW, origin: "measured", quality: "valid", qualityReason: null },
      ],
      serviceScope: ["indoor", "electrical"],
    });
    const steps: DetailLive["text"]["steps"] = [
      { label: "Accepted", at: NOW, atText: "14 Sep 9:00 am", sub: "by your company", state: "done" },
      { label: "Assigned", at: null, atText: null, sub: "Aina", state: "current" },
      { label: "Visit", at: null, atText: null, sub: "on site", state: "todo" },
    ];
    const alerts = [alert("al-1", "critical", "window_open"), alert("al-2", "warning", "other")];
    render(<JobDetailView live={detail(
      { symptom: "noise", problemPhotos: [{ id: "ph-1", name: "front.jpg", status: "ready" }, { id: "ph-2", name: "back.jpg", status: "scanning" }] },
      { steps, banner: { tone: "primary", text: "Aina visits 16 Sep" }, seen: "10:47 am", raised: { "al-1": "14 Sep 8:30 am" } },
      { unit, alerts, required: ["demo_indoor", "demo_electrical"] },
    )} />);
    expect(screen.getAllByText("job-p02- · Rooftop unit")).toHaveLength(2); // the breadcrumb and the job card
    expect(screen.getByText("Aina visits 16 Sep")).toBeTruthy();
    expect(screen.getByText("Assigned", { selector: "span" }).className).toContain("bg-primary-soft"); // the primary badge
    expect(screen.getByText("Request")).toBeTruthy();
    expect(text()).toContain("“noise”");
    expect(href("front.jpg")).toStrictEqual(["/partner/jobs/job-p02-aaaa-bbbb/photos/ph-1"]);
    expect(screen.queryByText("back.jpg")).toBeNull();
    expect(text()).toContain("Key at the guard house");
    expect(text()).toContain("Indoor unit work · Electrical work");
    expect(text()).toContain("Visit time (fixed)16 Sep 10:00 am–12:00 pm · from the periodic plan");
    expect(text()).toContain("16 Sep 10:00 am–12:00 pm · Aina");
    expect(text()).toContain("Tower A › Roof");
    expect(text()).toContain("AC-Co m1");
    expect(text()).toContain("Online · seen 10:47 am");
    expect(text()).toContain("2 · Possible open window");
    expect(text()).toContain("refrigerant pressure 820 kPa"); // the first reading with a value
    expect(text()).toContain("Indoor unit · Electrical / control");
    expect(text()).toContain("Possible open window — raised 14 Sep 8:30 am · linked to this job.");
    expect(text()).toContain("Sensor alert — raised  · linked to this job.");
    // the steps: a done dot shows a check, the others their number; a step without a time says "next" only when it is to do
    const items = screen.getAllByRole("listitem").map((li) => li.textContent);
    expect(items).toStrictEqual(["Accepted14 Sep 9:00 am · by your company", "2Assigned · Aina", "3Visitnext · on site"]);
    expect(screen.getByText("Visit", { selector: "b" }).className).toContain("text-muted-foreground");
    expect(screen.getByText("Assigned", { selector: "b" }).className).toContain("text-primary");
    expect(screen.queryByRole("link", { name: /Assign technician|Reassign technician/ })).toBeNull();
    expect(screen.queryByRole("link", { name: /Review report/ })).toBeNull();
    expect(href("Open unit")).toStrictEqual(["/partner/units/unit-p-01?jobId=job-p02-aaaa-bbbb", "/partner/units/unit-p-01?jobId=job-p02-aaaa-bbbb"]);
    expect(href("Job history")).toStrictEqual(["/partner/history?jobId=job-p02-aaaa-bbbb"]);
    // assigned with more than two days of delegation left: the plain note
    expect(screen.getByText("6 d 14 h of delegation left").className).toContain("text-muted-foreground");
  });

  it("asks to assign an accepted job, and words a unit it cannot read and the empty values", () => {
    const unit = unitDetail({ connection: "error", displayName: "Lobby AC", serviceScope: [] });
    render(<JobDetailView live={detail({ status: "accepted", assignment: null, offer: null, origin: "other" }, { scheduled: null, tech: null, access: null, delegation: null }, { unit, required: [], fits: [] })} />);
    expect(text()).toContain("Times the client agreed16 Sep 10:00 am–12:00 pm · agreed with the client");
    expect(screen.getByText("Not assigned yet")).toBeTruthy();
    expect(text()).toContain("Delegation (access) period—");
    expect(text()).toContain("Required qualification—");
    expect(text()).toContain("Site address—");
    expect(text()).toContain("Location—Model");
    expect(text()).toContain("ConnectionError");
    expect(text()).toContain("Open alertsnone");
    expect(text()).toContain("Last reading—");
    expect(href("Assign technician")).toStrictEqual(["/partner/schedule?jobId=job-p02-aaaa-bbbb"]);
    expect(text()).not.toContain("Client’s photos");
    expect(screen.queryByText("Request")).toBeNull(); // no symptom: no Request row
    expect(text()).toContain("No technicians in your company yet.");
    expect(text()).not.toContain("delegation left");
  });

  it("without the unit: its card says why, and an unknown status reads from its code", () => {
    render(<JobDetailView live={detail({ status: "paused_by_hq", assignment: null }, { scheduled: null })} />);
    expect(screen.getByText("job-p02- · Repair")).toBeTruthy();
    expect(screen.getByText("job-p02- · Unit")).toBeTruthy();
    expect(screen.getByText("Target unit — not readable now")).toBeTruthy();
    expect(screen.getByText("The unit is readable only inside the delegation period.")).toBeTruthy();
    expect(screen.getByText("paused by hq").className).toContain("bg-surface2"); // the muted badge
    // not assigned yet: the note asks to schedule, in red
    expect(screen.getByText("Schedule within 6 d 14 h").className).toContain("text-crit");
  });

  it("asks to reassign when the technician can't make it or the slot ended, and warns two days before the period ends", () => {
    render(<JobDetailView live={detail({ assignment: assignment({ acknowledgement: "cant_make" }) })} />);
    expect(href("Reassign technician")).toStrictEqual(["/partner/schedule?jobId=job-p02-aaaa-bbbb"]);
    for (const status of ["assigned", "in_progress"]) {
      cleanup();
      render(<JobDetailView live={detail({ status, assignment: assignment({ scheduledEnd: NOW }) })} />);
      expect(href("Reassign technician"), status).toHaveLength(1);
    }
    cleanup();
    render(<JobDetailView live={detail({ status: "on_hold", assignment: assignment({ scheduledEnd: NOW }) })} />);
    expect(screen.queryByRole("link", { name: /Reassign technician/ })).toBeNull();
    cleanup();
    const offerOf = detailJob().offer;
    if (!offerOf) throw new Error("the fixture has an offer");
    render(<JobDetailView live={detail({ offer: { ...offerOf, accessValidUntil: "2026-09-15T16:00:00Z" } })} />);
    expect(screen.getByText("6 d 14 h of delegation left").className).toContain("text-crit");
  });

  it("offers the report review of a submitted job and shows the delegation left instead of who can take it", () => {
    render(<JobDetailView live={detail({ status: "submitted" })} />);
    expect(href("Review report")).toStrictEqual(["/partner/jobs/job-p02-aaaa-bbbb/review"]);
    expect(screen.queryByText("Who can take it")).toBeNull();
    expect(screen.getByText("6 d 14 h of delegation left")).toBeTruthy();
    for (const status of ["completed", "cancelled"]) {
      cleanup();
      render(<JobDetailView live={detail({ status }, { delegation: null })} />);
      expect(screen.queryByText("Who can take it"), status).toBeNull();
      expect(screen.queryByRole("link", { name: /Review report/ }), status).toBeNull();
      expect(text(), status).not.toContain("delegation left");
    }
  });
});
