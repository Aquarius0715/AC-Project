import Foundation
import os

/// The display languages of the apps, the web apps' four (DEC-108).
public enum DisplayLanguage: String, CaseIterable, Sendable {
    case en, ms, ja, id

    /// The language of a locale or language code (`ja-JP`, `ms`), or nil for one the apps do not have.
    public init?(code: String) {
        let base = code.split(whereSeparator: { $0 == "-" || $0 == "_" }).first.map(String.init) ?? code
        self.init(rawValue: base.lowercased())
    }

    /// The first of the device's preferred languages the apps have, else English: before sign-in the device chooses,
    /// after it the membership's preference (preferences.get) does.
    public static func preferred(_ codes: [String] = Locale.preferredLanguages) -> Self {
        codes.lazy.compactMap { Self(code: $0) }.first ?? .en
    }
}

/// Where the texts are looked up: the app's String Catalog (service/ios/Shared/Localizable.xcstrings, generated from the
/// web apps' dictionaries), in the chosen display language; and the time zone the times are shown in. Before sign-in the
/// device chooses both; after it the membership's preferences do (preferences.get, FR-X01), as on the web.
public enum Localizer {
    private static let state = OSAllocatedUnfairLock(
        initialState: (language: DisplayLanguage.preferred(), table: table(.preferred(), in: .main), timeZone: TimeZone.current)
    )

    /// A language for one task and the tasks it starts (the tests: each sees its own, while they run in parallel).
    @TaskLocal static var scoped: (language: DisplayLanguage, table: Bundle?)?

    /// The display language now.
    public static var language: DisplayLanguage { scoped?.language ?? state.withLock { $0.language } }

    /// Runs `body` in a display language, without changing the app's.
    public static func with<T>(_ language: DisplayLanguage, in bundle: Bundle = .main, _ body: () throws -> T) rethrows -> T {
        try $scoped.withValue((language, table(language, in: bundle)), operation: body)
    }

    /// The time zone the times are shown in.
    public static var timeZone: TimeZone { state.withLock { $0.timeZone } }

    /// Chooses the display language and time zone; a language the app's bundle has no texts for shows the keys
    /// (English), never another language's texts.
    public static func use(_ language: DisplayLanguage, timeZone: TimeZone = .current, in bundle: Bundle = .main) {
        state.withLock { $0 = (language, table(language, in: bundle), timeZone) }
    }

    private static func table(_ language: DisplayLanguage, in bundle: Bundle) -> Bundle? {
        bundle.path(forResource: language.rawValue, ofType: "lproj").flatMap(Bundle.init(path:))
    }

    static func text(_ key: String) -> String {
        let table = scoped.map(\.table) ?? state.withLock { $0.table }
        return table?.localizedString(forKey: key, value: key, table: nil) ?? key
    }
}

/// A display text by its English text (the web apps' keys), in the display language, with its `{name}` placeholders
/// filled. Every text the apps show goes through here, so the generator finds it (codegen/swift-strings.ts).
public func tr(_ key: String, _ values: [String: String] = [:]) -> String {
    // a key that names its context before "::" ("tamper::Clear", a state, not the button "Clear") shows its words alone
    // where no translation replaces it (English), as the web's translator does
    var text = Localizer.text(key)
    if text == key, let context = key.range(of: "::") { text = String(key[context.upperBound...]) }
    // one pass, as the web's: a filled value is not read again for placeholders (a unit named "{place}" stays as named)
    // one pass, as the web's: a filled value is not read again for placeholders (a unit named "{place}" stays as named)
    return text.replacing(/\{(\w+)\}/) { match in values[String(match.1)] ?? String(match.0) }
}
