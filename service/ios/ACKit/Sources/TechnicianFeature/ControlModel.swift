import ACKit
import ACShared
import Foundation
import Observation

/// Technician diagnostic control of a unit for a job (FR-T10, DD-T10, the web's technician control page): the unit's
/// current state, a diagnostic command with its reason, and a test run (a start action now, an end action after 1–15
/// minutes), both inside the job's work window with "Run test runs" (control.diagnose, IR94). The setting changes only
/// when the device acknowledges it: the screen follows the job's commands and run until they settle.
@MainActor @Observable
public final class ControlModel {
    public enum Content: Sendable, Equatable {
        case unit(UnitDetail)
        case notStarted(String)
        case notFound
    }

    public let unitId: String
    public let jobId: String?
    public private(set) var content: Loadable<Content> = .loading
    public private(set) var canDiagnose = false
    public private(set) var job: JobDetail?
    public private(set) var runs: [DiagnosticRun] = []
    public private(set) var commands: [Command] = []
    public private(set) var isSending = false
    public private(set) var note: String?
    public private(set) var refusal: String?
    public private(set) var fieldErrors: [String: String] = [:]
    public private(set) var now = Date()
    /// The diagnostic command's choice and reason.
    public var actionId = ""
    public var reason = ""
    /// The test run's start and end actions, minutes and reason.
    public var startId = ""
    public var endId = ""
    public var minutes = 5
    public var runReason = ""

    let client: CoreClient
    let pause: @Sendable (Duration) async -> Void

    /// The polls of a waiting command or run: every 2 s, at most 30 times.
    static let pollInterval = Duration.seconds(2)
    static let polls = 30

    public init(
        unitId: String,
        jobId: String?,
        client: CoreClient,
        pause: @escaping @Sendable (Duration) async -> Void = UnitControlSleep.sleep
    ) {
        self.unitId = unitId
        self.jobId = jobId
        self.client = client
        self.pause = pause
    }

    public var unit: UnitDetail? {
        if case let .unit(unit)? = content.value { unit } else { nil }
    }

    public var options: [ControlOption] { unit.map { ControlWords.options($0.capabilities) } ?? [] }

    public var run: DiagnosticRun? { runs.first }

    /// A command or the test run is still waiting for the device.
    public var isActive: Bool {
        ControlWords.isActive(run) || commands.contains { $0.status == .requested || $0.status == .sent }
    }

    /// The job's work window is open now (commands can be sent).
    public var isInWindow: Bool {
        guard let assignment = job?.assignment, assignment.status == .active,
            let start = Format.parse(assignment.scheduledStart), let end = Format.parse(assignment.scheduledEnd)
        else { return false }
        return start <= now && now < end
    }

    public func load() async {
        content = await content.after { () async throws(CoreError) in try await read() }
        guard unit != nil else { return }
        let session = try? await client.call(Ops.SessionGet.self, Empty())
        canDiagnose = session?.data.permissions.contains(.controlDiagnose) ?? false
        if let at = session?.meta.snapshotAt.flatMap(Format.parse) { now = at }
        await readJob()
        let offered = options.map(\.id)
        if !offered.contains(actionId) { actionId = offered.first ?? "" }
        if !offered.contains(startId) { startId = offered.first ?? "" }
        if !offered.contains(endId) { endId = offered.dropFirst().first ?? offered.first ?? "" }
    }

    private func read() async throws(CoreError) -> Content {
        do {
            return .unit(try await client.call(Ops.UnitsGet.self, UnitsGetInput(id: unitId, jobId: jobId)).data)
        } catch {
            if error.error.messageKey == "errors.assignment_not_started" {
                guard let jobId,
                    case let .detail(job)? = try? await client.call(Ops.JobsGet.self, JobsGetInput(jobId: jobId)).data,
                    let start = job.assignment?.scheduledStart
                else { return .notStarted(tr("Available from the work start time of your assigned job.")) }
                return .notStarted(
                    tr("Diagnostic control opens at {time}, when your work window starts.", ["time": Format.dateTime(start)])
                )
            }
            let code = error.error.code
            guard code == .notFound || code == .forbidden || error.error.fieldErrors["id"] != nil else { throw error }
            return .notFound
        }
    }

    /// The job, its runs and its commands; without "Run test runs" the job's runs and commands are refused.
    private func readJob() async {
        guard let jobId, canDiagnose else { return }
        if case let .detail(job)? = try? await client.call(Ops.JobsGet.self, JobsGetInput(jobId: jobId)).data { self.job = job }
        runs =
            (try? await client.call(
                Ops.DiagnosticRunsList.self,
                DiagnosticRunsListInput(unitId: unitId, jobId: jobId, query: Query(limit: 10))
            ).data.items) ?? []
        commands =
            (try? await client.call(Ops.CommandsList.self, CommandsListInput(unitId: unitId, jobId: jobId, query: Query(limit: 20))).data
                .items) ?? []
    }

    // MARK: - What the screen shows

    public var tiles: [(String, String)] { unit.map(ControlWords.tiles) ?? [] }
    public var capability: String? { unit.map(ControlWords.capability) }
    public var restriction: String? { unit.flatMap(ControlWords.restriction) }
    public var blocked: String? { unit.flatMap(ControlWords.blocked) }
    public var history: [UnitRow] { ControlWords.history(commands, runs: runs) }
    public var banner: (title: String, text: String)? { run.map(ControlWords.banner) }

    /// Why a command cannot be sent now, or nil.
    public var unavailable: String? {
        guard unit != nil else { return nil }
        if !canDiagnose { return tr("Diagnostic control needs “Run test runs” on an assigned job. Ask HQ if you should have it.") }
        guard let job else {
            return tr(
                "Diagnostic commands and test runs belong to a job you are assigned to, inside its work window — open diagnostic control from the unit or the job."
            )
        }
        if !isInWindow {
            let window = job.assignment.map { Format.span($0.scheduledStart, $0.scheduledEnd) } ?? tr("no assignment")
            return tr("Outside your work window {window} — diagnostic commands can’t be sent now.", ["window": window])
        }
        return blocked.map { tr("Control is blocked right now ({reason}) — commands and test runs wait until it clears.", ["reason": $0]) }
    }

    // MARK: - The steps

    static func reasonError(_ reason: String) -> String? {
        let count = reason.trimmingCharacters(in: .whitespacesAndNewlines).count
        return count < 1 || count > 1000 ? tr("Reason is required (1–1000 characters)") : nil
    }

    /// Sends the chosen diagnostic command with its reason; the screen follows it until the device answers.
    public func send() async {
        guard let option = options.first(where: { $0.id == actionId }) else { return }
        if let error = Self.reasonError(reason) {
            fieldErrors = ["reason": error]
            return
        }
        let reason = reason.trimmingCharacters(in: .whitespacesAndNewlines)
        await write { [client, unitId] unit, job throws(CoreError) in
            let input = CommandsCreateInput(
                unitId: unitId,
                action: option.action,
                jobId: job.id,
                reason: reason,
                expectedUnitVersion: unit.version
            )
            let command = try await client.call(Ops.CommandsCreate.self, input).data
            return tr("{id} sent — waiting for the device", ["id": String(command.id.prefix(8))])
        }
        if refusal == nil, fieldErrors.isEmpty { self.reason = "" }
    }

    /// Starts a test run: the start action now, the end action after the minutes (1–15).
    public func startRun() async {
        guard let start = options.first(where: { $0.id == startId }), let end = options.first(where: { $0.id == endId }) else { return }
        var errors: [String: String] = [:]
        if let error = Self.reasonError(runReason) { errors["runReason"] = error }
        if !(1...15).contains(minutes) { errors["durationMinutes"] = tr("A test run lasts 1–15 minutes") }
        fieldErrors = errors
        guard errors.isEmpty else { return }
        let reason = runReason.trimmingCharacters(in: .whitespacesAndNewlines)
        let minutes = Double(minutes)
        await write { [client, unitId] unit, job throws(CoreError) in
            let input = DiagnosticRunsCreateInput(
                jobId: job.id,
                unitId: unitId,
                startAction: start.action,
                endAction: end.action,
                durationMinutes: minutes,
                reason: reason,
                expectedUnitVersion: unit.version,
                expectedJobVersion: job.version
            )
            _ = try await client.call(Ops.DiagnosticRunsCreate.self, input)
            return tr("Test run requested — waiting for the start acknowledgement")
        }
        if refusal == nil, fieldErrors.isEmpty { runReason = "" }
    }

    private func write(_ send: (UnitDetail, JobDetail) async throws(CoreError) -> String) async {
        guard let unit, let job, !isSending else { return }
        isSending = true
        defer { isSending = false }
        note = nil
        refusal = nil
        fieldErrors = [:]
        do {
            note = try await send(unit, job)
        } catch {
            refusal = ControlWords.refusal(error.error)
            fieldErrors = error.error.fieldErrors.mapValues(ControlWords.field)
        }
        await load()
        await follow()
    }

    /// Reads again while a command or the test run waits for the device, until it settles or the polls run out.
    func follow() async {
        var left = Self.polls
        while isActive, left > 0 {
            left -= 1
            await pause(Self.pollInterval)
            await load()
        }
    }
}

/// The wait between polls in the app, off the main actor (as UnitControlModel.sleep).
public enum UnitControlSleep {
    public static func sleep(_ duration: Duration) async {
        try? await Task.sleep(for: duration)  // a cancelled wait ends the polls: the screen was closed
    }
}
