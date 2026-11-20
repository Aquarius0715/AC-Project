import Foundation

/// The literal fields of the contract (`kind: 'set_power'`, `isDemo: true`, `severity: null`): the generated types do not
/// store them, they check them when decoding and write them when encoding.
enum Constant {
    /// Refuses a body whose field is not the contract's literal.
    static func expect<Value: Decodable & Equatable, Key: CodingKey>(
        _ value: Value,
        in container: KeyedDecodingContainer<Key>,
        at key: Key
    ) throws {
        let found = try container.decode(Value.self, forKey: key)
        guard found == value else {
            throw DecodingError.dataCorruptedError(
                forKey: key,
                in: container,
                debugDescription: "expected \(value), found \(found)"
            )
        }
    }

    /// Refuses a body whose field is not null (or is missing: absence is an explicit null in the contract's results).
    static func expectNull<Key: CodingKey>(in container: KeyedDecodingContainer<Key>, at key: Key) throws {
        guard try container.decodeNil(forKey: key) else {
            throw DecodingError.dataCorruptedError(forKey: key, in: container, debugDescription: "expected null")
        }
    }
}
