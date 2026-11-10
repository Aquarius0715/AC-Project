"use client";

import { useT } from "@ac/web/components/I18n";
import { Input } from "@ac/web/components/ui/input";
import { NativeSelect } from "@ac/web/components/ui/native-select";
import { HOURS, joinDateTime, joinTime, minuteOptions, splitDateTime, splitTime } from "@ac/web/lib/timeParts";
import { cn } from "@ac/web/lib/utils";

// Times are chosen, not typed (IR638): an hour and a minute in two native selects (the platform's own picker on a phone),
// the minutes a quarter of an hour apart. The value is the string the native time / datetime-local inputs had, so the
// forms and their validators are unchanged. The picker keeps no state of its own: it shows `value` and reports a change.

type PickerProps = Readonly<{
  value: string;
  onChange: (v: string) => void;
  /** On the first control, so a failed save can take focus to the field (focusField). */
  id?: string;
  disabled?: boolean;
  /** Names the pair when it stands outside a Field; inside a Field the Field's group already has the name. */
  "aria-label"?: string;
  "aria-invalid"?: boolean;
  className?: string;
}>;

type TimeProps = PickerProps & Readonly<{
  /** An optional time: the hour list starts with an empty choice, and clearing it clears the value. */
  allowEmpty?: boolean;
}>;

function TimeControls({ value, onChange, id, disabled, allowEmpty, invalid, locked }: Readonly<{
  value: string; onChange: (v: string) => void; id?: string; disabled?: boolean; allowEmpty?: boolean; invalid?: boolean; locked?: boolean;
}>) {
  const t = useT();
  const { hour, minute } = splitTime(value);
  // a value that is not a time yet (empty or unfinished) shows no hour; a required time then starts at the first hour on choosing
  const needsEmpty = hour === "" || allowEmpty === true;
  const off = disabled === true || locked === true;
  return (
    <>
      <NativeSelect aria-label={t("Hour")} id={id} aria-invalid={invalid} disabled={off} value={hour}
        onChange={(e) => { onChange(joinTime(e.target.value, minute)); }} className="min-w-0 flex-1">
        {needsEmpty && <option value="">--</option>}
        {HOURS.map((h) => <option key={h} value={h}>{h}</option>)}
      </NativeSelect>
      <span aria-hidden="true" className="text-sm text-muted-foreground">:</span>
      <NativeSelect aria-label={t("Minute")} aria-invalid={invalid} disabled={off || hour === ""} value={hour === "" ? "" : minute}
        onChange={(e) => { onChange(joinTime(hour, e.target.value)); }} className="min-w-0 flex-1">
        {hour === "" && <option value="">--</option>}
        {minuteOptions(minute).map((m) => <option key={m} value={m}>{m}</option>)}
      </NativeSelect>
    </>
  );
}

/** A time of day, "HH:mm" (empty: none). */
export function TimeSelect({ value, onChange, id, disabled, allowEmpty, "aria-label": label, "aria-invalid": invalid, className }: TimeProps) {
  return (
    <div role={label ? "group" : undefined} aria-label={label} className={cn("flex min-w-0 items-center gap-1.5", className)}>
      <TimeControls value={value} onChange={onChange} id={id} disabled={disabled} allowEmpty={allowEmpty} invalid={invalid} />
    </div>
  );
}

type DateTimeProps = PickerProps & Readonly<{
  /** The earliest moment, as the native input took it: only its day limits the date picker; the form's validator keeps the time of day. */
  min?: string;
  /** The time of day a chosen date starts with. */
  defaultTime?: string;
}>;

/** A date and a time of day, "YYYY-MM-DDTHH:mm" (empty: none). The time can be chosen once a date is. */
export function DateTimeSelect({ value, onChange, id, disabled, min, defaultTime, "aria-label": label, "aria-invalid": invalid, className }: DateTimeProps) {
  const t = useT();
  const { date, time } = splitDateTime(value);
  return (
    <div role={label ? "group" : undefined} aria-label={label} className={cn("flex min-w-0 flex-wrap items-center gap-1.5", className)}>
      <Input type="date" aria-label={t("Date")} id={id} aria-invalid={invalid} disabled={disabled} min={min?.slice(0, 10)} value={date}
        onChange={(e) => { onChange(joinDateTime(e.target.value, time, defaultTime)); }} className="w-auto min-w-0 flex-[2_1_9rem]" />
      <div className="flex min-w-0 flex-[1_1_8rem] items-center gap-1.5">
        <TimeControls value={time} onChange={(v) => { onChange(joinDateTime(date, v, defaultTime)); }} disabled={disabled} invalid={invalid} locked={date === ""} />
      </div>
    </div>
  );
}
