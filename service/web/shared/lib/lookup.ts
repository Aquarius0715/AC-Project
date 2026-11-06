// Reading a table by a key that came from outside (an API value, a URL) without indexing an object by a variable.

/** lookup is the table's value for `key`, or undefined when the table has no entry of that name. */
export function lookup<V>(table: Readonly<Partial<Record<string, V>>>, key: string): V | undefined {
  for (const [name, value] of Object.entries(table)) if (name === key) return value;
  return undefined;
}
