// @vitest-environment node
import { readdirSync, readFileSync } from "node:fs";
import { join, relative } from "node:path";
import { fileURLToPath } from "node:url";
import ts from "typescript";
import { describe, expect, it } from "vitest";

// The web's calls against the Core API contract (IR312): every coreOp / callOp / coreAll call with a literal operation
// and an object literal input may use only the input fields of OperationContracts[op] (docs/02-design/
// service-contracts.ts; the API refuses unknown fields), only the filters the operation's catalog row allows (the REST
// binding refuses others), and literal values the contract fixes (count: 8 of automations.nextRuns). Inputs built
// elsewhere are typed by their own mappers' tests.
const web = fileURLToPath(new URL("../../../", import.meta.url));
const contractFile = join(web, "../../docs/02-design/service-contracts.ts");
const catalogFile = join(web, "../api/internal/ops/catalog_gen.go");

type Field = { literals: (string | number | boolean)[] | null }; // the literal values the contract allows, null: any
function contractInputs(): Map<string, Map<string, Field>> {
  const program = ts.createProgram([contractFile], { strict: true, noEmit: true });
  const checker = program.getTypeChecker();
  const src = program.getSourceFile(contractFile)!;
  const literalsOf = (t: ts.Type): Field["literals"] => {
    const parts = t.isUnion() ? t.types : [t];
    const values: (string | number | boolean)[] = [];
    for (const p of parts) {
      if (p.isStringLiteral() || p.isNumberLiteral()) values.push(p.value);
      else if (p.flags & ts.TypeFlags.BooleanLiteral) values.push(checker.typeToString(p) === "true");
      else if (p.flags & (ts.TypeFlags.Undefined | ts.TypeFlags.Null)) continue;
      else return null;
    }
    return values.length ? values : null;
  };
  const out = new Map<string, Map<string, Field>>();
  ts.forEachChild(src, (n) => {
    if (!ts.isTypeAliasDeclaration(n) || n.name.text !== "OperationContracts") return;
    for (const op of checker.getPropertiesOfType(checker.getTypeAtLocation(n.name))) {
      const input = checker.getTypeOfSymbolAtLocation(op, n).getProperty("input");
      if (!input) continue;
      const t = checker.getTypeOfSymbolAtLocation(input, n);
      const fields = new Map<string, Field>();
      for (const member of t.isUnion() ? t.types : [t]) {
        for (const p of checker.getPropertiesOfType(member)) {
          const lit = literalsOf(checker.getTypeOfSymbolAtLocation(p, n));
          // a field of several union members allows the literals of all of them; any non-literal type allows anything
          const before = fields.get(p.getName());
          const literals = !lit ? null : !before ? lit : before.literals === null ? null : [...new Set([...before.literals, ...lit])];
          fields.set(p.getName(), { literals });
        }
      }
      out.set(op.getName(), fields);
    }
  });
  return out;
}

function catalogFilters(): Map<string, string[]> {
  const out = new Map<string, string[]>();
  for (const m of readFileSync(catalogFile, "utf8").matchAll(/\{Name: "([\w.]+)".*?Filters: (nil|\[\]string\{([^}]*)\})\}/g)) {
    out.set(m[1], m[2] === "nil" ? [] : [...m[3].matchAll(/"([^"]+)"/g)].map((x) => x[1]));
  }
  return out;
}

function sources(): string[] {
  const files: string[] = [];
  const walk = (d: string) => {
    for (const e of readdirSync(d, { withFileTypes: true })) {
      if (["node_modules", ".next", "__tests__", "e2e"].includes(e.name)) continue;
      const p = join(d, e.name);
      if (e.isDirectory()) walk(p);
      else if (/\.tsx?$/.test(e.name)) files.push(p);
    }
  };
  for (const d of ["customer", "partner", "technician", "admin", "shared"]) walk(join(web, d));
  return files;
}

describe("the web's calls against the Core API contract (IR312)", () => {
  it("use only known operations, input fields, catalog filters and the values the contract fixes", () => {
    const inputs = contractInputs(), filters = catalogFilters();
    const problems: string[] = [];
    let calls = 0;
    const name = (p: ts.ObjectLiteralElementLike) => (p.name ? p.name.getText().replace(/^["']|["']$/g, "") : null);
    for (const f of sources()) {
      const sf = ts.createSourceFile(f, readFileSync(f, "utf8"), ts.ScriptTarget.Latest, true, f.endsWith("x") ? ts.ScriptKind.TSX : ts.ScriptKind.TS);
      const visit = (n: ts.Node) => {
        if (ts.isCallExpression(n) && ts.isIdentifier(n.expression) && ["coreOp", "callOp", "coreAll"].includes(n.expression.text) && n.arguments[0] && ts.isStringLiteral(n.arguments[0])) {
          const op = n.arguments[0].text;
          const where = `${relative(web, f)}:${sf.getLineAndCharacterOfPosition(n.getStart(sf)).line + 1}`;
          const fields = inputs.get(op);
          if (!fields) problems.push(`${where} ${op}: not an operation of the contract`);
          const arg = n.arguments[1];
          if (fields && arg && ts.isObjectLiteralExpression(arg)) {
            calls++;
            for (const p of arg.properties) {
              const key = name(p);
              if (!key || ts.isSpreadAssignment(p)) continue;
              const field = fields.get(key);
              if (!field) { problems.push(`${where} ${op}: unknown field ${key}`); continue; }
              const init = ts.isPropertyAssignment(p) ? p.initializer : null;
              const value = init && (ts.isNumericLiteral(init) ? Number(init.text) : ts.isStringLiteral(init) ? init.text : init.kind === ts.SyntaxKind.TrueKeyword ? true : init.kind === ts.SyntaxKind.FalseKeyword ? false : undefined);
              if (field.literals && value !== undefined && value !== null && !field.literals.includes(value)) problems.push(`${where} ${op}: ${key} ${JSON.stringify(value)} is not one of ${JSON.stringify(field.literals)}`);
            }
            const prop = (o: ts.ObjectLiteralExpression, k: string) => o.properties.find((p): p is ts.PropertyAssignment => ts.isPropertyAssignment(p) && name(p) === k);
            const q = prop(arg, "query");
            const fp = prop(arg, "filters") ?? (q && ts.isObjectLiteralExpression(q.initializer) ? prop(q.initializer, "filters") : undefined);
            if (fp && ts.isObjectLiteralExpression(fp.initializer)) {
              for (const p of fp.initializer.properties) {
                const key = name(p);
                if (key && !ts.isSpreadAssignment(p) && !(filters.get(op) ?? []).includes(key)) problems.push(`${where} ${op}: filter ${key} is not in the catalog`);
              }
            }
          }
        }
        ts.forEachChild(n, visit);
      };
      visit(sf);
    }
    expect(problems).toEqual([]);
    expect(calls).toBeGreaterThan(300); // the scan sees the calls
  });
});
