// @vitest-environment node
import path from "node:path";
import { fileURLToPath } from "node:url";
import ts from "typescript";
import { describe, expect, it } from "vitest";

const isReference = (type: ts.Type): type is ts.TypeReference => "target" in type;
/** The type arguments of an array or Promise type (none for any other type). */
const typeArguments = (c: ts.TypeChecker, type: ts.Type): readonly ts.Type[] => (isReference(type) ? c.getTypeArguments(type) : []);
import { must } from "./must";

// The web's result types against the Core API contract (IR314). For every coreOp / coreAll / callOp / useOp call with a
// literal operation, the result type the caller names (or TypeScript infers from a typed variable) must accept
// OperationContracts[op].result of docs/02-design/service-contracts.ts (for coreAll the item of its page):
// - every field the web reads is in the contract, with a type the web's accepts — null included;
// - below the top every variant the contract allows must fit (a command's action kinds, a rule's condition types);
// - at the top (and the items of a page) the request or the role picks the variant: the screen names the projection or
//   kind it reads — a field that tells every variant apart — and each variant it names must fit. A union without such
//   a field (payments.simulate: the event decides) needs one variant that fits.
// The input side is typed by OpArgs (IR313) and contract.test.ts.
const web = fileURLToPath(new URL("../../../", import.meta.url));
const projects = ["shared", "customer", "partner", "technician", "admin"];
const calls = new Set(["coreOp", "coreAll", "callOp", "useOp"]);

function check(): { checked: number; problems: string[] } {
  const seen = new Set<string>();
  const problems: string[] = [];
  let checked = 0;
  for (const p of projects) {
    const directory = path.join(web, p);
    const roleConfig = ts.readConfigFile(path.join(directory, "tsconfig.json"), ts.sys.readFile);
    const parsed = ts.parseJsonConfigFileContent(roleConfig.config, ts.sys, directory);
    const program = ts.createProgram(parsed.fileNames, { ...parsed.options, noEmit: true, incremental: false });
    const c = program.getTypeChecker();
    const gen = must(program.getSourceFiles().find((f) => f.fileName.endsWith("shared/lib/contracts.gen.ts")));
    let ops: ts.Type | undefined;
    ts.forEachChild(gen, (n) => { if (ts.isTypeAliasDeclaration(n) && n.name.text === "OperationContracts") ops = c.getTypeAtLocation(n.name); });
    const resultOf = (op: string) => {
      const s = must(ops).getProperty(op);
      const r = s && c.getTypeOfSymbol(s).getProperty("result");
      return r ? c.getTypeOfSymbol(r) : null;
    };
    const singleText = (t: ts.Type) => c.typeToString(t, undefined, ts.TypeFormatFlags.NoTruncation | ts.TypeFormatFlags.InTypeAlias);
    const variants = (t: ts.Type) => (t.isUnion() ? t.types : [t]);
    const isObject = (t: ts.Type) => !!(t.flags & ts.TypeFlags.Object) || (t.isIntersection() && t.types.every((x) => x.flags & ts.TypeFlags.Object));
    const literal = ts.TypeFlags.StringLiteral | ts.TypeFlags.NumberLiteral | ts.TypeFlags.BooleanLiteral;
    const literals = (t: ts.Type) => { // the fields of literal types (candidate discriminants)
      const out = new Map<string, ts.Type>();
      if (!isObject(t)) return out;
      for (const s of c.getPropertiesOfType(t)) {
        const ft = c.getNonNullableType(c.getTypeOfSymbol(s));
        if (variants(ft).every((m) => m.flags & literal)) out.set(s.getName(), ft);
      }
      return out;
    };
    const overlap = (a: ts.Type, b: ts.Type) => variants(a).some((x) => variants(b).some((y) => c.isTypeAssignableTo(x, y) || c.isTypeAssignableTo(y, x)));
    const fits = (contract: ts.Type, own: ts.Type) => { // the literal fields both have can hold the same value
      const a = literals(contract);
      for (const [k, v] of literals(own)) if (a.has(k) && !overlap(must(a.get(k)), v)) return false;
      return true;
    };
    const compare = (source: ts.Type, target: ts.Type, path: string, depth: number, out: string[], top: boolean): void => {
      if (c.isTypeAssignableTo(source, target)) return;
      const at = path || "(result)";
      if (depth > 6) { out.push(`${at}: contract ${singleText(source)}, web ${singleText(target)}`); return; }
      if (c.isTypeAssignableTo(c.getNullType(), source) && !c.isTypeAssignableTo(c.getNullType(), target)) out.push(`${at}: may be null in the contract`);
      const s = c.getNonNullableType(source), d = c.getNonNullableType(target);
      if (c.isTypeAssignableTo(s, d)) return;
      if (c.isArrayType(s) && c.isArrayType(d)) { compare(must(typeArguments(c, s)[0]), must(typeArguments(c, d)[0]), `${path}[]`, depth + 1, out, top); return; }
      if (s.isUnion() && !(s.flags & ts.TypeFlags.Boolean) && variants(s).some((item) => isObject(item))) {
        const sv = variants(s), dv = variants(d);
        const keys = sv.every((item) => isObject(item))
          ? literals(must(sv[0])).keys().filter((k) => sv.every((m) => literals(m).has(k)) && sv.every((m, i) => sv.every((n, j) => i === j || !overlap(must(literals(m).get(k)), must(literals(n).get(k)))))).toArray()
          : [];
        if (top && keys.length === 0) { // the input picks the variant
          if (sv.some((m) => c.isTypeAssignableTo(m, d))) return;
          const tries = sv.map((m) => { const o: string[] = []; compare(m, d, path, depth + 1, o, false); return o; });
          out.push(...must(tries.toSorted((a, b) => a.length - b.length).at(0)));
          return;
        }
        if (top && dv.every((x) => !(isObject(x) && keys.some((k) => !!x.getProperty(k))))) {
          const firstKey = must(keys[0]);
          out.push(`${at}: name the ${keys.join(" / ")} the screen reads (the contract has ${sv.map((m) => singleText(must(literals(m).get(firstKey)))).join(", ")})`);
          return;
        }
        const candidates = top ? sv.filter((v) => dv.some((x) => fits(v, x))) : sv;
        for (const m of candidates) {
          if (dv.some((x) => c.isTypeAssignableTo(m, x))) continue;
          const own = dv.find((x) => isObject(m) && isObject(x) && literals(m).size > 0 && literals(x).size > 0 && fits(m, x) && fits(x, m));
          if (own || dv.length === 1) compare(m, own ?? d, path, depth + 1, out, false);
          else out.push(`${at}: the web does not cover ${singleText(m)}`);
        }
        return;
      }
      if (d.isUnion() && isObject(s)) {
        const own = variants(d).find((x) => isObject(x) && fits(s, x));
        if (own) { compare(s, own, path, depth + 1, out, false); return; }
      }
      if (isObject(s) && isObject(d) && !c.isArrayType(d)) {
        const isPage = top && !!s.getProperty("nextCursor");
        for (const f of c.getPropertiesOfType(d)) {
          const name = f.getName(), sf = s.getProperty(name), field = `${path}${path ? "." : ""}${name}`;
          if (!sf) { out.push(`${field}: not in the contract`); continue; }
          compare(c.getTypeOfSymbol(sf), c.getTypeOfSymbol(f), field, depth + 1, out, isPage && name === "items");
        }
        return;
      }
      out.push(`${at}: contract ${singleText(source)}, web ${singleText(target)}`);
    };
    for (const sf of program.getSourceFiles()) {
      const f = sf.fileName;
      if (!f.startsWith(directory + "/") || /node_modules|\/\.next\/|\/tests\/|(?:\.d\.ts$)/.test(f) || seen.has(f)) continue; // shared files once, by shared
      seen.add(f);
      const visit = (n: ts.Node) => {
        if (ts.isCallExpression(n) && ts.isIdentifier(n.expression) && calls.has(n.expression.text) && n.arguments[0] && ts.isStringLiteral(n.arguments[0])) {
          const op = n.arguments[0].text, fn = n.expression.text;
          const where = `${path.relative(web, f)}:${String(sf.getLineAndCharacterOfPosition(n.getStart(sf)).line + 1)} ${op}`;
          let result = resultOf(op);
          let own: ts.Type | undefined;
          if (n.typeArguments?.length) own = c.getTypeFromTypeNode(must(n.typeArguments[0]));
          else { // inferred from the context (a typed variable or return)
            const sig = c.getResolvedSignature(n);
            const returned = sig && c.getReturnTypeOfSignature(sig);
            own = returned?.getSymbol()?.getName() === "Promise" ? typeArguments(c, returned)[0] : returned;
            if (fn === "coreAll" && own && c.isArrayType(own)) own = typeArguments(c, own)[0];
          }
          if (fn === "coreAll" && result) {
            const items = result.getProperty("items");
            result = items ? typeArguments(c, c.getTypeOfSymbol(items))[0] ?? null : null;
          }
          if (!result) problems.push(`${where}: the contract has no result for it`);
          else if (own && !(own.flags & (ts.TypeFlags.Any | ts.TypeFlags.Unknown))) {
            checked++;
            const out: string[] = [];
            compare(result, own, "", 0, out, true);
            if (out.length > 0) problems.push(`${where}: ${[...new Set(out)].join("; ")}`);
          }
        }
        ts.forEachChild(n, visit);
      };
      visit(sf);
    }
  }
  return { checked, problems };
}

describe("the web's result types against the Core API contract (IR314)", () => {
  it("accept what the contract answers: every field, null and variant the screen reads", () => {
    const { checked, problems } = check();
    expect(problems).toEqual([]);
    expect(checked).toBeGreaterThan(350); // the scan sees the typed calls
  }, 120_000);
});
