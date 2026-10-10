// /admin/billing (FR-A08, FR-A23, SCR-A08): in API mode a Server Component reads invoices for the URL scope
// (customerId → propertyId → contractId, the billing months from / to as Kuala Lumpur months (IR321), overdueOnly
// from the dashboard, SR06), contracts, customers, properties and
// inquiries, the selected invoice (invoices.get) with its reminder recipients, and on the payouts tab the statements,
// through the DAL. URL keys follow the screen catalog; a notification link with only inquiryId opens the inquiries tab
// (D08). Writes are Server Actions (actions.ts). Texts in the display language; billing periods, due dates and pay
// dates are Kuala Lumpur business days, and the inquiry times are formatted here, in the display time zone (IR282,
// IR299). The Phase 1A demo keeps the fixture rows.
import { connection } from "next/server";
import { apiMode, coreDisplay, coreNow, coreOp } from "@ac/web/lib/dal";
import { i18nOf } from "@ac/web/lib/i18n";
import {
  billingMonths, contractOptions, inquiryRows, invoiceDetail, invoiceRows, monthsText, statementDetail, statementRows,
  type ApiContract, type ApiCustomer, type ApiInquiry, type ApiInvoice, type ApiInvoiceDetail, type ApiOrganization, type ApiProperty, type ApiRecipient, type ApiStatement,
} from "@ac/web/lib/billing";
import { BillingView, type BillingLive } from "./_components/billing-view";

type Page<T> = { items: T[] };

export default async function AdminBillingPage({ searchParams }: PageProps<"/admin/billing">) {
  await connection();
  if (!apiMode()) return <BillingView />;
  const sp = await searchParams;
  const one = (k: string) => (typeof sp[k] === "string" && sp[k] ? (sp[k] as string) : undefined);
  const tab = one("tab") === "payouts" ? "payouts" : one("tab") === "inquiries" || (!one("tab") && one("inquiryId") && !one("invoiceId")) ? "inquiries" : "invoices";
  const months = billingMonths(one("from"), one("to"));
  const scope = { customerId: one("customerId"), propertyId: one("propertyId"), contractId: one("contractId"), overdueOnly: one("overdueOnly") === "true", from: months.from, to: months.to };
  const filters = {
    ...(scope.customerId && { customerId: scope.customerId }), ...(scope.propertyId && { propertyId: scope.propertyId }), ...(scope.contractId && { contractId: scope.contractId }),
    ...(scope.overdueOnly && { overdueOnly: true }), ...months.filters,
  };
  const [now, display, invoices, contracts, customers, properties, inquiries] = await Promise.all([
    coreNow(), coreDisplay(),
    coreOp<Page<ApiInvoice>>("invoices.list", { limit: 100, filters }),
    coreOp<Page<ApiContract>>("contracts.list", { limit: 100 }),
    coreOp<Page<ApiCustomer>>("customers.list", { limit: 100 }),
    coreOp<Page<ApiProperty>>("properties.list", { limit: 100 }),
    coreOp<Page<ApiInquiry>>("inquiries.list", { limit: 100 }),
  ]);
  const i = i18nOf(display);
  const rows = invoiceRows(invoices.items, contracts.items, customers.items, now, i);
  const orgCustomer = new Map(customers.items.map((c) => [c.organizationId, c.id]));
  const live: BillingLive = {
    tab, scope, months: monthsText(months.from, months.to, i), rows, contracts: contractOptions(contracts.items, customers.items, i.t), inquiries: inquiryRows(inquiries.items, customers.items, i),
    customers: customers.items.map((c) => ({ id: c.id, name: c.name })),
    properties: properties.items.filter((p) => !p.archived).map((p) => ({ id: p.id, name: p.name, customerId: orgCustomer.get(p.customerOrgId) ?? "" })),
    contractsOfCustomer: contracts.items.map((k) => ({ id: k.id, customerId: k.customerId })),
    selectedInquiryId: one("inquiryId"),
  };
  if (tab === "invoices") {
    const sel = rows.find((r) => r.id === one("invoiceId")) ?? rows.find((r) => r.st === "Overdue") ?? rows[0];
    if (sel) {
      live.selectedInvoice = invoiceDetail(await coreOp<ApiInvoiceDetail>("invoices.get", { id: sel.id }), i.t);
      if (sel.st === "Overdue") {
        const r = await coreOp<Page<ApiRecipient>>("notifications.recipients", { target: { kind: "invoice", id: sel.id }, templateKey: "payment_reminder", channel: "email", query: { limit: 50 } });
        live.recipients = r.items;
      }
    }
  }
  if (tab === "payouts") {
    const [statements, orgs] = await Promise.all([coreOp<Page<ApiStatement>>("payouts.list", { limit: 100 }), coreOp<Page<ApiOrganization>>("organizations.list", { limit: 100 })]);
    live.statements = statementRows(statements.items, orgs.items, i);
    const selId = one("statementId") ?? live.statements[0]?.id;
    if (selId) live.statement = statementDetail(await coreOp<ApiStatement>("payouts.get", { id: selId }), orgs.items, now, i);
  }
  return <BillingView live={live} />;
}
