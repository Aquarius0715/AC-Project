// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { LocationPicker } from "@ac/web/components/LocationPicker";
import type { SearchResult } from "@ac/web/lib/geocode";

afterEach(cleanup);

const show = (onSearch: (query: string) => Promise<SearchResult>, address = "") => {
  render(<LocationPicker value={null} onChange={vi.fn()} styleUrl={null} onSearch={onSearch} address={address} />);
  return screen.getByRole("textbox", { name: "Search an address" });
};
const enter = (box: HTMLElement) => { fireEvent.keyDown(box, { key: "Enter" }); };
const searchButton = () => screen.getByRole("button", { name: /Search/ });

// The address search (IR835): Enter keeps the button's guards, and a failed call frees the button again
describe("LocationPicker address search", () => {
  it("does not search on Enter for a query shorter than 2 characters", () => {
    const onSearch = vi.fn<(query: string) => Promise<SearchResult>>();
    enter(show(onSearch, " a "));
    expect(onSearch).not.toHaveBeenCalled();
    expect(screen.queryByRole("status")).toBeNull();
  });
  it("does not start a second search on Enter while one runs", async () => {
    const pending = Promise.withResolvers<SearchResult>();
    const onSearch = vi.fn<(query: string) => Promise<SearchResult>>(() => pending.promise);
    const box = show(onSearch, "Jalan Ampang");
    enter(box);
    enter(box);
    expect(onSearch).toHaveBeenCalledOnce();
    await act(async () => { pending.resolve({ ok: true, places: [] }); await Promise.resolve(); });
    expect(screen.getByRole("status").textContent).toBe("No place found for this address.");
    expect(searchButton().hasAttribute("disabled")).toBe(false);
  });
  it("frees the button and says the search is not available when the call itself fails", async () => {
    const onSearch = vi.fn<(query: string) => Promise<SearchResult>>(() => Promise.reject(new Error("network")));
    const box = show(onSearch, "Jalan Ampang");
    await act(async () => { enter(box); await Promise.resolve(); });
    expect(searchButton().hasAttribute("disabled")).toBe(false);
    expect(screen.getByRole("status").textContent).toBe("The address search is not available. Place the pin on the map or type it.");
  });
});
