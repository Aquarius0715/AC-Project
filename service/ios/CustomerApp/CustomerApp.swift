import ACShared
import CustomerFeature
import SwiftUI

/// AC Customer (DEC-112): the customer's air conditioners and the home/away geofence.
@main
struct CustomerApp: App {
    @State private var launch = Launch.start(role: "client")

    var body: some Scene {
        WindowGroup {
            switch launch {
            case let .ready(session, web): CustomerRoot(session: session, web: web)
            case let .misconfigured(key): Text(verbatim: "Info.plist: \(key)")
            }
        }
    }
}
