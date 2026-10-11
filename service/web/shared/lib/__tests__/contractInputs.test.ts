// @vitest-environment node
import { existsSync, readFileSync, writeFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import ts from "typescript";
import { describe, expect, it } from "vitest";

// The input and result fields of every operation in the Core API contract (OperationContracts[op] of contracts.gen.ts,
// a copy of docs/02-design/service-contracts.ts): named types, intersections and unions resolved by the TypeScript
// checker (a union lists the fields of all its variants).
// - contractInputs.gen.json: the input fields. TestInputsMatchContract holds every operation's Go input type to it, so
//   a field outside the contract is an unknown field and every contract field is read (IR391).
// - contractResults.gen.json: the result fields, and for a page or an array the fields of its items.
//   TestResultsMatchContract holds every operation's Go result type to it, so the Core API answers no field outside
//   the contract and none of it is missing (IR392).
// After a contract change, regenerate both with
// UPDATE_CONTRACT_INPUTS=1 npx vitest run shared/lib/__tests__/contractInputs.test.ts
const gen = fileURLToPath(new URL("../contracts.gen.ts", import.meta.url));
const snapshot = fileURLToPath(new URL("../contractInputs.gen.json", import.meta.url));
const resultsSnapshot = fileURLToPath(new URL("../contractResults.gen.json", import.meta.url));

type ResultFields = { fields: string[]; items: string[] | null };

function contract(): { inputs: Record<string, string[]>; results: Record<string, ResultFields> } {
  const program = ts.createProgram([gen], { strict: true, noEmit: true, skipLibCheck: true });
  const c = program.getTypeChecker();
  const sf = program.getSourceFile(gen)!;
  let ops: ts.Type | undefined;
  ts.forEachChild(sf, (n) => { if (ts.isTypeAliasDeclaration(n) && n.name.text === "OperationContracts") ops = c.getTypeAtLocation(n.name); });
  const fields = (t: ts.Type, out: Set<string>) => {
    if (t.isUnion() || t.isIntersection()) t.types.forEach((x) => fields(x, out));
    else if (t.flags & ts.TypeFlags.Object) c.getPropertiesOfType(t).forEach((p) => out.add(p.getName()));
  };
  const sorted = (t: ts.Type) => { const set = new Set<string>(); fields(t, set); return [...set].sort(); };
  const variants = (t: ts.Type): ts.Type[] => (t.isUnion() ? t.types.flatMap(variants) : [t]);
  const element = (t: ts.Type) => (c.isArrayType(t) ? c.getIndexInfoOfType(t, ts.IndexKind.Number)?.type ?? null : null);
  const inputs: Record<string, string[]> = {};
  const results: Record<string, ResultFields> = {};
  for (const op of c.getPropertiesOfType(ops!).sort((a, b) => a.getName().localeCompare(b.getName()))) {
    const contract = c.getTypeOfSymbol(op);
    inputs[op.getName()] = sorted(c.getTypeOfSymbol(contract.getProperty("input")!));
    const result = c.getTypeOfSymbol(contract.getProperty("result")!);
    const array = element(result);
    if (array) {
      results[op.getName()] = { fields: [], items: sorted(array) };
      continue;
    }
    const items = variants(result).map((v) => v.getProperty("items")).filter((p): p is ts.Symbol => !!p).map((p) => element(c.getTypeOfSymbol(p))).filter((e): e is ts.Type => !!e);
    results[op.getName()] = { fields: sorted(result), items: items.length ? [...new Set(items.flatMap(sorted))].sort() : null };
  }
  return { inputs, results };
}

describe("the contract's field lists", () => {
  const now = contract();
  const check = (file: string, value: unknown) => {
    const text = JSON.stringify(value, null, 1) + "\n";
    if (process.env.UPDATE_CONTRACT_INPUTS === "1") writeFileSync(file, text);
    expect(existsSync(file) ? readFileSync(file, "utf8") : "", "regenerate with UPDATE_CONTRACT_INPUTS=1 (see the header)").toBe(text);
  };
  it("contractInputs.gen.json lists the input fields of every operation", () => check(snapshot, now.inputs));
  it("contractResults.gen.json lists the result fields of every operation", () => check(resultsSnapshot, now.results));
});
