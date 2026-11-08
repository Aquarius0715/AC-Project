// Package money holds the rule every amount in minor units (sen, cents) follows on its way in: a whole number within
// JavaScript's safe-integer range (implementation-contracts.md, Money), so the web shows the amount that was stored and
// a sum of lines cannot wrap around.
package money

// MaxMinor is the largest amount in minor units: 2^53 − 1, the largest integer a JavaScript number holds exactly.
const MaxMinor int64 = 1<<53 - 1

// InRange reports whether amount is a valid amount in minor units: not negative and at most MaxMinor.
func InRange(amount int64) bool { return amount >= 0 && amount <= MaxMinor }
