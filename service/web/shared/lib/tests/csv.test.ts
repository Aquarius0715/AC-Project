import { describe, expect, it } from "vitest";

import { csvCell, csvQuoted, csvText } from "@ac/web/lib/csv";
import { coverageCsv, errorReportCsv, type ApiImportRow, type CoverageRow } from "@ac/web/lib/assets";

// IR368: an exported cell never runs as a spreadsheet formula (CSV injection)
describe("CSV export cells", () => {
  it("keeps a formula-like cell as text and leaves numbers and plain text alone", () => {
    expect(["=HYPERLINK(\"http://x\",\"y\")", "+1+1", "-2+3", "@SUM(A1)", "\tx", "\rx"].map((item) => csvText(item))).toEqual(["'=HYPERLINK(\"http://x\",\"y\")", "'+1+1", "'-2+3", "'@SUM(A1)", "'\tx", "'\rx"]);
    expect(["-12.5", "+60123456789", "42", "Bedroom AC", "—", "a=b", ""].map((item) => csvText(item))).toEqual(["-12.5", "+60123456789", "42", "Bedroom AC", "—", "a=b", ""]);
    expect([csvCell("=1+1"), csvCell('=A1,"x"'), csvCell("a\rb"), csvQuoted("=1"), csvQuoted(7)]).toEqual(["'=1+1", `"'=A1,""x"""`, `"a\rb"`, `"'=1"`, `"7"`]);
  });

  it("guards the exports built from names users and import files chose", () => {
    const row: CoverageRow = {
      unitId: "u1", unit: "=cmd|' /C calc'!A0", sub: "", customerId: "c1", customer: "@Evil", model: "Daikin X", ends: "", endsDate: "2026-10-10", endsAt: null, days: null,
      contractIds: [], contracts: "—", status: "no_coverage", statusText: "-1+1",
    };
    expect(coverageCsv([row]).split("\n", 2)[1]).toBe("'=cmd|' /C calc'!A0,u1,'@Evil,Daikin X,2026-10-10,,'-1+1");
    const imported: ApiImportRow = {
      rowNumber: 2, propertyName: "=1+1", floorName: null, roomName: null, unitName: "+SUM(1)", modelCode: "SPL-100", serial: null, installedOn: null, warrantyEnd: null, result: "error", messageKey: "error.invalid",
    };
    expect(errorReportCsv([imported]).split("\n", 2)[1]?.startsWith("2,'=1+1,,,'+SUM(1),SPL-100,")).toBe(true);
  });
});
