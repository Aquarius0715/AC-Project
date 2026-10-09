#!/usr/bin/env python3
"""Bootstraps the swag annotations of every registered operation handler (IR220).

For each `ops.Register(r, "<operation>", handler)` call the handler function gets a comment block with the swag
annotations (summary, description from the operation / query / write-version catalogs, body and header parameters,
the {data, meta} envelope and the ServiceError failures, the bearer security and `@Router /v1/ops/<operation> [post]`).
An existing annotation block is replaced; the function's own doc comment is kept above it. The annotations live in the
code from then on and `make swagger` builds swagger.json from them (swag). Run from service/api:
    python3 scripts/swag_annotate.py            # rewrite the annotation blocks
    python3 scripts/swag_annotate.py --check    # exit 1 when a registered operation has no @Router annotation
"""
import csv, os, re, sys
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


def block(op, pkg, input_type, result_type):
    row = catalog[op]
    mode = row["mode"]
    lines = [f"//\t@Summary\t\t{op} ({mode})", f"//\t@ID\t\t\t{op}"]
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
    lines.append("//\t@Description\t" + " · ".join(extra))
    lines += [f"//\t@Tags\t\t\t{op.split('.')[0]}", "//\t@Accept\t\t\tjson", "//\t@Produce\t\tjson"]
    if mode == "write":
        lines.append('//\t@Param\t\t\tIdempotency-Key\theader\tstring\ttrue\t"D04: the same key replays the stored response; another body for the same key is CONFLICT"')
        need = [v["expected_version"] for v in vs]
        if "required" in need:
            required = "true" if all(x == "required" for x in need) else "false"
            desc = clean("; ".join(f"{v['branch']}: {v['expected_version']} (target {v['version_target']}, read {v['read_source']})" for v in vs))
            lines.append(f'//\t@Param\t\t\tX-Expected-Version\theader\tinteger\t{required}\t"{desc}"')
    if input_type != "struct{}":
        lines.append(f'//\t@Param\t\t\trequest\tbody\t{input_type}\ttrue\t"input"')
    page = re.fullmatch(r"paging\.Page\[(.+)\]", result_type)
    if result_type in PRIMITIVES or result_type.startswith("map["):
        lines.append("//\t@Success\t\t200\t{object}\tops.Envelope")
    elif page:  # swag cannot instantiate paging.Page with a type of this package from an annotation: an alias can
        lines.append(f"//\t@Success\t\t200\t{{object}}\tops.Envelope{{data={page_alias(page.group(1))}}}")
    else:
        lines.append(f"//\t@Success\t\t200\t{{object}}\tops.Envelope{{data={result_type}}}")
    for status, codes in STATUSES:
        lines.append(f'//\t@Failure\t\t{status}\t{{object}}\tapperr.DomainError\t"{codes}"')
    lines += ["//\t@Security\t\tBearerAuth", f"//\t@Router\t\t\t/v1/ops/{op} [post]"]
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


def main():
    check = "--check" in sys.argv
    edits = {}  # path → {line index of func: (op, block lines)}
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
                edits.setdefault(hp, {})[hi] = (op, block(op, pkg, input_type, result_type))
    if check:
        for p, blocks in edits.items():
            for hi, (op, _) in blocks.items():
                if not re.search(rf"@Router\s+/v1/ops/{re.escape(op)} \[post\]", sources[p]):
                    missing.append(op)
        print(f"{sum(len(b) for b in edits.values())} handlers, {len(missing)} without annotations: {sorted(set(missing))}")
        sys.exit(1 if missing else 0)
    for pkg_dir, (pkg, aliases) in page_aliases.items():
        write_page_aliases(pkg_dir, pkg, aliases)
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
    print(f"annotated {sum(len(b) for b in edits.values())} handlers in {len(edits)} files; primitive results: {primitives}; unresolved: {sorted(set(missing))}")


if __name__ == "__main__":
    main()
