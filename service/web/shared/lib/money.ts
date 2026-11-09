// Money in the screens: minor units shown with two decimals and thousands separators, as the Figma screens show them
// (“2,110.00 MYR”); one formatter for every screen that shows an amount before its currency.

/** “2,110.00” for 211000 minor units; a negative amount keeps its minus sign. */
export const amount = (minor: number) => (minor / 100).toLocaleString("en-MY", { minimumFractionDigits: 2, maximumFractionDigits: 2 });

/** “2,110.00 MYR”. */
export const money = (minor: number, currency: string) => `${amount(minor)} ${currency}`;

/** The amounts' total in each currency, in the order the currencies first appear (“130.00 MYR + 50.00 USD”): amounts
of different currencies are never added together (IR707). */
export function moneyByCurrency(items: readonly { amountMinor: number; currency: string }[]): string {
  const totals = new Map<string, number>();
  for (const x of items) totals.set(x.currency, (totals.get(x.currency) ?? 0) + x.amountMinor);
  return [...totals].map(([currency, minor]) => money(minor, currency)).join(" + ");
}
