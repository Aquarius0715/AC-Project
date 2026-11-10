/** A copy of an object without one key: a cleared field error, or a field an answer leaves out. */
export function without<T extends object, K extends keyof T>(o: T, key: K): Omit<T, K> {
  const copy = { ...o };
  Reflect.deleteProperty(copy, key);
  return copy;
}
