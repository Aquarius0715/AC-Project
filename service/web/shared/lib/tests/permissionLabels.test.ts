import { describe, expect, it } from "vitest";
import { translator } from "@ac/web/lib/i18n";
import { ms } from "@ac/web/lib/i18n-ms";
import { ja } from "@ac/web/lib/i18n-ja";
import { id } from "@ac/web/lib/i18n-id";
import { allPermissions } from "@ac/web/lib/members";
import { PERMISSION_LABEL_KEYS, permissionLabel } from "@ac/web/lib/permissionLabels";

// IR821: every permission the matrix offers has a label, in every display language; an unknown code stays as it is
describe("permission labels", () => {
  it("labels every permission in every language", () => {
    expect(allPermissions.filter((p) => permissionLabel(p) === p)).toStrictEqual([]);
    for (const dict of [ms, ja, id]) expect(PERMISSION_LABEL_KEYS.filter((k) => !Object.hasOwn(dict, k))).toStrictEqual([]);
  });
  it("words a permission and keeps an unknown code", () => {
    expect([permissionLabel("alert.resolve"), permissionLabel("alert.resolve", translator("ja")), permissionLabel("made.up")]).toStrictEqual(["Resolve alerts", "アラートの解決", "made.up"]);
  });
});
