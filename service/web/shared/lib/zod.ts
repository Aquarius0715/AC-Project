// zod for the code that runs in the browser (and shares files with it): the same `z`, with the JIT compiler off.
//
// zod 4 probes whether it may compile parsers by running `new Function("")`. Under this app's CSP (no unsafe-eval) the
// browser refuses it, and reports a securitypolicyviolation for the `eval` even though zod catches the error. With
// `jitless` zod skips the probe and parses with its interpreter; the schemas here are small, so nothing is lost.
// Import `z` from here, not from "zod", in any file a client component can reach: the setting must run before the first
// object schema is created.
import { z } from "zod";

z.config({ jitless: true });

export { z } from "zod";
