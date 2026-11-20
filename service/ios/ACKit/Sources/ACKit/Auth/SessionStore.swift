import Foundation
import Security

/// A signed-in session: the tokens and the claims of the access token.
public struct StoredSession: Codable, Sendable, Equatable {
    public let accessToken: String
    public let refreshToken: String?
    public let idToken: String?
    /// When the access token expires.
    public let expiresAt: Date
    public let claims: SessionClaims
}

/// Where the session is kept between launches.
public protocol SessionStore: Sendable {
    func load() -> StoredSession?
    func save(_ session: StoredSession) throws
    func clear()
}

/// The Keychain, readable after the first unlock (the Customer app reports a geofence crossing while the phone is
/// locked) and on this device only (no backup, no iCloud Keychain).
public struct KeychainSessionStore: SessionStore {
    let service: String

    /// `service` names the app's item (its bundle identifier).
    public init(service: String) {
        self.service = service
    }

    private var query: [String: Any] {
        [kSecClass as String: kSecClassGenericPassword, kSecAttrService as String: service, kSecAttrAccount as String: "session"]
    }

    public func load() -> StoredSession? {
        var search = query
        search[kSecReturnData as String] = true
        search[kSecMatchLimit as String] = kSecMatchLimitOne
        var item: CFTypeRef?
        guard SecItemCopyMatching(search as CFDictionary, &item) == errSecSuccess, let data = item as? Data else { return nil }
        // a stored item that is no session (an older format) is no sign-in
        return try? JSONDecoder().decode(StoredSession.self, from: data)
    }

    public func save(_ session: StoredSession) throws {
        let data = try JSONEncoder().encode(session)
        clear()
        var item = query
        item[kSecValueData as String] = data
        item[kSecAttrAccessible as String] = kSecAttrAccessibleAfterFirstUnlockThisDeviceOnly
        let status = SecItemAdd(item as CFDictionary, nil)
        guard status == errSecSuccess else { throw KeychainError(status: status) }
    }

    public func clear() {
        SecItemDelete(query as CFDictionary)
    }
}

/// A Keychain call that failed, with its OSStatus.
public struct KeychainError: Error, Equatable {
    public let status: OSStatus
}
