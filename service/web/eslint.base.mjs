// The web's lint, shared by the four apps, the shared package and the E2E specs. It enables every rule set the user
// asked for on 2026-11-03 ("全部入り"):
// - Next.js's own rules: core-web-vitals and typescript, set up as the Next.js docs do.
// - typescript-eslint strictTypeChecked and stylisticTypeChecked, with type information.
// - jsx-a11y strict, react recommended and jsx-runtime, and react-hooks recommended-latest (the React Compiler rules).
// - import recommended and typescript, plus checks for cycles and duplicates.
// - Every rule of unicorn, sonarjs and security.
// - vitest's recommended rules for the unit tests (playwright's come with the E2E config).
// The scripts run ESLint with --max-warnings=0, so every finding fails the lint.
import comments from "@eslint-community/eslint-plugin-eslint-comments/configs";
import js from "@eslint/js";
import vitest from "@vitest/eslint-plugin";
import nextVitals from "eslint-config-next/core-web-vitals";
import nextTs from "eslint-config-next/typescript";
import security from "eslint-plugin-security";
import sonarjs from "eslint-plugin-sonarjs";
import unicorn from "eslint-plugin-unicorn";
import { defineConfig, globalIgnores } from "eslint/config";
import path from "node:path";
import tseslint from "typescript-eslint";

// eslint-config-next registers react, react-hooks, import and jsx-a11y; their rule sets come from those same plugin
// objects (a second copy of a plugin under the same name is an error in flat config)
const nextPlugins = Object.assign({}, ...[...nextVitals, ...nextTs].map((c) => c.plugins ?? {}));

// every rule of a plugin that is not deprecated (a deprecated rule names its replacement, which is on)
const everyRule = (prefix, plugin) =>
  Object.fromEntries(
    Object.entries(plugin.rules)
      .filter(([, rule]) => !rule.meta?.deprecated)
      .map(([name]) => [`${prefix}/${name}`, "error"]),
  );

/** @param {string} tsconfigRootDir the directory of the tsconfig.json the files belong to */
export function webConfig(tsconfigRootDir) {
  return defineConfig([
    js.configs.recommended,
    ...nextVitals,
    ...nextTs,
    tseslint.configs.strictTypeChecked,
    tseslint.configs.stylisticTypeChecked,
    {
      languageOptions: {
        parserOptions: { projectService: { allowDefaultProject: ["*.config.mts"] }, tsconfigRootDir },
      },
    },
    { files: ["**/*.{js,mjs,cjs}"], extends: [tseslint.configs.disableTypeChecked] }, // config files have no types
    { rules: nextPlugins["jsx-a11y"].flatConfigs.strict.rules },
    // the shared form controls render a native control, so a <label> around one of them is associated with it
    {
      rules: {
        "jsx-a11y/label-has-associated-control": ["error", { controlComponents: ["Input", "Checkbox", "Switch", "Textarea", "NativeSelect"], assert: "either", depth: 3 }],
      },
    },
    { rules: { ...nextPlugins.react.configs.recommended.rules, ...nextPlugins.react.configs["jsx-runtime"].rules } },
    { rules: nextPlugins["react-hooks"].configs.flat["recommended-latest"].rules },
    {
      rules: {
        ...nextPlugins.import.flatConfigs.recommended.rules,
        ...nextPlugins.import.flatConfigs.typescript.rules,
        "import/no-cycle": "error",
        "import/no-duplicates": "error",
        "import/no-self-import": "error",
        "import/no-useless-path-segments": "error",
      },
    },
    unicorn.configs.all,
    { plugins: { sonarjs }, rules: everyRule("sonarjs", sonarjs) },
    { plugins: { security }, rules: everyRule("security", security) },
    comments.recommended,
    { files: ["**/tests/**", "**/*.test.{ts,tsx}"], ...vitest.configs.recommended },
    // Vitest's expect takes a message as its second argument (expect(value, "what failed"))
    {
      files: ["**/tests/**", "**/*.test.{ts,tsx}"],
      rules: {
        "vitest/valid-expect": ["error", { maxArgs: 2 }],
        // Round 3: the assertions that tell more. toStrictEqual compares undefined properties and classes; a throw is matched
        // by its message; hooks run in their order; a spy is checked for its one call
        "vitest/prefer-strict-equal": "error",
        "vitest/require-to-throw-message": "error",
        "vitest/prefer-hooks-in-order": "error",
        "vitest/prefer-called-once": "error",
      },
    },
    // Where a rule needs a fact of this project, or contradicts a contract or platform it has to follow:
    {
      rules: {
        // a consistency rule: this code writes (x) => …, so its options say so
        "sonarjs/arrow-function-convention": ["error", { requireParameterParentheses: true, requireBodyBraces: false }],
        // checks for a configured file header; the project has none
        "sonarjs/file-header": "off",
        // a consistency rule: this code writes a one-line doc comment on one line (/** … */), so its option says so
        "unicorn/single-line-block-comment-style": ["error", "single-line"],
        // React components are functions whose names start with a capital (JSX reads a lower-case name as a tag)
        // a notification type or a state code (certificate_expiry) names the method that words it, so snake_case is allowed too
        "sonarjs/function-name": ["error", { format: "^(?:[_a-z][a-zA-Z0-9]*|[A-Z][a-zA-Z0-9]*|[a-z][a-z0-9]*(?:_[a-z0-9]+)+)$" }],
        // React and the DOM name the class attribute className
        "unicorn/no-keyword-prefix": ["error", { disallowedPrefixes: ["new"] }],
        // the Core API contract (service-contracts.ts) uses null in request and response bodies (JSON.stringify drops
        // undefined), and a React component renders nothing for null
        "unicorn/no-null": "off",
        // Next.js targets Safari 16.4 and later, and Safari has no Temporal
        "unicorn/prefer-temporal": "off",
        // TypeScript's lib does not declare Iterator.concat yet, so a call to it does not type-check
        "unicorn/prefer-iterator-concat": "off",
        // file names follow the kind of module, and the design documents cite them: React components in PascalCase
        // (AppShell.tsx), modules in camelCase (adminAlerts.ts), Next.js route files and shadcn/ui components in
        // kebab-case
        "unicorn/filename-case": ["error", { cases: { camelCase: true, kebabCase: true, pascalCase: true } }],
        // JSX callbacks return their element in parentheses ((u) => (<li>…</li>)), React's convention
        "unicorn/consistent-arrow-return-style": "off",
      },
    },
    // The repository's lint rules (CLAUDE.md):
    {
      rules: {
        // 1: a suppression names its rule and says why, and a dependency array is never suppressed
        "@eslint-community/eslint-comments/require-description": "error",
        "@eslint-community/eslint-comments/no-restricted-disable": ["error", "react-hooks/exhaustive-deps"],
        // 1: no double assertion through unknown or any (no-unsafe-type-assertion, which needs types, is set below)
        "no-restricted-syntax": [
          "error",
          {
            selector: "TSAsExpression > TSAsExpression[typeAnnotation.type=/^TS(?:Unknown|Any)Keyword$/]",
            message: "Narrow the value with a runtime check or a Zod schema instead of asserting through unknown or any.",
          },
          {
            // a length of 0 is a number React renders ("0"); the number that stands alone in `&&` is this one
            selector: 'JSXExpressionContainer > LogicalExpression[operator="&&"] > MemberExpression.left[property.name="length"]',
            message: "`{list.length && <X />}` renders 0 for an empty list: compare it, `{list.length > 0 && <X />}`.",
          },
        ],
        // 3: no raw HTML
        "react/no-danger": "error",
        // 4-2: the cyclomatic count adds every case of a flat switch or lookup and every conditional render of a view, so a
        // screen component gets 60 (Go's limit is 30, which suits code without markup); the same 60 for the cognitive
        // measure. The few above it are split
        "sonarjs/cyclomatic-complexity": ["error", { threshold: 60 }],
        "sonarjs/cognitive-complexity": ["error", 60],
        // 4-2: a status, tone or kind is a union of its named values ("idle" | "running" | "failed" | "ok"); the default
        // (2 members) would flag every such union. The limit is 9 members: the Core API state machines (a command, a
        // proposal, a release) have up to 6 named states, and a page's modal state (new, decline, rate, ...) has up to 9.
        "sonarjs/max-union-size": ["error", { threshold: 9 }],
        // 4-2: a condition of four or five operators (a && b || !c ? x : y) still reads as one decision, so the limits are
        // 5 operators per expression and 4 nested calls (the defaults are 3 and 3); longer ones are split by hand
        "sonarjs/expression-complexity": ["error", { max: 5 }],
        "unicorn/max-nested-calls": ["error", { max: 4 }],
        // 4-2: a conditional suffix inside a template (`${a}${b ? \` · \${c}\` : ""}`) is the plainest way to write it; the
        // alternatives (a temporary for each suffix, or concatenation) read worse
        "sonarjs/no-nested-template-literals": "off",
        // 4-2: a try that wraps a request, its parse and its result is one unit, and its catch answers for all of it (return
        // null, show the error); the default (1) allows a single call. The widest such block here has complexity 9
        "unicorn/try-complexity": ["error", { max: 9 }],
        // 4-2: the order of a guard (`!reason.trim() || !isOrder || !tech`) follows how the form reads, and moving an operand
        // changes what short-circuits; the rule only saves a call that has no side effect
        "unicorn/prefer-simple-condition-first": "off",
        // t(a ? x : y) hides x and y from the test that checks every t("...") literal has a Malay text; two calls keep them visible
        "unicorn/prefer-minimal-ternary": "off",
        // 4-2: the module-level `let` cases here are single-owner caches on purpose: the session store client (one per
        // process), the BFF session cache, the demo stores and the focus handed to a remounted view. A holder object
        // would hide the same mutation behind a property
        "unicorn/no-top-level-assignment-in-function": "off",
        // getElementById takes any id as it is (member-row-${id}, ids with dots); querySelector(`#${id}`) would need escaping
        "unicorn/prefer-query-selector": "off",
        // an if / else if chain that ends at the last case needs no empty else (each branch sets one thing)
        "sonarjs/elseif-without-else": "off",
        // the status chains put other tests between the cases (a work window that ended, an acknowledgement), which a
        // switch cannot express
        "unicorn/prefer-switch": "off",
        // a read that is mapped by one step, in a list of parallel reads (Promise.all([read().then((r) => r.total), ...])), is
        // clearer than a wrapping async function; failures are handled with recover()
        "unicorn/prefer-await": "off",
        // safe-regex flags every optional group that holds a quantifier (^\d+(\.\d+)?$), which is linear; these patterns run
        // on short trimmed form values. The ones that are not linear are caught by sonarjs/slow-regex and super-linear-regex
        "security/detect-unsafe-regex": "off",
        // 2: names say what a value is, and are not lengthened only for a rule: DOM and React attributes (checked,
        // disabled), the contract's fields (enabled, online) and predicates (overlaps, fits) are not renamed is…/has…
        "unicorn/consistent-boolean-name": "off",
        // 4-2: a member of a new value reads as one expression (new Date(x).toISOString(), new Set(xs).size); a
        // variable for each would only lengthen the code
        "unicorn/no-unreadable-new-expression": "off",
        // 2: the conventional short names stay short. props, ref, params, req, res and env are the platform's and
        // React's; e, i, j, acc, prev, fn and el the usual names in handlers, loops, reducers and state updaters
        "unicorn/name-replacements": [
          "error",
          {
            replacements: {
              acc: false,
              args: false,
              // the app's button component (Btn) and shadcn/ui's lib/utils.ts keep the names their code and tooling use
              btn: false,
              utils: false,
              ctx: false,
              db: false,
              e: false,
              el: false,
              env: false,
              fn: false,
              i: false,
              j: false,
              params: false,
              prev: false,
              props: false,
              ref: false,
              refs: false,
              req: false,
              res: false,
            },
          },
        ],
      },
    },
    { settings: { "import/internal-regex": "^@ac/web(?:/|$)" } },
    // Round 3 (2026-11-07): rules outside the recommended sets, added as each reaches zero findings
    {
      rules: {
        // typescript-eslint and tsc check names and undeclared variables; the core rules would report them twice
        "no-undef": "off",
        // syntactic, so it flags every `cond && <X />` whose left side is a boolean, string or object (581 of 582 here), and
        // React renders none of them; only a number (0) leaks, and @typescript-eslint/strict-boolean-expressions finds that
        // one by its type
        "react/jsx-no-leaked-render": "off",
        "eqeqeq": "error",
        "no-promise-executor-return": "error",
        // `!!x` narrows the type of x in `!!x && x.y`; `Boolean(x)` does not (it returns a plain boolean), so the boolean form
        // stays. Numbers (`+x`) and strings (`"" + x`) are coerced with Number() and String()
        "no-implicit-coercion": ["error", { boolean: false }],
        "react/jsx-no-constructed-context-values": "error",
        "react/jsx-no-useless-fragment": "error",
        "import/no-relative-packages": "error",
        // console.warn and console.error are the logs of the Next.js server process (a failing session store, a missing
        // translation); console.log, info and debug stay forbidden, as leftovers of debugging
        "no-console": ["error", { allow: ["warn", "error"] }],
        "react/button-has-type": "error",
        "react/no-object-type-as-default-prop": "error",
        // jsx-a11y reads a <td> as a gridcell, a widget role, and asks for a label; an empty cell (the corner of a schedule grid,
        // a column of badges) is a plain data cell. Buttons, links and inputs are still checked
        "jsx-a11y/control-has-associated-label": ["error", { ignoreElements: ["td"] }],
        // a render prop (recharts' tick, dot and labelFormatter, a table column's render) is a function the library calls, not
        // a component that React mounts; a component declared inside another is still reported
        "react/no-unstable-nested-components": ["error", { allowAsProps: true }],
        // the ARIA roles here are chosen on purpose and checked by axe in the E2E suite: role="status" and "alert" are live regions
        // (<output> is for a calculation's result), role="img" labels a drawing (an <img> cannot hold the bar's segments),
        // role="radio" on a button is a radio group of buttons (a native <input type="radio"> cannot be styled as the chips),
        // role="group" and "region" name a block (<fieldset> brings a legend and form semantics, <section> no change at all)
        "jsx-a11y/prefer-tag-over-role": "off",
        // the lists keyed by index here have no identity of their own to key by: an append-only log (the assistant's messages),
        // the cells of a schedule row, the series of a chart, the rows of a static demo table, a text split at its separators.
        // The lists whose items can move or go (units, jobs, members, alerts) carry an id and use it
        "react/no-array-index-key": "off",
        // a state and its setter are named as a pair ([tab, setTab]); a wrapper that does more than set it has its own name (selectTab)
        "react/hook-use-state": "error",
        "import/no-mutable-exports": "error",
        "require-atomic-updates": "error",
        "no-await-in-loop": "error",
        // an import names a package of the workspace root or of the workspace itself (shared, an app, e2e); `@ac/web/*` is the
        // shared package's own path alias (tsconfig paths), an internal module and no dependency of itself (see settings above)
        "import/no-extraneous-dependencies": ["error", { packageDir: [tsconfigRootDir, path.resolve(tsconfigRootDir, "..")] }],
      },
    },
    {
      files: ["**/*.{ts,tsx,mts,cts}"],
      rules: {
        "@typescript-eslint/consistent-type-imports": ["error", { fixStyle: "inline-type-imports" }],
        // `import { type A }` leaves an empty import at runtime under verbatimModuleSyntax; a file that imports only types
        // writes `import type { A }`
        "@typescript-eslint/no-import-type-side-effects": "error",
        // a `default` that stands for the other members of a union (a case per kind, then "the other kinds") is the author's
        // statement that the rest behave alike, so it counts as exhaustive. Without a default, every member must have a case.
        // (@typescript-eslint/consistent-return is not enabled: tsc's noImplicitReturns checks the same with the exhaustiveness
        // analysis, and the rule reports a switch that covers a whole union as falling off the end.)
        "@typescript-eslint/switch-exhaustiveness-check": ["error", { considerDefaultExhaustiveForUnions: true }],
        "@typescript-eslint/strict-void-return": "error",
        // A condition names what it tests. A string, a number or an optional value may stand alone (an empty string, 0 and a
        // missing value all mean "not set" here, and React renders "" as nothing); an object that may be null may too. A value of
        // a mixed type (a ReactNode, a string-or-boolean) says what it tests: hasContent(node), !!value or a comparison. The
        // number that leaks into the page, `{n && <X />}`, is the selector in no-restricted-syntax
        "@typescript-eslint/strict-boolean-expressions": ["error", { allowNullableString: true, allowNullableBoolean: true, allowNullableNumber: true }],
        // a file reads top-down: the exported function first, its helpers below. A helper (a function, or a const holding one)
        // that a function calls is defined by the time the call runs; only a use in the same scope before the declaration (the
        // temporal dead zone) is a mistake, and that is what `variables: false` leaves
        "@typescript-eslint/no-use-before-define": ["error", { functions: false, variables: false }],
        "@typescript-eslint/no-shadow": "error",
      },
    },
    {
      // 1: no assertion that narrows a type. The rule reads types, so it applies to the TypeScript files only: the config
      // files (*.mjs) are linted without type information
      files: ["**/*.{ts,tsx,mts,cts}"],
      rules: { "@typescript-eslint/no-unsafe-type-assertion": "error" },
    },
    {
      // 4-2: a test reads as one scenario from its set-up to its assertions; cutting it up only to meet a size or
      // complexity limit hides its steps
      files: ["**/tests/**", "**/*.test.{ts,tsx}", "e2e/**", "**/*.e2e.ts"],
      rules: {
        "sonarjs/cognitive-complexity": "off",
        "sonarjs/cyclomatic-complexity": "off",
        "sonarjs/max-lines": "off",
        "sonarjs/max-lines-per-function": "off",
        // the size and shape limits are for code that is read to be changed; a test lists cases, scans sources with loops
        // and repeats the literals it checks (CLAUDE.md 4: tests are not held to them)
        "sonarjs/nested-control-flow": "off",
        "sonarjs/too-many-break-or-continue-in-loop": "off",
        "sonarjs/no-duplicate-string": "off",
        "sonarjs/regex-complexity": "off",
        "sonarjs/expression-complexity": "off",
        "unicorn/max-nested-calls": "off",
        // vi.fn() and the spies return any, so passing one as a callback reads as a value-returning function; a test steps through
        // pages, retries and polls in order, one await after the other, and assigns a variable after an await in a helper
        "@typescript-eslint/strict-void-return": "off",
        "no-await-in-loop": "off",
        "require-atomic-updates": "off",
        // a mock consumes its queue with shift(), and a case passes undefined on purpose to show the default
        "unicorn/no-array-front-mutation": "off",
        // the tests scan the repository's own sources with regular expressions
        "sonarjs/super-linear-regex": "off",
        "sonarjs/no-undefined-argument": "off",
        // tests read the repository's own files (the catalog, the sources they check) by a computed path
        "security/detect-non-literal-fs-filename": "off",
      },
    },
    {
      // code that also runs in the browser: Next.js targets Safari 16.4 and later, which lacks these newer built-ins
      // (Iterator helpers, RegExp.escape, Uint8Array base64/hex, explicit resource management); the tests and the E2E
      // specs run on Node and keep the rules
      ignores: ["**/tests/**", "**/*.test.{ts,tsx}", "e2e/**", "**/*.e2e.ts"],
      rules: {
        "unicorn/prefer-dispose": "off",
        "unicorn/prefer-iterator-to-array": "off",
        "unicorn/prefer-regexp-escape": "off",
        "unicorn/prefer-uint8array-base64": "off",
        "unicorn/prefer-uint8array-hex": "off",
      },
    },
    {
      // Screens read their tables with keys that are typed (a union of named tabs, fields, roles, states) or come from
      // their own literals, so a key here is never chosen by an outside caller. Off for the component files only: keys
      // from the API, the URL, a request or a session stay checked in lib, bff, loaders, server actions and the route
      // files (page.tsx, route.ts), which read them
      files: ["**/components/**", "**/_components/**"],
      rules: { "security/detect-object-injection": "off" },
    },
    {
      // TypeScript resolves every reference, including the route types Next.js generates (PageProps, LayoutProps)
      files: ["**/*.{ts,tsx,mts,cts}"],
      rules: { "sonarjs/no-reference-error": "off" },
    },
    {
      // Next.js fixes these file names; the default export names the route's own component. shared/screens keeps the
      // same layout (screens/<route>/page.tsx) because each file is the route an app re-exports; PostCSS and ESLint
      // read their configs under fixed names
      files: [
        "**/app/**/{page,layout,template,default,error,global-error,loading,not-found,forbidden,unauthorized,route}.{ts,tsx}",
        "**/screens/**/{page,layout,template,default,error,global-error,loading,not-found,forbidden,unauthorized,route}.{ts,tsx}",
        "**/postcss.config.mjs",
        "**/eslint.*config.mjs",
      ],
      rules: { "sonarjs/file-name-differ-from-class": "off" },
    },
    {
      // a route of an app is a file Next.js requires at its path; the four apps share their screens and BFF routes,
      // so an app's entry re-exports the shared module (`export { default } from "@ac/web/screens/…"`). That is the
      // route, not a barrel: nothing else imports it, and bundling is per route
      files: ["**/app/**/{page,layout,template,default,error,global-error,loading,not-found,forbidden,unauthorized,route}.{ts,tsx}"],
      rules: { "unicorn/no-barrel-files": "off" },
    },
    {
      // shadcn/ui imports cn from lib/utils; the file re-exports the one helper from its package, as shadcn's own
      // components.json expects ("utils": "@ac/web/lib/utils")
      files: ["**/lib/utils.ts"],
      rules: { "unicorn/no-barrel-files": "off" },
    },
    {
      // the four apps' and the shared package's ESLint configs share the rules of the workspace root by a relative path; the
      // root is not a package that resolves by name (it is no dependency of any workspace)
      files: ["**/eslint.*config.mjs"],
      rules: { "import/no-relative-packages": "off" },
    },
    {
      // These loops run one await after the other because each step depends on the one before or may end the loop: a cursor
      // (pagination of the Core API and of the telemetry series, the response stream), a poll that waits between reads (a
      // command's result, the demo clock), writes paced by the API's rate limit (a bulk send, one unit's commands), and a search
      // that stops at the first match (the restriction behind a contract's first invoices). Running them at once would send
      // requests the first answer makes needless. Everywhere else the rule stays on.
      files: [
        "**/lib/dal.ts",
        "**/bff/ops/*/route.ts",
        "**/lib/unitCommands.ts",
        "**/lib/writeLimit.ts",
        "**/screens/shell/demo/actions.ts",
        "**/customer/payments/page.tsx",
        "**/customer/properties/_components/properties-view.tsx",
        "**/customer/units/*/_components/unit-view.tsx",
        "**/customer/units/*/page.tsx",
        "**/technician/units/*/_lib/load.ts",
      ],
      rules: { "no-await-in-loop": "off" },
    },
    {
      // These rows carry `tone: undefined` (or `error: undefined`) for "no tone"; the expected rows list the tones they have.
      // toStrictEqual tells an undefined property from a missing one, which is not what these tests are about
      files: [
        "**/tests/energy.test.ts",
        "**/tests/partnerHistory.test.ts",
        "**/tests/partnerJobs.test.ts",
        "**/tests/partnerUnit.test.ts",
        "**/tests/techUnit.test.ts",
      ],
      rules: { "vitest/prefer-strict-equal": "off" },
    },
    {
      // useOpenedAt keeps the first value of a state that is never set again; a setter it did not use would only be dropped
      files: ["**/lib/useOpenedAt.ts"],
      rules: { "react/hook-use-state": "off" },
    },
    {
      // Next.js runs next.config.ts as CommonJS here (the packages have no "type": "module"), where __dirname is the
      // directory of the file
      files: ["**/next.config.ts"],
      rules: { "unicorn/prefer-module": "off" },
    },
    {
      // the Malay dictionary is one table of texts (a line per English text), not code to be split
      files: ["**/lib/i18n-ms.ts"],
      rules: { "sonarjs/max-lines": "off" },
    },
    {
      // the Japanese dictionary is the same table (IR639). Besides its length: the same Japanese text answers several
      // English keys (a constant per repeated text would make the table harder to read), and the texts about passwords
      // and sign-in ("パスワードを再設定") are labels, not credentials
      files: ["**/lib/i18n-ja.ts"],
      rules: { "sonarjs/max-lines": "off", "sonarjs/no-duplicate-string": "off", "sonarjs/no-hardcoded-passwords": "off" },
    },
    {
      // this test runs the compiled string literal of the build output in a vm to read the CSS the browser gets
      files: ["**/tests/styleHashes.test.ts"],
      rules: { "sonarjs/code-eval": "off" },
    },
    {
      // the contract tests assert through check(), which calls expect
      files: ["**/tests/contractInputs.test.ts"],
      rules: { "vitest/expect-expect": ["error", { assertFunctionNames: ["expect", "check"] }] },
    },
    {
      // a region that scrolls takes the focus so a keyboard can scroll it (axe scrollable-region-focusable, IR501, IR607);
      // it is named with aria-label
      files: ["**/*.tsx"],
      rules: { "jsx-a11y/no-noninteractive-tabindex": ["error", { roles: ["tabpanel", "region"], allowExpressionValues: true }] },
    },
    {
      // shadcn/ui's input-group addon: a click on the addon focuses the input beside it (a pointer convenience); the
      // input and its buttons are the keyboard path
      files: ["**/components/ui/input-group.tsx"],
      rules: { "jsx-a11y/click-events-have-key-events": "off", "jsx-a11y/no-noninteractive-element-interactions": "off" },
    },
    {
      // shadcn/ui's chart and field components are written against recharts' and react-hook-form's loose (any) types and
      // return mixed React nodes; they are the library's code, kept as the generator wrote it
      files: ["**/components/ui/chart.tsx", "**/components/ui/field.tsx"],
      rules: {
        "@typescript-eslint/no-unsafe-member-access": "off",
        "@typescript-eslint/no-unsafe-argument": "off",
        "sonarjs/function-return-type": "off",
      },
    },
    {
      // coreCall<T> is the one place a Core API reply gets its type: T is the reply's shape, named by each caller
      files: ["**/lib/dal.ts"],
      rules: { "@typescript-eslint/no-unnecessary-type-parameters": "off" },
    },
    {
      // these four files keep a handler that closes over a useState setter or a local table (setChange, setMsgs, commands);
      // it cannot move out of its component without passing them in. Everywhere else the rule stays on
      files: ["**/units/_components/units-view.tsx", "**/components/Assistant.tsx", "**/components/ui.tsx", "**/lib/restrictions.ts"],
      rules: { "unicorn/consistent-function-scoping": "off" },
    },
    { linterOptions: { reportUnusedDisableDirectives: "error" } },
    // generated files are fixed by their generators (make swagger / go run ./cmd/gen/ops), never by hand or by a fix
    globalIgnores([
      ".next/**",
      "out/**",
      "build/**",
      "next-env.d.ts",
      "playwright-report/**",
      "test-results/**",
      "**/*.gen.ts",
    ]),
  ]);
}
