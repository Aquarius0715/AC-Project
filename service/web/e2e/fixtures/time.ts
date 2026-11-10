// Choosing a date and a time in the time pickers (IR638): the date is typed into the date input, the hour and the
// minute are chosen in their selects. `names` gives the control names in another display language.
import type { Locator } from "@playwright/test";
import { must } from "./must";

type Names = Readonly<{ date: string; hour: string; minute: string }>;
const ENGLISH: Names = { date: "Date", hour: "Hour", minute: "Minute" };

/** A time of day, "HH:mm", in the hour and minute selects of `picker`. */
async function pickTime(picker: Locator, value: string, names: Pick<Names, "hour" | "minute"> = ENGLISH): Promise<void> {
  const [hour, minute] = value.split(":", 2);
  await picker.getByRole("combobox", { name: names.hour }).selectOption(must(hour, "an hour"));
  await picker.getByRole("combobox", { name: names.minute }).selectOption(must(minute, "a minute"));
}

/** A date and time, "YYYY-MM-DDTHH:mm", in the picker. */
export async function pickDateTime(picker: Locator, value: string, names: Names = ENGLISH): Promise<void> {
  const [date, time] = value.split("T", 2);
  await picker.getByLabel(names.date).fill(must(date, "a date"));
  await pickTime(picker, must(time, "a time"), names);
}
