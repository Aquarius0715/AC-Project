// Shortens or shrinks a scenario for trying the script (TIME_SCALE=0.05 makes a 2-hour soak 6 minutes, VU_SCALE=0.1 a tenth
// of the virtual users). A real run leaves both at 1; the report names them when they are not.
import { env } from "./env.ts";

/** A duration in minutes as k6 writes it, scaled. */
export function minutes(n: number): string {
  return `${String(Math.max(1, Math.round(n * 60 * env.timeScale)))}s`;
}

/** A duration in minutes in milliseconds, scaled (the end of a window). */
export function minutesMs(n: number): number {
  return Math.round(n * 60_000 * env.timeScale);
}

/** A number of virtual users, scaled, at least 1. */
export function vus(n: number): number {
  return Math.max(1, Math.round(n * env.vuScale));
}
