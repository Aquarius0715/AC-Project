import ACKit
import Foundation

/// The addresses an app is built for (its Info.plist, set per build configuration in service/ios/project.yml): the Core
/// API, the identity provider and the app's client. Checked when the app starts.
public struct AppConfiguration: Sendable, Equatable {
    public let coreAPI: URL
    public let oidc: OIDCConfiguration
    /// The `role` claim the app signs in (`client`, `technician`).
    public let role: String

    /// Why an Info.plist names no usable configuration.
    public struct Invalid: Error, Equatable {
        public let key: String
    }

    /// Reads `ACCoreAPIURL`, `ACIssuerURL`, `ACClientID` and `ACRedirectURI`; every one is required, the URLs absolute.
    public init(info: [String: Any], role: String) throws(Invalid) {
        try self.init(role: role) { info[$0] }
    }

    /// The same, from a lookup by key (a bundle's `object(forInfoDictionaryKey:)`).
    public init(role: String, lookup: (String) -> Any?) throws(Invalid) {
        func url(_ key: String) throws(Invalid) -> URL {
            guard let text = lookup(key) as? String, let url = URL(string: text), url.scheme != nil else { throw Invalid(key: key) }
            return url
        }
        guard let client = lookup("ACClientID") as? String, !client.isEmpty else { throw Invalid(key: "ACClientID") }
        coreAPI = try url("ACCoreAPIURL")
        oidc = OIDCConfiguration(issuer: try url("ACIssuerURL"), clientId: client, redirectURI: try url("ACRedirectURI"))
        self.role = role
    }
}
