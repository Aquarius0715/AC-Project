import ACKit
import Foundation

/// How an app starts: its configuration from the Info.plist, its session in the Keychain (or, in a stubbed UI test,
/// the stub's), or the Info.plist key that is missing.
@MainActor
public enum Launch {
    case ready(SessionModel, web: WebSignIn?)
    case misconfigured(key: String)

    public static func start(
        role: String,
        bundle: Bundle = .main,
        environment: [String: String] = ProcessInfo.processInfo.environment
    ) -> Self {
        Localizer.use(.preferred(), in: bundle)
        let configuration: AppConfiguration
        do {
            configuration = try AppConfiguration(role: role, lookup: bundle.object(forInfoDictionaryKey:))
        } catch {
            return .misconfigured(key: error.key)
        }
        #if DEBUG
        if UITestStub.isActive(environment) {
            return .ready(UITestStub.session(configuration: configuration, environment: environment), web: UITestStub.signIn)
        }
        #endif
        // the Keychain item is the app's: an app always has a bundle identifier, the fallback only names the item
        let store = KeychainSessionStore(service: bundle.bundleIdentifier ?? "ac")
        return .ready(SessionModel(configuration: configuration, store: store), web: nil)
    }
}
