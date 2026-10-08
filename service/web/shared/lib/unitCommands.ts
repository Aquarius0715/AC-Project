"use client";

// Client side of remote control: polls commands.get through the BFF Route Handler until the device answers or the
// command expires (D04). Creating the command is the Server Action in the unit page (Server Functions are for
// mutations; reads such as polling go through a Route Handler).
import { callOp } from "@ac/web/lib/ops";
import type { ApiCommand } from "@ac/web/lib/units";

const terminal = new Set<ApiCommand["status"]>(["acknowledged", "failed", "expired", "cancelled"]);

/** Polls every 2 s (at most 40 s) and reports each state. */
export async function waitForCommand(first: ApiCommand, onUpdate: (c: ApiCommand) => void): Promise<ApiCommand> {
  let c = first;
  onUpdate(c);
  for (let i = 0; i < 20 && !terminal.has(c.status); i++) {
    await new Promise((r) => setTimeout(r, 2000));
    c = await callOp<ApiCommand>("commands.get", { id: c.id });
    onUpdate(c);
  }
  return c;
}
