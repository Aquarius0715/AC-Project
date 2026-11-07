// REST requests of Core API operations (IR222): the web apps keep calling operations by name with their catalogued
// input, and this turns a call into the request of one of the operation's routes (routes.gen.ts, generated from the
// operation catalog). The rules mirror the Core API's bindings (service/api/internal/ops/rest.go):
//   - the route whose path parameters the input holds (an item route before a collection route, so a save with an id
//     is PUT /…/{id} and without one POST /…), and whose fixed field the input matches (action=approve → /approve);
//   - path parameters are taken out of the input ({target.kind} from a nested field) and percent-encoded;
//   - GET and DELETE send the rest as the query string: the paging query and the filters by their own names,
//     sort as field:direction, nested fields as object.field, lists as repeated parameters (an empty list as one
//     empty value), null and undefined left out;
//   - POST, PUT and PATCH send the rest as the JSON body.
// Pure code: the DAL (server) and the BFF relay share it; Vitest covers it.
import { lookup as ownEntry } from "@ac/web/lib/lookup";
import { coreRoutes, type CoreRoute } from "@ac/web/lib/routes.gen";

export interface CoreRequest { method: CoreRoute["method"]; path: string; body?: string }

type Json = Record<string, unknown>;
const isObject = (v: unknown): v is Json => typeof v === "object" && v !== null && !Array.isArray(v);
const EMPTY_VALUES: readonly unknown[] = [undefined, null, ""];
const pathParams = (path: string) => [...path.matchAll(/\{([^{}]+)\}/g)].map((m) => m[1] ?? "");

function lookup(node: Json, at: string[]): unknown {
  let current: unknown = node;
  for (const key of at) {
    if (!isObject(current)) return undefined;
    current = ownEntry(current, key);
  }
  return current;
}

function take(node: Json, at: string[]): unknown {
  const parent = lookup(node, at.slice(0, -1));
  const key = at.at(-1);
  if (key === undefined || !isObject(parent)) return undefined;
  // the key is a path parameter's name from the generated route table, read and removed as an own property
  const v: unknown = Reflect.get(parent, key);
  Reflect.deleteProperty(parent, key);
  return v;
}

const text = (v: unknown): string => {
  if (v === null || v === undefined) return "";
  if (typeof v === "string") return v;
  return typeof v === "number" || typeof v === "boolean" ? String(v) : JSON.stringify(v);
};

function pick(routes: readonly CoreRoute[], input: Json): CoreRoute {
  let best = routes[0];
  let score = -1;
  for (const r of routes) {
    if (r.fixed && text(input[r.fixed[0]]) !== r.fixed[1]) continue;
    const names = pathParams(r.path);
    const isHeld = names.every((n) => { const v = lookup(input, n.split(".")); return !EMPTY_VALUES.includes(v); });
    if (!(isHeld && names.length > score)) {
      continue;
    }

    best = r;
    score = names.length;
  }
  if (!best) throw new Error("an operation has at least one route");
  return best;
}

function flatten(q: URLSearchParams, prefix: string, node: Json, top: boolean) {
  for (const [k, v] of Object.entries(node)) {
    if (v === null || v === undefined) continue;
    if (Array.isArray(v)) {
      if (v.length === 0) q.append(prefix + k, ""); // the empty list
      for (const e of v) q.append(prefix + k, text(e));
    } else if (isObject(v)) {
      if (top && (k === "query" || k === "filters")) flatten(q, "", v, true);
      else if (k === "sort") q.set("sort", `${text(v.field)}:${text(v.direction)}`);
      else flatten(q, `${prefix}${k}.`, v, false);
    } else {
      q.append(prefix + k, text(v));
    }
  }
}

/** The REST request of one operation call, or null for an operation outside the catalog. */
export function coreRequest(operation: string, input: unknown): CoreRequest | null {
  const routes = ownEntry(coreRoutes, operation); // own entries only: "constructor" or "__proto__" name no operation
  if (!routes?.length) return null;
  const rest: Json = isObject(input) ? structuredClone(input) : {};
  const route = pick(routes, rest);
  let path = route.path;
  for (const name of pathParams(route.path)) {
    const value = text(take(rest, name.split(".")));
    // a path parameter is one whole segment: "." and ".." would turn the URL into another path once fetch resolves the
    // dot segments (encodeURIComponent keeps dots, and %2E is resolved as a dot too), so they name no route (IR372)
    if (value === "." || value === "..") return null;
    path = path.replace(`{${name}}`, () => encodeURIComponent(value));
  }
  const [fixedKey, fixedValue] = route.fixed ?? [];
  if (fixedKey !== undefined && text(Reflect.get(rest, fixedKey)) === fixedValue) Reflect.deleteProperty(rest, fixedKey);
  if (route.method === "GET" || route.method === "DELETE") {
    const q = new URLSearchParams();
    flatten(q, "", rest, true);
    const qs = q.toString();
    return { method: route.method, path: qs ? `${path}?${qs}` : path };
  }
  return { method: route.method, body: JSON.stringify(rest), path };
}
