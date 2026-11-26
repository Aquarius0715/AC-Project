#if DEBUG
import Foundation

/// The UI tests' stand-in for the phone's location services (Debug builds only, AC_UI_TEST=stub): always allowed, one
/// precise fix at the stub's home, and no crossing.
public struct StubLocationSource: LocationSource {
    public init() {
        // nothing to set up: every answer is fixed
    }

    public func permission() async -> LocationPermission { .always }
    public func requestAlways() async -> LocationPermission { .always }

    public func fix() async -> Fix? {
        Fix(latitude: 3.139, longitude: 101.6869, accuracyMeters: 10, at: Date())
    }

    public func watch(_ fences: [Geofence]) async -> AsyncStream<String> {
        AsyncStream { $0.finish() }
    }

    public func stop() async {
        // nothing is watched
    }
}

/// The switch of a UI test run: in memory, off at each launch.
public final class MemoryPresenceSettings: PresenceSettings, @unchecked Sendable {
    // one UI test drives one app: the lock only keeps the type honest about Sendable
    private let lock = NSLock()
    private var on = false

    public init() {
        // off until the test switches it on
    }

    public var isOn: Bool { lock.withLock { on } }
    public func set(_ isOn: Bool) { lock.withLock { on = isOn } }
}
#endif
