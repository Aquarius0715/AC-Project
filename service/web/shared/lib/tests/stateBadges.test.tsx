// @vitest-environment jsdom
import { afterEach, describe, expect, it } from "vitest";
import { cleanup, render } from "@testing-library/react";
import { ConnBadge, PowerBadge } from "@ac/web/components/ui";

afterEach(cleanup);

// UI/UX loop R1: the power and the connection badge stand side by side in the unit lists, so neither may read as the
// other — "Unknown" twice was one state to the eye. Each names what it describes, visibly when unknown and for screen
// readers always.
describe("power and connection badges", () => {
  it("say which state is unknown", () => {
    const { container } = render(<><PowerBadge s="unknown" /><ConnBadge s="unknown" /></>);
    const texts = [...container.children].map((el) => el.textContent);
    expect(texts).toStrictEqual(["Power state: Power unknown", "Connection: Connection unknown"]);
  });

  it("name the state they describe for screen readers", () => {
    const { container } = render(<><PowerBadge s="running" /><ConnBadge s="online" /></>);
    const hidden = [...container.querySelectorAll(".sr-only")].map((el) => el.textContent);
    expect(hidden).toStrictEqual(["Power state: ", "Connection: "]);
  });
});
