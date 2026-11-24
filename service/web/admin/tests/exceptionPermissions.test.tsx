// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import type { ComponentProps } from "react";
import type { ApiRestriction } from "@ac/web/lib/restrictions";

// The restriction exception screen names the permissions by their word labels, as Access & roles does (IR879), not by
// their codes (restriction.write, restriction.override, audit.read)
vi.mock("../app/admin/restrictions/actions", () => ({}));
vi.mock("@ac/web/lib/useAction", () => ({ useAction: () => [false, vi.fn()] as const }));

import { ExceptionView } from "../app/admin/restrictions/[id]/_components/exception-view";

afterEach(cleanup);

const r: ApiRestriction = {
  id: "r-11112222", version: 3, createdAt: "2026-09-12T00:00:00Z", unitIds: [], rulesVersion: "demo-v1", policy: { kind: "power_off" }, state: "applied", perUnit: [], recoveryCases: [],
  noticeAt: "2026-09-12T00:00:00Z", executeAfter: "2026-09-13T00:00:00Z", releaseIntent: null,
};
const live = (over: Partial<ComponentProps<typeof ExceptionView>["live"]> = {}): ComponentProps<typeof ExceptionView>["live"] => ({
  now: "2026-09-14T01:00:00.000Z", r, units: [], bypass: [], canWrite: true, canOverride: true, canAudit: true, period: null, intent: "none", audit: [], ...over,
});

describe("restriction exception permissions (IR879)", () => {
  it("lists the held permissions by their word labels and names the audit card's permission the same way", () => {
    const { container } = render(<ExceptionView live={live()} />);
    expect(screen.getByText("Your permissions").nextElementSibling?.textContent).toBe("Edit restrictions · Override restrictions · View the audit log");
    expect(screen.getByText("View the audit log", { selector: "[data-slot=card-description]" })).toBeDefined();
    expect(container.textContent).not.toMatch(/restriction\.(write|override)|audit\.read/);
  });

  it("says read only without any of them", () => {
    render(<ExceptionView live={live({ canWrite: false, canOverride: false, canAudit: false })} />);
    expect(screen.getByText("Your permissions").nextElementSibling?.textContent).toBe("read only");
  });
});
