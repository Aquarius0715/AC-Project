// @vitest-environment jsdom
import { afterEach, describe, expect, it } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import { JobStatusBadge } from "@ac/web/components/JobBits";

afterEach(cleanup);

// IR574: the client's requests read "Scheduled" for every booked stage, as their tab does; HQ and the partner keep the
// stage itself.
describe("a job's status badge", () => {
  it("reads Scheduled for a client's booked visit", () => {
    render(<>{(["offered", "accepted", "assigned"] as const).map((s) => <JobStatusBadge key={s} s={s} client />)}</>);
    expect(screen.getAllByText("Scheduled")).toHaveLength(3);
  });
  it("keeps the stage for HQ and the partner, and every other stage for the client", () => {
    render(<><JobStatusBadge s="assigned" /><JobStatusBadge s="in_progress" client /><JobStatusBadge s="time_proposed" client /></>);
    expect(screen.getByText("Assigned")).toBeTruthy();
    expect(screen.getByText("In progress")).toBeTruthy();
    expect(screen.getByText("Time proposed")).toBeTruthy();
  });
});
