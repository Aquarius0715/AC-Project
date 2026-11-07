// @vitest-environment jsdom
import { useState } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { DataTable, type Col } from "@ac/web/components/ui";
import { must } from "./must";

const BEDROOM_AC = "Bedroom AC";

afterEach(cleanup);

interface Unit { id: string; name: string; status: string }
const units: Unit[] = [{ id: "u-1", name: BEDROOM_AC, status: "Online" }, { id: "u-2", name: "Office AC", status: "Offline" }];
const cols: Col<Unit>[] = [
  { key: "name", label: "Unit", render: (u) => <strong>{u.name}</strong> },
  { key: "status", label: "Status", render: (u) => u.status, hideBelow: "sm", className: "text-right" },
];
const cells = (row: HTMLElement) => within(row).getAllByRole("cell").map((c) => c.textContent);

// DataTable on TanStack Table and the shadcn/ui Table (DEC-75, IR403): the same columns, rows, row keys and keyboard
// access as before (IR294)
function Page() {
  const [n, setN] = useState(0);
  return (
    <>
      <p>Opened {n}</p>
      <DataTable rows={units} rowKey={(u) => u.id} cols={[{ key: "a", label: "Actions", render: (u) => <button type="button" onClick={() => { setN(n + 1); }}>Edit {u.name}</button> }]} />
    </>
  );
}

describe("DataTable", () => {
  it("renders a header row and one row per record, keyed and in order", () => {
    render(<DataTable cols={cols} rows={units} rowKey={(u) => u.id} />);
    const [head, ...body] = screen.getAllByRole("row");
    expect(within(must(head)).getAllByRole("columnheader").map((c) => c.textContent)).toEqual(["Unit", "Status"]);
    expect(body.map((item) => cells(item))).toEqual([[BEDROOM_AC, "Online"], ["Office AC", "Offline"]]);
    // a column's classes reach its header and its cells; hideBelow hides it on a narrow screen
    const status = [within(must(head)).getAllByRole("columnheader")[1], within(must(body[0])).getAllByRole("cell")[1]];
    expect(status.map((c) => [c?.classList.contains("max-sm:hidden"), c?.classList.contains("text-right")])).toEqual([[true, true], [true, true]]);
    // a table that opens nothing has no focusable rows
    expect(body.map((r) => r.getAttribute("tabindex"))).toEqual([null, null]);
  });

  it("shows one No data row across every column when there are no records", () => {
    render(<DataTable cols={cols} rows={[]} rowKey={(u) => u.id} />);
    const rows = screen.getAllByRole("row");
    expect(rows).toHaveLength(2);
    const cell = within(must(rows[1])).getByRole("cell");
    expect([cell.textContent, cell.getAttribute("colspan")]).toEqual(["No data", "2"]);
  });

  it("opens a record by a click, Enter or Space, and marks the open one", () => {
    const open = vi.fn<(unit: (typeof units)[number]) => void>();
    render(<DataTable cols={cols} rows={units} rowKey={(u) => u.id} onRowClick={open} selectedKey="u-2" />);
    const [, bedroom, office] = screen.getAllByRole("row");
    expect([bedroom?.getAttribute("tabindex"), office?.getAttribute("tabindex")]).toEqual(["0", "0"]);
    expect([bedroom?.getAttribute("aria-current"), office?.getAttribute("aria-current"), office?.dataset.state]).toEqual([null, "true", "selected"]);
    fireEvent.click(within(must(bedroom)).getByText(BEDROOM_AC));
    fireEvent.keyDown(must(office), { key: "Enter" });
    fireEvent.keyDown(must(bedroom), { key: " " });
    // a key pressed on a control inside the row belongs to that control
    fireEvent.keyDown(within(must(office)).getByText("Office AC"), { key: "Enter" });
    expect(open.mock.calls.map(([u]) => u.id)).toEqual(["u-1", "u-2", "u-1"]);
  });

  // IR620: pages write their columns inline, so every render brings new render functions; the cells keep their
  // elements all the same — the focus stays on a row's button, and a dialog it opened can give the focus back to it
  it("keeps a cell's controls across renders of the page", () => {
    render(<Page />);
    const edit = screen.getByRole("button", { name: "Edit Bedroom AC" });
    edit.focus();
    fireEvent.click(edit);
    expect(screen.getByText("Opened 1")).toBeTruthy();
    expect(screen.getByRole("button", { name: "Edit Bedroom AC" })).toBe(edit);
    expect(document.activeElement).toBe(edit);
  });
});
