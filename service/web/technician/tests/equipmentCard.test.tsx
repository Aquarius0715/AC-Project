// @vitest-environment jsdom
import { useState } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import type { Draft, PhotoTag } from "@ac/web/lib/techJob";

// the English text with its {placeholders} filled, as the real translator does for the English display language
function fill(s: string, values: Record<string, string | number> = {}) {
  let text = s;
  for (const [k, x] of Object.entries(values)) text = text.split(`{${k}}`).join(String(x));
  return text;
}
vi.mock("@ac/web/components/I18n", () => ({ useT: () => fill }));

import { EquipmentCard } from "../app/technician/jobs/[id]/_components/equipment-card";

// the equipment check of an installation job (FR-T16, DEC-106): the model confirmation, the serial and one photo per
// tag; only a ready photo counts as added, and a read-only workspace has no upload controls. The harness keeps the
// draft in state, so what the card shows after an edit is what its edit function made of the draft.
interface Photo { id: string; status: string; tag: PhotoTag | null }
const base: Draft = { installation: { confirmed: false, serial: "" }, items: [], readings: [], parts: [], refrigerant: [], workText: "", nextAction: { kind: "none" }, attachmentIds: [] };
const noPhotos: Photo[] = [];
const addPhoto = vi.fn<(files: FileList | null, tag: PhotoTag) => void>();

function Harness({ start = base, isEditable = true, model = "FTKM35", photos = noPhotos, pending = false }: Readonly<{ start?: Draft; isEditable?: boolean; model?: string | null; photos?: Photo[]; pending?: boolean }>) {
  const [draft, setDraft] = useState(start);
  return <EquipmentCard draft={draft} edit={(fn) => { setDraft(fn); }} isEditable={isEditable} model={model} photos={photos} pending={pending} addPhoto={addPhoto} />;
}

const row = (label: string) => {
  const item = screen.getByText(label).closest("li");
  if (!item) throw new Error(`no row ${label}`);
  return within(item);
};
const box = () => screen.getByRole("checkbox");
const serial = () => screen.getByLabelText<HTMLInputElement>("Serial number");

afterEach(() => { cleanup(); addPhoto.mockReset(); });

describe("EquipmentCard", () => {
  it("renders nothing on a job that is not an installation", () => {
    const { container } = render(<Harness start={{ ...base, installation: null }} />);
    expect(container.childElementCount).toBe(0);
  });

  it("confirms the registered model and keeps the serial as typed", () => {
    render(<Harness />);
    expect(screen.getByText("The installed model is FTKM35")).toBeTruthy();
    expect(box().getAttribute("aria-disabled")).not.toBe("true");
    expect(box().getAttribute("aria-checked")).toBe("false");
    fireEvent.click(box());
    expect(box().getAttribute("aria-checked")).toBe("true");
    fireEvent.change(serial(), { target: { value: "SN-42 " } });
    expect(serial().value).toBe("SN-42 ");
    // the serial edit keeps the confirmation
    expect(box().getAttribute("aria-checked")).toBe("true");
  });

  it("cannot confirm a model the register does not have", () => {
    render(<Harness model={null} />);
    expect(screen.getByText("The installed model is —")).toBeTruthy();
    expect(box().getAttribute("aria-disabled")).toBe("true");
    fireEvent.click(box());
    expect(box().getAttribute("aria-checked")).toBe("false");
  });

  it("is read-only when the workspace is not editable", () => {
    render(<Harness isEditable={false} start={{ ...base, installation: { confirmed: true, serial: "SN-1" } }} />);
    expect(box().getAttribute("aria-disabled")).toBe("true");
    expect(box().getAttribute("aria-checked")).toBe("true");
    expect(serial().readOnly).toBe(true);
    expect(serial().value).toBe("SN-1");
    expect(screen.queryByText("Add photo")).toBeNull();
    expect(screen.queryByText("Replace")).toBeNull();
  });

  it("counts only a ready photo of the tag as added", () => {
    render(<Harness photos={[{ id: "p1", status: "ready", tag: "name_plate" }, { id: "p2", status: "pending", tag: "indoor_unit" }, { id: "p3", status: "ready", tag: null }]} />);
    expect(row("Name plate").getByText("Added")).toBeTruthy();
    expect(row("Name plate").getByText("Replace")).toBeTruthy();
    expect(row("Indoor unit").getByText("Missing")).toBeTruthy();
    expect(row("Indoor unit").getByText("Add photo")).toBeTruthy();
    expect(row("Outdoor unit").getByText("Missing")).toBeTruthy();
  });

  it("hands the chosen file to addPhoto with its tag and clears the input", () => {
    render(<Harness />);
    const input = screen.getByLabelText<HTMLInputElement>("Photo of the outdoor unit");
    const file = new File(["x"], "plate.png", { type: "image/png" });
    fireEvent.change(input, { target: { files: [file] } });
    expect(addPhoto).toHaveBeenCalledOnce();
    const [files, tag] = addPhoto.mock.calls[0] ?? [null, null];
    expect(tag).toBe("outdoor_unit");
    expect(files?.[0]).toBe(file);
    expect(input.value).toBe("");
  });

  it("dims the upload controls while a photo is being sent", () => {
    render(<Harness pending />);
    const control = row("Name plate").getByText("Add photo");
    expect(control.classList.contains("pointer-events-none")).toBe(true);
    expect(control.classList.contains("opacity-60")).toBe(true);
  });

  it("links to the device screens for the optional IoT device", () => {
    render(<Harness />);
    expect(screen.getByRole("link", { name: /Devices/u }).getAttribute("href")).toBe("/technician/devices");
  });
});
