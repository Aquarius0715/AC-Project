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
import vitest from "@vitest/eslint-plugin";
import nextVitals from "eslint-config-next/core-web-vitals";
import nextTs from "eslint-config-next/typescript";
import security from "eslint-plugin-security";
import sonarjs from "eslint-plugin-sonarjs";
import unicorn from "eslint-plugin-unicorn";
import { defineConfig, globalIgnores } from "eslint/config";
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
    { files: ["**/tests/**", "**/*.test.{ts,tsx}"], rules: { "vitest/valid-expect": ["error", { maxArgs: 2 }] } },
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
        ],
        // 3: no raw HTML
        "react/no-danger": "error",
        // 4-2: the cyclomatic count adds every case of a flat switch or lookup, so it takes Go's limit (30);
        // sonarjs/cognitive-complexity is the measure Go's gocognit takes, with its limit (30)
        "sonarjs/cyclomatic-complexity": ["error", { threshold: 30 }],
        "sonarjs/cognitive-complexity": ["error", 30],
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
        // 4-2: a try block with a fetch or a parse, one branch and its result is small enough to read with its catch; the default
        // (1) allows a single call. A block above 4 is split by hand
        "unicorn/try-complexity": ["error", { max: 4 }],
        // 4-2: the order of a guard (`!reason.trim() || !isOrder || !tech`) follows how the form reads, and moving an operand
        // changes what short-circuits; the rule only saves a call that has no side effect
        "unicorn/prefer-simple-condition-first": "off",
        // 4-2: the module-level `let` cases here are single-owner caches on purpose: the session store client (one per
        // process), the BFF session cache, the demo stores and the focus handed to a remounted view. A holder object
        // would hide the same mutation behind a property
        "unicorn/no-top-level-assignment-in-function": "off",
        // getElementById takes any id as it is (member-row-${id}, ids with dots); querySelector(`#${id}`) would need escaping
        "unicorn/prefer-query-selector": "off",
        // an if / else if chain that ends at the last case needs no empty else (each branch sets one thing)
        "sonarjs/elseif-without-else": "off",
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
