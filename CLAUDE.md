# Code quality and lint rules

These rules apply to every change in this repository, by people and by agents. They adapt the strict linting
constitution adopted on 2026-11-03 to this project's tools: Go with golangci-lint in `service/api`, and Next.js with
ESLint in `service/web` (npm workspaces).

## 1. Escape hatches and types

- A suppression (`//nolint`, `eslint-disable`, `@ts-expect-error`) names its rule and gives the specific reason on the
  same line, for example `//nolint:gosec // G704: the request goes to 127.0.0.1, this process's own /healthz`.
  `@ts-ignore` and `@ts-nocheck` are never used. The linters refuse a suppression without a reason, one that is not
  used, and any suppression of `react-hooks/exhaustive-deps`. The list of suppressions is reported for review at each
  milestone.
- TypeScript has no `any`, no `as any`, no `as unknown as T`, and no assertion that narrows a type. A value typed
  `unknown` is narrowed by a runtime check or a Zod schema before it is used.
- Go uses a concrete or constrained type wherever one applies, not `any`.
- Input from outside the process is validated where it enters:
  - web apps: Zod for the environment, cookies and tokens, browser storage, and JSON from the BFF and the Core API;
  - Go: typed binding structs with `Validate()` for requests, events and the environment; database rows scan into typed
    values.

## 2. Go

- Every error is handled. An error from another package is wrapped with where it happened,
  `apperr.Wrap(err, "select assets.units")` (the same as `fmt.Errorf("select assets.units: %w", err)`), or replaced by a
  domain error (`apperr.E`, `apperr.Fields`).
- HTTP response bodies, rows and files are closed right after they are opened, with `defer`.
- I/O and database functions take `context.Context` as their first parameter.
- Names follow Go's conventions: short where the scope is short (`err`, `ctx`, `ok`, `id`, `i`, `tx`, `db`). A name is
  never made longer only to satisfy a length linter.

## 3. TypeScript, Next.js and React

- typescript-eslint `strictTypeChecked` and `stylisticTypeChecked` hold everywhere.
- The react-hooks rules hold; a dependency array is never suppressed.
- jsx-a11y `strict` holds, and there is no `dangerouslySetInnerHTML`.

## 4. Fixing lint findings

1. Formatting is the formatters' job, never fixed by hand: `golangci-lint fmt` (gofumpt, gci, goimports, golines) in Go,
   and Prettier in the web apps once it is added.
2. Fix the cause: the type, the logic or the leak. Do not split a function into trivial pieces, or reshuffle unrelated
   files, only to pass a size or complexity limit. The limits in `service/api/.golangci.yml` and
   `service/web/eslint.base.mjs` are set to flag code that is hard to read; tests are not held to them.
3. If a finding is still there after three attempts to fix it, stop: show the exact log, explain the conflict and ask.

## 5. Done means

- Go (`service/api`): `golangci-lint run ./...` reports 0 issues, and `make test-all` passes. That target runs
  `go vet ./...` and `go test ./...` on fresh test databases, plus the cluster suite.
- Web (`service/web`): `npm run lint:strict`, `npm run typecheck` and `npm test` report 0 errors.
- `sh scripts/check.sh` exits 0 before a commit. Capture its exit code; never pipe it.
- The raw output of these commands is shown before asking for review.
- While the lint backlog of November 2026 is being cleared, a Go change keeps the packages it touches at 0 issues and a
  web change adds no finding. The whole tree reaches 0 when the backlog is done, and CI then requires it.
