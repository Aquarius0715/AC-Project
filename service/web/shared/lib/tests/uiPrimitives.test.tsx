// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import {
  AsyncSearchSelect, Badge, Banner, Card, Choice, ConnBadge, DataTable, EmptyState, ErrorState, Kpi, ListRow, OnOffBadge, PageHead, ScrollX, SearchSelect, SeverityBadge, Stat, Steps,
  SummaryList, Tabs, Timeline, type Col,
} from "@ac/web/components/ui";
import type { PickerOption } from "@ac/web/lib/pickerOption";

// The shared screen parts (components/ui.tsx): what each one shows and announces for the props it takes — the badges'
// words, a card's and an empty state's heading levels, a banner's role, a KPI's link, the steps done and to do, the
// tabs, the choice buttons with a coming-soon option, the search selects (local and server-side), the table and the
// list row.

afterEach(cleanup);

describe("badges", () => {
  it("words every connection, severity and on/off state", () => {
    const { container } = render(<>
      <ConnBadge s="online" /><ConnBadge s="offline" /><ConnBadge s="offline" offlineTone="crit" /><ConnBadge s="error" /><ConnBadge s="connecting" />
      <SeverityBadge s="critical" /><SeverityBadge s="warning" /><SeverityBadge s="normal" /><OnOffBadge on /><OnOffBadge on={false} />
      <Badge tone="ok" icon="check">Done</Badge>
    </>);
    expect([...container.children].map((el) => el.textContent)).toStrictEqual([
      "Connection: Online", "Connection: Offline", "Connection: Offline", "Connection: Error", "Connection: Connecting", "Critical", "Warning", "Info", "On", "Off", "Done",
    ]);
    const [, offline, offlineRed] = container.children;
    expect([offline?.className, offlineRed?.className]).toStrictEqual([expect.stringMatching(/text-unknown/), expect.stringMatching(/text-crit/)]); // the technician app's red Offline
  });
});

describe("layout parts", () => {
  it("titles a card as a level-2 heading and an empty state under it as level 3, on its own as level 2", () => {
    render(<>
      <Card title="Units" sub="2 units" action={<button type="button">Add</button>} tone="crit"><EmptyState title="No units" action={<button type="button">Import</button>}>Add one.</EmptyState></Card>
      <EmptyState title="Nothing here" />
    </>);
    expect(screen.getByRole("heading", { level: 2, name: "Units" })).toBeDefined();
    expect(screen.getByRole("heading", { level: 3, name: "No units" })).toBeDefined();
    expect(screen.getByRole("heading", { level: 2, name: "Nothing here" })).toBeDefined();
    expect([screen.getByText("2 units").textContent, screen.getByText("Add one.").textContent]).toStrictEqual(["2 units", "Add one."]);
    expect(screen.getAllByRole("button").map((b) => b.textContent)).toStrictEqual(["Add", "Import"]);
  });

  it("leaves out a card header with neither a title nor an action, and a page head's empty sub", () => {
    const { container } = render(<><Card pad={false}>Body</Card><PageHead title="Jobs" /></>);
    expect(container.querySelectorAll("h2")).toHaveLength(0);
    expect(screen.getByRole("heading", { level: 1, name: "Jobs" }).parentElement?.querySelector("p")).toBeNull();
    cleanup();
    render(<PageHead title="Jobs" sub="12 open" action={<button type="button">New</button>} />);
    expect([screen.getByText("12 open").tagName, screen.getByRole("button").textContent]).toStrictEqual(["P", "New"]);
  });

  it("announces a critical banner at once and the others as a status, each with its tone's icon", () => {
    render(<><Banner tone="crit">Failed</Banner><Banner tone="warn" action={<button type="button">Fix</button>}>Check it</Banner><Banner>Note</Banner></>);
    expect(screen.getByRole("alert").textContent).toBe("Failed");
    expect(screen.getAllByRole("status").map((s) => s.textContent)).toStrictEqual(["Check itFix", "Note"]);
  });

  it("retries from an error state", () => {
    const onRetry = vi.fn();
    render(<ErrorState title="Could not load">The server did not answer.</ErrorState>);
    expect(screen.getByRole("heading", { level: 3, name: "Could not load" })).toBeDefined();
    cleanup();
    render(<ErrorState title="Could not load" onRetry={onRetry} />);
    fireEvent.click(screen.getByRole("button", { name: /Retry/ }));
    expect(onRetry).toHaveBeenCalledOnce();
  });

  it("shows a KPI's value, unit, sub line and link, and a stat's sub line only when given", () => {
    render(<Kpi label="Open jobs" value={7} unit="jobs" icon="warning" badge={<span>new</span>} sub="2 overdue" href="/admin/jobs" link="View jobs" tone="warn" />);
    expect(screen.getByRole("link", { name: /View jobs/ }).getAttribute("href")).toBe("/admin/jobs");
    expect(screen.getByText("7").className).toMatch(/text-warn/);
    expect(["jobs", "2 overdue", "new"].map((x) => screen.getByText(x).textContent)).toStrictEqual(["jobs", "2 overdue", "new"]);
    cleanup();
    const { container } = render(<><Kpi label="Units" value={3} tone="crit" href="/x" /><Kpi label="Online" value={2} tone="ok" /><Stat label="Energy" value="12 kWh" /><Stat label="Cost" value="5" sub="MYR" /></>);
    expect(container.querySelectorAll("a")).toHaveLength(0); // a link needs both its address and its words
    expect([screen.getByText("3").className, screen.getByText("2").className]).toStrictEqual([expect.stringMatching(/text-crit/), expect.stringMatching(/text-ok/)]);
    expect(screen.getByText("MYR").textContent).toBe("MYR");
  });

  it("names a scrolling part as a region only when labelled", () => {
    render(<><ScrollX label="Jobs table"><p>wide</p></ScrollX><ScrollX><p>plain</p></ScrollX></>);
    expect(screen.getByRole("region", { name: "Jobs table" }).textContent).toBe("wide");
    expect(screen.getAllByRole("region")).toHaveLength(1);
  });

  it("marks the steps done with a check, numbers the current and later ones", () => {
    render(<Steps steps={["Choose", "Check", "Confirm"]} current={1} />);
    const items = screen.getAllByRole("listitem");
    expect(items.map((li) => li.textContent)).toStrictEqual(["Choose", "2Check", "3Confirm"]); // the done step shows its check icon, not a number
    expect([items[0]?.className, items[1]?.className, items[2]?.className].map((c = "") => /text-(ok|primary|muted-foreground)/.exec(c)?.[1])).toStrictEqual(["ok", "primary", "muted-foreground"]);
  });

  it("lists a summary and a timeline with its details and tones", () => {
    render(<><SummaryList cols={2} items={[["Customer", "Tan"], ["Units", 2]]} /><Timeline items={[{ time: "09:00", title: "Assigned", detail: "to Kamal", tone: "ok" }, { time: "10:00", title: "Started" }]} /></>);
    expect(screen.getAllByRole("term").map((d) => d.textContent)).toStrictEqual(["Customer", "Units"]);
    expect(screen.getAllByRole("definition").map((d) => d.textContent)).toStrictEqual(["Tan", "2"]);
    const [first, second] = screen.getAllByRole("listitem");
    expect([first?.textContent, second?.textContent]).toStrictEqual(["09:00Assignedto Kamal", "10:00Started"]);
    expect([first?.querySelector(".bg-ok"), second?.querySelector(".bg-primary")].every(Boolean)).toBe(true);
  });
});

describe("tabs and choices", () => {
  it("switches tabs and shows each count", () => {
    const onChange = vi.fn();
    render(<Tabs tabs={[{ id: "open", label: "Open", count: 3 }, { id: "done", label: "Done" }]} value="open" onChange={onChange} />);
    expect(screen.getAllByRole("tab").map((t) => t.textContent)).toStrictEqual(["Open3", "Done"]);
    fireEvent.click(screen.getByRole("tab", { name: "Done" }));
    expect(onChange).toHaveBeenCalledExactlyOnceWith("done");
  });

  it("chooses another option, never the chosen one again or one coming soon, which a saved rule keeps showing", () => {
    const onChange = vi.fn();
    const options = [{ id: "a", label: "Schedule" }, { id: "b", label: "Weather" }, { id: "c", label: "Voice", soon: true }];
    render(<Choice options={options} value="a" onChange={onChange} />);
    fireEvent.click(screen.getByRole("button", { name: "Schedule" }));
    fireEvent.click(screen.getByRole("button", { name: "Weather" }));
    expect(onChange).toHaveBeenCalledExactlyOnceWith("b");
    expect(screen.getByRole<HTMLButtonElement>("button", { name: /Voice/ }).textContent).toBe("VoiceComing soon");
    cleanup();
    render(<Choice options={options} value="c" onChange={onChange} />);
    expect(screen.getByRole("button", { name: "Voice" }).getAttribute("aria-pressed")).toBe("true"); // the saved value, without the note
  });
});

describe("the search selects", () => {
  const options = Array.from({ length: 25 }, (_, i) => ({ id: `u-${String(i)}`, label: `Unit ${String(i).padStart(2, "0")}` }));

  it("narrows a long list as one types, says how many more there are, and chooses one", async () => {
    const onChange = vi.fn();
    render(<SearchSelect value="u-1" onChange={onChange} options={options} label="Unit" limit={5} />);
    const input = screen.getByRole("combobox", { name: "Unit" });
    expect(input).toHaveProperty("value", "Unit 01");
    fireEvent.keyDown(input, { key: "ArrowDown" });
    expect(await screen.findByText("20 more — type to narrow the list")).toBeDefined(); // its own label lists everything
    fireEvent.change(input, { target: { value: "unit 2" } });
    await waitFor(() => { expect(within(screen.getByRole("listbox", { name: "Unit" })).getAllByRole("option").map((o) => o.textContent)).toStrictEqual(["Unit 20", "Unit 21", "Unit 22", "Unit 23", "Unit 24"]); });
    fireEvent.click(screen.getByRole("option", { name: "Unit 21" }));
    expect(onChange).toHaveBeenCalledExactlyOnceWith("u-21");
    fireEvent.change(input, { target: { value: "zzz" } });
    fireEvent.keyDown(input, { key: "ArrowDown" });
    expect(await screen.findByText("No match — try another name")).toBeDefined();
  });

  it("asks the server after a pause, shows each option's details or why it cannot be chosen, and says when the search failed", async () => {
    const found: PickerOption[] = [{ id: "d-1", label: "AC-001", detail: "Tan · Home", version: 1, unitId: null, blocked: null }, { id: "d-2", label: "AC-002", detail: "", version: 1, unitId: "u-1", blocked: "Bedroom AC" }];
    const search = vi.fn((q: string) => (q === "fail" ? Promise.reject(new Error("down")) : Promise.resolve(found)));
    const onChange = vi.fn();
    render(<AsyncSearchSelect value={null} onChange={onChange} search={search} label="Device" />);
    const input = screen.getByRole("combobox", { name: "Device" });
    fireEvent.change(input, { target: { value: "AC" } });
    await waitFor(() => { expect(search).toHaveBeenLastCalledWith("AC"); });
    fireEvent.keyDown(input, { key: "ArrowDown" });
    expect(await screen.findByText("Tan · Home")).toBeDefined();
    expect(screen.getByText("On Bedroom AC — move it from its page")).toBeDefined();
    expect(screen.getByRole("option", { name: /AC-002/ }).getAttribute("aria-disabled")).toBe("true");
    fireEvent.click(screen.getByRole("option", { name: /AC-001/ }));
    expect(onChange).toHaveBeenCalledWith(found[0]);
    cleanup();
    render(<AsyncSearchSelect value={null} onChange={onChange} search={search} label="Device" />);
    const again = screen.getByRole("combobox", { name: "Device" });
    fireEvent.change(again, { target: { value: "fail" } });
    await waitFor(() => { expect(search).toHaveBeenLastCalledWith("fail"); });
    fireEvent.keyDown(again, { key: "ArrowDown" });
    expect(await screen.findByText("The search failed — try again")).toBeDefined();
  });

  it("searches for everything while the input shows the chosen option", async () => {
    const search = vi.fn(() => Promise.resolve<PickerOption[]>([]));
    render(<AsyncSearchSelect value={{ id: "d-1", label: "AC-001", detail: "", version: 1, unitId: null, blocked: null }} onChange={vi.fn()} search={search} label="Device" />);
    await waitFor(() => { expect(search).toHaveBeenCalledWith(""); });
  });
});

describe("the table and list rows", () => {
  interface Row { id: string; name: string }
  const cols: Col<Row>[] = [{ key: "name", label: "Name", render: (r) => r.name, hideBelow: "md" }, { key: "act", label: "", render: (r) => <button type="button">Open {r.name}</button> }];

  it("opens a row by click, Enter or Space, marks the selected one, and says when there are no rows", () => {
    const onRowClick = vi.fn();
    render(<DataTable cols={cols} rows={[{ id: "r1", name: "Alpha" }, { id: "r2", name: "Beta" }]} rowKey={(r) => r.id} onRowClick={onRowClick} selectedKey="r2" />);
    expect(screen.getAllByRole("columnheader").map((h) => h.textContent)).toStrictEqual(["Name", "Actions"]);
    const rows = screen.getAllByRole("row").slice(1);
    expect(rows.map((r) => r.getAttribute("aria-current"))).toStrictEqual([null, "true"]);
    fireEvent.click(rows[0] ?? document.body);
    fireEvent.keyDown(rows[1] ?? document.body, { key: "Enter" });
    fireEvent.keyDown(rows[1] ?? document.body, { key: " " });
    fireEvent.keyDown(rows[1] ?? document.body, { key: "a" });
    fireEvent.keyDown(screen.getByRole("button", { name: "Open Beta" }), { key: "Enter" }); // a key on the row's own button is the button's
    expect(onRowClick.mock.calls.map((c: unknown[]) => c[0])).toStrictEqual([{ id: "r1", name: "Alpha" }, { id: "r2", name: "Beta" }, { id: "r2", name: "Beta" }]);
    cleanup();
    render(<DataTable cols={cols} rows={[]} rowKey={(r) => r.id} />);
    expect(screen.getByText("No data")).toBeDefined();
  });

  it("makes a list row a link with an address, otherwise a pressed button", () => {
    const onClick = vi.fn();
    render(<><ListRow href="/admin/units/u-1">Unit 1</ListRow><ListRow selected onClick={onClick}>Unit 2</ListRow></>);
    expect(screen.getByRole("link", { name: "Unit 1" }).getAttribute("href")).toBe("/admin/units/u-1");
    fireEvent.click(screen.getByRole("button", { name: "Unit 2", pressed: true }));
    expect(onClick).toHaveBeenCalledOnce();
  });
});
