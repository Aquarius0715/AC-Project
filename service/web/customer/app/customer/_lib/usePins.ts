"use client";
// The pins on screen (IR819): they start from the server's preferences, change at once on a tap and are saved with
// togglePinned; a refused save puts them back and says so.
import { useState } from "react";
import { togglePin } from "@ac/web/lib/pinnedUnits";
import { useToast } from "@ac/web/components/ui";
import { useT } from "@ac/web/components/I18n";
import { togglePinned } from "./pins";

export function usePins(initial: string[]): [string[], (id: string) => void] {
  const t = useT();
  const toast = useToast();
  const [pins, setPins] = useState(initial);
  const toggle = (id: string) => {
    const before = pins;
    setPins(togglePin(pins, id));
    void togglePinned(id).then((r) => {
      if (r.ok) setPins(r.value);
      else { setPins(before); toast(t("The pin was not saved — try again."), "warn"); }
    });
  };
  return [pins, toggle];
}
