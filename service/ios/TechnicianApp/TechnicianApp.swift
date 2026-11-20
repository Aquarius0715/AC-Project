import ACShared
import SwiftUI
import TechnicianFeature

/// AC Technician (DEC-112): the jobs assigned to the signed-in technician.
@main
struct TechnicianApp: App {
    @State private var launch = Launch.start(role: "technician")

    var body: some Scene {
        WindowGroup {
            switch launch {
            case let .ready(session, web): TechnicianRoot(session: session, position: Self.position(stubbed: web != nil), web: web)
            case let .misconfigured(key): Text(verbatim: "Info.plist: \(key)")
            }
        }
    }

    /// The phone's position, or the UI tests' one with the stubbed Core API (Debug builds only).
    private static func position(stubbed: Bool) -> any PositionSource {
        #if DEBUG
        if stubbed { return StubPosition() }
        #endif
        return CoreLocationPosition()
    }
}
