import Foundation

/// A JSON value with its type: what an operation's input becomes before it is split into the path, the query and the
/// body of a REST request (``RestRequest``).
public enum JSONValue: Codable, Sendable, Equatable {
    case null
    case bool(Bool)
    case number(Double)
    case string(String)
    case array([Self])
    case object([String: Self])

    public init(from decoder: any Decoder) throws {
        let c = try decoder.singleValueContainer()
        if c.decodeNil() {
            self = .null
        } else if let value = try? c.decode(Bool.self) {
            self = .bool(value)
        } else if let value = try? c.decode(Double.self) {
            self = .number(value)
        } else if let value = try? c.decode(String.self) {
            self = .string(value)
        } else if let value = try? c.decode([Self].self) {
            self = .array(value)
        } else {
            self = .object(try c.decode([String: Self].self))
        }
    }

    public func encode(to encoder: any Encoder) throws {
        var c = encoder.singleValueContainer()
        switch self {
        case .null: try c.encodeNil()
        case let .bool(value): try c.encode(value)
        case let .number(value): try c.encode(value)
        case let .string(value): try c.encode(value)
        case let .array(value): try c.encode(value)
        case let .object(value): try c.encode(value)
        }
    }

    /// The value of an Encodable as JSON: the same encoder the request body uses.
    static func of(_ value: some Encodable) throws -> Self {
        let data = try JSONEncoder().encode(value)
        return try JSONDecoder().decode(Self.self, from: data)
    }
}
