#if DEBUG
import ACKit
import Foundation

/// The technician's diagnostic commands and test runs of the UI tests (AC_UI_TEST=stub): a command sent for a job is
/// acknowledged by the AC at once; a test run is answered awaiting its start, then reads as completed (its start and
/// end acknowledged). Commands without a job are the customer's (UITestStub).
final class StubDiagnostics {
    private var commands: [Command] = []
    private var runs: [DiagnosticRun] = []

    func answer(_ method: String, _ path: String, _ query: String?, _ body: Data?) throws -> (Int, Data)? {
        let encoder = JSONEncoder()
        let items = URLComponents(string: "?\(query ?? "")")?.queryItems ?? []
        let forJob = items.contains { $0.name == "jobId" }
        switch (method, path) {
        case ("GET", "/v1/commands") where forJob:
            return (
                200,
                try encoder.encode(
                    StubDiagnosticAnswer(
                        data: Page(items: commands.reversed(), nextCursor: nil, total: Double(commands.count), snapshotVersion: 1)
                    )
                )
            )
        case ("POST", "/v1/commands"):
            let input = try JSONDecoder().decode(StubCommandBody.self, from: body ?? Data())
            guard let jobId = input.jobId else { return nil }
            let command = make(input.action, jobId: jobId, reason: input.reason, run: nil)
            return (200, try encoder.encode(StubDiagnosticAnswer(data: command)))
        case ("GET", "/v1/diagnostic-runs"):
            return (
                200,
                try encoder.encode(
                    StubDiagnosticAnswer(data: Page(items: runs.reversed(), nextCursor: nil, total: Double(runs.count), snapshotVersion: 1))
                )
            )
        case ("POST", "/v1/diagnostic-runs"):
            let input = try JSONDecoder().decode(StubRunBody.self, from: body ?? Data())
            let id = "run\(runs.count + 1)"
            let start = make(input.startAction, jobId: input.jobId, reason: input.reason, run: id)
            let end = make(input.endAction, jobId: input.jobId, reason: input.reason, run: id)
            let at = Date().ISO8601Format()
            var run = DiagnosticRun(
                id: id,
                tenantId: "t1",
                version: 1,
                createdAt: at,
                updatedAt: at,
                jobId: input.jobId,
                unitId: input.unitId,
                actorMembershipId: "m1",
                startAction: input.startAction,
                endAction: input.endAction,
                durationMinutes: input.durationMinutes,
                reason: input.reason,
                state: .awaitingStart,
                startCommandId: start.id,
                endCommandId: nil,
                startedAt: nil,
                endAt: nil,
                failureCode: nil
            )
            let answered = run
            run.state = .completed
            run.endCommandId = end.id
            run.startedAt = at
            run.endAt = at
            runs.append(run)
            return (200, try encoder.encode(StubDiagnosticAnswer(data: answered)))
        default:
            return nil
        }
    }

    private func make(_ action: UnitAction, jobId: String, reason: String?, run: String?) -> Command {
        let id = "dc\(commands.count + 1)"
        let at = Date().ISO8601Format()
        let command = Command(
            id: id,
            tenantId: "t1",
            version: 1,
            createdAt: at,
            updatedAt: at,
            unitId: "u1",
            actorMembershipId: "m1",
            action: Self.command(action),
            diagnosticRunId: run,
            jobId: jobId,
            reason: reason,
            status: .acknowledged,
            delivery: .sent,
            requestedAt: at,
            sentAt: at,
            acknowledgedAt: at,
            expiresAt: at,
            failureCode: nil,
            correlationId: id,
            source: .diagnostic
        )
        commands.append(command)
        return command
    }

    static func command(_ action: UnitAction) -> CommandAction {
        switch action {
        case let .setPower(change): .setPower(change)
        case let .setTemperature(change): .setTemperature(change)
        case let .setMode(change): .setMode(change)
        case let .setFan(change): .setFan(change)
        case let .ventilate(change): .ventilate(change)
        }
    }
}

private struct StubCommandBody: Decodable {
    let action: UnitAction
    let jobId: String?
    let reason: String?
}

private struct StubRunBody: Decodable {
    let jobId: String
    let unitId: String
    let startAction: UnitAction
    let endAction: UnitAction
    let durationMinutes: Double
    let reason: String
}

private struct StubDiagnosticAnswer<Result: Encodable>: Encodable {
    let data: Result
    let meta = ["correlationId": "stub"]
}
#endif
