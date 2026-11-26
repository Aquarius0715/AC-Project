#if DEBUG
import ACKit

/// The UI tests' phone position (AC_UI_TEST=stub): a fix of ±12 m at the stub's site.
public struct StubPosition: PositionSource {
    public init() {
        // the same fix every time
    }

    public func locate() async -> PositionAnswer {
        .fix(CheckInPosition(latitude: 3.139, longitude: 101.6869, accuracyMeters: 12))
    }
}
#endif
