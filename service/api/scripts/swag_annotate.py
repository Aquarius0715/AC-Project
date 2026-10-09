#!/usr/bin/env python3
"""Bootstraps the swag annotations of every registered operation handler (IR220, REST routes IR222).

For each `ops.Register(r, "<operation>", handler)` call the handler function gets a comment block with the swag
annotations of the operation's first REST route (summary, description from the operation / query / write-version
catalogs, path, query, body and header parameters, the {data, meta} envelope and the ServiceError failures, the
bearer security and `@Router <path> [<method>]`); every further route of the operation (the update route of a save,
the verb routes of payouts.transition and firmwareCampaigns.control) is annotated on a documentation stub in the
package's generated swagger_routes.go. The path and query parameters come from cmd/gen/restdoc (the bindings the
server mounts). An existing annotation block is replaced; the function's own doc comment is kept above it. The
annotations live in the code from then on and `make swagger` builds swagger.json from them (swag). Run from
service/api:
    go run ./cmd/gen/restdoc | python3 scripts/swag_annotate.py --routes -            # rewrite the annotations
    go run ./cmd/gen/restdoc | python3 scripts/swag_annotate.py --routes - --check    # exit 1 when they are stale
"""
import csv, json, os, re, subprocess, sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
DOCS = ROOT.parent.parent / "docs" / "02-design"
STATUSES = [(401, "UNAUTHENTICATED"), (403, "FORBIDDEN"), (404, "NOT_FOUND"), (409, "CONFLICT / OFFLINE"), (422, "VALIDATION (fieldErrors)"),
            (429, "RATE_LIMITED (retryAfterSeconds)"), (503, "UNAVAILABLE"), (504, "TIMEOUT")]
PRIMITIVES = {"string", "bool", "int", "int64", "float64", "any", "struct{}", "map[string]any", "map[string]string", "[]byte"}


def rows(name):
    with open(DOCS / name, newline="", encoding="utf-8") as f:
        return list(csv.DictReader(f))


catalog = {r["operation"]: r for r in rows("operation-catalog.csv")}
queries = {r["operation"]: r for r in rows("query-catalog.csv")}
versions = {}
for r in rows("write-version-catalog.csv"):
    versions.setdefault(r["operation"], []).append(r)

go_files = [p for p in (ROOT / "internal").rglob("*.go") if not p.name.endswith("_test.go")]
sources = {p: p.read_text(encoding="utf-8") for p in go_files}
REGISTER = re.compile(r'ops\.Register\(\s*\w+\s*,\s*"([\w.]+)"\s*,\s*([\w.]+)\s*\)')
FUNC_HEAD = re.compile(r"^func (?:\((?:(\w+) )?\*?([\w.]+)\) )?(\w+)\((.*)\) \((.+), error\) \{$")


def package_of(src):
    return re.search(r"^package (\w+)", src, re.M).group(1)


def qualify(t, pkg):
    """Type names stay as written in the handler's signature: swag resolves bare names in the file's own package and
    qualified names through its imports (the file never imports its own package)."""
    return t


def find_handler(pkg_dir, recv_type, name):
    for p in go_files:
        if p.parent != pkg_dir:
            continue
        for i, line in enumerate(sources[p].split("\n")):
            m = FUNC_HEAD.match(line)
            if m and m.group(3) == name and (recv_type is None) == (m.group(2) is None) and (recv_type is None or m.group(2) == recv_type):
                return p, i, m
    return None, None, None


def param_maps(src):
    """line index → {var: type} of the enclosing Register function (its parameters)."""
    out, current = {}, {}
    for i, line in enumerate(src.split("\n")):
        m = re.match(r"^func \w*[Rr]egister\w*\((.*)\) \{", line)
        if m:
            current = {}
            for part in m.group(1).split(","):
                bits = part.strip().split()
                if len(bits) == 2:
                    current[bits[0]] = bits[1].lstrip("*")
        sv = re.match(r"^\t(\w+) := (\w+)\{", line) or re.match(r"^\tvar (\w+) \*?(\w+)$", line)
        if sv:
            current = {**current, sv.group(1): sv.group(2)}
        out[i] = current
    return out


def clean(text):
    return re.sub(r"\s+", " ", text.replace('"', "'")).strip()


def mapping_of(op, name):
    """The query catalog's filter_mapping entry of one filter ("unitId => unitId")."""
    for part in (queries.get(op, {}).get("filter_mapping") or "").split(";"):
        left, _, right = part.partition("=>")
        if name in [x.strip() for x in left.split("/")]:
            return clean(right)
    return ""


def param_line(op, prm):
    kind = ("[]" if prm.get("list") else "") + prm["type"]
    role = prm["role"]
    if role == "path":
        desc = f"input field {prm['field']}"
    elif role == "cursor":
        desc = "page cursor: nextCursor of the previous page (D12)"
    elif role == "limit":
        desc = "page size 1–100, default 25"
    elif role == "sort":
        q = queries.get(op, {})
        desc = f"field:direction — fields {q.get('allowed_sort') or 'none'}; default {q.get('default_sort') or 'none'}"
    elif role == "filter":
        m = mapping_of(op, prm["name"])
        desc = "filter" + (f" → {m}" if m else "")
    else:
        desc = f"input field {prm['field']}"
    if prm.get("list"):
        desc += " (repeat the parameter or separate values with commas; an empty value is the empty list)"
    required = "true" if prm.get("required") else "false"
    line = f'//\t@Param\t\t\t{prm["name"]}\t{prm["in"]}\t{kind}\t{required}\t"{clean(desc)}"'
    if prm.get("list"):
        line += "\tcollectionFormat(multi)"
    return line


def route_suffix(route, index):
    """@ID suffix of a further route: the fixed value, update for the item route of a save, else its position."""
    if route.get("fixedValue"):
        return route["fixedValue"]
    if route["method"] == "PUT" and route["path"].endswith("/{id}"):
        return "update"
    return str(index + 1)


def block(op, pkg, input_type, result_type, route, suffix=None):
    row = catalog[op]
    mode = row["mode"]
    op_id = op if suffix is None else f"{op}.{suffix}"
    title = f"{op} ({mode})" if suffix is None else f"{op} ({mode}) — {suffix.replace('_', ' ')}"
    lines = [f"//\t@Summary\t\t{title}", f"//\t@ID\t\t\t{op_id}"]
    lines.append(f"//\t@Description\tAuthorization: {clean(row['authorization'])}")
    lines.append(f"//\t@Description\tValidation: {clean(row['ui_validation'])}")
    lines.append(f"//\t@Description\tRecovery: {clean(row['ui_recovery'])}")
    extra = [f"Design: {', '.join(row['design_ids'].split(';'))}"]
    q = queries.get(op)
    if q:
        extra.append(f"Query: filters {q['allowed_filters'] or 'none'} · sort {q['allowed_sort'] or 'none'} (default {q['default_sort']})")
    vs = versions.get(op, [])
    inputs = sorted({v["input_versions"] for v in vs if v["input_versions"] and v["input_versions"] != "none"})
    if inputs:
        extra.append("Input versions: " + "; ".join(inputs))
    if route.get("fixedField"):
        extra.append(f"This route sets {route['fixedField']}={route['fixedValue']}")
    lines.append("//\t@Description\t" + " · ".join(extra))
    lines += [f"//\t@Tags\t\t\t{op.split('.')[0]}", "//\t@Accept\t\t\tjson", "//\t@Produce\t\tjson"]
    if {"id omitted", "id present"} <= {v["branch"] for v in vs}:  # a save: create on the collection, update on the item
        has_id = any(x["in"] == "path" and x["name"] == "id" for x in route["params"])
        vs = [v for v in vs if v["branch"] == ("id present" if has_id else "id omitted")]
    if mode == "write":
        lines.append('//\t@Param\t\t\tIdempotency-Key\theader\tstring\ttrue\t"D04: the same key replays the stored response; another body for the same key is CONFLICT"')
        need = [v["expected_version"] for v in vs]
        if "required" in need:
            required = "true" if all(x == "required" for x in need) else "false"
            desc = clean("; ".join(f"{v['branch']}: {v['expected_version']} (target {v['version_target']}, read {v['read_source']})" for v in vs))
            lines.append(f'//\t@Param\t\t\tX-Expected-Version\theader\tinteger\t{required}\t"{desc}"')
    for prm in route["params"]:
        lines.append(param_line(op, prm))
    if route["body"] and input_type != "struct{}":
        note = "input; the path parameters" + (" and the fixed field" if route.get("fixedField") else "") + " come from the route" if any(x["in"] == "path" for x in route["params"]) or route.get("fixedField") else "input"
        lines.append(f'//\t@Param\t\t\trequest\tbody\t{input_type}\ttrue\t"{note}"')
    page = re.fullmatch(r"paging\.Page\[(.+)\]", result_type)
    if result_type in PRIMITIVES or result_type.startswith("map["):
        lines.append("//\t@Success\t\t200\t{object}\tops.Envelope")
    elif page:  # swag cannot instantiate paging.Page with a type of this package from an annotation: an alias can
        lines.append(f"//\t@Success\t\t200\t{{object}}\tops.Envelope{{data={page_alias(page.group(1))}}}")
    else:
        lines.append(f"//\t@Success\t\t200\t{{object}}\tops.Envelope{{data={result_type}}}")
    for status, codes in STATUSES:
        lines.append(f'//\t@Failure\t\t{status}\t{{object}}\tapperr.DomainError\t"{codes}"')
    lines += ["//\t@Security\t\tBearerAuth", f"//\t@Router\t\t\t{route['path']} [{route['method'].lower()}]"]
    return lines


MODULE = "github.com/pradita/ac-project/service/api"
page_aliases = {}  # package dir → {alias: inner type}


def page_alias(inner):
    return inner.split(".")[-1] + "Page"


def write_page_aliases(pkg_dir, pkg, aliases):
    """<pkg>/swagger_pages.go: one alias per list result, imports resolved from the package's own files."""
    imports = {f'"{MODULE}/internal/platform/paging"'}
    for inner in aliases.values():
        if "." in inner:
            name = inner.split(".")[0]
            for p in go_files:
                if p.parent == pkg_dir:
                    m = re.search(rf'^\s*"([^"]+/{name})"$', sources[p], re.M)
                    if m:
                        imports.add(f'"{m.group(1)}"')
                        break
    body = ["// Code generated by scripts/swag_annotate.py; DO NOT EDIT.", "", "// Page types of the list operations for the API description (IR220): swag cannot instantiate paging.Page with a",
            "// type of this package from an annotation, so each list result gets a named page with the same fields; the",
            "// conversions below make the build fail when paging.Page changes.", "", f"package {pkg}", "", "import (", *sorted(f"\t{i}" for i in imports), ")", ""]
    for alias, inner in sorted(aliases.items()):
        body += [f"// {alias} is paging.Page[{inner}].", f"type {alias} struct {{", f"\tItems           []{inner} `json:\"items\"`", "\tNextCursor      *string `json:\"nextCursor\"`",
                 "\tTotal           int     `json:\"total\"`", "\tSnapshotVersion int     `json:\"snapshotVersion\"`", "}", "", f"var _ = paging.Page[{inner}]({alias}{{}})", ""]
    (pkg_dir / "swagger_pages.go").write_text("\n".join(body), encoding="utf-8")
    subprocess.run(["gofmt", "-w", str(pkg_dir / "swagger_pages.go")], check=True)


def norm(text):
    return re.sub(r"\s+", " ", text).strip()


def stub_name(op, suffix):
    words = re.split(r"[._]", op + "." + suffix)
    return "swagger" + "".join(w[:1].upper() + w[1:] for w in words if w)


def write_route_stubs(pkg_dir, pkg, stubs, check):
    """<pkg>/swagger_routes.go: one documentation stub per further REST route of the package's operations."""
    path = pkg_dir / "swagger_routes.go"
    if not stubs:
        if path.exists():
            if check:
                return [str(path)]
            path.unlink()
        return []
    refs, imports = set(), {f'"{MODULE}/internal/ops"', f'"{MODULE}/internal/platform/apperr"'}
    for _, _, types, src in stubs:
        for qual in re.findall(r"\b([a-z]\w*)\.([A-Z]\w*)", " ".join(types)):
            if qual[0] in ("ops", "apperr"):
                continue
            m = re.search(rf'^\s*(?:\w+ )?"([^"]+/{qual[0]})"$', src, re.M)
            if m:
                imports.add(f'"{m.group(1)}"')
                refs.add(f"{qual[0]}.{qual[1]}")
    body = ["// Code generated by scripts/swag_annotate.py; DO NOT EDIT.", "",
            "// Documentation stubs of the further REST routes of this package's operations (IR222): swag documents one",
            "// route per function, so the update route of a save and the verb routes of a transition are annotated here; the",
            "// handlers carry the first route of each operation.", "", f"package {pkg}", "", "import (",
            *sorted(f"\t{i}" for i in imports), ")", "", "var (", "\t_ ops.Envelope", "\t_ apperr.DomainError",
            *sorted(f"\t_ *{r}" for r in refs), *sorted(f"\t_ = {name}" for name, _, _, _ in stubs), ")", ""]
    for name, annotations, _, _ in sorted(stubs):
        body += [f"// {name} documents a further REST route.", "//", *annotations, f"func {name}() {{}}", ""]
    text = "\n".join(body)
    if check:  # swag fmt and gofmt realign the file: compare without layout
        return [] if path.exists() and norm(path.read_text(encoding="utf-8")) == norm(text) else [str(path)]
    path.write_text(text, encoding="utf-8")
    subprocess.run(["gofmt", "-w", str(path)], check=True)
    return []


def main():
    check = "--check" in sys.argv
    if "--routes" not in sys.argv:
        sys.exit("--routes <file|-> is required (go run ./cmd/gen/restdoc)")
    source = sys.argv[sys.argv.index("--routes") + 1]
    routes = json.load(sys.stdin if source == "-" else open(source, encoding="utf-8"))
    edits = {}  # path → {line index of func: (op, block lines)}
    stubs = {}  # package dir → (pkg, [(stub name, block lines, types, handler source)])
    missing, primitives = [], []
    for p in go_files:
        src = sources[p]
        maps = param_maps(src)
        for i, line in enumerate(src.split("\n")):
            for m in REGISTER.finditer(line):
                op, handler = m.group(1), m.group(2)
                if op not in catalog:
                    print(f"{p}:{i + 1}: {op} is not in the operation catalog", file=sys.stderr)
                    continue
                if op not in routes:
                    print(f"{p}:{i + 1}: {op} has no REST route in the restdoc output", file=sys.stderr)
                    missing.append(op)
                    continue
                recv = None
                if "." in handler:
                    var, name = handler.split(".", 1)
                    recv = maps.get(i, {}).get(var)
                    if recv is None:
                        print(f"{p}:{i + 1}: cannot resolve receiver of {handler}", file=sys.stderr)
                        missing.append(op)
                        continue
                else:
                    name = handler
                hp, hi, hm = find_handler(p.parent, recv, name)
                if hp is None:
                    print(f"{p}:{i + 1}: no handler function for {op} ({handler})", file=sys.stderr)
                    missing.append(op)
                    continue
                pkg = package_of(sources[hp])
                params = hm.group(4)
                im = re.search(r"(\w+) \*?([\w.\[\]{}]+)$", params.strip())
                input_type = qualify(im.group(2), pkg) if im else "struct{}"
                if input_type == f"{pkg}.struct{{}}":
                    input_type = "struct{}"
                result_type = qualify(hm.group(5).strip(), pkg)
                page = re.fullmatch(r"paging\.Page\[(.+)\]", result_type)
                if page:
                    page_aliases.setdefault(hp.parent, (pkg, {}))[1][page_alias(page.group(1))] = page.group(1)
                if hm.group(5).strip() in PRIMITIVES or hm.group(5).strip().startswith("map["):
                    primitives.append((op, hm.group(5).strip()))
                first, *further = routes[op]
                edits.setdefault(hp, {})[hi] = (op, block(op, pkg, input_type, result_type, first))
                entry = stubs.setdefault(hp.parent, (pkg, []))
                for n, rt in enumerate(further):
                    suffix = route_suffix(rt, n + 1)
                    types = [input_type, page_alias(page.group(1)) if page else result_type]
                    entry[1].append((stub_name(op, suffix), block(op, pkg, input_type, result_type, rt, suffix), types, sources[hp]))
    for pkg_dir in {p.parent for p in edits}:
        stubs.setdefault(pkg_dir, (package_of(sources[next(p for p in edits if p.parent == pkg_dir)]), []))
    if check:
        stale = []
        for p, blocks in edits.items():
            for hi, (op, annotations) in blocks.items():
                if norm("\n".join(annotations)) not in norm(sources[p]):
                    stale.append(op)
        for pkg_dir, (pkg, entries) in stubs.items():
            stale += write_route_stubs(pkg_dir, pkg, entries, True)
        print(f"{sum(len(b) for b in edits.values())} handlers, {len(missing)} unresolved, {len(stale)} stale: {sorted(set(missing))} {sorted(set(stale))}")
        sys.exit(1 if missing or stale else 0)
    for pkg_dir, (pkg, aliases) in page_aliases.items():
        write_page_aliases(pkg_dir, pkg, aliases)
    for pkg_dir, (pkg, entries) in stubs.items():
        write_route_stubs(pkg_dir, pkg, entries, False)
    for p, blocks in edits.items():
        lines = sources[p].split("\n")
        for hi in sorted(blocks, reverse=True):  # bottom-up keeps earlier indexes valid
            op, annotations = blocks[hi]
            start = hi
            while start > 0 and lines[start - 1].startswith("//"):
                start -= 1
            doc = [l for l in lines[start:hi] if not re.match(r"^//\s*@", l) and l.strip() != "//"]
            lines[start:hi] = doc + ["//"] + annotations
        p.write_text("\n".join(lines), encoding="utf-8")
    print(f"annotated {sum(len(b) for b in edits.values())} handlers in {len(edits)} files, {sum(len(e) for _, e in stubs.values())} route stubs; primitive results: {primitives}; unresolved: {sorted(set(missing))}")


if __name__ == "__main__":
    main()
