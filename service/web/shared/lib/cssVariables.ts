import type { CSSProperties } from "react";

/** Inline CSS custom properties (a style prop takes them only as CSSProperties). */
export function cssVariables(variables: Readonly<Record<`--${string}`, string | number | undefined>>): CSSProperties {
  return Object.assign({}, variables);
}
