import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { waitForCommand } from "@ac/web/lib/unitCommands";
import type { ApiCommand } from "@ac/web/lib/units";

// commands.get goes through the BFF Route Handler (callOp); the polling is the client's (D04)
const callOp = vi.hoisted(() => vi.fn());
vi.mock("@ac/web/lib/ops", () => ({ callOp }));

const deviceCommand = (status: ApiCommand["status"]): ApiCommand => ({ id: "c1", action: { kind: "set_temperature", celsius: 24 }, requestedAt: "2026-09-15T01:45:01Z", failureCode: status === "failed" ? "OFFLINE" : null, status });

beforeEach(() => vi.useFakeTimers());
afterEach(() => {
  vi.useRealTimers();
  callOp.mockReset();
});

describe("following a command until the device answers (D04, Unit Control and the assistant)", () => {
  it("reports every state every 2 s and stops at the first final one", async () => {
    callOp.mockResolvedValueOnce(deviceCommand("sent")).mockResolvedValueOnce(deviceCommand("acknowledged"));
    const seen: string[] = [];
    const done = waitForCommand(deviceCommand("requested"), (c) => seen.push(c.status));
    expect(seen).toEqual(["requested"]); // the first state at once
    await vi.advanceTimersByTimeAsync(1999);
    expect(callOp).not.toHaveBeenCalled();
    await vi.advanceTimersByTimeAsync(2001);
    const acknowledged = await done;
    expect(acknowledged.status).toBe("acknowledged");
    expect(seen).toEqual(["requested", "sent", "acknowledged"]);
    expect(callOp.mock.calls).toEqual([["commands.get", { id: "c1" }], ["commands.get", { id: "c1" }]]);
  });

  it("does not poll a command that is already final", async () => {
    for (const s of ["acknowledged", "failed", "expired", "cancelled"] as const) {
      const seen: string[] = [];
      const final = await waitForCommand(deviceCommand(s), (c) => seen.push(c.status));
      expect(final.status).toBe(s);
      expect(seen).toEqual([s]);
    }
    expect(callOp).not.toHaveBeenCalled();
  });

  it("gives up after 50 s (25 reads: the window, the expiry grace of IR453 and a tick fit) and returns the last state it saw", async () => {
    callOp.mockResolvedValue(deviceCommand("sent"));
    const done = waitForCommand(deviceCommand("sent"), () => {});
    await vi.advanceTimersByTimeAsync(50_000);
    const sent = await done;
    expect(sent.status).toBe("sent");
    expect(callOp).toHaveBeenCalledTimes(25);
  });

  it("passes a failed read to the caller", async () => {
    callOp.mockRejectedValueOnce(new Error("UNAVAILABLE"));
    // the read fails on the first poll, 2 s in: the expectation waits while the fake clock moves
    await Promise.all([
      expect(waitForCommand(deviceCommand("sent"), () => {})).rejects.toThrow("UNAVAILABLE"),
      vi.advanceTimersByTimeAsync(2000),
    ]);
  });
});
