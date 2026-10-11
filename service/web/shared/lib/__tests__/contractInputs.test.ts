// @vitest-environment node
import { existsSync, readFileSync, writeFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import ts from "typescript";
import { describe, expect, it } from "vitest";

// The input fields of every operation in the Core API contract (OperationContracts[op].input of contracts.gen.ts, a
// copy of docs/02-design/service-contracts.ts), as contractInputs.gen.json: named types, intersections and unions
// resolved by the TypeScript checker (a union lists the fields of all its variants). The Core API's integration test
// TestInputsMatchContract holds every operation's Go input type to this list, so a field outside the contract is an
// unknown field and every contract field is read (IR391). After a contract change, regenerate the list with
// UPDATE_CONTRACT_INPUTS=1 npx vitest run shared/lib/__tests__/contractInputs.test.ts
const gen = fileURLToPath(new URL("../contracts.gen.ts", import.meta.url));
const snapshot = fileURLToPath(new URL("../contractInputs.gen.json", import.meta.url));

function contractInputs(): Record<string, string[]> {
  const program = ts.createProgram([gen], { strict: true, noEmit: true, skipLibCheck: true });
  const c = program.getTypeChecker();
  const sf = program.getSourceFile(gen)!;
  let ops: ts.Type | undefined;
  ts.forEachChild(sf, (n) => { if (ts.isTypeAliasDeclaration(n) && n.name.text === "OperationContracts") ops = c.getTypeAtLocation(n.name); });
  const fields = (t: ts.Type, out: Set<string>) => {
    if (t.isUnion() || t.isIntersection()) t.types.forEach((x) => fields(x, out));
    else c.getPropertiesOfType(t).forEach((p) => out.add(p.getName()));
  };
  const out: Record<string, string[]> = {};
  for (const op of c.getPropertiesOfType(ops!).sort((a, b) => a.getName().localeCompare(b.getName()))) {
    const input = c.getTypeOfSymbol(op).getProperty("input")!;
    const set = new Set<string>();
    fields(c.getTypeOfSymbol(input), set);
    out[op.getName()] = [...set].sort();
  }
  return out;
}

describe("contractInputs.gen.json", () => {
  it("lists the input fields of every operation in the contract", () => {
    const now = JSON.stringify(contractInputs(), null, 1) + "\n";
    if (process.env.UPDATE_CONTRACT_INPUTS === "1") writeFileSync(snapshot, now);
    expect(existsSync(snapshot) ? readFileSync(snapshot, "utf8") : "", "regenerate with UPDATE_CONTRACT_INPUTS=1 (see the header)").toBe(now);
  });
});
