// A page's URL parameters (Next.js searchParams), read one text value at a time.

/** SearchParams is what Next.js passes a page as its awaited searchParams. */
export type SearchParams = Readonly<Partial<Record<string, string | string[]>>>;

/** queryReader reads a parameter's text: undefined when it is missing, empty or given more than once. */
export function queryReader(params: SearchParams): (key: string) => string | undefined {
  const values = new Map(Object.entries(params));
  return (key) => {
    const value = values.get(key);
    return typeof value === "string" && value !== "" ? value : undefined;
  };
}
