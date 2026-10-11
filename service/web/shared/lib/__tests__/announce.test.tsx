// @vitest-environment jsdom
import { afterEach, describe, expect, it } from "vitest";
import { act, cleanup, render, screen } from "@testing-library/react";
import { Banner, ToastProvider, useToast } from "@ac/web/components/ui";

afterEach(cleanup);

function Pusher() {
  const toast = useToast();
  return <><button onClick={() => toast("Saved")}>ok</button><button onClick={() => toast("Not saved: CONFLICT", "crit")}>crit</button></>;
}

// IR362: critical errors are announced at once (UX-06), other results politely
describe("announcements", () => {
  it("makes an error banner an alert and other banners a status", () => {
    render(<><Banner tone="crit">Not submitted</Banner><Banner tone="warn">Window ends soon</Banner><Banner>Saved</Banner></>);
    expect(screen.getByRole("alert").textContent).toContain("Not submitted");
    expect(screen.getAllByRole("status").map((x) => x.textContent)).toEqual(["Window ends soon", "Saved"]);
    // each banner's icon is a Lucide drawing, hidden from assistive technology (DEC-74)
    expect([...screen.getByRole("alert").querySelectorAll("svg"), ...screen.getAllByRole("status").flatMap((x) => [...x.querySelectorAll("svg")])].map((x) => [x.getAttribute("aria-hidden"), x.classList.contains("lucide")])).toEqual([["true", true], ["true", true], ["true", true]]);
  });
  it("puts an error toast in the alert region and other toasts in the status region", () => {
    render(<ToastProvider><Pusher /></ToastProvider>);
    act(() => { screen.getByRole("button", { name: "ok" }).click(); screen.getByRole("button", { name: "crit" }).click(); });
    expect(screen.getByRole("alert").textContent).toBe("Not saved: CONFLICT");
    expect(screen.getByRole("status").textContent).toBe("Saved");
  });
});
