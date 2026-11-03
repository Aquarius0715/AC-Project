// Command ops generates ops/catalog_gen.go from the operation catalog, the write version catalog, the operation
// persistence map, the query catalog and service-contracts.ts (backend Go design §3), and the REST route table of the
// web apps' data access layer, service/web/shared/lib/routes.gen.ts (IR222). Run from service/api: go run ./cmd/gen/ops
package main

import (
	"bytes"
	"encoding/csv"
	"flag"
	"fmt"
	"go/format"
	"log"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
)

// read returns the rows of a catalog CSV as maps from its header names.
func read(path string) []map[string]string {
	body, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		log.Fatal(err)
	}

	recs, err := csv.NewReader(bytes.NewReader(body)).ReadAll()
	if err != nil {
		log.Fatal(err)
	}

	out := make([]map[string]string, 0, len(recs)-1)

	for _, r := range recs[1:] {
		m := map[string]string{}
		for i, h := range recs[0] {
			m[h] = r[i]
		}

		out = append(out, m)
	}

	return out
}

// route is one rest_routes entry: "METHOD /v1/path" with an optional " [field=value]".
var route = regexp.MustCompile(
	`^(GET|POST|PUT|PATCH|DELETE) (/v1/\S+)(?: \[([A-Za-z]\w*)=([a-z_]+)\])?$`,
)

type restRoute struct{ method, path, field, value string }

func parseRoutes(operation, column string) []restRoute {
	if column == "" {
		log.Fatalf("%s has no rest_routes", operation)
	}

	var out []restRoute

	for entry := range strings.SplitSeq(column, ";") {
		m := route.FindStringSubmatch(entry)
		if m == nil {
			log.Fatalf("%s: malformed REST route %q", operation, entry)
		}

		out = append(out, restRoute{m[1], m[2], m[3], m[4]})
	}

	return out
}

func goRoutes(rs []restRoute) string {
	out := make([]string, 0, len(rs))

	for _, r := range rs {
		s := fmt.Sprintf("{Method: %q, Path: %q", r.method, r.path)
		if r.field != "" {
			s += fmt.Sprintf(", FixedField: %q, FixedValue: %q", r.field, r.value)
		}

		out = append(out, s+"}")
	}

	return strings.Join(out, ", ")
}

func tsRoutes(rs []restRoute) string {
	out := make([]string, 0, len(rs))

	for _, r := range rs {
		s := fmt.Sprintf(`{ method: %q, path: %q`, r.method, r.path)
		if r.field != "" {
			s += fmt.Sprintf(`, fixed: [%q, %q]`, r.field, r.value)
		}

		out = append(out, s+" }")
	}

	return "[" + strings.Join(out, ", ") + "]"
}

func quoted(list []string) string {
	if len(list) == 0 {
		return "nil"
	}

	q := make([]string, len(list))
	for i, s := range list {
		q[i] = strconv.Quote(s)
	}

	return "[]string{" + strings.Join(q, ", ") + "}"
}

// filterKinds reads Query.filters of service-contracts.ts: arrays are lists of strings, boolean and number keep
// their kind, everything else (IDs, instants, enums) is a string.
func filterKinds(path string) map[string]string {
	src, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		log.Fatal(err)
	}

	match := regexp.MustCompile(`filters\?:\{(.*?)\}\};`).FindSubmatch(src)
	if match == nil {
		log.Fatal("service-contracts.ts: Query.filters not found")
	}

	kinds := map[string]string{}

	for field := range strings.SplitSeq(string(match[1]), ";") {
		name, typ, ok := strings.Cut(field, "?:")
		if !ok {
			log.Fatalf("service-contracts.ts: unexpected filter %q", field)
		}

		switch {
		case strings.HasSuffix(typ, "[]"):
			kinds[name] = "{List: true}"
		case typ == "boolean":
			kinds[name] = "{Kind: KindBool}"
		case typ == "number":
			kinds[name] = "{Kind: KindNumber}"
		default:
			kinds[name] = "{}"
		}
	}

	return kinds
}

// catalogs are the per-operation columns of the other catalogs that the operation catalog's rows join: the write
// version rules, the module of each operation and the filters each query allows.
type catalogs struct {
	versions map[string][]string
	modules  map[string]string
	filters  map[string][]string
}

func readCatalogs(docs string) catalogs {
	joined := catalogs{
		versions: map[string][]string{},
		modules:  map[string]string{},
		filters:  map[string][]string{},
	}

	for _, v := range read(docs + "/write-version-catalog.csv") {
		joined.versions[v["operation"]] = append(
			joined.versions[v["operation"]],
			fmt.Sprintf("{Branch: %q, Rule: %q}", v["branch"], v["expected_version"]),
		)
	}

	for _, p := range read(docs + "/operation-persistence-map.csv") {
		joined.modules[p["operation"]] = p["module"]
	}

	for _, q := range read(docs + "/query-catalog.csv") {
		for f := range strings.SplitSeq(q["allowed_filters"], ",") {
			if f = strings.TrimSpace(f); f != "" {
				joined.filters[q["operation"]] = append(joined.filters[q["operation"]], f)
			}
		}
	}

	return joined
}

// generate returns catalog_gen.go, routes.gen.ts and contracts.gen.ts from the design documents.
func generate(docs string) ([]byte, []byte, []byte, int) {
	ops := read(docs + "/operation-catalog.csv")
	sort.Slice(ops, func(i, j int) bool { return ops[i]["operation"] < ops[j]["operation"] })

	return catalogGo(ops, readCatalogs(docs), filterKinds(docs+"/service-contracts.ts")),
		routesTypeScript(ops),
		contracts(docs + "/service-contracts.ts"),
		len(ops)
}

// catalogGo is catalog_gen.go: the Catalog of every operation and the FilterKinds of the query filters.
func catalogGo(ops []map[string]string, joined catalogs, kinds map[string]string) []byte {
	var buf bytes.Buffer

	buf.WriteString(
		"// Code generated by cmd/gen/ops from docs/02-design; DO NOT EDIT.\n\npackage ops\n\n",
	)
	buf.WriteString(
		"// Catalog lists every operation of operation-catalog.csv.\nvar Catalog = []Spec{\n",
	)

	for _, row := range ops {
		name := row["operation"]

		mode := "Read"
		if row["mode"] == "write" {
			mode = "Write"
		}

		if joined.modules[name] == "" {
			log.Fatalf("%s missing in operation-persistence-map.csv", name)
		}

		fmt.Fprintf(
			&buf,
			"\t{Name: %q, Mode: %s, Module: %q, Authorization: %q, Versions: []VersionRule{%s}, DesignIDs: %q, Routes: "+
				"[]Route{%s}, Filters: %s},\n",
			name,
			mode,
			joined.modules[name],
			strings.TrimSpace(row["authorization"]),
			strings.Join(joined.versions[name], ", "),
			row["design_ids"],
			goRoutes(parseRoutes(name, row["rest_routes"])),
			quoted(joined.filters[name]),
		)
	}

	buf.WriteString("}\n\n")
	buf.WriteString(
		"// FilterKinds types the Query.filters fields of service-contracts.ts as query parameters (IR222); a filter\n// " +
			"missing here is a string. Lists repeat the parameter or separate the values with commas.\n",
	)
	buf.WriteString("var FilterKinds = map[string]FilterKind{\n")

	for _, n := range slices.Sorted(maps.Keys(kinds)) {
		fmt.Fprintf(&buf, "\t%q: %s,\n", n, kinds[n])
	}

	buf.WriteString("}\n")

	src, err := format.Source(buf.Bytes())
	if err != nil {
		log.Fatal(err)
	}

	return src
}

// routesTypeScript is routes.gen.ts: the REST routes of every operation and the set of the operations that write.
func routesTypeScript(ops []map[string]string) []byte {
	var (
		buf    bytes.Buffer
		writes []string
	)

	buf.WriteString(
		"// Code generated by service/api/cmd/gen/ops from docs/02-design/operation-catalog.csv (rest_routes); DO NOT " +
			"EDIT.\n\n",
	)
	buf.WriteString(
		"/** One REST route of a Core API operation (IR222): {field} path parameters named after the input fields they " +
			"fill, and\n * optionally one input field the route fixes (payouts.transition at /approve sets action=approve). " +
			"*/\n",
	)
	buf.WriteString(
		"export type CoreRoute = { method: \"GET\" | \"POST\" | \"PUT\" | \"PATCH\" | \"DELETE\"; path: string; fixed?: " +
			"readonly [string, string] };\n\n",
	)
	buf.WriteString(
		"/** The REST routes of every Core API operation. */\nexport const coreRoutes: Readonly<Record<string, readonly " +
			"CoreRoute[]>> = {\n",
	)

	for _, row := range ops {
		name := row["operation"]
		if row["mode"] == "write" {
			writes = append(writes, name)
		}

		fmt.Fprintf(&buf, "  %q: %s,\n", name, tsRoutes(parseRoutes(name, row["rest_routes"])))
	}

	buf.WriteString("};\n\n")
	buf.WriteString(
		"/** The operations that change state (mode write): the apps send them through Server Actions, never through the " +
			"BFF\n * relay /bff/ops; a contract test checks the browser's calls (IR358). */\n",
	)
	buf.WriteString("export const coreWrites: ReadonlySet<string> = new Set([\n")

	for _, w := range writes {
		fmt.Fprintf(&buf, "  %q,\n", w)
	}

	buf.WriteString("]);\n")

	return buf.Bytes()
}

// contracts copies the Core API's contract types into the web apps (IR313): coreOp / callOp type each operation's input
// by OperationContracts, so tsc checks every call against the contract.
func contracts(path string) []byte {
	body, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		log.Fatal(err)
	}

	var buf bytes.Buffer
	buf.WriteString(
		"// Code generated by service/api/cmd/gen/ops from docs/02-design/service-contracts.ts; DO NOT EDIT.\n",
	)
	buf.WriteString(
		"// The Core API contract types for the web apps' calls (coreOp, callOp, coreAll; IR313).\n\n",
	)
	buf.Write(body)

	return buf.Bytes()
}

// generatedMode is the mode a newly generated file gets (git keeps only the executable bit, so checkouts stay 0644).
const generatedMode = 0o600

func main() {
	docs := flag.String("docs", "../../docs/02-design", "docs/02-design directory")
	out := flag.String("out", "internal/ops/catalog_gen.go", "output file")
	web := flag.String("web", "../web/shared/lib/routes.gen.ts", "REST route table of the web apps")
	types := flag.String(
		"contracts",
		"../web/shared/lib/contracts.gen.ts",
		"contract types of the web apps",
	)

	flag.Parse()

	src, routesTS, contractsTS, operations := generate(*docs)
	for path, data := range map[string][]byte{*out: src, *web: routesTS, *types: contractsTS} {
		err := os.WriteFile(path, data, generatedMode)
		if err != nil {
			log.Fatal(err)
		}
	}

	log.Printf("%d operations → %s, %s, %s", operations, *out, *web, *types)
}
