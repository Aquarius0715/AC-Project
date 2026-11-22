import ACKit
import ACShared
import Foundation
import Observation

/// What the app may know of the phone's position.
public enum LocationPermission: Sendable, Equatable {
    case notDetermined, whenInUse, always, denied
}

/// A position the phone measured: WGS 84 degrees, its accuracy in metres and when it was taken.
public struct Fix: Sendable, Equatable {
    public let latitude: Double
    public let longitude: Double
    public let accuracyMeters: Double
    public let at: Date

    public init(latitude: Double, longitude: Double, accuracyMeters: Double, at: Date) {
        self.latitude = latitude
        self.longitude = longitude
        self.accuracyMeters = accuracyMeters
        self.at = at
    }
}

/// A home's or an office's circle around its map pin (DEC-109): the phone watches it while the app is closed.
public struct Geofence: Sendable, Equatable {
    public let propertyId: String
    public let latitude: Double
    public let longitude: Double
    public let radiusMeters: Double
}

/// The phone's location services: its permission, one fix, and the circles it watches. Core Location in the app
/// (CoreLocationPlatform.swift), a stand-in in the tests.
public protocol LocationSource: Sendable {
    func permission() async -> LocationPermission
    /// Asks for the permission to know the position also while the app is closed; answers the permission given.
    func requestAlways() async -> LocationPermission
    /// One fix now, or nil when the phone gives none.
    func fix() async -> Fix?
    /// Watches these circles instead of the ones before; each crossing (in or out) is one element.
    func watch(_ fences: [Geofence]) async -> AsyncStream<String>
    /// Watches nothing any more.
    func stop() async
}

/// Where the choice of this phone is kept (it is the device's, as the web keeps it in the browser).
public protocol PresenceSettings: Sendable {
    var isOn: Bool { get }
    func set(_ isOn: Bool)
}

/// The choice in the app's UserDefaults.
public struct UserDefaultsPresenceSettings: PresenceSettings, @unchecked Sendable {
    // UserDefaults is thread-safe; it is not marked Sendable in the SDK
    let defaults: UserDefaults
    static let key = "ac.presence.on"

    public init(defaults: UserDefaults = .standard) {
        self.defaults = defaults
    }

    public var isOn: Bool { defaults.bool(forKey: Self.key) }

    public func set(_ isOn: Bool) {
        defaults.set(isOn, forKey: Self.key)
    }
}

/// Home or away (DEC-109, IR826), as the web's presence switch, but also while the app is closed: with the switch on,
/// the phone watches the circles around the customer's homes and offices and, when it crosses one, reports its
/// position (presence.report); the Core API compares it with the pins and keeps only home or away. No coordinate is
/// stored. The household's arrival or departure runs the location automations.
@MainActor @Observable
public final class PresenceModel {
    public private(set) var isOn: Bool
    /// What the last step came to, in words (the permission, the pins, the last report).
    public private(set) var line: String?
    public private(set) var permission: LocationPermission = .notDetermined
    /// The location consent (location_automation): the reports need it, as the location automations do (IR53).
    public private(set) var consent: Consent?
    /// Whether the account has a consent record at all (without one, location automations cannot be used).
    public private(set) var hasConsentRecord = true

    let client: CoreClient
    let location: any LocationSource
    let settings: any PresenceSettings
    private var names: [String: String] = [:]
    private var watching: Task<Void, Never>?
    /// Counts the starts and stops: a start that waited (for the permission, the pins, the report or the monitor) while
    /// the switch went off, the user signed out or a later start began goes no further.
    private var run = 0
    private var isStarting = false

    /// The coarsest fix worth sending: the Core API refuses coarser ones (errors.position_inaccurate).
    static let maxAccuracy = 100.0
    /// The circle of a pin without its own radius (the Core API's default).
    static let defaultRadius = 150.0

    public init(client: CoreClient, location: any LocationSource, settings: any PresenceSettings) {
        self.client = client
        self.location = location
        self.settings = settings
        isOn = settings.isOn
    }

    /// At launch (also when the phone starts the app in the background for a crossing): watching again if it was on.
    /// Already watching (the app's start and the signed-in screen both resume) asks nothing again.
    public func resume() async {
        if isOn, watching == nil, !isStarting { await start() }
    }

    public var isConsentGranted: Bool { consent?.granted ?? false }

    /// Reads the location consent.
    public func loadConsent() async {
        do {
            consent = try await client.call(Ops.ConsentsGet.self, ConsentsGetInput()).data
            hasConsentRecord = true
        } catch {
            // no consent record: location automations are unavailable for the account, as the web says
            hasConsentRecord = error.error.code != .notFound
            consent = nil
            if hasConsentRecord { line = ErrorText.of(error) }
        }
    }

    /// Grants or withdraws the consent; withdrawing also stops this phone's reports (they need it).
    public func setConsent(_ granted: Bool) async {
        guard let version = consent?.version else { return }
        do {
            consent = try await client.call(Ops.ConsentsUpdate.self, ConsentsUpdateInput(granted: granted), expectedVersion: Int(version))
                .data
            line = granted ? tr("Location consent granted") : tr("Location consent withdrawn — location automations are off")
        } catch {
            line = ErrorText.of(error)
            return
        }
        if !granted, isOn { await set(false) }
    }

    /// The switch.
    public func set(_ on: Bool) async {
        settings.set(on)
        isOn = on
        if on {
            await start()
        } else {
            await pause()
        }
    }

    /// Stops watching without changing the switch (signed out: the next sign-in watches that account's places).
    public func pause() async {
        run += 1
        isStarting = false
        watching?.cancel()
        watching = nil
        line = nil
        await location.stop()
    }

    private func start() async {
        run += 1
        let mine = run
        isStarting = true
        defer { if run == mine { isStarting = false } }
        permission = await location.requestAlways()
        guard run == mine else { return }
        switch permission {
        case .denied, .notDetermined:
            line = tr("Location is off for this app — allow it in Settings, or switch this off.")
            return
        case .whenInUse, .always:
            break
        }
        let fences: [Geofence]
        do {
            fences = try await geofences()
        } catch {
            if run == mine { line = ErrorText.of(error) }
            return
        }
        guard run == mine else { return }
        guard !fences.isEmpty else {
            line = tr("None of your homes or offices has a map pin yet — HQ sets them.")
            return
        }
        await report()
        guard run == mine else { return }
        if permission == .whenInUse {
            line = tr("Allow location “Always” so that arriving and leaving are reported while the app is closed.")
        }
        let crossings = await location.watch(fences)
        guard run == mine else {
            // stopped while the circles were set up: they are removed again, unless a later start set up its own
            if !isStarting, watching == nil { await location.stop() }
            return
        }
        watching?.cancel()
        watching = Task { [weak self] in
            for await _ in crossings {
                await self?.report()
            }
        }
    }

    /// The circles of the customer's homes and offices that have a pin.
    private func geofences() async throws(CoreError) -> [Geofence] {
        let properties = try await client.call(Ops.PropertiesList.self, Query(limit: 100)).data.items.filter { !$0.archived }
        names = properties.reduce(into: [:]) { names, property in names[property.id] = property.name }
        return properties.compactMap { property in
            property.location.map {
                Geofence(
                    propertyId: property.id,
                    latitude: $0.latitude,
                    longitude: $0.longitude,
                    radiusMeters: property.geofenceRadiusMeters ?? Self.defaultRadius
                )
            }
        }
    }

    /// Reports the phone's position now; a fix too coarse or none at all is not sent.
    /// What it came to is not said once the switch went off meanwhile.
    func report() async {
        let mine = run
        let said = await reported()
        if run == mine { line = said }
    }

    private func reported() async -> String {
        guard let fix = await location.fix(), fix.accuracyMeters <= Self.maxAccuracy else {
            return tr("This phone’s position is not precise enough to send — it is tried again at the next crossing.")
        }
        let input = PresenceReportInput(latitude: fix.latitude, longitude: fix.longitude, accuracyMeters: fix.accuracyMeters)
        do {
            let result = try await client.call(Ops.PresenceReport.self, input).data
            return Self.text(result, names: names) ?? tr("Location sent · nothing changed.")
        } catch  where error.error.messageKey == "errors.consent_required" {
            return tr(
                "Location automations need your consent. It is used only for arrival/departure automations: your device’s location is compared with the map pins of your homes and offices, and only home or away is kept."
            )
        } catch {
            return tr("This device’s location could not be sent — the next one is tried in a moment.")
        }
    }

    /// What a report changed, in words: an arrival or a departure and how many automations it started (presence.ts).
    static func text(_ result: PresenceResult, names: [String: String]) -> String? {
        guard let first = result.transitions.first else { return nil }
        let name = names[first.propertyId] ?? tr("your property")
        let ran = first.commandIds.count
        let arrived = first.event == .arrival
        switch ran {
        case 0: return arrived ? tr("Arrived at {name}.", ["name": name]) : tr("Left {name}.", ["name": name])
        case 1:
            return arrived
                ? tr("Arrived at {name} — 1 automation ran.", ["name": name]) : tr("Left {name} — 1 automation ran.", ["name": name])
        default:
            let values = ["name": name, "n": String(ran)]
            return arrived ? tr("Arrived at {name} — {n} automations ran.", values) : tr("Left {name} — {n} automations ran.", values)
        }
    }
}
