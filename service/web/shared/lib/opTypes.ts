// Typed Core API calls (IR313): an operation's input is OperationContracts[op].input of the contract types generated
// from docs/02-design/service-contracts.ts (contracts.gen.ts, `make gen` in service/api), so tsc checks every coreOp /
// coreAll / callOp / useOp call against the contract. The literal operation name picks the input type.
import type { OperationContracts } from "@ac/web/lib/contracts.gen";

export type { OperationContracts };
export type OpName = keyof OperationContracts;
export type OpInput<K extends OpName> = OperationContracts[K]["input"];
/** [operation, input, options] for every operation: a union of tuples, so callers keep naming only the result type. */
export type OpArgs<O> = { [K in OpName]: [operation: K, input: OpInput<K>, opts?: O] }[OpName];
/** [operation, input] for every operation. */
export type OpCall = { [K in OpName]: [operation: K, input: OpInput<K>] }[OpName];
/** The query part of a list operation's input: its filters and sort (coreAll adds the paging). */
export type ListQuery<K extends OpName> = Partial<Pick<OpInput<K>, Extract<keyof OpInput<K>, "filters" | "sort">>>;
/** [operation, query, max] for every operation. */
export type ListArgs = { [K in OpName]: [operation: K, query?: ListQuery<K>, max?: number] }[OpName];
