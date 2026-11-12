// Where a job came from (IR113, DEC-106): kept apart from lib/jobs.ts, a client module, so server code can call it too.

/** The origin badge of a job: the client asked, a periodic plan made it, or HQ asked for it (an installation). */
export type Origin = "request" | "plan" | "hq";

const ORIGINS = new Map<string, Origin>([["periodic_plan", "plan"], ["hq_request", "hq"]]);

/** The badge of a job from the Core API's `origin`; the client's request when it is neither. */
export const originOf = (origin: string): Origin => ORIGINS.get(origin) ?? "request";
