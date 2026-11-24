// The display texts of the iOS apps (DEC-112) in the web apps' four languages (DEC-108): every `tr("…")` key of the
// Swift sources, and the refusal texts of shared/lib/actionMessage.ts (its fieldText, keyText and codeText tables, read
// from its syntax tree), with the web's translations (shared/lib/i18n-*.ts,
// keyed by the English text) and the iOS apps' own (service/ios/strings.json, for texts no web page shows).
//   npm run gen:swift              writes service/ios/Shared/Localizable.xcstrings and ACShared/Generated/ErrorTexts.gen.swift
//   npm run gen:swift -- --check   fails when one is not current
// A key without a translation in one of the languages stops the generator: the apps never show an untranslated text.
import { readdirSync, readFileSync, writeFileSync } from "node:fs";
import path from "node:path";
import ts from "typescript";
import { z } from "zod";
import { id } from "@ac/web/lib/i18n-id.ts";
import { ja } from "@ac/web/lib/i18n-ja.ts";
import { ms } from "@ac/web/lib/i18n-ms.ts";

const ios = path.join(import.meta.dirname, "..", "..", "ios");
const actionMessagePath = path.join(import.meta.dirname, "..", "shared", "lib", "actionMessage.ts");

const actionFile = ts.createSourceFile(actionMessagePath, readFileSync(actionMessagePath, "utf8"), ts.ScriptTarget.ES2022, true);

/** A table `const name: Record<string, string> = { "key": "text", … }` of actionMessage.ts, as written. */
function actionTable(name: string): Record<string, string> {
  const declaration = actionFile.statements
    .filter((s) => ts.isVariableStatement(s))
    .flatMap((s) => [...s.declarationList.declarations])
    .find((d) => ts.isIdentifier(d.name) && d.name.text === name);
  const literal = declaration?.initializer;
  if (!literal || !ts.isObjectLiteralExpression(literal)) throw new Error(`actionMessage.ts has no table ${name}`);
  return Object.fromEntries(
    literal.properties.map((p) => {
      const key = ts.isPropertyAssignment(p) && (ts.isStringLiteral(p.name) || ts.isIdentifier(p.name)) ? p.name.text : null;
      const value = ts.isPropertyAssignment(p) && ts.isStringLiteral(p.initializer) ? p.initializer.text : null;
      if (key === null || value === null) throw new Error(`actionMessage.ts ${name}: an entry that is not "key": "text"`);
      return [key, value];
    }),
  );
}
const actionTexts = { fieldText: actionTable("fieldText"), keyText: actionTable("keyText"), codeText: actionTable("codeText") };
type Language = "ms" | "ja" | "id";
const languages: Language[] = ["ms", "ja", "id"];
const dictionaries = new Map<Language, Map<string, string>>([
  ["ms", new Map(Object.entries(ms))],
  ["ja", new Map(Object.entries(ja))],
  ["id", new Map(Object.entries(id))],
]);

const own = z
  .record(z.string(), z.object({ ms: z.string().min(1), ja: z.string().min(1), id: z.string().min(1) }))
  .parse(JSON.parse(readFileSync(path.join(ios, "strings.json"), "utf8")));

const SKIPPED = new Set([".build", ".tools", "Generated", "Tests"]);

/** The Swift sources of the package and the apps, without the generated ones and the tests. */
function swiftFiles(directory: string, found: string[] = []): string[] {
  const entries = readdirSync(directory, { withFileTypes: true });
  for (const entry of entries) {
    const at = path.join(directory, entry.name);
    if (entry.isDirectory() && !SKIPPED.has(entry.name)) swiftFiles(at, found);
    else if (entry.isFile() && entry.name.endsWith(".swift")) found.push(at);
  }
  return found;
}

/** A Swift string literal's text: its escapes (\" \\ \n \u{…}) resolved. */
const unescaped = (literal: string) =>
  literal.replaceAll(/\\(u\{([0-9A-Fa-f]+)\}|.)/g, (_, sequence: string, hex: string | undefined) => {
    if (hex !== undefined) return String.fromCodePoint(Number.parseInt(hex, 16));
    if (sequence === "n") return "\n";
    if (sequence === "t") return "\t";
    return sequence;
  });

const keys = new Set<string>();
for (const file of swiftFiles(ios)) {
  for (const match of readFileSync(file, "utf8").matchAll(/\btr\(\s*"((?:[^"\\]|\\.)*)"/g)) {
    const literal = match[1] ?? "";
    // an interpolated key (tr("Hi \(name)")) differs at run time from any key in the catalog, and an empty one is a
    // multi-line literal ("""): either would show in English only
    if (literal === "" || /(?<!\\)(?:\\\\)*\\\(/.test(literal)) {
      throw new Error(`${path.relative(ios, file)}: tr("${literal}") is no fixed key; write a placeholder in braces and pass its value in tr's second argument`);
    }
    keys.add(unescaped(literal));
  }
}
for (const texts of Object.values(actionTexts)) for (const text of Object.values(texts)) keys.add(text);
const mine = new Map(Object.entries(own));

const translation = (key: string, language: Language) => {
  const entry = mine.get(key);
  return dictionaries.get(language)?.get(key) ?? (entry ? new Map(Object.entries(entry)).get(language) : undefined);
};
const sorted = [...keys].toSorted((a, b) => a.localeCompare(b, "en"));
const missing = sorted.flatMap((key) => languages.filter((l) => translation(key, l) === undefined).map((l) => `${l}: ${key}`));
if (missing.length > 0) {
  throw new Error(`texts without a translation (add them to service/ios/strings.json or the web dictionaries):\n${missing.join("\n")}`);
}
const unused = [...mine.keys()].filter((key) => !keys.has(key));
if (unused.length > 0) throw new Error(`service/ios/strings.json has texts no Swift source uses:\n${unused.join("\n")}`);

const catalog = {
  sourceLanguage: "en",
  strings: Object.fromEntries(
    sorted.map((key) => [
      key,
      {
        extractionState: "manual",
        localizations: Object.fromEntries(languages.map((l) => [l, { stringUnit: { state: "translated", value: translation(key, l) } }])),
      },
    ]),
  ),
  version: "1.0",
};

/** A text inside a Swift string literal. */
const swiftString = (text: string) =>
  text.replaceAll("\\", "\\\\").replaceAll('"', String.raw`\"`).replaceAll("\n", String.raw`\n`).replaceAll("\t", String.raw`\t`);
const swiftTable = (name: string, entries: Record<string, string>) =>
  [
    `    static let ${name}: [String: String] = [`,
    ...Object.entries(entries).toSorted(([a], [b]) => a.localeCompare(b, "en")).map(([k, v]) => `        "${swiftString(k)}": "${swiftString(v)}",`),
    "    ]",
  ].join("\n");
const errorTexts = [
  "// Code generated by service/web/codegen/swift-strings.ts from service/web/shared/lib/actionMessage.ts; DO NOT EDIT.",
  "// swift-format-ignore-file",
  "",
  "/// The refusal texts of the web apps (actionMessage.ts): by the field error key, by the message key and by the code.",
  "enum ErrorTexts {",
  swiftTable("field", actionTexts.fieldText),
  "",
  swiftTable("key", actionTexts.keyText),
  "",
  swiftTable("code", actionTexts.codeText),
  "}",
  "",
].join("\n");

// the texts of an app's Info.plist (the location prompts): their own catalog per app, in the four languages
const plistTexts = z
  .record(z.string(), z.record(z.string(), z.object({ en: z.string().min(1), ms: z.string().min(1), ja: z.string().min(1), id: z.string().min(1) })))
  .parse(JSON.parse(readFileSync(path.join(ios, "infoplist.json"), "utf8")));
const plistCatalog = (texts: Record<string, Record<string, string>>) => ({
  sourceLanguage: "en",
  strings: Object.fromEntries(
    Object.entries(texts)
      .toSorted(([a], [b]) => a.localeCompare(b, "en"))
      .map(([key, values]) => [
        key,
        {
          extractionState: "manual",
          localizations: Object.fromEntries(
            [...new Map(Object.entries(values))].map(([l, value]) => [l, { stringUnit: { state: "translated", value } }]),
          ),
        },
      ]),
  ),
  version: "1.0",
});

const files: [string, string][] = [
  ...Object.entries(plistTexts).map(([app, texts]): [string, string] => [path.join(ios, app, "InfoPlist.xcstrings"), `${JSON.stringify(plistCatalog(texts), null, 2)}\n`]),
  [path.join(ios, "Shared", "Localizable.xcstrings"), `${JSON.stringify(catalog, null, 2)}\n`],
  [path.join(ios, "ACKit", "Sources", "ACShared", "Generated", "ErrorTexts.gen.swift"), errorTexts],
];
const current = (file: string, text: string) => {
  try {
    return readFileSync(file, "utf8") === text;
  } catch {
    return false;
  }
};
if (process.argv.includes("--check")) {
  const stale = files.filter(([file, text]) => !current(file, text)).map(([file]) => path.relative(ios, file));
  if (stale.length > 0) throw new Error(`not current: ${stale.join(", ")} (run npm run gen:swift in service/web)`);
} else {
  for (const [file, text] of files) writeFileSync(file, text);
  process.stdout.write(`${String(sorted.length)} texts in ${String(languages.length + 1)} languages → ${ios}\n`);
}
