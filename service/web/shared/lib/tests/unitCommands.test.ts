import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { followCommand, waitForCommand } from "@ac/web/lib/unitCommands";
import type { ApiCommand } from "@ac/web/lib/units";

const ACKNOWLEDGED = "acknowledged";

// commands.get goes through the BFF Route Handler (callOp); the polling is the client's (D04)
const callOp = vi.hoisted(() => vi.fn());
vi.mock("@ac/web/lib/ops", () => ({ callOp }));

const deviceCommand = (status: ApiCommand["status"]): ApiCommand => ({ id: "c1", action: { kind: "set_temperature", celsius: 24 }, requestedAt: "2026-09-15T01:45:01Z", failureCode: status === "failed" ? "OFFLINE" : null, status });

beforeEach(() => vi.useFakeTimers());
afterEach(() => {
  vi.useRealTimers();
  callOp.mockReset();
});

describe("following a command until the device answers (D04, Unit Control)", () => {
  it("reports every state every 2 s and stops at the first final one", async () => {
    callOp.mockResolvedValueOnce(deviceCommand("sent")).mockResolvedValueOnce(deviceCommand(ACKNOWLEDGED));
    const seen: string[] = [];
    const done = waitForCommand(deviceCommand("requested"), (c) => { seen.push(c.status); });
    expect(seen).toStrictEqual(["requested"]); // the first state at once
    await vi.advanceTimersByTimeAsync(1999);
    expect(callOp).not.toHaveBeenCalled();
    await vi.advanceTimersByTimeAsync(2001);
    const acknowledged = await done;
    expect(acknowledged.status).toBe(ACKNOWLEDGED);
    expect(seen).toStrictEqual(["requested", "sent", ACKNOWLEDGED]);
    expect(callOp.mock.calls).toStrictEqual([["commands.get", { id: "c1" }], ["commands.get", { id: "c1" }]]);
  });

  it("does not poll a command that is already final", async () => {
    for (const s of [ACKNOWLEDGED, "failed", "expired", "cancelled"] as const) {
      const seen: string[] = [];
      const final = await waitForCommand(deviceCommand(s), (c) => { seen.push(c.status); });
      expect(final.status).toBe(s);
      expect(seen).toStrictEqual([s]);
    }
    expect(callOp).not.toHaveBeenCalled();
  });

  it("gives up after 50 s (25 reads: the window, the expiry grace of IR453 and a tick fit) and returns the last state it saw", async () => {
    callOp.mockResolvedValue(deviceCommand("sent"));
    const done = waitForCommand(deviceCommand("sent"), vi.fn());
    await vi.advanceTimersByTimeAsync(50_000);
    const sent = await done;
    expect(sent.status).toBe("sent");
    expect(callOp).toHaveBeenCalledTimes(25);
  });

  it("passes a failed read to the caller", async () => {
    callOp.mockRejectedValueOnce(new Error("UNAVAILABLE"));
    // the read fails on the first poll, 2 s in: the expectation waits while the fake clock moves
    const waiting = waitForCommand(deviceCommand("sent"), vi.fn());
    await Promise.all([
      expect(waiting).rejects.toThrow("UNAVAILABLE"),
      vi.advanceTimersByTimeAsync(2000),
    ]);
  });
});

// a command the screen created was sent: a poll that gets no answer afterwards does not make it "not sent" (IR728)
describe("followCommand", () => {
  it("answers the last state the device reported", async () => {
    callOp.mockResolvedValueOnce(deviceCommand(ACKNOWLEDGED));
    const following = followCommand(deviceCommand("sent"), vi.fn());
    await vi.advanceTimersByTimeAsync(2000);
    expect(await following).toStrictEqual({ answered: true, command: deviceCommand(ACKNOWLEDGED) });
  });

  it("answers the command as sent but unread when a poll gets no answer", async () => {
    callOp.mockRejectedValueOnce(new TypeError("Failed to fetch"));
    const seen: string[] = [];
    const following = followCommand(deviceCommand("sent"), (c) => { seen.push(c.status); });
    await vi.advanceTimersByTimeAsync(2000);
    expect([await following, seen]).toStrictEqual([{ answered: false, command: deviceCommand("sent") }, ["sent"]]);
  });
});
