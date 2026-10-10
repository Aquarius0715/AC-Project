// @vitest-environment node
import { join, relative } from "node:path";
import { fileURLToPath } from "node:url";
import ts from "typescript";
import { describe, expect, it } from "vitest";

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
const calls = ["coreOp", "coreAll", "callOp", "useOp"];

function check(): { checked: number; problems: string[] } {
  const seen = new Set<string>();
  const problems: string[] = [];
  let checked = 0;
  for (const p of projects) {
    const dir = join(web, p);
    const cfg = ts.readConfigFile(join(dir, "tsconfig.json"), ts.sys.readFile);
    const parsed = ts.parseJsonConfigFileContent(cfg.config, ts.sys, dir);
    const program = ts.createProgram(parsed.fileNames, { ...parsed.options, noEmit: true, incremental: false });
    const c = program.getTypeChecker();
    const gen = program.getSourceFiles().find((f) => f.fileName.endsWith("shared/lib/contracts.gen.ts"))!;
    let ops: ts.Type | undefined;
    ts.forEachChild(gen, (n) => { if (ts.isTypeAliasDeclaration(n) && n.name.text === "OperationContracts") ops = c.getTypeAtLocation(n.name); });
    const resultOf = (op: string) => {
      const s = ops!.getProperty(op);
      const r = s && c.getTypeOfSymbol(s).getProperty("result");
      return r ? c.getTypeOfSymbol(r) : null;
    };
    const str = (t: ts.Type) => c.typeToString(t, undefined, ts.TypeFormatFlags.NoTruncation | ts.TypeFormatFlags.InTypeAlias);
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
      for (const [k, v] of literals(own)) if (a.has(k) && !overlap(a.get(k)!, v)) return false;
      return true;
    };
    const compare = (src: ts.Type, dst: ts.Type, path: string, depth: number, out: string[], top: boolean): void => {
      if (c.isTypeAssignableTo(src, dst)) return;
      const at = path || "(result)";
      if (depth > 6) { out.push(`${at}: contract ${str(src)}, web ${str(dst)}`); return; }
      if (c.isTypeAssignableTo(c.getNullType(), src) && !c.isTypeAssignableTo(c.getNullType(), dst)) out.push(`${at}: may be null in the contract`);
      const s = c.getNonNullableType(src), d = c.getNonNullableType(dst);
      if (c.isTypeAssignableTo(s, d)) return;
      if (c.isArrayType(s) && c.isArrayType(d)) return compare(c.getTypeArguments(s as ts.TypeReference)[0], c.getTypeArguments(d as ts.TypeReference)[0], `${path}[]`, depth + 1, out, top);
      if (s.isUnion() && !(s.flags & ts.TypeFlags.Boolean) && variants(s).some(isObject)) {
        const sv = variants(s), dv = variants(d);
        const keys = sv.every(isObject)
          ? [...literals(sv[0]).keys()].filter((k) => sv.every((m) => literals(m).has(k)) && sv.every((m, i) => sv.every((n, j) => i === j || !overlap(literals(m).get(k)!, literals(n).get(k)!))))
          : [];
        if (top && !keys.length) { // the input picks the variant
          if (sv.some((m) => c.isTypeAssignableTo(m, d))) return;
          const tries = sv.map((m) => { const o: string[] = []; compare(m, d, path, depth + 1, o, false); return o; });
          out.push(...tries.reduce((a, b) => (b.length < a.length ? b : a)));
          return;
        }
        if (top && !dv.some((x) => isObject(x) && keys.some((k) => !!x.getProperty(k)))) {
          out.push(`${at}: name the ${keys.join(" / ")} the screen reads (the contract has ${sv.map((m) => str(literals(m).get(keys[0])!)).join(", ")})`);
          return;
        }
        for (const m of top ? sv.filter((v) => dv.some((x) => fits(v, x))) : sv) {
          if (dv.some((x) => c.isTypeAssignableTo(m, x))) continue;
          const own = dv.find((x) => isObject(m) && isObject(x) && literals(m).size > 0 && literals(x).size > 0 && fits(m, x) && fits(x, m));
          if (own || dv.length === 1) compare(m, own ?? d, path, depth + 1, out, false);
          else out.push(`${at}: the web does not cover ${str(m)}`);
        }
        return;
      }
      if (d.isUnion() && isObject(s)) {
        const own = variants(d).find((x) => isObject(x) && fits(s, x));
        if (own) return compare(s, own, path, depth + 1, out, false);
      }
      if (isObject(s) && isObject(d) && !c.isArrayType(d)) {
        const page = top && !!s.getProperty("nextCursor");
        for (const f of c.getPropertiesOfType(d)) {
          const name = f.getName(), sf = s.getProperty(name), field = `${path}${path ? "." : ""}${name}`;
          if (!sf) { out.push(`${field}: not in the contract`); continue; }
          compare(c.getTypeOfSymbol(sf), c.getTypeOfSymbol(f), field, depth + 1, out, page && name === "items");
        }
        return;
      }
      out.push(`${at}: contract ${str(src)}, web ${str(dst)}`);
    };
    for (const sf of program.getSourceFiles()) {
      const f = sf.fileName;
      if (!f.startsWith(dir + "/") || /node_modules|\/\.next\/|__tests__|\.d\.ts$/.test(f) || seen.has(f)) continue; // shared files once, by shared
      seen.add(f);
      const visit = (n: ts.Node) => {
        if (ts.isCallExpression(n) && ts.isIdentifier(n.expression) && calls.includes(n.expression.text) && n.arguments[0] && ts.isStringLiteral(n.arguments[0])) {
          const op = n.arguments[0].text, fn = n.expression.text;
          const where = `${relative(web, f)}:${sf.getLineAndCharacterOfPosition(n.getStart(sf)).line + 1} ${op}`;
          let result = resultOf(op);
          let own: ts.Type | undefined;
          if (n.typeArguments?.length) own = c.getTypeFromTypeNode(n.typeArguments[0]);
          else { // inferred from the context (a typed variable or return)
            const sig = c.getResolvedSignature(n);
            const ret = sig && c.getReturnTypeOfSignature(sig);
            own = ret?.symbol?.getName() === "Promise" ? c.getTypeArguments(ret as ts.TypeReference)[0] : ret;
            if (fn === "coreAll" && own && c.isArrayType(own)) own = c.getTypeArguments(own as ts.TypeReference)[0];
          }
          if (fn === "coreAll" && result) {
            const items = result.getProperty("items");
            result = items ? c.getTypeArguments(c.getTypeOfSymbol(items) as ts.TypeReference)[0] : null;
          }
          if (!result) problems.push(`${where}: the contract has no result for it`);
          else if (own && !(own.flags & (ts.TypeFlags.Any | ts.TypeFlags.Unknown))) {
            checked++;
            const out: string[] = [];
            compare(result, own, "", 0, out, true);
            if (out.length) problems.push(`${where}: ${[...new Set(out)].join("; ")}`);
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
