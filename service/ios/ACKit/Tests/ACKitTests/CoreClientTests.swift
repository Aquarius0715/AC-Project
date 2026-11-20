import ACTestSupport
import Foundation
import Testing

@testable import ACKit

@Suite struct CoreClientTests {
    private let base = URL(string: "http://core.test") ?? URL(filePath: "/")
    private let okUnits =
        #"""
        {"data":{"items":[],"nextCursor":null,"total":0,"snapshotVersion":1},
         "meta":{"correlationId":"c1","snapshotAt":"2026-11-20T00:00:00Z"}}
        """#
    private let refusal =
        #"{"code":"FORBIDDEN","messageKey":"error.forbidden","fieldErrors":{},"correlationId":"c2","retryAfterSeconds":null}"#

    private func client(_ transport: StubTransport, _ credentials: FixedCredentials = FixedCredentials()) -> CoreClient {
        CoreClient(baseURL: base, transport: transport, credentials: credentials)
    }

    @Test func readSendsTheMembershipAndNoIdempotencyKey() async throws {
        let transport = StubTransport([.http(200, okUnits)])
        let answer = try await client(transport).call(Ops.UnitsList.self, Query(limit: 5))
        #expect(answer.data.items.isEmpty)
        #expect(answer.meta == AnswerMeta(correlationId: "c1", snapshotAt: "2026-11-20T00:00:00Z"))
        let request = try #require(await transport.requests.first)
        #expect(request.url?.absoluteString == "http://core.test/v1/units?limit=5")
        #expect(request.httpMethod == "GET")
        #expect(request.value(forHTTPHeaderField: "Authorization") == "Bearer tok")
        #expect(request.value(forHTTPHeaderField: "X-Tenant-Id") == "t1")
        #expect(request.value(forHTTPHeaderField: "X-Membership-Id") == "m1")
        #expect(request.value(forHTTPHeaderField: "Idempotency-Key") == nil)
        #expect(request.value(forHTTPHeaderField: "Content-Type") == nil)
        #expect(request.httpBody == nil)
    }

    @Test func writeSendsItsKeyTheExpectedVersionAndAJSONBody() async throws {
        // the request is what this test is about: the API refuses it
        let transport = StubTransport([.http(422, refusal.replacingOccurrences(of: "FORBIDDEN", with: "VALIDATION"))])
        let key = UUID()
        let input = JobsCheckInInput(jobId: "j1", method: .manual, position: nil, qrUnitId: nil, reason: "no signal")
        await #expect(throws: CoreError.self) {
            try await client(transport).call(Ops.JobsCheckIn.self, input, expectedVersion: 7, idempotencyKey: key)
        }
        let request = try #require(await transport.requests.first)
        #expect(request.httpMethod == "POST")
        #expect(request.url?.path() == "/v1/jobs/j1/check-in")
        #expect(request.value(forHTTPHeaderField: "Idempotency-Key") == key.uuidString.lowercased())
        #expect(request.value(forHTTPHeaderField: "X-Expected-Version") == "7")
        #expect(request.value(forHTTPHeaderField: "Content-Type") == "application/json")
        #expect(request.httpBody == Data(#"{"method":"manual","position":null,"qrUnitId":null,"reason":"no signal"}"#.utf8))
    }

    @Test func refusalKeepsTheCoreAPIsDomainError() async throws {
        let transport = StubTransport([.http(403, refusal)])
        let credentials = FixedCredentials()
        do {
            _ = try await client(transport, credentials).call(Ops.UnitsList.self, Query())
            Issue.record("a refusal throws")
        } catch {
            #expect(error.status == 403)
            #expect(error.error.code == .forbidden)
            #expect(error.error.correlationId == "c2")
        }
        #expect(await credentials.refusals == 0)
    }

    @Test func unauthenticatedAnswerEndsTheSignIn() async throws {
        let body = refusal.replacingOccurrences(of: "FORBIDDEN", with: "UNAUTHENTICATED")
        let credentials = FixedCredentials()
        await #expect(throws: CoreError.self) {
            try await client(StubTransport([.http(401, body)]), credentials).call(Ops.UnitsList.self, Query())
        }
        #expect(await credentials.refusals == 1)
    }

    @Test(arguments: [(500, "not json"), (502, "<html>bad gateway</html>"), (200, #"{"data":{"items":"no"}}"#), (200, "")])
    func answerThatIsNoContractReadsAsUnavailable(status: Int, body: String) async throws {
        do {
            _ = try await client(StubTransport([.http(status, body)])).call(Ops.UnitsList.self, Query())
            Issue.record("no contract answer throws")
        } catch {
            #expect(error.status == status)
            #expect(error.error.code == .unavailable)
            #expect(error.error.messageKey == "error.unavailable")
        }
    }

    @Test(arguments: [
        (URLError.Code.notConnectedToInternet, ErrorCode.offline),
        (.networkConnectionLost, .offline),
        (.dataNotAllowed, .offline),
        (.internationalRoamingOff, .offline),
        (.timedOut, .timeout),
        (.cannotFindHost, .unavailable),
    ])
    func callWithoutAnAnswerSaysWhy(failure: URLError.Code, code: ErrorCode) async throws {
        do {
            _ = try await client(StubTransport([.failure(URLError(failure))])).call(Ops.UnitsList.self, Query())
            Issue.record("a failed call throws")
        } catch {
            #expect(error.status == 0)
            #expect(error.error.code == code)
        }
    }

    @Test func otherFailureReadsAsUnavailable() async throws {
        do {
            _ = try await client(StubTransport([.failure(CocoaError(.featureUnsupported))])).call(Ops.UnitsList.self, Query())
            Issue.record("a failed call throws")
        } catch {
            #expect(error.error.code == .unavailable)
        }
    }

    @Test func noSignInSendsNothing() async throws {
        let none = CoreError.local(.unauthenticated, "error.unauthenticated", status: 401)
        let transport = StubTransport([])
        do {
            _ = try await client(transport, FixedCredentials(.failure(none))).call(Ops.UnitsList.self, Query())
            Issue.record("no sign-in throws")
        } catch {
            #expect(error == none)
        }
        #expect(await transport.requests.isEmpty)
    }

    @Test func inputThatNamesNoRouteSendsNothing() async throws {
        let transport = StubTransport([])
        do {
            _ = try await client(transport).call(Ops.UnitsGet.self, UnitsGetInput(id: ".."))
            Issue.record("a dot segment throws")
        } catch {
            #expect(error.status == 404)
            #expect(error.error.code == .notFound)
        }
        #expect(await transport.requests.isEmpty)
    }

    @Test func inputThatDoesNotEncodeIsRefusedBeforeSending() async throws {
        struct Unencodable: Encodable, Sendable {
            func encode(to encoder: any Encoder) throws {
                throw EncodingError.invalidValue(0, .init(codingPath: [], debugDescription: "no"))
            }
        }
        struct Broken: CoreOperation {
            typealias Input = Unencodable
            typealias Result = Empty
            static let name = "broken"
            static let isWrite = true
            static let routes: [CoreRoute] = [.init(method: .post, path: "/v1/broken")]
        }
        do {
            _ = try await client(StubTransport([])).call(Broken.self, Unencodable())
            Issue.record("an input that does not encode throws")
        } catch {
            #expect(error.error.code == .validation)
        }
    }

    @Test func urlSessionTransportRefusesAnAnswerThatIsNotHTTP() async throws {
        // a file URL answers without an HTTPURLResponse
        let file = FileManager.default.temporaryDirectory.appending(path: "ackit-transport.txt")
        try Data("x".utf8).write(to: file)
        await #expect(throws: URLError.self) {
            try await URLSessionTransport().send(URLRequest(url: file))
        }
    }
}
