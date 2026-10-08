#!/usr/bin/env python3
"""Merge Go cover profiles produced per package with -coverpkg (keeps the max count per block) and print the total."""
import sys
blocks, mode = {}, "set"
for path in sys.argv[2:]:
    for i, line in enumerate(open(path)):
        if i == 0 or line.startswith("mode:"):
            mode = line.split(":", 1)[1].strip() if line.startswith("mode:") else mode
            continue
        key, n_stmt, cnt = line.rsplit(" ", 2)
        prev = blocks.get(key, (int(n_stmt), 0))
        blocks[key] = (int(n_stmt), max(prev[1], int(cnt)))
with open(sys.argv[1], "w") as out:
    out.write(f"mode: {mode}\n")
    for k, (n, c) in sorted(blocks.items()):
        out.write(f"{k} {n} {c}\n")
total = sum(n for n, _ in blocks.values())
covered = sum(n for n, c in blocks.values() if c > 0)
print(f"total: {covered / total * 100:.1f}% of {total} statements")
