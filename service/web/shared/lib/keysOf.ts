/** The keys of a record, typed as its key type (Object.keys gives plain strings). */
export function keysOf<K extends string>(record: Readonly<Record<K, unknown>>): K[] {
  return Object.keys(record).filter((key): key is K => Object.hasOwn(record, key));
}
