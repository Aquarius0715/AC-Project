import Foundation

/// The REST request of one operation call (IR222), by the rules of the web apps' `shared/lib/rest.ts` and the Core API's
/// bindings (`service/api/internal/ops/rest.go`):
/// - the route whose path parameters the input holds (an item route before a collection route) and whose fixed field
///   the input matches;
/// - path parameters are taken out of the input (`{target.kind}` from a nested field) and percent-encoded; `.` and `..`
///   name no route (IR372);
/// - GET and DELETE send the rest as the query string: `query` and `filters` by their own names, `sort` as
///   `field:direction`, nested fields as `object.field`, lists as repeated parameters (an empty list as one empty
///   value), null left out;
/// - POST, PUT and PATCH send the rest as the JSON body.
public struct RestRequest: Sendable, Equatable {
    public let method: CoreRoute.Method
    /// The path with its query string.
    public let path: String
    public let body: Data?

    /// The request, or nil when the input names no route (a path parameter that is `.` or `..`).
    public static func of<Operation: CoreOperation>(_: Operation.Type, _ input: Operation.Input) throws -> Self? {
        guard case var .object(rest) = try JSONValue.of(input) else { return nil }
        return try make(routes: Operation.routes, input: &rest)
    }

    /// Throws when the body does not encode (a number that is no JSON number: NaN or infinity).
    static func make(routes: [CoreRoute], input rest: inout [String: JSONValue]) throws -> Self? {
        guard let route = pick(routes, rest) else { return nil }
        var path = route.path
        for name in parameters(of: route.path) {
            let value = text(take(&rest, name.split(separator: ".").map(String.init)))
            if value == "." || value == ".." { return nil }
            path = path.replacingOccurrences(of: "{\(name)}", with: componentEncoded(value))
        }
        if let fixed = route.fixed, text(rest[fixed.field]) == fixed.value {
            rest[fixed.field] = nil
        }
        switch route.method {
        case .get, .delete:
            var items: [(String, String)] = []
            flatten(&items, prefix: "", rest, top: true)
            let query = items.map { "\(formEncoded($0.0))=\(formEncoded($0.1))" }.joined(separator: "&")
            return Self(method: route.method, path: query.isEmpty ? path : "\(path)?\(query)", body: nil)
        case .post, .put, .patch:
            let encoder = JSONEncoder()
            encoder.outputFormatting = .sortedKeys
            return Self(method: route.method, path: path, body: try encoder.encode(rest))
        }
    }

    // MARK: - the rules

    static func parameters(of path: String) -> [String] {
        var names: [String] = []
        var rest = Substring(path)
        while let open = rest.firstIndex(of: "{"), let close = rest[open...].firstIndex(of: "}") {
            names.append(String(rest[rest.index(after: open)..<close]))
            rest = rest[rest.index(after: close)...]
        }
        return names
    }

    static func lookup(_ node: [String: JSONValue], _ at: [String]) -> JSONValue? {
        var current = JSONValue.object(node)
        for key in at {
            guard case let .object(object) = current, let next = object[key] else { return nil }
            current = next
        }
        return current
    }

    static func take(_ node: inout [String: JSONValue], _ at: [String]) -> JSONValue? {
        guard let key = at.first else { return nil }
        if at.count == 1 {
            return node.removeValue(forKey: key)
        }
        guard case var .object(child) = node[key] else { return nil }
        let value = take(&child, Array(at.dropFirst()))
        node[key] = .object(child)
        return value
    }

    static func text(_ value: JSONValue?) -> String {
        switch value {
        case .none, .null: ""
        case let .string(s): s
        case let .bool(b): b ? "true" : "false"
        case let .number(n): number(n)
        case let .some(other): json(other)
        }
    }

    /// A number as JavaScript's `String(n)` writes the values the contract uses: whole numbers without a fraction.
    static func number(_ n: Double) -> String {
        if n.rounded() == n, abs(n) < 1e15 {
            return String(Int64(n))
        }
        return String(n)
    }

    static func json(_ value: JSONValue) -> String {
        let encoder = JSONEncoder()
        encoder.outputFormatting = .sortedKeys
        return (try? encoder.encode(value)).flatMap { String(bytes: $0, encoding: .utf8) } ?? ""
    }

    static func pick(_ routes: [CoreRoute], _ input: [String: JSONValue]) -> CoreRoute? {
        var best = routes.first
        var score = -1
        let fitting = routes.filter { route in
            guard let fixed = route.fixed else { return true }
            return text(input[fixed.field]) == fixed.value
        }
        for route in fitting {
            let names = parameters(of: route.path)
            let held = names.allSatisfy { name in
                switch lookup(input, name.split(separator: ".").map(String.init)) {
                case .none, .null, .string(""): false
                default: true
                }
            }
            if !held || names.count <= score { continue }
            best = route
            score = names.count
        }
        return best
    }

    static func flatten(_ items: inout [(String, String)], prefix: String, _ node: [String: JSONValue], top: Bool) {
        for (key, value) in node.sorted(by: { $0.key < $1.key }) {
            switch value {
            case .null:
                continue
            case let .array(list):
                if list.isEmpty { items.append((prefix + key, "")) }
                for element in list { items.append((prefix + key, text(element))) }
            case let .object(object):
                if top, key == "query" || key == "filters" {
                    flatten(&items, prefix: "", object, top: true)
                } else if key == "sort" {
                    items.append(("sort", "\(text(object["field"])):\(text(object["direction"]))"))
                } else {
                    flatten(&items, prefix: "\(prefix)\(key).", object, top: false)
                }
            default:
                items.append((prefix + key, text(value)))
            }
        }
    }

    static let asciiAlphanumerics = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"

    /// `encodeURIComponent`: everything but the unreserved characters and `!'()*`, as UTF-8 bytes.
    static func componentEncoded(_ value: String) -> String {
        percentEncoded(value, keeping: asciiAlphanumerics + "-_.!~*'()", space: "%20")
    }

    /// `URLSearchParams`: application/x-www-form-urlencoded, a space as `+`.
    static func formEncoded(_ value: String) -> String {
        percentEncoded(value, keeping: asciiAlphanumerics + "*-._", space: "+")
    }

    private static func percentEncoded(_ value: String, keeping: String, space: String) -> String {
        let kept = Set(keeping.utf8)
        var out = ""
        for byte in value.utf8 {
            if kept.contains(byte) {
                out.unicodeScalars.append(Unicode.Scalar(byte))
            } else if byte == UInt8(ascii: " ") {
                out += space
            } else {
                out += String(format: "%%%02X", byte)
            }
        }
        return out
    }
}
