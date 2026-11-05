/** A list known to hold at least one item, such as a fixture or a fixed choice list: its first item needs no check. */
export type NonEmpty<T> = [T, ...T[]];
export const nonEmpty = <T>(items: NonEmpty<T>): NonEmpty<T> => items;
