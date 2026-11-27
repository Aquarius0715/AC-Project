import ACKit
import Foundation

/// What a screen shows of a read: nothing yet, the answer, or the refusal's line (kept beside the last answer, so a
/// failed reload does not empty the screen).
public enum Loadable<Value: Sendable>: Sendable {
    case loading
    case loaded(Value)
    case failed(message: String, last: Value?)

    /// The answer to show, the last one when a reload failed.
    public var value: Value? {
        switch self {
        case .loading: nil
        case let .loaded(value): value
        case let .failed(_, last): last
        }
    }

    /// Whether the first read is still on its way (nothing to show yet).
    public var isLoading: Bool {
        if case .loading = self { true } else { false }
    }

    /// The line of the refusal, if the last read failed.
    public var message: String? {
        if case let .failed(message, _) = self { message } else { nil }
    }

    /// The state after a read: its answer, or its refusal with the value shown before. The screens' models read on the
    /// main actor.
    @MainActor
    public func after(_ read: @MainActor () async throws(CoreError) -> Value) async -> Self {
        do {
            return .loaded(try await read())
        } catch {
            return .failed(message: ErrorText.of(error), last: value)
        }
    }
}
