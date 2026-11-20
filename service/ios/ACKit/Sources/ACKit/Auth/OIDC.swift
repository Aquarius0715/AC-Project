import CryptoKit
import Foundation

/// The identity provider an app signs in with (Keycloak locally and in the shared test environment, Cognito in
/// production): its issuer, the app's public client (`ac-customer-ios`, `ac-technician-ios`, IR860) and the address the
/// sign-in returns to.
public struct OIDCConfiguration: Sendable, Equatable {
    public let issuer: URL
    public let clientId: String
    public let redirectURI: URL

    public init(issuer: URL, clientId: String, redirectURI: URL) {
        self.issuer = issuer
        self.clientId = clientId
        self.redirectURI = redirectURI
    }

    /// The scheme of the redirect, which the web authentication session waits for.
    public var callbackScheme: String { redirectURI.scheme ?? "" }
}

/// The endpoints of the provider (`/.well-known/openid-configuration`), so the app names no provider's paths.
public struct OIDCEndpoints: Decodable, Sendable, Equatable {
    public let authorizationEndpoint: URL
    public let tokenEndpoint: URL
    public let endSessionEndpoint: URL?

    enum CodingKeys: String, CodingKey {
        case authorizationEndpoint = "authorization_endpoint"
        case tokenEndpoint = "token_endpoint"
        case endSessionEndpoint = "end_session_endpoint"
    }
}

/// The token endpoint's answer to the authorization code and refresh grants.
struct TokenResponse: Decodable, Sendable {
    let accessToken: String
    let refreshToken: String?
    let idToken: String?
    let expiresIn: Double

    enum CodingKeys: String, CodingKey {
        case accessToken = "access_token"
        case refreshToken = "refresh_token"
        case idToken = "id_token"
        case expiresIn = "expires_in"
    }
}

/// The claims of an access token the apps need: the tenant, the membership and the role (the web apps' session.ts).
public struct SessionClaims: Codable, Sendable, Equatable {
    public let tenantId: String
    public let membershipId: String
    public let role: String

    enum CodingKeys: String, CodingKey {
        case tenantId = "tenant_id"
        case membershipId = "membership_id"
        case role
    }

    /// The claims of a JWT's payload, or nil when one is missing or empty. The signature is the Core API's to check;
    /// the app reads the claims to name the membership and to refuse a sign-in of another role.
    public static func of(accessToken: String) -> Self? {
        let parts = accessToken.split(separator: ".")
        guard parts.count == 3, let payload = Data(base64URL: String(parts[1])) else { return nil }
        guard let claims = try? JSONDecoder().decode(Self.self, from: payload) else { return nil }
        let filled = [claims.tenantId, claims.membershipId, claims.role].allSatisfy { !$0.isEmpty }
        return filled ? claims : nil
    }
}

/// One sign-in in progress: the PKCE verifier and the state it must come back with (RFC 7636, RFC 6749 §10.12).
public struct PendingSignIn: Sendable, Equatable {
    public let url: URL
    let state: String
    let verifier: String

    /// The authorization request of the code flow with PKCE S256.
    static func make(_ configuration: OIDCConfiguration, _ endpoints: OIDCEndpoints, locale: String?) -> Self {
        let verifier = randomURLSafe(bytes: 48)
        let state = randomURLSafe(bytes: 24)
        let challenge = Data(SHA256.hash(data: Data(verifier.utf8))).base64URLEncoded()
        var items = [
            URLQueryItem(name: "client_id", value: configuration.clientId),
            URLQueryItem(name: "response_type", value: "code"),
            URLQueryItem(name: "scope", value: "openid"),
            URLQueryItem(name: "redirect_uri", value: configuration.redirectURI.absoluteString),
            URLQueryItem(name: "code_challenge", value: challenge),
            URLQueryItem(name: "code_challenge_method", value: "S256"),
            URLQueryItem(name: "state", value: state),
        ]
        if let locale { items.append(URLQueryItem(name: "ui_locales", value: locale)) }
        let url = endpoints.authorizationEndpoint.appending(queryItems: items)
        return Self(url: url, state: state, verifier: verifier)
    }

    /// The authorization code of the redirect, or nil when it is not this sign-in's (another state) or the provider
    /// refused it.
    func code(from callback: URL) -> String? {
        var components = URLComponents()
        components.percentEncodedQuery = callback.query(percentEncoded: true)
        let items = components.queryItems ?? []  // a redirect without a query names no code
        guard items.first(where: { $0.name == "state" })?.value == state else { return nil }
        return items.first { $0.name == "code" }?.value
    }
}

func randomURLSafe(bytes count: Int) -> String {
    var generator = SystemRandomNumberGenerator()
    let bytes = (0..<count).map { _ in UInt8.random(in: .min ... .max, using: &generator) }
    return Data(bytes).base64URLEncoded()
}

extension Data {
    init?(base64URL text: String) {
        var base64 = text.replacingOccurrences(of: "-", with: "+").replacingOccurrences(of: "_", with: "/")
        base64 += String(repeating: "=", count: (4 - base64.count % 4) % 4)
        self.init(base64Encoded: base64)
    }

    func base64URLEncoded() -> String {
        base64EncodedString().replacingOccurrences(of: "+", with: "-").replacingOccurrences(of: "/", with: "_")
            .replacingOccurrences(of: "=", with: "")
    }
}
