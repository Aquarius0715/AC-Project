import Foundation

/// Times as the screens show them: the contract's ISO instants in the display language and time zone (Localizer).
public enum Format {
    /// "1 Dec 2026, 09:00" for `2026-12-01T01:00:00Z` (en, +08:00); a text that is no instant is shown as it is.
    public static func dateTime(_ instant: String, timeZone: TimeZone = Localizer.timeZone) -> String {
        guard let date = parse(instant) else { return instant }
        var style = Date.FormatStyle(date: .abbreviated, time: .shortened)
        style.locale = Locale(identifier: Localizer.language.rawValue)
        style.timeZone = timeZone
        return date.formatted(style)
    }

    /// "1 Dec 2026": the day of an instant (a reminder's due day), in the display language and time zone.
    public static func date(_ instant: String, timeZone: TimeZone = Localizer.timeZone) -> String {
        guard let date = parse(instant) else { return instant }
        var style = Date.FormatStyle(date: .abbreviated, time: .omitted)
        style.locale = Locale(identifier: Localizer.language.rawValue)
        style.timeZone = timeZone
        return date.formatted(style)
    }

    /// An ISO 8601 instant with or without fractional seconds (the Core API writes milliseconds when it has them).
    public static func parse(_ instant: String) -> Date? {
        let fractional = Date.ISO8601FormatStyle(includingFractionalSeconds: true)
        return (try? fractional.parse(instant)) ?? (try? Date.ISO8601FormatStyle().parse(instant))
    }
}
