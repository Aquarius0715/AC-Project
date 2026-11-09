// Time windows of a run, so the report can show how latency and errors change over it: before / during / after a spike,
// or each quarter of an hour of a soak. A scenario names its windows once (at the top of its file); every measurement is
// tagged with the window it fell in.
export interface Phase { name: string; untilMs: number }

let phases: Phase[] = [];
let startedAtMs = 0;

/** The windows of the scenario, each ending `untilMs` after the start of the test. */
export function configurePhases(list: Phase[]): void {
  phases = list;
}

/** The start of the test (from setup()), which the windows are counted from. */
export function markStart(ms: number): void {
  startedAtMs = ms;
}

/** The names of the windows, in order; `all` when the scenario named none. */
export function phaseNames(): string[] {
  return phases.length === 0 ? ["all"] : phases.map((p) => p.name);
}

/** The window the present moment is in (the last one after the end of the last). */
export function currentPhase(): string {
  if (phases.length === 0) return "all";
  const elapsed = Date.now() - startedAtMs;
  const found = phases.find((p) => elapsed < p.untilMs) ?? phases.at(-1);
  return found?.name ?? "all";
}
