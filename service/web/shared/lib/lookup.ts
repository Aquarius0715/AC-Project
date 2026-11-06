// Reading a table by a key that came from outside (an API value, a URL) without indexing an object by a variable.

/** lookup is the table's value for `key`, or undefined when the table has no entry of that name. */
export function lookup<V>(table: Readonly<Partial<Record<string, V>>>, key: string): V | undefined {
  for (const [name, value] of Object.entries(table)) if (name === key) return value;
  return undefined;
}

/** entryOf is the value of a table that has an entry for every key of its type (a key of that type is always found). */
export function entryOf<K extends string, V>(table: Readonly<Record<K, V>>, key: K): V {
  for (const [name, value] of Object.entries<V>(table)) if (name === key) return value;
  throw new Error(`no entry for ${key}`);
}
