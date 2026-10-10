import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { waitForCommand } from "@ac/web/lib/unitCommands";
import type { ApiCommand } from "@ac/web/lib/units";

// commands.get goes through the BFF Route Handler (callOp); the polling is the client's (D04)
const callOp = vi.hoisted(() => vi.fn());
vi.mock("@ac/web/lib/ops", () => ({ callOp }));

const cmd = (status: ApiCommand["status"]): ApiCommand => ({ id: "c1", action: { kind: "set_temperature", celsius: 24 }, status, requestedAt: "2026-09-15T01:45:01Z", failureCode: status === "failed" ? "OFFLINE" : null });

beforeEach(() => vi.useFakeTimers());
afterEach(() => {
  vi.useRealTimers();
  callOp.mockReset();
});

describe("following a command until the device answers (D04, Unit Control and the assistant)", () => {
  it("reports every state every 2 s and stops at the first final one", async () => {
    callOp.mockResolvedValueOnce(cmd("sent")).mockResolvedValueOnce(cmd("acknowledged"));
    const seen: string[] = [];
    const done = waitForCommand(cmd("requested"), (c) => seen.push(c.status));
    expect(seen).toEqual(["requested"]); // the first state at once
    await vi.advanceTimersByTimeAsync(1999);
    expect(callOp).not.toHaveBeenCalled();
    await vi.advanceTimersByTimeAsync(2001);
    expect((await done).status).toBe("acknowledged");
    expect(seen).toEqual(["requested", "sent", "acknowledged"]);
    expect(callOp.mock.calls).toEqual([["commands.get", { id: "c1" }], ["commands.get", { id: "c1" }]]);
  });

  it("does not poll a command that is already final", async () => {
    for (const s of ["acknowledged", "failed", "expired", "cancelled"] as const) {
      const seen: string[] = [];
      expect((await waitForCommand(cmd(s), (c) => seen.push(c.status))).status).toBe(s);
      expect(seen).toEqual([s]);
    }
    expect(callOp).not.toHaveBeenCalled();
  });

  it("gives up after 40 s (20 reads) and returns the last state it saw", async () => {
    callOp.mockResolvedValue(cmd("sent"));
    const done = waitForCommand(cmd("sent"), () => {});
    await vi.advanceTimersByTimeAsync(40_000);
    expect((await done).status).toBe("sent");
    expect(callOp).toHaveBeenCalledTimes(20);
  });

  it("passes a failed read to the caller", async () => {
    callOp.mockRejectedValueOnce(new Error("UNAVAILABLE"));
    const failed = expect(waitForCommand(cmd("sent"), () => {})).rejects.toThrow("UNAVAILABLE");
    await vi.advanceTimersByTimeAsync(2000);
    await failed;
  });
});
