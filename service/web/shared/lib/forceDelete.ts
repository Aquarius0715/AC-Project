// The accounts step of a forced delete of a customer or a contractor (data.forceDelete): every membership of the
// organization, including the ended ones, is deleted with members.delete force, one after the other, before the
// organization itself. The Core API refuses a caller without the permission, and refuses an account with an assignment
// or a certificate (memberUsed): the delete stops there, and what is left can be deleted again.
import { coreAll, coreOp } from "@ac/web/lib/dal";

interface Account { id: string; version: number }

async function deleteOne(accounts: Account[], reason: string): Promise<void> {
  const [first, ...rest] = accounts;
  if (!first) return;
  await coreOp("members.delete", { id: first.id, force: true, reason }, { write: true, expectedVersion: first.version });
  await deleteOne(rest, reason);
}

/** members.delete force for every account of the organization, one at a time. */
export async function deleteAccounts(organizationId: string, reason: string): Promise<void> {
  const accounts = await coreAll<Account>("members.list", { filters: { organizationId } });
  await deleteOne(accounts, reason);
}
