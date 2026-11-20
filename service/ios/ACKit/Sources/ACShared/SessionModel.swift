import ACKit
import Foundation
import Observation

/// Opens the provider's sign-in page and returns the redirect: SwiftUI's web authentication session in the apps, a
/// stub in the tests.
public typealias WebSignIn = @MainActor (_ url: URL, _ callbackScheme: String) async throws -> URL

/// The app's sign-in state for its screens: signed out (with the last refusal), signing in, or signed in.
@MainActor @Observable
public final class SessionModel {
    public enum State: Equatable {
        case signedOut(message: String?)
        case signingIn
        case signedIn(SessionClaims)
    }

    public private(set) var state: State
    /// The display language the screens show; it changes when the membership's preferences are read.
    public private(set) var language = Localizer.language
    public let client: CoreClient
    let authenticator: Authenticator
    let callbackScheme: String

    /// Where the membership's display language and time zone go (the app's Localizer; a test's recorder).
    let display: @MainActor (DisplayLanguage, TimeZone) -> Void

    public init(
        configuration: AppConfiguration,
        store: any SessionStore,
        transport: any HTTPTransport = URLSessionTransport(),
        display: @escaping @MainActor (DisplayLanguage, TimeZone) -> Void = SessionModel.show
    ) {
        self.display = display
        // the session that ends by itself (refused, the refresh failed) is shown as signed out with the reason
        let ended = Ended()
        let authenticator = Authenticator(
            configuration: configuration.oidc,
            role: configuration.role,
            transport: transport,
            store: store,
            signedOut: { await ended.notify() }
        )
        self.authenticator = authenticator
        callbackScheme = configuration.oidc.callbackScheme
        client = CoreClient(baseURL: configuration.coreAPI, transport: transport, credentials: authenticator)
        state = store.load().map { .signedIn($0.claims) } ?? .signedOut(message: nil)
        ended.model = self
    }

    /// Signs in through the provider's page.
    public func signIn(using web: WebSignIn) async {
        state = .signingIn
        do {
            let pending = try await authenticator.begin(locale: Localizer.language.rawValue)
            let callback: URL
            do {
                callback = try await web(pending.url, callbackScheme)
            } catch {
                // the user closed the page: back to the sign-in screen without a refusal
                state = .signedOut(message: nil)
                return
            }
            try await authenticator.complete(pending, callback: callback)
            if let claims = await authenticator.claims { state = .signedIn(claims) }
        } catch {
            state = .signedOut(message: ErrorText.of(error))
        }
    }

    /// The membership's display language and time zone (preferences.get), once signed in; unreadable preferences keep
    /// the device's, as the web apps show the defaults.
    public func applyPreferences() async {
        guard case .signedIn = state, let preferences = try? await client.call(Ops.PreferencesGet.self, Empty()).data else { return }
        let language = Self.language(preferences.locale)
        display(language, TimeZone(identifier: preferences.timezone) ?? .current)
        self.language = language
    }

    /// The app's display: the language and time zone of every screen.
    public static func show(_ language: DisplayLanguage, in timeZone: TimeZone) {
        Localizer.use(language, timeZone: timeZone)
    }

    /// The display language of a membership's locale: the contract's four are the apps' four.
    static func language(_ locale: CoreLocale) -> DisplayLanguage {
        switch locale {
        case .en: .en
        case .ms: .ms
        case .ja: .ja
        case .id: .id
        }
    }

    /// The signed-in membership.
    public var membershipId: String? {
        if case let .signedIn(claims) = state { return claims.membershipId }
        return nil
    }

    public func signOut() async {
        await authenticator.signOut()
        state = .signedOut(message: nil)
    }

    func sessionEnded() {
        state = .signedOut(message: tr("Your session ended — sign in again"))
    }
}

/// The bridge from the authenticator (an actor) to the model on the main actor.
@MainActor
private final class Ended {
    weak var model: SessionModel?

    nonisolated func notify() async {
        await MainActor.run { model?.sessionEnded() }
    }
}
