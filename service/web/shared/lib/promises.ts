// Promise helpers for server and browser code.

/** orNull is `read`'s value, or null when it fails for any reason: for optional extras a page shows only when it
can read them. */
export async function orNull<T>(read: PromiseLike<T>): Promise<T | null> {
  try {
    return await read;
  } catch {
    return null;
  }
}
