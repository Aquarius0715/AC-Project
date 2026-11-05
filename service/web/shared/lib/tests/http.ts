/** The body a fetch call sent, as text: a form or a JSON text; "" for none or another kind of body. */
export function bodyText(init: RequestInit | undefined): string {
  const body = init?.body;
  if (body instanceof URLSearchParams) return body.toString();
  return typeof body === "string" ? body : "";
}

/** The form a fetch mock's nth call sent, read from its arguments without a cast. */
export function sentForm(calls: unknown[][], n = 0): URLSearchParams {
  const init: unknown = calls.at(n)?.at(1);
  return new URLSearchParams(typeof init === "object" && init !== null ? bodyText(init) : "");
}

/** The URL and options of a fetch mock's nth call, read from its arguments without a cast. */
export function sentCall(calls: unknown[][], n = 0): [string, RequestInit] {
  const call = calls.at(n);
  const url: unknown = call?.at(0);
  if (typeof url !== "string") throw new TypeError(`call ${String(n)} of the fetch mock has no URL`);
  const init: unknown = call?.at(1);
  return [url, typeof init === "object" && init !== null ? init : {}];
}
