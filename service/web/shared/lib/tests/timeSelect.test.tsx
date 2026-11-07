// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { Field } from "@ac/web/components/ui";
import { DateTimeSelect, TimeSelect } from "@ac/web/components/TimeSelect";

const ARIA_INVALID = "aria-invalid";
afterEach(cleanup);

describe("TimeSelect", () => {
  it("shows the hour and the minute of the value", () => {
    render(<TimeSelect value="09:30" onChange={vi.fn()} />);
    expect(screen.getByRole<HTMLSelectElement>("combobox", { name: "Hour" }).value).toBe("09");
    expect(screen.getByRole<HTMLSelectElement>("combobox", { name: "Minute" }).value).toBe("30");
  });
  it("reports the whole time when the hour or the minute is chosen", () => {
    const onChange = vi.fn();
    render(<TimeSelect value="09:30" onChange={onChange} />);
    fireEvent.change(screen.getByRole("combobox", { name: "Hour" }), { target: { value: "14" } });
    expect(onChange).toHaveBeenLastCalledWith("14:30");
    fireEvent.change(screen.getByRole("combobox", { name: "Minute" }), { target: { value: "45" } });
    expect(onChange).toHaveBeenLastCalledWith("09:45");
  });
  it("shows a stored minute off the quarter hours as it is", () => {
    render(<TimeSelect value="07:10" onChange={vi.fn()} />);
    expect(screen.getByRole<HTMLSelectElement>("combobox", { name: "Minute" }).value).toBe("10");
  });
  it("lets the minute be chosen only after an hour, and starts it at :00", () => {
    const onChange = vi.fn();
    render(<TimeSelect value="" onChange={onChange} />);
    expect(screen.getByRole<HTMLSelectElement>("combobox", { name: "Minute" }).disabled).toBe(true);
    fireEvent.change(screen.getByRole("combobox", { name: "Hour" }), { target: { value: "08" } });
    expect(onChange).toHaveBeenLastCalledWith("08:00");
  });
  it("clears an optional time when the hour is emptied", () => {
    const onChange = vi.fn();
    render(<TimeSelect value="08:15" allowEmpty onChange={onChange} />);
    fireEvent.change(screen.getByRole("combobox", { name: "Hour" }), { target: { value: "" } });
    expect(onChange).toHaveBeenLastCalledWith("");
  });
});

describe("DateTimeSelect", () => {
  it("keeps the time locked until a date is chosen, then starts it at the default time", () => {
    const onChange = vi.fn();
    render(<DateTimeSelect value="" onChange={onChange} defaultTime="09:00" />);
    expect(screen.getByRole<HTMLSelectElement>("combobox", { name: "Hour" }).disabled).toBe(true);
    fireEvent.change(screen.getByLabelText("Date"), { target: { value: "2026-09-01" } });
    expect(onChange).toHaveBeenLastCalledWith("2026-09-01T09:00");
  });
  it("changes the time of the chosen date and clears the value with the date", () => {
    const onChange = vi.fn();
    render(<DateTimeSelect value="2026-09-01T09:30" onChange={onChange} />);
    fireEvent.change(screen.getByRole("combobox", { name: "Hour" }), { target: { value: "16" } });
    expect(onChange).toHaveBeenLastCalledWith("2026-09-01T16:30");
    fireEvent.change(screen.getByLabelText("Date"), { target: { value: "" } });
    expect(onChange).toHaveBeenLastCalledWith("");
  });
});

// IR573 / IR638: a picker in a Field is a named group; an error marks each of its controls invalid.
describe("a picker in a form field", () => {
  it("is a group named by the label, and an error marks every control invalid", () => {
    render(<Field label="Valid from" error="Choose a time"><DateTimeSelect value="2026-09-01T09:30" onChange={vi.fn()} /></Field>);
    expect(screen.getByRole("group", { name: "Valid from" })).toBeTruthy();
    for (const c of [screen.getByLabelText("Date"), screen.getByRole("combobox", { name: "Hour" }), screen.getByRole("combobox", { name: "Minute" })]) expect(c.getAttribute(ARIA_INVALID)).toBe("true");
  });
  it("leaves the controls unmarked without an error", () => {
    render(<Field label="Start"><TimeSelect value="09:30" onChange={vi.fn()} /></Field>);
    expect(screen.getByRole("combobox", { name: "Hour" }).getAttribute(ARIA_INVALID)).toBeNull();
  });
});
