// @vitest-environment jsdom
import { useState } from "react";
import { afterEach, describe, expect, it } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { CodeInput } from "@ac/web/components/ui";

afterEach(cleanup);

// A one-time code is shown by authenticator apps and in messages as "123 456" and copied with its space. The input keeps
// the digits and cuts them to its length itself, so it must not carry a maxlength: the browser would cut the pasted text
// to 6 characters first ("123 45") and the digits left would be five.
function Code() {
  const [value, setValue] = useState("");
  return <CodeInput label="Code" value={value} onChange={setValue} />;
}

describe("the one-time code input", () => {
  it("keeps the six digits of a code pasted with a space or a dash", () => {
    render(<Code />);
    const input = screen.getByLabelText<HTMLInputElement>("Code");
    fireEvent.change(input, { target: { value: "123 456" } });
    expect(input.value).toBe("123456");
    fireEvent.change(input, { target: { value: "654-321" } });
    expect(input.value).toBe("654321");
  });

  it("cuts a longer text to the length and drops what is not a digit", () => {
    render(<Code />);
    const input = screen.getByLabelText<HTMLInputElement>("Code");
    fireEvent.change(input, { target: { value: "12ab34567890" } });
    expect(input.value).toBe("123456");
  });

  it("does not let the browser cut a pasted text before the digits are picked out", () => {
    render(<Code />);
    expect(screen.getByLabelText("Code").hasAttribute("maxlength")).toBe(false);
  });
});
