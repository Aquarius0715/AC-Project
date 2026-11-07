"use client";

import { useState } from "react";

/** useOpenedAt is the time (ms) this component first rendered, the same for its whole life: a render that compares dates
with "now" reads it and stays pure. */
export function useOpenedAt(): number {
  return useState(Date.now)[0];
}
