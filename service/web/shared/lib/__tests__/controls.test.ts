// @vitest-environment node
import { readdirSync, readFileSync, statSync } from "node:fs";
import { join, relative } from "node:path";
import { fileURLToPath } from "node:url";
import ts from "typescript";
import { describe, expect, it } from "vitest";

// Phase 5 of the shadcn/ui adoption (DEC-75): the screens build their controls from the shadcn/ui components (through the
// product names in components/ui.tsx), not from raw <button>, <input>, <select> and <textarea> elements. The raw elements
// that remain are listed per file below; the list only shrinks. A file that gains one fails, and a file that loses its
// last one must leave the list, so the count is always exact.
const root = fileURLToPath(new URL("../../../", import.meta.url));
const DIRS = ["customer", "partner", "technician", "admin", "shared"];
const RAW = new Set(["button", "input", "select", "textarea"]);
// the shadcn components and the facade over them: they render the raw element themselves
const FACADE = fileURLToPath(new URL("../../components/ui", import.meta.url));

/** The raw form elements of one file, by tag. */
export function rawElements(file: string): Record<string, number> {
  const sf = ts.createSourceFile(file, readFileSync(file, "utf8"), ts.ScriptTarget.Latest, true, file.endsWith(".tsx") ? ts.ScriptKind.TSX : ts.ScriptKind.TS);
  const counts: Record<string, number> = {};
  const visit = (n: ts.Node) => {
    if (ts.isJsxOpeningElement(n) || ts.isJsxSelfClosingElement(n)) {
      const tag = n.tagName.getText(sf);
      if (RAW.has(tag)) counts[tag] = (counts[tag] ?? 0) + 1;
    }
    ts.forEachChild(n, visit);
  };
  visit(sf);
  return counts;
}

function sources(): string[] {
  const out: string[] = [];
  const walk = (dir: string) => {
    for (const name of readdirSync(dir)) {
      if (["node_modules", ".next", "__tests__"].includes(name)) continue;
      const path = join(dir, name);
      if (statSync(path).isDirectory()) walk(path);
      else if (/\.tsx$/.test(name) && !path.startsWith(FACADE) && path !== FACADE + ".tsx") out.push(path);
    }
  };
  for (const d of DIRS) walk(join(root, d));
  return out;
}

/** The baseline: the raw elements each file still has, relative to service/web. */
const BASELINE: Record<string, Record<string, number>> = {
  "admin/app/admin/(overview)/_components/overview-demo.tsx": { button: 3 },
  "admin/app/admin/alerts/_components/alerts-view.tsx": { button: 1 },
  "admin/app/admin/alerts/_components/policy-parts.tsx": { button: 1 },
  "admin/app/admin/audit/_components/audit-view.tsx": { button: 2 },
  "admin/app/admin/billing/_components/billing-view.tsx": { button: 2 },
  "admin/app/admin/devices/_components/device-tab.tsx": { button: 3 },
  "admin/app/admin/devices/_components/models-tab.tsx": { button: 2 },
  "admin/app/admin/jobs/_components/contractors-view.tsx": { input: 1 },
  "admin/app/admin/jobs/_components/jobs-demo.tsx": { button: 1 },
  "admin/app/admin/jobs/_components/jobs-view.tsx": { button: 4, input: 2 },
  "admin/app/admin/mrv/_components/evidence.tsx": { button: 1 },
  "admin/app/admin/restrictions/[id]/_components/exception-demo.tsx": { button: 1 },
  "admin/app/admin/restrictions/[id]/_components/exception-view.tsx": { button: 1 },
  "admin/app/admin/restrictions/_components/restrictions-view.tsx": { button: 1 },
  "admin/app/admin/settings/access/_components/access-view.tsx": { button: 1 },
  "admin/app/admin/settings/automation/_components/automation-view.tsx": { button: 1 },
  "admin/app/admin/units/_components/units-demo.tsx": { button: 2 },
  "admin/app/admin/units/_components/units-view.tsx": { button: 4, input: 1 },
  "customer/app/customer/(overview)/_components/overview-demo.tsx": { button: 1, select: 2 },
  "customer/app/customer/air-quality/_components/air-quality-demo.tsx": { button: 1, select: 1 },
  "customer/app/customer/air-quality/_components/air-quality-view.tsx": { button: 1 },
  "customer/app/customer/alerts/_components/alerts-live.tsx": { button: 2 },
  "customer/app/customer/alerts/_components/alerts-view.tsx": { button: 2 },
  "customer/app/customer/alerts/_components/policies-live.tsx": { button: 2 },
  "customer/app/customer/automations/_components/automations-view.tsx": { button: 4, input: 1 },
  "customer/app/customer/energy/_components/energy-view.tsx": { button: 1 },
  "customer/app/customer/energy/offsets/_components/offsets-demo.tsx": { button: 1 },
  "customer/app/customer/energy/offsets/_components/offsets-view.tsx": { button: 1 },
  "customer/app/customer/maintenance/_components/filter-care.tsx": { button: 1 },
  "customer/app/customer/maintenance/_components/maintenance-demo.tsx": { button: 2, input: 1 },
  "customer/app/customer/maintenance/_components/maintenance-view.tsx": { button: 2, input: 3 },
  "customer/app/customer/payments/[id]/_components/invoice-demo.tsx": { button: 1 },
  "customer/app/customer/payments/[id]/_components/invoice-view.tsx": { button: 1 },
  "customer/app/customer/properties/_components/properties-demo.tsx": { button: 3 },
  "customer/app/customer/properties/_components/properties-view.tsx": { button: 4 },
  "partner/app/partner/history/_components/history-view.tsx": { button: 2 },
  "partner/app/partner/jobs/(list)/_components/jobs-demo.tsx": { button: 3 },
  "partner/app/partner/jobs/(list)/_components/jobs-view.tsx": { button: 2 },
  "partner/app/partner/jobs/[id]/(detail)/_components/job-demo.tsx": { button: 1 },
  "partner/app/partner/payouts/_components/payouts-view.tsx": { button: 1 },
  "partner/app/partner/schedule/_components/schedule-view.tsx": { button: 1 },
  "partner/app/partner/team/_components/certifications.tsx": { input: 1 },
  "partner/app/partner/units/[id]/_components/mock-unit.tsx": { button: 1 },
  "shared/components/Assistant.tsx": { input: 1 },
  "shared/components/TechDevices.tsx": { button: 1 },
  "shared/screens/shell/demo/_components/demo-frame.tsx": { button: 3 },
  "technician/app/technician/(overview)/_components/overview-view.tsx": { button: 2 },
  "technician/app/technician/devices/_components/devices-view.tsx": { button: 4 },
  "technician/app/technician/jobs/[id]/_components/workspace-demo.tsx": { button: 5 },
  "technician/app/technician/jobs/[id]/_components/workspace-view.tsx": { button: 6, input: 2 },
  "technician/app/technician/units/[id]/_components/unit-demo.tsx": { button: 1 },
  "technician/app/technician/units/[id]/_components/unit-view.tsx": { button: 1 },
  "technician/app/technician/units/[id]/alerts/_components/alert-evidence.tsx": { button: 1 },
  "technician/app/technician/units/[id]/alerts/_components/mock-evidence.tsx": { input: 1 },
};

describe("the screens' controls are shadcn/ui components (DEC-75)", () => {
  it("every raw form element left in a screen is on the baseline, and the baseline has no spare entry", () => {
    const got: Record<string, Record<string, number>> = {};
    for (const f of sources()) {
      const c = rawElements(f);
      if (Object.keys(c).length) got[relative(root, f)] = c;
    }
    expect(got).toEqual(BASELINE);
  });
});
