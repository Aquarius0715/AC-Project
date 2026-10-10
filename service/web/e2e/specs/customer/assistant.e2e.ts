// FR-X02 / AT-X02 in API mode (IR306): the customer assistant reads voice.resolveIntent with the fixed grammar (D09) —
// help, a temperature question, and a change that is confirmed before anything is sent; cancelling sends nothing
// (AT-X02-E ④) and an unknown intent says so. It speaks its chosen language: switching to Bahasa Melayu discards the
// unconfirmed change and keeps the typed text (D09, AT-X01-B ③). Voice input follows the device's microphone consent
// from Preferences: off is text only, and turning it off while listening discards the request (AT-X02-E ③). An
// ambiguous room is asked, never guessed: the room, then the AC (Figma 09d, AT-X02-B). No command is sent.
import { test, expect } from "../../fixtures/test";

test("the assistant answers with the fixed grammar and confirms a change before sending", async ({ page }) => {
  await page.goto("/customer");
  await page.getByRole("button", { name: /Assistant/ }).click();
  const panel = page.getByRole("dialog", { name: /^(Assistant|Pembantu)$/ }); // the panel speaks its chosen language
  await expect(panel).toContainText("Idle");
  await panel.getByRole("button", { name: "“help”" }).click();
  await expect(panel).toContainText("Ask “temperature <room>”");
  await panel.getByRole("button", { name: "“temperature Bedroom”" }).click();
  await expect(panel).toContainText(/Bedroom AC (is \d|has no temperature reading|last read)/);
  // a change: the target, the current setting and the allowed range are shown first; Cancel sends nothing
  await panel.getByRole("button", { name: "“set Bedroom to 24 degrees”" }).click();
  await expect(panel.getByText("I understood:")).toBeVisible();
  await expect(panel).toContainText("Home A › 1F › Bedroom › Bedroom AC");
  await expect(panel).toContainText("Needs confirmation");
  await panel.getByRole("button", { name: "Cancel", exact: true }).click();
  await expect(panel).toContainText("Cancelled — nothing was sent.");
  // an unknown intent falls back to text input
  const message = panel.getByRole("textbox", { name: "Message" });
  await message.fill("make it cooler");
  await message.press("Enter");
  await expect(panel).toContainText("Action unavailable.");
  // the chosen language: a change waiting for confirmation is discarded, the typed text stays, Malay grammar answers
  await panel.getByRole("button", { name: "“set Bedroom to 24 degrees”" }).click();
  await expect(panel.getByText("I understood:")).toBeVisible();
  await message.fill("suhu Bedroom");
  await panel.getByRole("combobox", { name: "Language" }).selectOption("ms");
  await expect(panel.getByText("Saya faham:")).toHaveCount(0);
  const mesej = panel.getByRole("textbox", { name: "Mesej" });
  await expect(mesej).toHaveValue("suhu Bedroom");
  await mesej.press("Enter");
  await expect(panel).toContainText(/Bedroom AC (ialah \d|belum mempunyai bacaan suhu|kali terakhir)/);
});

test("voice input follows the device's microphone consent and falls back to text", async ({ page, context }) => {
  await page.goto("/customer");
  await page.getByRole("button", { name: /Assistant/ }).click();
  const panel = page.getByRole("dialog", { name: "Assistant" });
  // the demo transcript (no real microphone) goes through the same grammar and is confirmed before anything is sent
  await panel.getByRole("button", { name: "Voice input" }).click();
  await expect(panel).toContainText("Listening…");
  await panel.getByRole("button", { name: "Stop" }).click();
  await expect(panel.getByText("I understood:")).toBeVisible();
  await expect(panel).toContainText("You · voice");
  await panel.getByRole("button", { name: "Cancel", exact: true }).click();
  // the microphone is turned off in Preferences in another tab while listening: the voice request is discarded
  await panel.getByRole("button", { name: "Voice input" }).click();
  await expect(panel).toContainText("Listening…");
  const prefs = await context.newPage();
  const mic = prefs.getByRole("switch", { name: "Microphone consent" });
  try {
    await prefs.goto("/settings/preferences");
    await expect(mic).toHaveAttribute("aria-checked", "true");
    await mic.click();
    await expect(mic).toHaveAttribute("aria-checked", "false");
    await expect(panel).toContainText("Voice input stopped — the microphone was turned off. Nothing was sent.");
    await expect(panel.getByText("Listening…")).toHaveCount(0);
    // text only now: the voice button says why and hands over to the text box
    await expect(panel).toContainText("Text only — the microphone is off on this device");
    await panel.getByRole("button", { name: "Voice input" }).click();
    await expect(panel).toContainText("The microphone is off on this device, so voice input is unavailable.");
    await expect(panel.getByRole("textbox", { name: "Message" })).toBeFocused();
  } finally {
    if ((await mic.getAttribute("aria-checked")) === "false") await mic.click();
    await prefs.close();
  }
});

// The seed has no customer with two rooms of one name (AT-X02-B uses an acceptance patch), so the first answer is
// rewritten into candidates. The choice asks the Core API again with selectedUnitId, which accepts only a candidate of
// that request (IR65): the real unit answers, an invented one is refused.
test("an ambiguous room is asked, never guessed: the room, then the AC", async ({ page }) => {
  let real = "";
  await page.route("**/bff/ops/voice.resolveIntent", async (route) => {
    if ((route.request().postDataJSON() as { selectedUnitId?: string }).selectedUnitId) return route.continue();
    const response = await route.fetch();
    const json = (await response.json()) as { data: { unitId: string } };
    real = json.data.unitId;
    await route.fulfill({ response, json: { ...json, data: { kind: "candidates", candidates: [
      { unitId: real, pathLabel: "Home A > 1F > Bedroom > Bedroom AC" },
      { unitId: "00000000-0000-4000-8000-0000000000b2", pathLabel: "Home A > 1F > Bedroom > Bedroom AC" },
      { unitId: "00000000-0000-4000-8000-0000000000b3", pathLabel: "Office A > 2F > Bedroom > Office Bedroom AC" },
    ] } } });
  });
  await page.goto("/customer");
  await page.getByRole("button", { name: /Assistant/ }).click();
  const panel = page.getByRole("dialog", { name: "Assistant" });
  await panel.getByRole("button", { name: "“temperature Bedroom”" }).click();
  await expect(panel.getByText("Which Bedroom did you mean?")).toBeVisible();
  await expect(panel).toContainText("I found more than one. I won’t choose for you.");
  const next = panel.getByRole("button", { name: "Continue" });
  await expect(next).toBeDisabled(); // nothing is chosen for the user
  await panel.getByRole("radio", { name: /^Home A › 1F › Bedroom/ }).check();
  await expect(panel).toContainText("Then: which AC in Home A › 1F › Bedroom?");
  await expect(next).toBeDisabled();
  // the same name twice in one room: their IDs tell them apart (IR101)
  await panel.getByRole("radio", { name: new RegExp(`^Bedroom AC · ${real.slice(0, 8)}`) }).check();
  await next.click();
  await expect(panel).toContainText(/Bedroom AC (is \d|has no temperature reading|last read)/);
  // a room with one AC needs no second choice; a unit outside the request's candidates is refused (AT-X02-B ③)
  await panel.getByRole("button", { name: "“temperature Bedroom”" }).click();
  await panel.getByRole("radio", { name: /^Office A › 2F › Bedroom/ }).check();
  await expect(panel).toContainText("1 AC: Office Bedroom AC");
  await next.click();
  await expect(panel).toContainText("This item no longer exists or is outside your scope");
});
