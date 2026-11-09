// The synthetic accounts loadseed made (service/api/cmd/loadseed): who signs in, as which membership, and which units and
// jobs their screens open. The file is read once in the init context and checked here, because it comes from outside the
// script.
import { SharedArray } from "k6/data";

type Group = "customer" | "partner" | "technician" | "hq";

export interface User {
  username: string;
  password: string;
  group: Group;
  tenantId: string;
  membershipId: string;
  customerId: string;
  unitIds: string[];
  jobIds: string[];
}

const GROUPS = new Set(["customer", "partner", "technician", "hq"]);

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function isGroup(value: unknown): value is Group {
  return typeof value === "string" && GROUPS.has(value);
}

function items(value: unknown): unknown[] | null {
  return Array.isArray(value) ? value : null;
}

function strings(value: unknown): string[] | null {
  const list = items(value);
  if (!list) return null;
  const out: string[] = [];
  for (const item of list) {
    if (typeof item !== "string") return null;
    out.push(item);
  }
  return out;
}

function userOf(value: unknown): User | null {
  if (!isRecord(value)) return null;
  const { username, password, group, tenantId, membershipId, customerId } = value;
  const malformed =
    !isGroup(group) ||
    typeof username !== "string" ||
    typeof password !== "string" ||
    typeof tenantId !== "string" ||
    typeof membershipId !== "string" ||
    typeof customerId !== "string";
  if (malformed) return null;
  const unitIds = strings(value.unitIds);
  const jobIds = strings(value.jobIds);
  return unitIds && jobIds ? { username, password, group, tenantId, membershipId, customerId, unitIds, jobIds } : null;
}

/** The accounts of the manifest file, shared between the virtual users; a malformed file stops the test. */
export function loadUsers(path: string): User[] {
  return new SharedArray("users", () => {
    const parsed: unknown = JSON.parse(open(path));
    const list = items(isRecord(parsed) ? parsed.users : null);
    if (!list) throw new Error(`${path}: no users`);
    return list.map((entry, i) => {
      const user = userOf(entry);
      if (!user) throw new Error(`${path}: user ${String(i)} is malformed`);
      return user;
    });
  });
}
