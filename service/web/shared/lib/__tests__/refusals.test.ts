import { describe, expect, it } from "vitest";
import { EN, translator } from "@ac/web/lib/i18n";
import { contractorRefusal } from "@ac/web/lib/adminContractors";
import { hqRefusal } from "@ac/web/lib/adminJobs";
import { planRefusal } from "@ac/web/lib/adminPlans";
import { slaRefusal } from "@ac/web/lib/adminSla";
import { filterRefusal } from "@ac/web/lib/customerFilterCare";
import { clientJobRefusal } from "@ac/web/lib/customerMaintenance";
import { decisionRefusal } from "@ac/web/lib/partnerJobDetail";
import { assignRefusal } from "@ac/web/lib/partnerSchedule";
import { refusal as alertRefusal } from "@ac/web/lib/techAlerts";
import { refusal as controlRefusal } from "@ac/web/lib/techControl";

const limited = { code: "RATE_LIMITED", messageKey: "error.rateLimited", fieldErrors: {}, retryAfterSeconds: 12 };
const mappers: [string, (f: typeof limited, t: typeof EN.t) => string | null][] = [
  ["contractorRefusal", contractorRefusal], ["hqRefusal", hqRefusal], ["planRefusal", planRefusal], ["slaRefusal", slaRefusal],
  ["filterRefusal", filterRefusal], ["clientJobRefusal", clientJobRefusal], ["decisionRefusal", decisionRefusal],
  ["assignRefusal", (f, t) => assignRefusal(f, "Ali", t)], ["techAlerts.refusal", (f, t) => alertRefusal(f, "forbidden", t)], ["techControl.refusal", controlRefusal],
];

// IR380: every screen's refusal text says how long the write limit asks to wait (IR375), in the display language,
// instead of "RATE_LIMITED — error.rateLimited"
describe("refusals over the write limit", () => {
  it.each(mappers)("%s names the wait", (_, refusal) => {
    expect(refusal(limited, EN.t)).toBe("Too many changes in a short time — try again in 12 s");
    expect(refusal(limited, translator("ms"))).toBe("Terlalu banyak perubahan dalam masa yang singkat — cuba lagi dalam 12 saat");
  });
  it.each(mappers)("%s words an unmapped code without the raw key", (_, refusal) => {
    expect(refusal({ ...limited, code: "UNAUTHENTICATED", messageKey: "error.unauthenticated", retryAfterSeconds: 0 }, EN.t)).toBe("Your session ended — sign in again");
  });
});
