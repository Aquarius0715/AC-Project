#if os(iOS)
import CoreLocation

/// The phone's location services (Core Location): the permission and one fix through a location manager, the circles
/// through a monitor, which the system keeps watching while the app is closed and relaunches it for a crossing. The
/// UI tests and the live checks drive it; the models' tests use a stand-in (LocationSource).
@MainActor
public final class CoreLocationSource: NSObject, LocationSource, CLLocationManagerDelegate {
    private let manager = CLLocationManager()
    private var permissionWaiters: [CheckedContinuation<LocationPermission, Never>] = []
    private var fixWaiters: [CheckedContinuation<Fix?, Never>] = []
    private var monitor: CLMonitor?
    private var events: Task<Void, Never>?
    /// letters only: CLMonitor refuses a name with a dot ("Monitor name is not valid")
    static let monitorName = "ACPresence"

    override public init() {
        super.init()
        manager.delegate = self
        manager.desiredAccuracy = kCLLocationAccuracyNearestTenMeters
    }

    public func permission() async -> LocationPermission {
        Self.permission(manager.authorizationStatus)
    }

    public func requestAlways() async -> LocationPermission {
        let now = Self.permission(manager.authorizationStatus)
        guard now == .notDetermined || now == .whenInUse else { return now }
        return await withCheckedContinuation { waiter in
            permissionWaiters.append(waiter)
            manager.requestAlwaysAuthorization()
        }
    }

    public func fix() async -> Fix? {
        await withCheckedContinuation { waiter in
            fixWaiters.append(waiter)
            manager.requestLocation()
        }
    }

    public func watch(_ fences: [Geofence]) async -> AsyncStream<String> {
        let monitor = await monitor()
        for id in await monitor.identifiers { await monitor.remove(id) }
        for fence in fences {
            let center = CLLocationCoordinate2D(latitude: fence.latitude, longitude: fence.longitude)
            await monitor.add(
                CLMonitor.CircularGeographicCondition(center: center, radius: fence.radiusMeters),
                identifier: fence.propertyId
            )
        }
        let (stream, continuation) = AsyncStream<String>.makeStream()
        events?.cancel()
        events = Task {
            do {
                for try await event in await monitor.events where event.state != .unknown {
                    continuation.yield(event.identifier)
                }
            } catch {
                continuation.finish()
            }
        }
        continuation.onTermination = { [weak self] _ in Task { @MainActor in self?.events?.cancel() } }
        return stream
    }

    public func stop() async {
        events?.cancel()
        events = nil
        let monitor = await monitor()
        for id in await monitor.identifiers { await monitor.remove(id) }
    }

    /// The one monitor of the app, by its name: the system hands the same one back after a relaunch.
    private func monitor() async -> CLMonitor {
        if let monitor { return monitor }
        let made = await CLMonitor(Self.monitorName)
        monitor = made
        return made
    }

    nonisolated static func permission(_ status: CLAuthorizationStatus) -> LocationPermission {
        switch status {
        case .authorizedAlways: .always
        case .authorizedWhenInUse: .whenInUse
        case .denied, .restricted: .denied
        case .notDetermined: .notDetermined
        @unknown default: .denied
        }
    }

    // MARK: - CLLocationManagerDelegate

    nonisolated public func locationManagerDidChangeAuthorization(_ manager: CLLocationManager) {
        let permission = Self.permission(manager.authorizationStatus)
        Task { @MainActor in
            // the first answer is the one asked for; a change while no one asks is read at the next start
            guard permission != .notDetermined else { return }
            let waiters = permissionWaiters
            permissionWaiters = []
            for waiter in waiters { waiter.resume(returning: permission) }
        }
    }

    nonisolated public func locationManager(_ manager: CLLocationManager, didUpdateLocations locations: [CLLocation]) {
        let fix = locations.last.map {
            Fix(
                latitude: $0.coordinate.latitude,
                longitude: $0.coordinate.longitude,
                accuracyMeters: $0.horizontalAccuracy,
                at: $0.timestamp
            )
        }
        Task { @MainActor in resumeFixes(fix) }
    }

    nonisolated public func locationManager(_ manager: CLLocationManager, didFailWithError error: any Error) {
        Task { @MainActor in resumeFixes(nil) }
    }

    private func resumeFixes(_ fix: Fix?) {
        let waiters = fixWaiters
        fixWaiters = []
        for waiter in waiters { waiter.resume(returning: fix) }
    }
}
#endif
