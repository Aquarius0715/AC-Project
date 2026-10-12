// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { DataTable, type Col } from "@ac/web/components/ui";

afterEach(cleanup);

type Unit = { id: string; name: string; status: string };
const units: Unit[] = [{ id: "u-1", name: "Bedroom AC", status: "Online" }, { id: "u-2", name: "Office AC", status: "Offline" }];
const cols: Col<Unit>[] = [
  { key: "name", label: "Unit", render: (u) => <strong>{u.name}</strong> },
  { key: "status", label: "Status", render: (u) => u.status, hideBelow: "sm", className: "text-right" },
];
const cells = (row: HTMLElement) => within(row).getAllByRole("cell").map((c) => c.textContent);

// DataTable on TanStack Table and the shadcn/ui Table (DEC-75, IR403): the same columns, rows, row keys and keyboard
// access as before (IR294)
describe("DataTable", () => {
  it("renders a header row and one row per record, keyed and in order", () => {
    render(<DataTable cols={cols} rows={units} rowKey={(u) => u.id} />);
    const [head, ...body] = screen.getAllByRole("row");
    expect(within(head).getAllByRole("columnheader").map((c) => c.textContent)).toEqual(["Unit", "Status"]);
    expect(body.map(cells)).toEqual([["Bedroom AC", "Online"], ["Office AC", "Offline"]]);
    // a column's classes reach its header and its cells; hideBelow hides it on a narrow screen
    const status = [within(head).getAllByRole("columnheader")[1], within(body[0]).getAllByRole("cell")[1]];
    expect(status.map((c) => [c.classList.contains("max-sm:hidden"), c.classList.contains("text-right")])).toEqual([[true, true], [true, true]]);
    // a table that opens nothing has no focusable rows
    expect(body.map((r) => r.getAttribute("tabindex"))).toEqual([null, null]);
  });

  it("shows one No data row across every column when there are no records", () => {
    render(<DataTable cols={cols} rows={[]} rowKey={(u) => u.id} />);
    const rows = screen.getAllByRole("row");
    expect(rows).toHaveLength(2);
    const cell = within(rows[1]).getByRole("cell");
    expect([cell.textContent, cell.getAttribute("colspan")]).toEqual(["No data", "2"]);
  });

  it("opens a record by a click, Enter or Space, and marks the open one", () => {
    const open = vi.fn();
    render(<DataTable cols={cols} rows={units} rowKey={(u) => u.id} onRowClick={open} selectedKey="u-2" />);
    const [, bedroom, office] = screen.getAllByRole("row");
    expect([bedroom.getAttribute("tabindex"), office.getAttribute("tabindex")]).toEqual(["0", "0"]);
    expect([bedroom.getAttribute("aria-current"), office.getAttribute("aria-current"), office.getAttribute("data-state")]).toEqual([null, "true", "selected"]);
    fireEvent.click(within(bedroom).getByText("Bedroom AC"));
    fireEvent.keyDown(office, { key: "Enter" });
    fireEvent.keyDown(bedroom, { key: " " });
    // a key pressed on a control inside the row belongs to that control
    fireEvent.keyDown(within(office).getByText("Office AC"), { key: "Enter" });
    expect(open.mock.calls.map(([u]) => u.id)).toEqual(["u-1", "u-2", "u-1"]);
  });
});
