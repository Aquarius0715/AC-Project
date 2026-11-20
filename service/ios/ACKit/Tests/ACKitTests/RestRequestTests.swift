import ACTestSupport
import Foundation
import Testing

@testable import ACKit

/// The same cases as the web apps' shared/lib/tests/rest.test.ts: both clients send one operation to the same request.
@Suite struct RestRequestTests {
    private func request(_ routes: [CoreRoute], _ input: [String: JSONValue]) -> RestRequest? {
        var rest = input
        // the inputs of these cases all encode: a throw would be a failure of the case, read here as no request
        return try? RestRequest.make(routes: routes, input: &rest)
    }

    private let unitsSave: [CoreRoute] = [.init(method: .post, path: "/v1/units"), .init(method: .put, path: "/v1/units/{id}")]

    @Test func listReadSendsPagingFiltersSortAndListsInTheQuery() throws {
        let input: [String: JSONValue] = [
            "filters": .object([
                "status": .string("open"),
                "unitIds": .array([.string("u1"), .string("u2")]),
                "overdueOnly": .bool(false),
                "statuses": .array([]),
                "origin": .null,
            ]),
            "sort": .object(["field": .string("dueAt"), "direction": .string("asc")]),
            "limit": .number(50),
        ]
        let r = try #require(request(Ops.JobsList.routes, input))
        #expect(r.method == .get)
        #expect(r.body == nil)
        let q = query(r.path)
        #expect(q.path == "/v1/jobs")
        #expect(
            q.values == [
                "status": ["open"], "unitIds": ["u1", "u2"], "overdueOnly": ["false"], "statuses": [""], "sort": ["dueAt:asc"],
                "limit": ["50"],
            ]
        )
    }

    @Test(arguments: [".", ".."])
    func dotSegmentNamesNoRoute(value: String) {
        #expect(request(Ops.UnitsGet.routes, ["id": .string(value)]) == nil)
        let content: [String: JSONValue] = [
            "jobId": .string(value), "reportId": .string("r1"), "reportVersion": .number(1), "attachmentId": .string("a1"),
        ]
        #expect(request(Ops.AttachmentsGetContent.routes, content) == nil)
        #expect(request(Ops.SummariesGet.routes, ["kind": .string(value), "filters": .object([:])]) == nil)
    }

    @Test func numbersAreWrittenAsJavaScriptWritesThem() {
        // whole numbers without a fraction (the web's String(n)); a fraction, and a number too large for one, as they are
        #expect(RestRequest.number(50) == "50")
        #expect(RestRequest.number(-3) == "-3")
        #expect(RestRequest.number(1.5) == "1.5")
        #expect(RestRequest.number(1e16) == String(1e16))
    }

    @Test func dotsInsideAValueStayOneEncodedSegment() throws {
        #expect(try #require(request(Ops.UnitsGet.routes, ["id": .string("../x")])).path == "/v1/units/..%2Fx")
        #expect(try #require(request(Ops.UnitsGet.routes, ["id": .string("%2e%2e")])).path == "/v1/units/%252e%252e")
        #expect(
            try #require(request(Ops.UnitsGet.routes, ["id": .string("..."), "jobId": .string("j1")])).path == "/v1/units/...?jobId=j1"
        )
    }

    @Test func pathParametersAreFilledAndEncodedAndTheRestGoesToTheQuery() throws {
        let unit = try #require(request(Ops.UnitsGet.routes, ["id": .string("a/b c"), "jobId": .string("j1")]))
        #expect(unit == RestRequest(method: .get, path: "/v1/units/a%2Fb%20c?jobId=j1", body: nil))
        #expect(request(Ops.SessionGet.routes, [:]) == RestRequest(method: .get, path: "/v1/session", body: nil))
        let commands = try #require(
            request(
                Ops.CommandsList.routes,
                [
                    "unitId": .string("u1"),
                    "query": .object([
                        "limit": .number(20), "sort": .object(["field": .string("requestedAt"), "direction": .string("desc")]),
                    ]),
                ]
            )
        )
        #expect(query(commands.path).values == ["unitId": ["u1"], "limit": ["20"], "sort": ["requestedAt:desc"]])
        let period = try #require(
            request(
                [.init(method: .get, path: "/v1/sla/scorecard")],
                [
                    "period": .object(["from": .string("2026-08-01T00:00:00Z"), "to": .string("2026-09-01T00:00:00Z")])
                ]
            )
        )
        #expect(query(period.path).values == ["period.from": ["2026-08-01T00:00:00Z"], "period.to": ["2026-09-01T00:00:00Z"]])
        let events = try #require(
            request(
                Ops.DevicesEvents.routes,
                [
                    "id": .string("d1"), "query": .object(["limit": .number(50), "filters": .object(["from": .string("x")])]),
                ]
            )
        )
        #expect(query(events.path) == ("/v1/devices/d1/events", ["limit": ["50"], "from": ["x"]]))
    }

    @Test func createsOnTheCollectionAndUpdatesOnTheItem() {
        #expect(
            request(unitsSave, ["displayName": .string("A"), "spaceId": .null])
                == RestRequest(method: .post, path: "/v1/units", body: Data(#"{"displayName":"A","spaceId":null}"#.utf8))
        )
        #expect(
            request(unitsSave, ["id": .string("u1"), "displayName": .string("A")])
                == RestRequest(method: .put, path: "/v1/units/u1", body: Data(#"{"displayName":"A"}"#.utf8))
        )
        #expect(
            request([.init(method: .delete, path: "/v1/units/{id}")], ["id": .string("u1"), "reason": .string("duplicate entry")])
                == RestRequest(method: .delete, path: "/v1/units/u1?reason=duplicate+entry", body: nil)
        )
    }

    @Test func fixedFieldChoosesTheRouteAndNestedParametersLeaveTheBody() {
        let transition: [CoreRoute] = [
            .init(method: .post, path: "/v1/payouts/{statementId}/approve", fixed: ("action", "approve")),
            .init(method: .post, path: "/v1/payouts/{statementId}/mark-paid", fixed: ("action", "mark_paid")),
        ]
        #expect(
            request(transition, ["statementId": .string("s1"), "action": .string("mark_paid"), "reason": .string("paid")])
                == RestRequest(method: .post, path: "/v1/payouts/s1/mark-paid", body: Data(#"{"reason":"paid"}"#.utf8))
        )
        #expect(
            request(transition, ["statementId": .string("s1"), "action": .string("approve")])
                == RestRequest(method: .post, path: "/v1/payouts/s1/approve", body: Data("{}".utf8))
        )
        // an action no route fixes stays in the body, so the API refuses it (error.pathMismatch)
        #expect(
            request(transition, ["statementId": .string("s1"), "action": .string("nope")])
                == RestRequest(method: .post, path: "/v1/payouts/s1/approve", body: Data(#"{"action":"nope"}"#.utf8))
        )
        let rename: [CoreRoute] = [.init(method: .post, path: "/v1/locations/{target.kind}/{target.id}/rename")]
        let input: [String: JSONValue] = ["target": .object(["kind": .string("space"), "id": .string("s1")]), "name": .string("Lobby")]
        #expect(
            request(rename, input)
                == RestRequest(method: .post, path: "/v1/locations/space/s1/rename", body: Data(#"{"name":"Lobby","target":{}}"#.utf8))
        )
    }

    @Test func noRouteIsNoRequest() {
        #expect(request([], ["id": .string("u1")]) == nil)
    }

    @Test(arguments: [(3.0, "3"), (-2.0, "-2"), (1.5, "1.5"), (0.0, "0"), (1e16, "1e+16")])
    func numbersAreWrittenAsJavaScriptWritesThem(value: Double, text: String) {
        #expect(RestRequest.text(.number(value)) == text)
    }

    @Test func otherValuesAsText() {
        #expect(RestRequest.text(nil).isEmpty)
        #expect(RestRequest.text(.null).isEmpty)
        #expect(RestRequest.text(.bool(true)) == "true")
        #expect(RestRequest.text(.array([.number(1), .string("a")])) == #"[1,"a"]"#)
        #expect(RestRequest.text(.object(["b": .null, "a": .bool(false)])) == #"{"a":false,"b":null}"#)
    }

    @Test func numberThatIsNoJSONIsRefused() {
        var rest: [String: JSONValue] = ["celsius": .number(.nan)]
        #expect(throws: EncodingError.self) { try RestRequest.make(routes: unitsSave, input: &rest) }
        #expect(RestRequest.text(.array([.number(.infinity)])).isEmpty)
    }

    @Test func textIsEncodedAsUTF8Bytes() {
        #expect(RestRequest.componentEncoded("é a/b~") == "%C3%A9%20a%2Fb~")
        #expect(RestRequest.formEncoded("é a/b~*") == "%C3%A9+a%2Fb%7E*")
    }

    @Test func lookupAndTakeFollowNestedFields() {
        var node: [String: JSONValue] = ["a": .object(["b": .string("x")]), "c": .string("y")]
        #expect(RestRequest.lookup(node, ["a", "b"]) == .string("x"))
        #expect(RestRequest.lookup(node, ["c", "d"]) == nil)
        #expect(RestRequest.take(&node, []) == nil)
        #expect(RestRequest.take(&node, ["c", "d"]) == nil)
        #expect(RestRequest.take(&node, ["a", "b"]) == .string("x"))
        #expect(node == ["a": .object([:]), "c": .string("y")])
    }

    @Test func typedInputKeepsExplicitNullsAndLeavesOutOptionalFields() throws {
        let input = JobsCheckInInput(jobId: "j1", method: .manual, position: nil, qrUnitId: nil, reason: nil)
        let r = try #require(try RestRequest.of(Ops.JobsCheckIn.self, input))
        #expect(r.path == "/v1/jobs/j1/check-in")
        #expect(r.body == Data(#"{"method":"manual","position":null,"qrUnitId":null}"#.utf8))
    }

    @Test func aBareValueInputNamesNoRequest() throws {
        // every operation's input is an object; a bare value never reaches a route
        struct Bare: CoreOperation {
            typealias Input = String
            typealias Result = Empty
            static let name = "bare"
            static let isWrite = false
            static let routes: [CoreRoute] = [.init(method: .get, path: "/v1/bare")]
        }
        #expect(try RestRequest.of(Bare.self, "x") == nil)
    }
}

extension RestRequestTests {
    @Test func operationThatTakesNothingSendsNoInput() throws {
        #expect(try RestRequest.of(Ops.PreferencesGet.self, Empty()) == RestRequest(method: .get, path: "/v1/preferences", body: nil))
        #expect(try JSONEncoder().encode(Empty()) == Data("{}".utf8))
    }
}
