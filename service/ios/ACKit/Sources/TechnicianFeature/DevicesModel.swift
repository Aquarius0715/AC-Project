import ACKit
import ACShared
import Foundation
import Observation

/// The devices of the technician's assigned units (FR-T11, DD-T11, the web's technician devices page): the list with
/// its filter, and a new device registered on an assigned unit that has none, under the job of that unit (IR94).
@MainActor @Observable
public final class DevicesModel {
    /// A unit a new device can be registered on: an assigned unit without a device, with its jobs and its model's
    /// sensors.
    public struct Candidate: Sendable, Equatable, Identifiable {
        public let id: String
        public let label: String
        public let sensors: [Metric]
        public let jobIds: [String]
    }

    public private(set) var devices: Loadable<[Device]> = .loading
    public private(set) var names: [String: String] = [:]
    public private(set) var jobs: [JobSummary] = []
    public private(set) var candidates: [Candidate] = []
    public private(set) var canMaintain = false
    public private(set) var now = Date()
    public var filter = DeviceFilter.all
    public private(set) var isSending = false
    public private(set) var note: String?
    public private(set) var refusal: String?
    public private(set) var fieldErrors: [String: String] = [:]
    /// The registration form.
    public var serial = ""
    public var unitId = ""
    public var sensorTypes: Set<Metric> = []

    let client: CoreClient

    public init(client: CoreClient) {
        self.client = client
    }

    public var rows: [UnitRow] {
        (devices.value ?? []).filter { filter.keeps($0) }.sorted { $0.serial < $1.serial }
            .map { DeviceWords.row($0, names: names, now: now) }
    }

    public func load() async {
        devices = await devices.after { () async throws(CoreError) in
            let answer = try await client.call(Ops.DevicesList.self, Query(limit: 100))
            now = answer.meta.snapshotAt.flatMap(Format.parse) ?? Date()
            return answer.data.items
        }
        let units = (try? await client.call(Ops.UnitsList.self, Query(limit: 100)).data.items) ?? []
        names = [:]
        for unit in units where names[unit.id] == nil { names[unit.id] = unit.displayName }
        jobs = ((try? await client.call(Ops.JobsList.self, Query(limit: 100)).data.items) ?? []).compactMap { item in
            if case let .summary(job) = item { job } else { nil }
        }
        let session = try? await client.call(Ops.SessionGet.self, Empty())
        canMaintain = session?.data.permissions.contains(.deviceMaintain) ?? false
        await loadCandidates(units)
        if !candidates.contains(where: { $0.id == unitId }) { blank() }
    }

    /// The assigned units without a device (bound or planned, IR722) that have a job a write can run under.
    private func loadCandidates(_ units: [UnitSummary]) async {
        let taken = Set((devices.value ?? []).compactMap { $0.unitId ?? $0.targetUnitId })
        var out: [Candidate] = []
        for unit in units where !unit.archived && !taken.contains(unit.id) {
            let jobIds = DeviceWords.jobs(for: unit.id, jobs).map(\.id)
            guard !jobIds.isEmpty, let detail = try? await client.call(Ops.UnitsGet.self, UnitsGetInput(id: unit.id)).data else { continue }
            let place = detail.location.pathLabels.joined(separator: " › ")
            out.append(
                Candidate(
                    id: unit.id,
                    label: place.isEmpty ? detail.displayName : "\(detail.displayName) · \(place)",
                    sensors: detail.iotModel?.sensors.map(\.metric) ?? [],
                    jobIds: jobIds
                )
            )
        }
        candidates = out
    }

    /// The registration form as it opens: the first candidate, no serial and no sensors (made afresh each time, IR701).
    public func blank() {
        serial = ""
        unitId = candidates.first?.id ?? ""
        sensorTypes = []
        fieldErrors = [:]
    }

    /// Registers the device on the chosen unit under its first job; true when the Core API took it.
    public func register() async -> Bool {
        guard let candidate = candidates.first(where: { $0.id == unitId }), let jobId = candidate.jobIds.first, !isSending else {
            return false
        }
        let serial = serial.trimmingCharacters(in: .whitespaces)
        guard !serial.isEmpty else {
            fieldErrors = ["serial": tr("Required")]
            return false
        }
        isSending = true
        defer { isSending = false }
        note = nil
        refusal = nil
        fieldErrors = [:]
        let sensors = candidate.sensors.filter { sensorTypes.contains($0) }
        let input = DevicesRegisterInput(serial: serial, sensorTypes: sensors, unitId: candidate.id, jobId: jobId)
        do {
            let device = try await client.call(Ops.DevicesRegister.self, input).data
            note = "\(device.serial) · \(tr("Device registered — bind it to the unit once it is installed"))"
        } catch {
            refusal = DeviceWords.refusal(error.error)
            fieldErrors = error.error.fieldErrors.mapValues(DeviceWords.field)
            return false
        }
        await load()
        blank()
        return true
    }
}
