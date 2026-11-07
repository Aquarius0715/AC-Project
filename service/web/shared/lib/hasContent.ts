import type { ReactNode } from "react";

/** hasContent is whether a node renders anything: it is not null, undefined, a boolean or an empty string. A number does (0
shows as "0"), which is what `{count && <X />}` leaks; a slot that can hold a number states it. */
export function hasContent(node: ReactNode): boolean {
  return node !== null && node !== undefined && typeof node !== "boolean" && node !== "";
}
