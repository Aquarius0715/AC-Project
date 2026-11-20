import Foundation

/// One Core API operation of the catalog (Operations.gen.swift): its input and result types and its REST routes.
public protocol CoreOperation: Sendable {
    associatedtype Input: Encodable, Sendable
    associatedtype Result: Decodable, Sendable
    static var name: String { get }
    /// A write sends an `Idempotency-Key` (D04: the same key replays the stored answer).
    static var isWrite: Bool { get }
    static var routes: [CoreRoute] { get }
}

/// One REST route of an operation (IR222): `{field}` path parameters named after the input fields they take, and
/// optionally one input field the route fixes (`payouts.transition` at `/approve` sets `action=approve`).
public struct CoreRoute: Sendable, Equatable {
    public enum Method: String, Sendable {
        case get = "GET"
        case post = "POST"
        case put = "PUT"
        case patch = "PATCH"
        case delete = "DELETE"
    }

    public let method: Method
    public let path: String
    public let fixed: Fixed?

    /// The input field a route fixes and the value it fixes it to.
    public struct Fixed: Sendable, Equatable {
        public let field: String
        public let value: String
    }

    public init(method: Method, path: String, fixed: (String, String)? = nil) {
        self.method = method
        self.path = path
        self.fixed = fixed.map { Fixed(field: $0.0, value: $0.1) }
    }
}
