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
import { coreRoutes, type CoreRoute } from "@ac/web/lib/routes.gen";

export type CoreRequest = { method: CoreRoute["method"]; path: string; body?: string };

type Json = Record<string, unknown>;
const isObject = (v: unknown): v is Json => typeof v === "object" && v !== null && !Array.isArray(v);
const pathParams = (path: string) => [...path.matchAll(/\{([^}]+)\}/g)].map((m) => m[1]);

function lookup(obj: Json, at: string[]): unknown {
  let cur: unknown = obj;
  for (const key of at) {
    if (!isObject(cur)) return undefined;
    cur = cur[key];
  }
  return cur;
}

function take(obj: Json, at: string[]): unknown {
  const parent = lookup(obj, at.slice(0, -1));
  if (!isObject(parent)) return undefined;
  const v = parent[at[at.length - 1]];
  delete parent[at[at.length - 1]];
  return v;
}

const text = (v: unknown): string => (v === null || v === undefined ? "" : typeof v === "string" ? v : typeof v === "number" || typeof v === "boolean" ? String(v) : JSON.stringify(v));

function pick(routes: readonly CoreRoute[], input: Json): CoreRoute {
  let best = routes[0];
  let score = -1;
  for (const r of routes) {
    if (r.fixed && text(input[r.fixed[0]]) !== r.fixed[1]) continue;
    const names = pathParams(r.path);
    const held = names.every((n) => { const v = lookup(input, n.split(".")); return v !== undefined && v !== null && v !== ""; });
    if (held && names.length > score) {
      best = r;
      score = names.length;
    }
  }
  return best;
}

function flatten(q: URLSearchParams, prefix: string, obj: Json, top: boolean) {
  for (const [k, v] of Object.entries(obj)) {
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
  const routes = coreRoutes[operation];
  if (!routes?.length) return null;
  const rest: Json = isObject(input) ? structuredClone(input) : {};
  const route = pick(routes, rest);
  let path = route.path;
  for (const name of pathParams(route.path)) path = path.replace(`{${name}}`, encodeURIComponent(text(take(rest, name.split(".")))));
  if (route.fixed && text(rest[route.fixed[0]]) === route.fixed[1]) delete rest[route.fixed[0]];
  if (route.method === "GET" || route.method === "DELETE") {
    const q = new URLSearchParams();
    flatten(q, "", rest, true);
    const qs = q.toString();
    return { method: route.method, path: qs ? `${path}?${qs}` : path };
  }
  return { method: route.method, path, body: JSON.stringify(rest) };
}
