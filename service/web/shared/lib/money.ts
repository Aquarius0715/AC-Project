// Money in the screens: minor units shown with two decimals and thousands separators, as the Figma screens show them
// (“2,110.00 MYR”); one formatter for every screen that shows an amount before its currency.

/** “2,110.00” for 211000 minor units; a negative amount keeps its minus sign. */
export const amount = (minor: number) => (minor / 100).toLocaleString("en-MY", { minimumFractionDigits: 2, maximumFractionDigits: 2 });

/** “2,110.00 MYR”. */
export const money = (minor: number, currency: string) => `${amount(minor)} ${currency}`;
