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

/** recover is `read`'s value, or what `handle` returns for the error it fails with; `handle` throws the errors it does
not recover from. It is `read.catch(handle)` as a function of an awaited read. */
export async function recover<T, R>(read: PromiseLike<T>, handle: (error: unknown) => R | PromiseLike<R>): Promise<T | R> {
  try {
    return await read;
  } catch (error) {
    return handle(error);
  }
}
