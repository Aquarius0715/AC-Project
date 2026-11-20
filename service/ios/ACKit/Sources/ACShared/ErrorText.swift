import ACKit
import Foundation

/// The line a refusal is shown with: the web apps' actionMessage (shared/lib/actionMessage.ts) with the same tables
/// (ErrorTexts.gen.swift), so an app and a web page word a refusal alike.
public enum ErrorText {
    public static func of(_ error: CoreError) -> String {
        of(error.error)
    }

    public static func of(_ error: DomainError) -> String {
        if let (field, key) = error.fieldErrors.min(by: { $0.key < $1.key }) {
            let name = fieldName(field)
            if let text = ErrorTexts.field[key] { return tr(text, ["field": name]) }
            return tr("{field} — {problem}", ["field": name, "problem": humanized(key).lowercased()])
        }
        if error.messageKey == "error.versionConflict" {
            return tr("Someone changed this in the meantime — the screen now shows the latest version")
        }
        if error.code == .rateLimited {
            if let seconds = error.retryAfterSeconds, seconds > 0 {
                return tr("Too many changes in a short time — try again in {s} s", ["s": String(Int(seconds))])
            }
            return tr("Too many changes in a short time — try again shortly")
        }
        if error.messageKey == "ios.noConnection" { return tr("No connection — check the network and try again") }
        if let text = ErrorTexts.key[error.messageKey] { return tr(text) }
        if error.messageKey.hasPrefix("errors.") { return humanized(error.messageKey) }
        if let text = ErrorTexts.code[error.code.rawValue] { return tr(text) }
        return "\(error.code.rawValue): \(humanized(error.messageKey))"
    }

    /// The line of a sign-in that did not complete.
    public static func of(_ error: SignInError) -> String {
        switch error {
        case .unavailable: tr("The sign-in service is unavailable — try again shortly")
        case .refused: tr("The sign-in did not complete — try again")
        case .wrongRole: tr("This account is not for this app — sign in with another account")
        }
    }

    /// `"invoice_has_payment"` → `"Invoice has payment"`; `"paymentReference"` → `"Payment reference"`.
    static func humanized(_ key: String) -> String {
        var text = key
        if let prefix = text.range(of: #"^errors?\."#, options: .regularExpression) { text.removeSubrange(prefix) }
        text = text.replacingOccurrences(of: #"[._]"#, with: " ", options: .regularExpression)
        text = text.replacingOccurrences(of: #"([a-z])([A-Z])"#, with: "$1 $2", options: .regularExpression)
        text = text.lowercased().trimmingCharacters(in: .whitespaces)
        return text.prefix(1).uppercased() + text.dropFirst()
    }

    /// A field of a refusal in words: its last part (`"newUser.email"` → `"Email"`).
    static func fieldName(_ field: String) -> String {
        humanized(field.split(separator: ".").last.map(String.init) ?? field)
    }
}
