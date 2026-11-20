import ACKit
import ACShared
import CustomerFeature
import SwiftUI

/// AC Customer (DEC-112): the customer's air conditioners, notifications and the home/away geofence. The geofence is
/// set up when the app starts, also when the phone starts it in the background for a crossing: no screen is shown then.
@main
struct CustomerApp: App {
    private let launch: Launch
    private let presence: PresenceModel?

    init() {
        launch = Launch.start(role: "client")
        guard case let .ready(session, web) = launch else {
            presence = nil
            return
        }
        #if DEBUG
        let isStubbed = web != nil
        #else
        let isStubbed = false
        #endif
        let presence = Self.presence(client: session.client, stubbed: isStubbed)
        self.presence = presence
        if case .signedIn = session.state { Task { await presence.resume() } }
    }

    private static func presence(client: CoreClient, stubbed: Bool) -> PresenceModel {
        #if DEBUG
        if stubbed { return PresenceModel(client: client, location: StubLocationSource(), settings: MemoryPresenceSettings()) }
        #endif
        return PresenceModel(client: client, location: CoreLocationSource(), settings: UserDefaultsPresenceSettings())
    }

    var body: some Scene {
        WindowGroup {
            switch launch {
            case let .ready(session, web):
                if let presence { CustomerRoot(session: session, presence: presence, web: web) }
            case let .misconfigured(key): Text(verbatim: "Info.plist: \(key)")
            }
        }
    }
}
