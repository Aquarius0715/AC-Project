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
    public let client: CoreClient
    let authenticator: Authenticator
    let callbackScheme: String

    public init(configuration: AppConfiguration, store: any SessionStore, transport: any HTTPTransport = URLSessionTransport()) {
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
